package retrieval

import (
	"context"
	"fmt"
	"math"
	"sort"

	"go.uber.org/zap"
)

// Reranker 重排序器接口
type Reranker interface {
	// Rerank 重排序结果
	Rerank(ctx context.Context, query string, results []RetrievalResult) ([]RetrievalResult, error)
}

// RerankerType 重排序器类型
type RerankerType string

const (
	RerankerSimple       RerankerType = "simple"        // 简单词重叠
	RerankerCrossEncoder RerankerType = "cross_encoder" // Cross-Encoder 模型
	RerankerLLM          RerankerType = "llm"           // LLM 重排序
)

// ====== Cross-Encoder 重排序器 ======

// CrossEncoderReranker Cross-Encoder 重排序器（生产级）
// 基于 Sentence Transformers 的 Cross-Encoder 模型
type CrossEncoderReranker struct {
	modelProvider CrossEncoderProvider
	config        CrossEncoderConfig
	logger        *zap.Logger
}

// CrossEncoderConfig Cross-Encoder 配置
type CrossEncoderConfig struct {
	ModelName    string  `json:"model_name"`    // 模型名称
	MaxLength    int     `json:"max_length"`    // 最大输入长度
	BatchSize    int     `json:"batch_size"`    // 批处理大小
	ScoreWeight  float64 `json:"score_weight"`  // 重排序分数权重
	OriginalWeight float64 `json:"original_weight"` // 原始分数权重
}

// DefaultCrossEncoderConfig 默认配置
func DefaultCrossEncoderConfig() CrossEncoderConfig {
	return CrossEncoderConfig{
		ModelName:      "cross-encoder/ms-marco-MiniLM-L-6-v2",
		MaxLength:      512,
		BatchSize:      32,
		ScoreWeight:    0.7,
		OriginalWeight: 0.3,
	}
}

// CrossEncoderProvider Cross-Encoder 提供器接口
type CrossEncoderProvider interface {
	// Score 计算查询-文档对的相关性分数
	Score(ctx context.Context, pairs []QueryDocPair) ([]float64, error)
}

// QueryDocPair 查询-文档对
type QueryDocPair struct {
	Query    string
	Document string
}

// NewCrossEncoderReranker 创建 Cross-Encoder 重排序器
func NewCrossEncoderReranker(
	provider CrossEncoderProvider,
	config CrossEncoderConfig,
	logger *zap.Logger,
) *CrossEncoderReranker {
	return &CrossEncoderReranker{
		modelProvider: provider,
		config:        config,
		logger:        logger,
	}
}

// Rerank 重排序
func (r *CrossEncoderReranker) Rerank(ctx context.Context, query string, results []RetrievalResult) ([]RetrievalResult, error) {
	if len(results) == 0 {
		return results, nil
	}
	
	// 限制候选数量（Cross-Encoder 计算成本高）
	// 基于 2025 最佳实践：100-200 个候选
	maxCandidates := 200
	if len(results) > maxCandidates {
		r.logger.Info("limiting candidates for reranking",
			zap.Int("original", len(results)),
			zap.Int("limited", maxCandidates))
		results = results[:maxCandidates]
	}
	
	r.logger.Info("cross-encoder reranking",
		zap.Int("results", len(results)),
		zap.String("model", r.config.ModelName))
	
	// 1. 准备查询-文档对
	pairs := make([]QueryDocPair, len(results))
	for i, result := range results {
		// 截断文档内容
		content := result.Document.Content
		if len(content) > r.config.MaxLength*4 {
			content = content[:r.config.MaxLength*4]
		}
		
		pairs[i] = QueryDocPair{
			Query:    query,
			Document: content,
		}
	}
	
	// 2. 批量计算分数
	scores, err := r.batchScore(ctx, pairs)
	if err != nil {
		return nil, fmt.Errorf("failed to score pairs: %w", err)
	}
	
	// 3. 混合原始分数和重排序分数
	for i := range results {
		originalScore := results[i].FinalScore
		rerankScore := scores[i]
		
		// 归一化重排序分数（sigmoid）
		rerankScore = 1.0 / (1.0 + math.Exp(-rerankScore))
		
		// 混合分数
		results[i].RerankScore = rerankScore
		results[i].FinalScore = originalScore*r.config.OriginalWeight +
			rerankScore*r.config.ScoreWeight
	}
	
	// 4. 重新排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
	
	r.logger.Info("reranking completed",
		zap.Float64("top_score", results[0].FinalScore))
	
	return results, nil
}

// batchScore 批量计算分数
func (r *CrossEncoderReranker) batchScore(ctx context.Context, pairs []QueryDocPair) ([]float64, error) {
	scores := make([]float64, len(pairs))
	
	// 分批处理
	for i := 0; i < len(pairs); i += r.config.BatchSize {
		end := i + r.config.BatchSize
		if end > len(pairs) {
			end = len(pairs)
		}
		
		batch := pairs[i:end]
		batchScores, err := r.modelProvider.Score(ctx, batch)
		if err != nil {
			return nil, err
		}
		
		copy(scores[i:end], batchScores)
	}
	
	return scores, nil
}

// ====== LLM 重排序器 ======

// LLMReranker LLM 重排序器（使用 LLM 判断相关性）
type LLMReranker struct {
	llmProvider LLMRerankerProvider
	config      LLMRerankerConfig
	logger      *zap.Logger
}

// LLMRerankerConfig LLM 重排序配置
type LLMRerankerConfig struct {
	MaxCandidates int     `json:"max_candidates"` // 最大候选数
	Temperature   float64 `json:"temperature"`    // 温度
	PromptTemplate string `json:"prompt_template"` // 提示模板
}

// DefaultLLMRerankerConfig 默认配置
func DefaultLLMRerankerConfig() LLMRerankerConfig {
	return LLMRerankerConfig{
		MaxCandidates: 20,
		Temperature:   0.0,
		PromptTemplate: `Given the query and document, rate the relevance on a scale of 0-10.

Query: {{query}}

Document: {{document}}

Relevance score (0-10):`,
	}
}

// LLMRerankerProvider LLM 重排序提供器
type LLMRerankerProvider interface {
	// ScoreRelevance 评估相关性
	ScoreRelevance(ctx context.Context, query, document string) (float64, error)
}

// NewLLMReranker 创建 LLM 重排序器
func NewLLMReranker(
	provider LLMRerankerProvider,
	config LLMRerankerConfig,
	logger *zap.Logger,
) *LLMReranker {
	return &LLMReranker{
		llmProvider: provider,
		config:      config,
		logger:      logger,
	}
}

// Rerank 重排序
func (r *LLMReranker) Rerank(ctx context.Context, query string, results []RetrievalResult) ([]RetrievalResult, error) {
	if len(results) == 0 {
		return results, nil
	}
	
	// 限制候选数量（LLM 重排序成本高）
	candidates := results
	if len(candidates) > r.config.MaxCandidates {
		candidates = candidates[:r.config.MaxCandidates]
	}
	
	r.logger.Info("LLM reranking",
		zap.Int("candidates", len(candidates)))
	
	// 逐个评分
	for i := range candidates {
		score, err := r.llmProvider.ScoreRelevance(ctx, query, candidates[i].Document.Content)
		if err != nil {
			r.logger.Warn("failed to score document",
				zap.String("doc_id", candidates[i].Document.ID),
				zap.Error(err))
			score = candidates[i].FinalScore * 10 // 回退到原始分数
		}
		
		// 归一化到 0-1
		candidates[i].RerankScore = score / 10.0
		candidates[i].FinalScore = candidates[i].RerankScore
	}
	
	// 重新排序
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FinalScore > candidates[j].FinalScore
	})
	
	return candidates, nil
}

// ====== 简单重排序器 ======

// SimpleReranker 简单重排序器（基于词重叠和位置）
type SimpleReranker struct {
	logger *zap.Logger
}

// NewSimpleReranker 创建简单重排序器
func NewSimpleReranker(logger *zap.Logger) *SimpleReranker {
	return &SimpleReranker{logger: logger}
}

// Rerank 重排序
func (r *SimpleReranker) Rerank(ctx context.Context, query string, results []RetrievalResult) ([]RetrievalResult, error) {
	if len(results) == 0 {
		return results, nil
	}
	
	queryTerms := tokenize(query)
	
	for i := range results {
		docTerms := tokenize(results[i].Document.Content)
		
		// 计算多个特征
		exactMatch := r.exactMatchScore(queryTerms, docTerms)
		termFreq := r.termFrequencyScore(queryTerms, docTerms)
		proximity := r.proximityScore(queryTerms, docTerms)
		
		// 混合分数
		rerankScore := exactMatch*0.4 + termFreq*0.4 + proximity*0.2
		
		results[i].RerankScore = rerankScore
		results[i].FinalScore = results[i].FinalScore*0.5 + rerankScore*0.5
	}
	
	// 重新排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
	
	return results, nil
}

// exactMatchScore 精确匹配分数
func (r *SimpleReranker) exactMatchScore(queryTerms, docTerms []string) float64 {
	if len(queryTerms) == 0 {
		return 0.0
	}
	
	matchCount := 0
	for _, qt := range queryTerms {
		for _, dt := range docTerms {
			if qt == dt {
				matchCount++
				break
			}
		}
	}
	
	return float64(matchCount) / float64(len(queryTerms))
}

// termFrequencyScore 词频分数
func (r *SimpleReranker) termFrequencyScore(queryTerms, docTerms []string) float64 {
	if len(queryTerms) == 0 {
		return 0.0
	}
	
	termFreq := make(map[string]int)
	for _, dt := range docTerms {
		termFreq[dt]++
	}
	
	totalFreq := 0
	for _, qt := range queryTerms {
		totalFreq += termFreq[qt]
	}
	
	// 归一化
	return math.Min(float64(totalFreq)/float64(len(queryTerms)*3), 1.0)
}

// proximityScore 邻近度分数（查询词在文档中的距离）
func (r *SimpleReranker) proximityScore(queryTerms, docTerms []string) float64 {
	if len(queryTerms) <= 1 {
		return 1.0
	}
	
	// 查找查询词在文档中的位置
	positions := make(map[string][]int)
	for i, dt := range docTerms {
		for _, qt := range queryTerms {
			if dt == qt {
				positions[qt] = append(positions[qt], i)
			}
		}
	}
	
	// 计算最小跨度
	minSpan := len(docTerms)
	for _, pos1 := range positions {
		for _, pos2 := range positions {
			for _, p1 := range pos1 {
				for _, p2 := range pos2 {
					span := abs(p1 - p2)
					if span < minSpan && span > 0 {
						minSpan = span
					}
				}
			}
		}
	}
	
	// 归一化（跨度越小，分数越高）
	if minSpan == len(docTerms) {
		return 0.0
	}
	
	return 1.0 / (1.0 + float64(minSpan)/10.0)
}

// ====== 辅助函数 ======

func tokenize(text string) []string {
	// 简化分词
	words := []string{}
	current := ""
	
	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\t' {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	
	if current != "" {
		words = append(words, current)
	}
	
	return words
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
