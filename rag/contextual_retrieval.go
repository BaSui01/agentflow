package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ContextualRetrieval Anthropic 上下文检索（2025 最佳实践）
// 为每个 chunk 添加文档级上下文，提高检索准确率 50-60%
type ContextualRetrieval struct {
	retriever       *HybridRetriever
	contextProvider ContextProvider
	config          ContextualRetrievalConfig
	logger          *zap.Logger
	// 缓存
	contextCache sync.Map           // key: docID+chunkHash -> *cacheEntry
	idfCache     map[string]float64 // 词的 IDF 缓存
	avgDocLen    float64            // 平均文档长度（用于 BM25）
	totalDocs    int                // 总文档数
	totalDocLen  int                // 累积文档总长度（用于计算全局平均）
	mu           sync.RWMutex
}

// contextCacheEntry 上下文缓存条目
type contextCacheEntry struct {
	context   string
	createdAt time.Time
}

// ContextualRetrievalConfig 上下文检索配置
type ContextualRetrievalConfig struct {
	// 上下文生成
	UseContextPrefix bool   `json:"use_context_prefix"`
	ContextTemplate  string `json:"context_template"`
	MaxContextLength int    `json:"max_context_length"`

	// 检索增强
	UseReranking  bool    `json:"use_reranking"`
	ContextWeight float64 `json:"context_weight"`

	// 缓存
	CacheContexts bool          `json:"cache_contexts"`
	CacheTTL      time.Duration `json:"cache_ttl"` // 缓存过期时间，默认 1h

	// 分块配置（新增）
	ChunkSize     int  `json:"chunk_size"`       // 分块大小，默认 500
	ChunkOverlap  int  `json:"chunk_overlap"`    // 重叠大小，默认 50
	ChunkByTokens bool `json:"chunk_by_tokens"`  // 按 token 还是字符分块

	// BM25 参数（新增）
	BM25K1 float64 `json:"bm25_k1"` // BM25 k1 参数，默认 1.2
	BM25B  float64 `json:"bm25_b"`  // BM25 b 参数，默认 0.75
}

// DefaultContextualRetrievalConfig 默认配置
func DefaultContextualRetrievalConfig() ContextualRetrievalConfig {
	return ContextualRetrievalConfig{
		UseContextPrefix: true,
		ContextTemplate:  "Document: {{document_title}}\nSection: {{section_title}}\nContext: {{context}}\n\nContent: {{content}}",
		MaxContextLength: 200,
		UseReranking:     true,
		ContextWeight:    0.4,
		CacheContexts:    true,
		CacheTTL:         time.Hour,
		ChunkSize:        500,
		ChunkOverlap:     50,
		ChunkByTokens:    false,
		BM25K1:           1.2,
		BM25B:            0.75,
	}
}

// ContextProvider 上下文提供器接口
type ContextProvider interface {
	// GenerateContext 为 chunk 生成上下文
	GenerateContext(ctx context.Context, doc Document, chunk string) (string, error)
}

// LLMContextProvider 基于 LLM 的上下文生成器
type LLMContextProvider struct {
	llmProvider func(context.Context, string) (string, error)
	logger      *zap.Logger
}

// NewLLMContextProvider 创建 LLM 上下文提供器
func NewLLMContextProvider(
	llmProvider func(context.Context, string) (string, error),
	logger *zap.Logger,
) *LLMContextProvider {
	return &LLMContextProvider{
		llmProvider: llmProvider,
		logger:      logger,
	}
}

// GenerateContext 生成上下文
func (p *LLMContextProvider) GenerateContext(ctx context.Context, doc Document, chunk string) (string, error) {
	prompt := fmt.Sprintf(`Given the following document and chunk, provide a brief context (1-2 sentences) that explains what this chunk is about in relation to the whole document.

Document Title: %s
Full Document: %s

Chunk: %s

Context:`, doc.Metadata["title"], doc.Content, chunk)

	context, err := p.llmProvider(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate context: %w", err)
	}

	return strings.TrimSpace(context), nil
}

// NewContextualRetrieval 创建上下文检索器
func NewContextualRetrieval(
	retriever *HybridRetriever,
	contextProvider ContextProvider,
	config ContextualRetrievalConfig,
	logger *zap.Logger,
) *ContextualRetrieval {
	return &ContextualRetrieval{
		retriever:       retriever,
		contextProvider: contextProvider,
		config:          config,
		logger:          logger,
	}
}

// IndexDocumentsWithContext 索引文档（添加上下文）
func (r *ContextualRetrieval) IndexDocumentsWithContext(ctx context.Context, docs []Document) error {
	if !r.config.UseContextPrefix {
		return r.retriever.IndexDocuments(docs)
	}

	enrichedDocs := make([]Document, 0)

	for _, doc := range docs {
		chunks := r.chunkDocument(doc)

		for i, chunk := range chunks {
			var contextStr string
			var err error

			// 检查缓存
			if r.config.CacheContexts {
				cacheKey := r.buildCacheKey(doc.ID, chunk)
				if cached, ok := r.getFromCache(cacheKey); ok {
					contextStr = cached
					goto buildDoc
				}
			}

			// 生成上下文
			contextStr, err = r.contextProvider.GenerateContext(ctx, doc, chunk)
			if err != nil {
				r.logger.Warn("failed to generate context, using original chunk",
					zap.String("doc_id", doc.ID),
					zap.Int("chunk_idx", i),
					zap.Error(err))
				contextStr = ""
			}

			// 写入缓存
			if r.config.CacheContexts && contextStr != "" {
				cacheKey := r.buildCacheKey(doc.ID, chunk)
				r.putToCache(cacheKey, contextStr)
			}

		buildDoc:
			enrichedContent := r.renderContextTemplate(doc, chunk, contextStr)
			enrichedDoc := Document{
				ID:        fmt.Sprintf("%s_chunk_%d", doc.ID, i),
				Content:   enrichedContent,
				Embedding: nil,
				Metadata: map[string]any{
					"original_doc_id": doc.ID,
					"chunk_index":     i,
					"context":         contextStr,
					"original_chunk":  chunk,
				},
			}
			enrichedDocs = append(enrichedDocs, enrichedDoc)
		}
	}

	// 更新 BM25 统计
	r.UpdateIDFStats(enrichedDocs)

	r.logger.Info("indexed documents with context",
		zap.Int("original_docs", len(docs)),
		zap.Int("enriched_chunks", len(enrichedDocs)))

	return r.retriever.IndexDocuments(enrichedDocs)
}

// Retrieve 检索（使用上下文增强）
func (r *ContextualRetrieval) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]RetrievalResult, error) {
	results, err := r.retriever.Retrieve(ctx, query, queryEmbedding)
	if err != nil {
		return nil, err
	}

	if r.config.UseReranking {
		results = r.rerankWithContext(query, results)
	}

	return results, nil
}

// chunkDocument 使用滑动窗口分块文档
func (r *ContextualRetrieval) chunkDocument(doc Document) []string {
	content := doc.Content
	if content == "" {
		return nil
	}

	chunkSize := r.config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 500
	}
	overlap := r.config.ChunkOverlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	paragraphs := strings.Split(content, "\n\n")

	chunks := make([]string, 0)
	currentChunk := ""

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// 如果单个段落超过 chunkSize，按句子拆分
		if len(para) > chunkSize {
			subChunks := r.splitLongParagraph(para, chunkSize, overlap)
			for _, sc := range subChunks {
				if currentChunk != "" && len(currentChunk)+len(sc) > chunkSize {
					chunks = append(chunks, strings.TrimSpace(currentChunk))
					currentChunk = r.getOverlapSuffix(currentChunk, overlap)
				}
				if currentChunk != "" {
					currentChunk += "\n\n"
				}
				currentChunk += sc
			}
			continue
		}

		if len(currentChunk)+len(para)+2 > chunkSize {
			if currentChunk != "" {
				chunks = append(chunks, strings.TrimSpace(currentChunk))
				currentChunk = r.getOverlapSuffix(currentChunk, overlap)
			}
		}

		if currentChunk != "" {
			currentChunk += "\n\n"
		}
		currentChunk += para
	}

	if strings.TrimSpace(currentChunk) != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}

// splitLongParagraph 拆分超长段落
func (r *ContextualRetrieval) splitLongParagraph(para string, chunkSize, overlap int) []string {
	sentences := splitSentences(para)
	chunks := make([]string, 0)
	current := ""

	for _, sent := range sentences {
		if len(current)+len(sent) > chunkSize && current != "" {
			chunks = append(chunks, strings.TrimSpace(current))
			current = r.getOverlapSuffix(current, overlap)
		}
		if current != "" {
			current += " "
		}
		current += sent
	}

	if strings.TrimSpace(current) != "" {
		chunks = append(chunks, strings.TrimSpace(current))
	}

	return chunks
}

// splitSentences 按句子分割
func splitSentences(text string) []string {
	var sentences []string
	current := ""
	for _, r := range text {
		current += string(r)
		if r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' {
			if strings.TrimSpace(current) != "" {
				sentences = append(sentences, strings.TrimSpace(current))
			}
			current = ""
		}
	}
	if strings.TrimSpace(current) != "" {
		sentences = append(sentences, strings.TrimSpace(current))
	}
	return sentences
}

// getOverlapSuffix 获取文本末尾的 overlap 部分
func (r *ContextualRetrieval) getOverlapSuffix(text string, overlap int) string {
	if overlap <= 0 || len(text) <= overlap {
		return ""
	}
	return text[len(text)-overlap:]
}

// renderContextTemplate 渲染上下文模板
func (r *ContextualRetrieval) renderContextTemplate(doc Document, chunk, context string) string {
	template := r.config.ContextTemplate

	template = strings.ReplaceAll(template, "{{document_title}}", getMetadataString(doc.Metadata, "title"))
	template = strings.ReplaceAll(template, "{{section_title}}", getMetadataString(doc.Metadata, "section"))
	template = strings.ReplaceAll(template, "{{context}}", context)
	template = strings.ReplaceAll(template, "{{content}}", chunk)

	return template
}

// rerankWithContext 基于上下文重排序
func (r *ContextualRetrieval) rerankWithContext(query string, results []RetrievalResult) []RetrievalResult {
	for i := range results {
		context := getMetadataString(results[i].Document.Metadata, "context")
		contextScore := r.calculateContextRelevance(query, context)
		results[i].FinalScore = results[i].FinalScore*(1-r.config.ContextWeight) +
			contextScore*r.config.ContextWeight
	}

	sortResultsByFinalScore(results)

	return results
}

// calculateContextRelevance 使用 BM25 算法计算上下文相关性
func (r *ContextualRetrieval) calculateContextRelevance(query, context string) float64 {
	if context == "" || query == "" {
		return 0.0
	}

	queryTerms := contextualTokenize(query)
	contextTerms := contextualTokenize(context)

	if len(queryTerms) == 0 || len(contextTerms) == 0 {
		return 0.0
	}

	// 计算 context 中每个词的词频
	tf := make(map[string]int)
	for _, term := range contextTerms {
		tf[term]++
	}

	docLen := float64(len(contextTerms))
	k1 := r.config.BM25K1
	b := r.config.BM25B

	// 获取平均文档长度
	r.mu.RLock()
	avgDL := r.avgDocLen
	if avgDL == 0 {
		avgDL = 100.0
	}
	totalDocs := r.totalDocs
	if totalDocs == 0 {
		totalDocs = 1
	}
	r.mu.RUnlock()

	score := 0.0
	for _, term := range queryTerms {
		termFreq := float64(tf[term])
		if termFreq == 0 {
			continue
		}

		idf := r.getIDF(term, totalDocs)
		tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*docLen/avgDL))
		score += idf * tfNorm
	}

	// 归一化到 [0, 1]
	maxScore := float64(len(queryTerms)) * math.Log(float64(totalDocs)+1)
	if maxScore > 0 {
		score = score / maxScore
	}
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// getIDF 获取词的 IDF 值
func (r *ContextualRetrieval) getIDF(term string, totalDocs int) float64 {
	r.mu.RLock()
	if idf, ok := r.idfCache[term]; ok {
		r.mu.RUnlock()
		return idf
	}
	r.mu.RUnlock()

	// 简化 IDF：假设每个词在约 10% 的文档中出现
	n := float64(totalDocs) * 0.1
	if n < 1 {
		n = 1
	}
	idf := math.Log((float64(totalDocs)-n+0.5)/(n+0.5) + 1)

	r.mu.Lock()
	if r.idfCache == nil {
		r.idfCache = make(map[string]float64)
	}
	r.idfCache[term] = idf
	r.mu.Unlock()

	return idf
}

// UpdateIDFStats 更新 IDF 统计信息（在索引文档后调用）
func (r *ContextualRetrieval) UpdateIDFStats(docs []Document) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.totalDocs += len(docs)

	totalLen := 0
	docFreq := make(map[string]int)

	for _, doc := range docs {
		terms := contextualTokenize(doc.Content)
		totalLen += len(terms)

		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				docFreq[term]++
				seen[term] = true
			}
		}
	}

	if r.totalDocs > 0 {
		r.totalDocLen += totalLen
		r.avgDocLen = float64(r.totalDocLen) / float64(r.totalDocs)
	}

	if r.idfCache == nil {
		r.idfCache = make(map[string]float64)
	}
	for term, df := range docFreq {
		n := float64(df)
		r.idfCache[term] = math.Log((float64(r.totalDocs)-n+0.5)/(n+0.5) + 1)
	}
}

// contextualTokenize 分词（支持中英文），用于 BM25 计算
func contextualTokenize(text string) []string {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x4e00 && r <= 0x9fff)
	})

	result := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) > 1 || (len([]rune(w)) == 1 && []rune(w)[0] >= 0x4e00) {
			result = append(result, w)
		}
	}
	return result
}

// buildCacheKey 构建缓存 key
func (r *ContextualRetrieval) buildCacheKey(docID, chunk string) string {
	h := sha256.Sum256([]byte(chunk))
	return docID + ":" + hex.EncodeToString(h[:8])
}

// getFromCache 从缓存获取上下文
func (r *ContextualRetrieval) getFromCache(key string) (string, bool) {
	val, ok := r.contextCache.Load(key)
	if !ok {
		return "", false
	}
	entry := val.(*contextCacheEntry)
	if r.config.CacheTTL > 0 && time.Since(entry.createdAt) > r.config.CacheTTL {
		r.contextCache.Delete(key)
		return "", false
	}
	return entry.context, true
}

// putToCache 写入缓存
func (r *ContextualRetrieval) putToCache(key, context string) {
	r.contextCache.Store(key, &contextCacheEntry{
		context:   context,
		createdAt: time.Now(),
	})
}

// CleanExpiredCache 清理过期缓存
func (r *ContextualRetrieval) CleanExpiredCache() int {
	cleaned := 0
	r.contextCache.Range(func(key, value any) bool {
		entry := value.(*contextCacheEntry)
		if r.config.CacheTTL > 0 && time.Since(entry.createdAt) > r.config.CacheTTL {
			r.contextCache.Delete(key)
			cleaned++
		}
		return true
	})
	return cleaned
}

// EmbeddingSimilarity 计算两个 embedding 向量的余弦相似度
func EmbeddingSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
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

// rerankWithEmbedding 使用 embedding 相似度重排序
func (r *ContextualRetrieval) rerankWithEmbedding(queryEmbedding []float64, results []RetrievalResult) []RetrievalResult {
	if len(queryEmbedding) == 0 {
		return results
	}

	for i := range results {
		if results[i].Document.Embedding != nil {
			embScore := EmbeddingSimilarity(queryEmbedding, results[i].Document.Embedding)
			results[i].FinalScore = results[i].FinalScore*0.6 + embScore*0.4
		}
	}

	sortResultsByFinalScore(results)
	return results
}

// getMetadataString 获取元数据字符串
func getMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}

	val, ok := metadata[key]
	if !ok {
		return ""
	}

	str, ok := val.(string)
	if !ok {
		return fmt.Sprintf("%v", val)
	}

	return str
}

// sortResultsByFinalScore 按最终分数排序
func sortResultsByFinalScore(results []RetrievalResult) {
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].FinalScore < results[j+1].FinalScore {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
