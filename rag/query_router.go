// 包布为检索策略提供智能查询路由.
// 这个模块会根据查询特性,向最合适的检索方法查询.
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// QQ 查询运行类型QQ

// 检索策略代表检索策略
type RetrievalStrategy string

const (
	StrategyVector      RetrievalStrategy = "vector"       // Pure vector/semantic search
	StrategyBM25        RetrievalStrategy = "bm25"         // Pure keyword/BM25 search
	StrategyHybrid      RetrievalStrategy = "hybrid"       // Combined vector + BM25
	StrategyMultiHop    RetrievalStrategy = "multi_hop"    // Multi-hop reasoning
	StrategyGraphRAG    RetrievalStrategy = "graph_rag"    // Graph-based retrieval
	StrategyContextual  RetrievalStrategy = "contextual"   // Contextual retrieval
	StrategyDense       RetrievalStrategy = "dense"        // Dense passage retrieval
	StrategySparse      RetrievalStrategy = "sparse"       // Sparse retrieval (TF-IDF)
)

// 运行决定代表查询的路径决定
type RoutingDecision struct {
	Query            string                       `json:"query"`
	SelectedStrategy RetrievalStrategy            `json:"selected_strategy"`
	Confidence       float64                      `json:"confidence"`
	Scores           map[RetrievalStrategy]float64 `json:"scores"`
	Reasoning        string                       `json:"reasoning,omitempty"`
	Metadata         map[string]any               `json:"metadata,omitempty"`
	Timestamp        time.Time                    `json:"timestamp"`
}

// 策略Config 配置检索策略
type StrategyConfig struct {
	Strategy    RetrievalStrategy `json:"strategy"`
	Enabled     bool              `json:"enabled"`
	Weight      float64           `json:"weight"`       // Base weight for this strategy
	MinScore    float64           `json:"min_score"`    // Minimum score to use this strategy
	MaxTokens   int               `json:"max_tokens"`   // Max query tokens for this strategy
	Conditions  []RoutingCondition `json:"conditions"`  // Conditions that favor this strategy
}

// 路由条件代表了路由条件
type RoutingCondition struct {
	Type      string  `json:"type"`       // "intent", "keyword", "length", "complexity"
	Value     string  `json:"value"`      // Value to match
	Weight    float64 `json:"weight"`     // Weight adjustment when matched
	Operator  string  `json:"operator"`   // "equals", "contains", "greater", "less"
}

// 查询路透社 Config 配置查询路由器
type QueryRouterConfig struct {
	// 战略配置
	Strategies []StrategyConfig `json:"strategies"`

	// 默认策略
	DefaultStrategy RetrievalStrategy `json:"default_strategy"`

	// 运行设置
	EnableLLMRouting     bool    `json:"enable_llm_routing"`      // Use LLM for routing decisions
	EnableAdaptiveRouting bool   `json:"enable_adaptive_routing"` // Learn from feedback
	ConfidenceThreshold  float64 `json:"confidence_threshold"`    // Min confidence for routing

	// 后退设置
	EnableFallback       bool              `json:"enable_fallback"`
	FallbackStrategy     RetrievalStrategy `json:"fallback_strategy"`

	// 缓存
	EnableCache bool          `json:"enable_cache"`
	CacheTTL    time.Duration `json:"cache_ttl"`

	// 日志
	LogDecisions bool `json:"log_decisions"`
}

// 默认查询程序 Config 返回默认配置
func DefaultQueryRouterConfig() QueryRouterConfig {
	return QueryRouterConfig{
		Strategies: []StrategyConfig{
			{
				Strategy: StrategyVector,
				Enabled:  true,
				Weight:   1.0,
				MinScore: 0.3,
				Conditions: []RoutingCondition{
					{Type: "intent", Value: "factual", Weight: 0.2, Operator: "equals"},
					{Type: "intent", Value: "explanation", Weight: 0.3, Operator: "equals"},
					{Type: "length", Value: "short", Weight: 0.2, Operator: "equals"},
				},
			},
			{
				Strategy: StrategyBM25,
				Enabled:  true,
				Weight:   0.8,
				MinScore: 0.3,
				Conditions: []RoutingCondition{
					{Type: "keyword", Value: "exact", Weight: 0.3, Operator: "contains"},
					{Type: "keyword", Value: "specific", Weight: 0.2, Operator: "contains"},
					{Type: "intent", Value: "factual", Weight: 0.1, Operator: "equals"},
				},
			},
			{
				Strategy: StrategyHybrid,
				Enabled:  true,
				Weight:   1.2,
				MinScore: 0.3,
				Conditions: []RoutingCondition{
					{Type: "complexity", Value: "medium", Weight: 0.2, Operator: "equals"},
					{Type: "intent", Value: "comparison", Weight: 0.3, Operator: "equals"},
				},
			},
			{
				Strategy: StrategyMultiHop,
				Enabled:  true,
				Weight:   0.9,
				MinScore: 0.5,
				Conditions: []RoutingCondition{
					{Type: "complexity", Value: "high", Weight: 0.4, Operator: "equals"},
					{Type: "intent", Value: "analytical", Weight: 0.3, Operator: "equals"},
					{Type: "intent", Value: "causal", Weight: 0.3, Operator: "equals"},
					{Type: "length", Value: "long", Weight: 0.2, Operator: "equals"},
				},
			},
			{
				Strategy: StrategyGraphRAG,
				Enabled:  true,
				Weight:   0.85,
				MinScore: 0.4,
				Conditions: []RoutingCondition{
					{Type: "intent", Value: "causal", Weight: 0.4, Operator: "equals"},
					{Type: "keyword", Value: "relationship", Weight: 0.3, Operator: "contains"},
					{Type: "keyword", Value: "connected", Weight: 0.2, Operator: "contains"},
				},
			},
			{
				Strategy: StrategyContextual,
				Enabled:  true,
				Weight:   1.0,
				MinScore: 0.3,
				Conditions: []RoutingCondition{
					{Type: "has_context", Value: "true", Weight: 0.4, Operator: "equals"},
					{Type: "intent", Value: "procedural", Weight: 0.2, Operator: "equals"},
				},
			},
		},
		DefaultStrategy:       StrategyHybrid,
		EnableLLMRouting:      true,
		EnableAdaptiveRouting: false,
		ConfidenceThreshold:   0.5,
		EnableFallback:        true,
		FallbackStrategy:      StrategyHybrid,
		EnableCache:           true,
		CacheTTL:              10 * time.Minute,
		LogDecisions:          true,
	}
}

// 查询路由器

// 查询路透查询到适当的检索策略
type QueryRouter struct {
	config           QueryRouterConfig
	queryTransformer *QueryTransformer
	llmProvider      QueryLLMProvider
	cache            *routingCache
	feedbackStore    *feedbackStore
	logger           *zap.Logger
}

// 路径 缓存缓存路由决定
type routingCache struct {
	entries map[string]*RoutingDecision
	mu      sync.RWMutex
	ttl     time.Duration
}

func newRoutingCache(ttl time.Duration) *routingCache {
	return &routingCache{
		entries: make(map[string]*RoutingDecision),
		ttl:     ttl,
	}
}

func (c *routingCache) get(key string) (*RoutingDecision, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	decision, ok := c.entries[key]
	if !ok || time.Since(decision.Timestamp) > c.ttl {
		return nil, false
	}
	return decision, true
}

func (c *routingCache) set(key string, decision *RoutingDecision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = decision
}

// 储存用于适应性学习的路由反馈
type feedbackStore struct {
	feedback map[string][]RoutingFeedback
	mu       sync.RWMutex
}

// RoutingFeedback代表了对路线决定的反馈
type RoutingFeedback struct {
	Query            string            `json:"query"`
	SelectedStrategy RetrievalStrategy `json:"selected_strategy"`
	Success          bool              `json:"success"`
	Score            float64           `json:"score"`
	Timestamp        time.Time         `json:"timestamp"`
}

func newFeedbackStore() *feedbackStore {
	return &feedbackStore{
		feedback: make(map[string][]RoutingFeedback),
	}
}

func (s *feedbackStore) add(feedback RoutingFeedback) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(feedback.SelectedStrategy)
	s.feedback[key] = append(s.feedback[key], feedback)

	// 只保留最近反馈
	if len(s.feedback[key]) > 1000 {
		s.feedback[key] = s.feedback[key][500:]
	}
}

func (s *feedbackStore) getSuccessRate(strategy RetrievalStrategy) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	feedbacks := s.feedback[string(strategy)]
	if len(feedbacks) == 0 {
		return 0.5 // Default neutral rate
	}

	successCount := 0
	for _, f := range feedbacks {
		if f.Success {
			successCount++
		}
	}

	return float64(successCount) / float64(len(feedbacks))
}

// 新建查询路由器创建新的查询路由器
func NewQueryRouter(
	config QueryRouterConfig,
	queryTransformer *QueryTransformer,
	llmProvider QueryLLMProvider,
	logger *zap.Logger,
) *QueryRouter {
	if logger == nil {
		logger = zap.NewNop()
	}

	var cache *routingCache
	if config.EnableCache {
		cache = newRoutingCache(config.CacheTTL)
	}

	var feedback *feedbackStore
	if config.EnableAdaptiveRouting {
		feedback = newFeedbackStore()
	}

	return &QueryRouter{
		config:           config,
		queryTransformer: queryTransformer,
		llmProvider:      llmProvider,
		cache:            cache,
		feedbackStore:    feedback,
		logger:           logger.With(zap.String("component", "query_router")),
	}
}

// 路由决定查询的最佳检索策略
func (r *QueryRouter) Route(ctx context.Context, query string) (*RoutingDecision, error) {
	// 检查缓存
	if r.cache != nil {
		if cached, ok := r.cache.get(query); ok {
			r.logger.Debug("cache hit", zap.String("query", query))
			return cached, nil
		}
	}

	decision := &RoutingDecision{
		Query:     query,
		Scores:    make(map[RetrievalStrategy]float64),
		Metadata:  make(map[string]any),
		Timestamp: time.Now(),
	}

	// 分析查询特性
	queryFeatures := r.analyzeQuery(ctx, query)
	decision.Metadata["features"] = queryFeatures

	// 计算每个策略的分数
	for _, strategyConfig := range r.config.Strategies {
		if !strategyConfig.Enabled {
			continue
		}

		score := r.calculateStrategyScore(strategyConfig, queryFeatures)

		// 应用适应性学习调整
		if r.config.EnableAdaptiveRouting && r.feedbackStore != nil {
			successRate := r.feedbackStore.getSuccessRate(strategyConfig.Strategy)
			score *= (0.5 + successRate) // Adjust based on historical success
		}

		decision.Scores[strategyConfig.Strategy] = score
	}

	// 在复杂的路由决定中使用 LLM
	if r.config.EnableLLMRouting && r.llmProvider != nil {
		llmDecision, err := r.routeWithLLM(ctx, query, queryFeatures)
		if err == nil && llmDecision != nil {
			// 将 LLM 决定与基于规则的分数合并
			for strategy, score := range llmDecision.Scores {
				if existingScore, ok := decision.Scores[strategy]; ok {
					decision.Scores[strategy] = (existingScore + score) / 2
				} else {
					decision.Scores[strategy] = score
				}
			}
			decision.Reasoning = llmDecision.Reasoning
		}
	}

	// 选择最佳策略
	bestStrategy, bestScore := r.selectBestStrategy(decision.Scores)

	// 检查信任阈值
	if bestScore < r.config.ConfidenceThreshold {
		if r.config.EnableFallback {
			bestStrategy = r.config.FallbackStrategy
			decision.Metadata["fallback_used"] = true
		} else {
			bestStrategy = r.config.DefaultStrategy
			decision.Metadata["default_used"] = true
		}
	}

	decision.SelectedStrategy = bestStrategy
	decision.Confidence = bestScore

	// 缓存决定
	if r.cache != nil {
		r.cache.set(query, decision)
	}

	// 日志决定
	if r.config.LogDecisions {
		r.logger.Info("routing decision",
			zap.String("query", truncateContext(query, 50)),
			zap.String("strategy", string(bestStrategy)),
			zap.Float64("confidence", bestScore))
	}

	return decision, nil
}

// 查询Features 代表已分析的查询特性
type QueryFeatures struct {
	Intent      QueryIntent `json:"intent"`
	Complexity  string      `json:"complexity"`  // "low", "medium", "high"
	Length      string      `json:"length"`      // "short", "medium", "long"
	HasEntities bool        `json:"has_entities"`
	HasKeywords bool        `json:"has_keywords"`
	IsQuestion  bool        `json:"is_question"`
	Keywords    []string    `json:"keywords"`
	Entities    []string    `json:"entities"`
	WordCount   int         `json:"word_count"`
}

// 分析查询特性
func (r *QueryRouter) analyzeQuery(ctx context.Context, query string) QueryFeatures {
	features := QueryFeatures{
		Intent:     IntentUnknown,
		Complexity: "medium",
		Length:     "medium",
		WordCount:  len(strings.Fields(query)),
	}

	// 确定长度类别
	if features.WordCount <= 5 {
		features.Length = "short"
	} else if features.WordCount <= 15 {
		features.Length = "medium"
	} else {
		features.Length = "long"
	}

	// 检查问题
	features.IsQuestion = strings.HasSuffix(strings.TrimSpace(query), "?") ||
		strings.HasPrefix(strings.ToLower(query), "what") ||
		strings.HasPrefix(strings.ToLower(query), "how") ||
		strings.HasPrefix(strings.ToLower(query), "why") ||
		strings.HasPrefix(strings.ToLower(query), "when") ||
		strings.HasPrefix(strings.ToLower(query), "where") ||
		strings.HasPrefix(strings.ToLower(query), "who")

	// 使用查询变压器进行详细分析
	if r.queryTransformer != nil {
		transformed, err := r.queryTransformer.Transform(ctx, query)
		if err == nil {
			features.Intent = transformed.Intent
			features.Keywords = transformed.Keywords
			features.Entities = transformed.Entities
			features.HasKeywords = len(transformed.Keywords) > 0
			features.HasEntities = len(transformed.Entities) > 0
		}
	}

	// 确定复杂性
	features.Complexity = r.determineComplexity(query, features)

	return features
}

// 复杂度决定了查询的复杂性
func (r *QueryRouter) determineComplexity(query string, features QueryFeatures) string {
	complexityScore := 0.0

	// 长度增加复杂性
	if features.Length == "long" {
		complexityScore += 0.3
	} else if features.Length == "medium" {
		complexityScore += 0.15
	}

	// 多个实体增加复杂性
	if len(features.Entities) > 2 {
		complexityScore += 0.2
	} else if len(features.Entities) > 0 {
		complexityScore += 0.1
	}

	// 某些意图比较复杂
	complexIntents := map[QueryIntent]float64{
		IntentAnalytical:   0.3,
		IntentComparison:   0.25,
		IntentCausal:       0.3,
		IntentHypothetical: 0.25,
		IntentAggregation:  0.2,
	}
	if score, ok := complexIntents[features.Intent]; ok {
		complexityScore += score
	}

	// 检查复杂的查询模式
	queryLower := strings.ToLower(query)
	complexPatterns := []string{
		"compare", "difference between", "relationship",
		"impact of", "effect of", "analyze",
		"multiple", "several", "various",
	}
	for _, pattern := range complexPatterns {
		if strings.Contains(queryLower, pattern) {
			complexityScore += 0.1
		}
	}

	// 分类复杂性
	if complexityScore >= 0.6 {
		return "high"
	} else if complexityScore >= 0.3 {
		return "medium"
	}
	return "low"
}

// 计算策略的分数
func (r *QueryRouter) calculateStrategyScore(config StrategyConfig, features QueryFeatures) float64 {
	score := config.Weight

	// 应用条件
	for _, condition := range config.Conditions {
		if r.matchCondition(condition, features) {
			score += condition.Weight
		}
	}

	// 将分数正常化
	return math.Min(score, 2.0) / 2.0
}

// 匹配条件检查, 如果条件符合查询功能
func (r *QueryRouter) matchCondition(condition RoutingCondition, features QueryFeatures) bool {
	switch condition.Type {
	case "intent":
		return string(features.Intent) == condition.Value

	case "complexity":
		return features.Complexity == condition.Value

	case "length":
		return features.Length == condition.Value

	case "keyword":
		for _, kw := range features.Keywords {
			if strings.Contains(strings.ToLower(kw), strings.ToLower(condition.Value)) {
				return true
			}
		}
		return false

	case "has_entities":
		return fmt.Sprintf("%v", features.HasEntities) == condition.Value

	case "has_context":
		// 外部根据对话背景设置
		return false

	case "is_question":
		return fmt.Sprintf("%v", features.IsQuestion) == condition.Value
	}

	return false
}

// 路由 WithLLM 在路由决定中使用LLM
func (r *QueryRouter) routeWithLLM(ctx context.Context, query string, features QueryFeatures) (*RoutingDecision, error) {
	// 构建战略说明
	strategyDescriptions := `
- vector: Best for semantic similarity search, conceptual queries, and finding related content
- bm25: Best for exact keyword matching, specific terms, and technical queries
- hybrid: Best for balanced queries that need both semantic and keyword matching
- multi_hop: Best for complex queries requiring multiple retrieval steps and reasoning
- graph_rag: Best for queries about relationships, connections, and entity networks
- contextual: Best for queries that depend on conversation context or follow-up questions`

	prompt := fmt.Sprintf(`Given the following query and its characteristics, select the most appropriate retrieval strategy.

Query: %s

Query characteristics:
- Intent: %s
- Complexity: %s
- Length: %s
- Is question: %v
- Has entities: %v
- Keywords: %v

Available strategies:
%s

Respond in JSON format:
{
  "strategy": "strategy_name",
  "confidence": 0.0-1.0,
  "reasoning": "brief explanation"
}`,
		query,
		features.Intent,
		features.Complexity,
		features.Length,
		features.IsQuestion,
		features.HasEntities,
		features.Keywords,
		strategyDescriptions)

	response, err := r.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// 解析响应
	var llmResponse struct {
		Strategy   string  `json:"strategy"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}

	// 尝试从响应中提取 JSON
	response = strings.TrimSpace(response)
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")
	if startIdx >= 0 && endIdx > startIdx {
		response = response[startIdx : endIdx+1]
	}

	if err := json.Unmarshal([]byte(response), &llmResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	decision := &RoutingDecision{
		Query:            query,
		SelectedStrategy: RetrievalStrategy(llmResponse.Strategy),
		Confidence:       llmResponse.Confidence,
		Reasoning:        llmResponse.Reasoning,
		Scores:           make(map[RetrievalStrategy]float64),
		Timestamp:        time.Now(),
	}

	// 设定选定策略的分数
	decision.Scores[decision.SelectedStrategy] = llmResponse.Confidence

	return decision, nil
}

// 选择得分最高的战略
func (r *QueryRouter) selectBestStrategy(scores map[RetrievalStrategy]float64) (RetrievalStrategy, float64) {
	var bestStrategy RetrievalStrategy
	var bestScore float64 = -1

	for strategy, score := range scores {
		if score > bestScore {
			bestScore = score
			bestStrategy = strategy
		}
	}

	if bestStrategy == "" {
		return r.config.DefaultStrategy, 0.5
	}

	return bestStrategy, bestScore
}

// 反馈方法

// 记录 Feedback 记录对路径决定的反馈
func (r *QueryRouter) RecordFeedback(feedback RoutingFeedback) {
	if r.feedbackStore != nil {
		r.feedbackStore.add(feedback)
		r.logger.Debug("feedback recorded",
			zap.String("strategy", string(feedback.SelectedStrategy)),
			zap.Bool("success", feedback.Success))
	}
}

// 获取战略数据返回每个战略的统计数据
func (r *QueryRouter) GetStrategyStats() map[RetrievalStrategy]StrategyStats {
	stats := make(map[RetrievalStrategy]StrategyStats)

	if r.feedbackStore == nil {
		return stats
	}

	r.feedbackStore.mu.RLock()
	defer r.feedbackStore.mu.RUnlock()

	for strategyStr, feedbacks := range r.feedbackStore.feedback {
		strategy := RetrievalStrategy(strategyStr)
		stat := StrategyStats{
			Strategy:   strategy,
			TotalCalls: len(feedbacks),
		}

		if len(feedbacks) > 0 {
			successCount := 0
			totalScore := 0.0
			for _, f := range feedbacks {
				if f.Success {
					successCount++
				}
				totalScore += f.Score
			}
			stat.SuccessRate = float64(successCount) / float64(len(feedbacks))
			stat.AverageScore = totalScore / float64(len(feedbacks))
		}

		stats[strategy] = stat
	}

	return stats
}

// 战略统计数据代表一项战略的统计数据
type StrategyStats struct {
	Strategy     RetrievalStrategy `json:"strategy"`
	TotalCalls   int               `json:"total_calls"`
	SuccessRate  float64           `json:"success_rate"`
	AverageScore float64           `json:"average_score"`
}

// 多战略路线

// 多战略决定代表使用多战略的决定
type MultiStrategyDecision struct {
	Query      string                        `json:"query"`
	Strategies []StrategyWithWeight          `json:"strategies"`
	Reasoning  string                        `json:"reasoning,omitempty"`
	Timestamp  time.Time                     `json:"timestamp"`
}

// 战略 用Weight代表着一个有分量的策略
type StrategyWithWeight struct {
	Strategy RetrievalStrategy `json:"strategy"`
	Weight   float64           `json:"weight"`
}

// Route Multision 确定多个组合检索策略
func (r *QueryRouter) RouteMulti(ctx context.Context, query string, maxStrategies int) (*MultiStrategyDecision, error) {
	// 先获得单一路线决定
	decision, err := r.Route(ctx, query)
	if err != nil {
		return nil, err
	}

	// 按分数排序策略
	type strategyScore struct {
		strategy RetrievalStrategy
		score    float64
	}

	scores := make([]strategyScore, 0, len(decision.Scores))
	for strategy, score := range decision.Scores {
		scores = append(scores, strategyScore{strategy, score})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// 选择顶级策略
	multiDecision := &MultiStrategyDecision{
		Query:      query,
		Strategies: make([]StrategyWithWeight, 0, maxStrategies),
		Reasoning:  decision.Reasoning,
		Timestamp:  time.Now(),
	}

	totalScore := 0.0
	for i := 0; i < maxStrategies && i < len(scores); i++ {
		if scores[i].score >= r.config.ConfidenceThreshold/2 {
			multiDecision.Strategies = append(multiDecision.Strategies, StrategyWithWeight{
				Strategy: scores[i].strategy,
				Weight:   scores[i].score,
			})
			totalScore += scores[i].score
		}
	}

	// 使权重正常化
	if totalScore > 0 {
		for i := range multiDecision.Strategies {
			multiDecision.Strategies[i].Weight /= totalScore
		}
	}

	return multiDecision, nil
}

// 批量运行

// RouteBatch 路线 多个查询
func (r *QueryRouter) RouteBatch(ctx context.Context, queries []string) ([]*RoutingDecision, error) {
	results := make([]*RoutingDecision, len(queries))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, query := range queries {
		wg.Add(1)
		go func(idx int, q string) {
			defer wg.Done()

			decision, err := r.Route(ctx, q)
			mu.Lock()
			defer mu.Unlock()

			if err != nil && firstErr == nil {
				firstErr = err
			}
			results[idx] = decision
		}(i, query)
	}

	wg.Wait()
	return results, firstErr
}

// JSON 序列化

// ToJSON 串行决定给JSON
func (d *RoutingDecision) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

// JSON将JSON的例行决定断章取义
func (d *RoutingDecision) FromJSON(data []byte) error {
	return json.Unmarshal(data, d)
}
