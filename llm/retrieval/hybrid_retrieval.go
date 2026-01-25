package retrieval

import (
	"context"
	"math"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// HybridRetrievalConfig 混合检索配置（基于 2025 年最佳实践）
type HybridRetrievalConfig struct {
	// BM25 配置
	UseBM25    bool    `json:"use_bm25"`
	BM25Weight float64 `json:"bm25_weight"`
	BM25K1     float64 `json:"bm25_k1"` // BM25 参数 k1 (1.2-2.0)
	BM25B      float64 `json:"bm25_b"`  // BM25 参数 b (0.75)

	// 向量检索配置
	UseVector    bool    `json:"use_vector"`
	VectorWeight float64 `json:"vector_weight"`

	// Reranking 配置
	UseReranking bool `json:"use_reranking"`
	RerankTopK   int  `json:"rerank_top_k"`

	// 检索参数
	TopK     int     `json:"top_k"`
	MinScore float64 `json:"min_score"`
}

// DefaultHybridRetrievalConfig 返回默认混合检索配置
func DefaultHybridRetrievalConfig() HybridRetrievalConfig {
	return HybridRetrievalConfig{
		UseBM25:      true,
		BM25Weight:   0.5,
		BM25K1:       1.5,
		BM25B:        0.75,
		UseVector:    true,
		VectorWeight: 0.5,
		UseReranking: true,
		RerankTopK:   50,
		TopK:         5,
		MinScore:     0.3,
	}
}

// Document 文档
type Document struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Embedding []float64              `json:"embedding,omitempty"`
}

// RetrievalResult 检索结果
type RetrievalResult struct {
	Document    Document `json:"document"`
	BM25Score   float64  `json:"bm25_score"`
	VectorScore float64  `json:"vector_score"`
	HybridScore float64  `json:"hybrid_score"`
	RerankScore float64  `json:"rerank_score,omitempty"`
	FinalScore  float64  `json:"final_score"`
}

// HybridRetriever 混合检索器
type HybridRetriever struct {
	config    HybridRetrievalConfig
	documents []Document

	// BM25 统计
	avgDocLen float64
	docLens   []int
	idf       map[string]float64

	// 向量存储（可选）
	vectorStore VectorStore

	logger *zap.Logger
}

// NewHybridRetriever 创建混合检索器
func NewHybridRetriever(config HybridRetrievalConfig, logger *zap.Logger) *HybridRetriever {
	return &HybridRetriever{
		config: config,
		idf:    make(map[string]float64),
		logger: logger,
	}
}

// NewHybridRetrieverWithVectorStore 创建带向量存储的混合检索器
func NewHybridRetrieverWithVectorStore(
	config HybridRetrievalConfig,
	vectorStore VectorStore,
	logger *zap.Logger,
) *HybridRetriever {
	return &HybridRetriever{
		config:      config,
		idf:         make(map[string]float64),
		vectorStore: vectorStore,
		logger:      logger,
	}
}

// IndexDocuments 索引文档
func (r *HybridRetriever) IndexDocuments(docs []Document) error {
	r.documents = docs

	// 计算 BM25 统计信息
	if r.config.UseBM25 {
		r.computeBM25Stats()
	}

	// 添加到向量存储
	if r.vectorStore != nil && r.config.UseVector {
		if err := r.vectorStore.AddDocuments(context.Background(), docs); err != nil {
			r.logger.Warn("failed to add documents to vector store", zap.Error(err))
		}
	}

	r.logger.Info("documents indexed",
		zap.Int("count", len(docs)))

	return nil
}

// Retrieve 混合检索
func (r *HybridRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]RetrievalResult, error) {
	results := []RetrievalResult{}

	// 1. BM25 检索
	var bm25Results map[string]float64
	if r.config.UseBM25 {
		bm25Results = r.bm25Retrieve(query)
	}

	// 2. 向量检索
	var vectorResults map[string]float64
	if r.config.UseVector && queryEmbedding != nil {
		vectorResults = r.vectorRetrieve(queryEmbedding)
	}

	// 3. 合并结果
	merged := r.mergeResults(bm25Results, vectorResults)

	// 4. 转换为 RetrievalResult
	for docID, scores := range merged {
		doc := r.getDocumentByID(docID)
		if doc == nil {
			continue
		}

		result := RetrievalResult{
			Document:    *doc,
			BM25Score:   scores["bm25"],
			VectorScore: scores["vector"],
			HybridScore: scores["hybrid"],
			FinalScore:  scores["hybrid"],
		}
		results = append(results, result)
	}

	// 5. 排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	// 6. Reranking（可选）
	if r.config.UseReranking && len(results) > 0 {
		topK := r.config.RerankTopK
		if topK > len(results) {
			topK = len(results)
		}
		results = r.rerank(query, results[:topK])
	}

	// 7. 返回 Top-K
	if len(results) > r.config.TopK {
		results = results[:r.config.TopK]
	}

	// 8. 过滤低分结果
	filtered := []RetrievalResult{}
	for _, res := range results {
		if res.FinalScore >= r.config.MinScore {
			filtered = append(filtered, res)
		}
	}

	return filtered, nil
}

// computeBM25Stats 计算 BM25 统计信息
func (r *HybridRetriever) computeBM25Stats() {
	totalLen := 0
	r.docLens = make([]int, len(r.documents))
	termDocCount := make(map[string]int)

	for i, doc := range r.documents {
		terms := r.tokenize(doc.Content)
		r.docLens[i] = len(terms)
		totalLen += len(terms)

		// 统计包含每个词的文档数
		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				termDocCount[term]++
				seen[term] = true
			}
		}
	}

	// 计算平均文档长度
	if len(r.documents) > 0 {
		r.avgDocLen = float64(totalLen) / float64(len(r.documents))
	}

	// 计算 IDF
	N := float64(len(r.documents))
	for term, df := range termDocCount {
		r.idf[term] = math.Log((N-float64(df)+0.5)/(float64(df)+0.5) + 1.0)
	}
}

// bm25Retrieve BM25 检索
func (r *HybridRetriever) bm25Retrieve(query string) map[string]float64 {
	queryTerms := r.tokenize(query)
	scores := make(map[string]float64)

	for i, doc := range r.documents {
		docTerms := r.tokenize(doc.Content)
		termFreq := make(map[string]int)
		for _, term := range docTerms {
			termFreq[term]++
		}

		score := 0.0
		docLen := float64(r.docLens[i])

		for _, qTerm := range queryTerms {
			if tf, ok := termFreq[qTerm]; ok {
				idf := r.idf[qTerm]

				// BM25 公式
				numerator := float64(tf) * (r.config.BM25K1 + 1.0)
				denominator := float64(tf) + r.config.BM25K1*(1.0-r.config.BM25B+r.config.BM25B*(docLen/r.avgDocLen))

				score += idf * (numerator / denominator)
			}
		}

		scores[doc.ID] = score
	}

	return scores
}

// vectorRetrieve 向量检索（余弦相似度）
func (r *HybridRetriever) vectorRetrieve(queryEmbedding []float64) map[string]float64 {
	scores := make(map[string]float64)

	// 优先使用向量存储
	if r.vectorStore != nil {
		results, err := r.vectorStore.Search(context.Background(), queryEmbedding, r.config.RerankTopK)
		if err != nil {
			r.logger.Warn("vector store search failed, falling back to in-memory", zap.Error(err))
		} else {
			for _, result := range results {
				scores[result.Document.ID] = result.Score
			}
			return scores
		}
	}

	// 回退到内存搜索
	for _, doc := range r.documents {
		if doc.Embedding == nil {
			continue
		}

		// 计算余弦相似度
		similarity := r.cosineSimilarity(queryEmbedding, doc.Embedding)
		scores[doc.ID] = similarity
	}

	return scores
}

// cosineSimilarity 计算余弦相似度
func (r *HybridRetriever) cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// mergeResults 合并 BM25 和向量检索结果
func (r *HybridRetriever) mergeResults(bm25Results, vectorResults map[string]float64) map[string]map[string]float64 {
	merged := make(map[string]map[string]float64)

	// 归一化 BM25 分数
	bm25Normalized := r.normalizeScores(bm25Results)
	vectorNormalized := r.normalizeScores(vectorResults)

	// 合并所有文档 ID
	allIDs := make(map[string]bool)
	for id := range bm25Normalized {
		allIDs[id] = true
	}
	for id := range vectorNormalized {
		allIDs[id] = true
	}

	// 计算混合分数
	for id := range allIDs {
		bm25Score := bm25Normalized[id]
		vectorScore := vectorNormalized[id]

		hybridScore := bm25Score*r.config.BM25Weight + vectorScore*r.config.VectorWeight

		merged[id] = map[string]float64{
			"bm25":   bm25Score,
			"vector": vectorScore,
			"hybrid": hybridScore,
		}
	}

	return merged
}

// normalizeScores 归一化分数（Min-Max）
func (r *HybridRetriever) normalizeScores(scores map[string]float64) map[string]float64 {
	if len(scores) == 0 {
		return scores
	}

	// 找到最小和最大值
	minScore := math.MaxFloat64
	maxScore := -math.MaxFloat64

	for _, score := range scores {
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}

	// 归一化
	normalized := make(map[string]float64)
	scoreRange := maxScore - minScore

	if scoreRange == 0 {
		// 所有分数相同
		for id := range scores {
			normalized[id] = 1.0
		}
	} else {
		for id, score := range scores {
			normalized[id] = (score - minScore) / scoreRange
		}
	}

	return normalized
}

// rerank 重排序（使用交叉编码器）
func (r *HybridRetriever) rerank(query string, results []RetrievalResult) []RetrievalResult {
	// 简化版：基于查询-文档对的深度匹配
	// 生产环境应使用 Cross-Encoder 模型（如 Sentence Transformers）

	for i := range results {
		// 计算更精细的相关性分数
		rerankScore := r.calculateRerankScore(query, results[i].Document.Content)
		results[i].RerankScore = rerankScore
		results[i].FinalScore = rerankScore
	}

	// 重新排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	return results
}

// calculateRerankScore 计算重排序分数
func (r *HybridRetriever) calculateRerankScore(query, content string) float64 {
	// 简化实现：基于词重叠和位置
	queryTerms := r.tokenize(query)
	contentTerms := r.tokenize(content)

	if len(queryTerms) == 0 {
		return 0.0
	}

	matchCount := 0
	for _, qTerm := range queryTerms {
		for _, cTerm := range contentTerms {
			if qTerm == cTerm {
				matchCount++
				break
			}
		}
	}

	return float64(matchCount) / float64(len(queryTerms))
}

// tokenize 分词
func (r *HybridRetriever) tokenize(text string) []string {
	// 简化分词：转小写并按空格分割
	text = strings.ToLower(text)
	return strings.Fields(text)
}

// getDocumentByID 根据 ID 获取文档
func (r *HybridRetriever) getDocumentByID(id string) *Document {
	for i := range r.documents {
		if r.documents[i].ID == id {
			return &r.documents[i]
		}
	}
	return nil
}
