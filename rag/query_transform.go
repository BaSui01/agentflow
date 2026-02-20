// 包布提供由LlamaIndex所启发的RAG查询转换能力.
// 这个模块执行查询扩展,重写,意向检测,和子克分解等功能.
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// QQ 查询转换类型QQ

// 查询意向表示用户查询的检测意图
type QueryIntent string

const (
	IntentFactual       QueryIntent = "factual"        // Simple fact lookup
	IntentComparison    QueryIntent = "comparison"     // Compare multiple items
	IntentExplanation   QueryIntent = "explanation"    // Explain a concept
	IntentProcedural    QueryIntent = "procedural"     // How-to questions
	IntentAnalytical    QueryIntent = "analytical"     // Analysis/reasoning required
	IntentCreative      QueryIntent = "creative"       // Creative/generative tasks
	IntentAggregation   QueryIntent = "aggregation"    // Aggregate information
	IntentTemporal      QueryIntent = "temporal"       // Time-based queries
	IntentCausal        QueryIntent = "causal"         // Cause-effect relationships
	IntentHypothetical  QueryIntent = "hypothetical"   // What-if scenarios
	IntentUnknown       QueryIntent = "unknown"        // Cannot determine intent
)

// 转变 类型代表查询转换类型
type TransformationType string

const (
	TransformExpansion     TransformationType = "expansion"      // Generate related queries
	TransformRewrite       TransformationType = "rewrite"        // Rewrite for better retrieval
	TransformDecomposition TransformationType = "decomposition"  // Break into sub-queries
	TransformHyDE          TransformationType = "hyde"           // Hypothetical Document Embedding
	TransformStepBack      TransformationType = "step_back"      // Step-back prompting
)

// 已变形查询代表一个带有元数据的已变形查询
type TransformedQuery struct {
	Original       string             `json:"original"`
	Transformed    string             `json:"transformed"`
	Type           TransformationType `json:"type"`
	Intent         QueryIntent        `json:"intent,omitempty"`
	Confidence     float64            `json:"confidence"`
	SubQueries     []string           `json:"sub_queries,omitempty"`
	Keywords       []string           `json:"keywords,omitempty"`
	Entities       []string           `json:"entities,omitempty"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
}

// 查询 TransformConfig 配置查询转换器
type QueryTransformConfig struct {
	// 扩展设置
	EnableExpansion     bool    `json:"enable_expansion"`
	MaxExpansions       int     `json:"max_expansions"`        // Max expanded queries (3-5)
	ExpansionDiversity  float64 `json:"expansion_diversity"`   // 0-1, higher = more diverse

	// 重写设置
	EnableRewriting     bool    `json:"enable_rewriting"`
	RewriteForRetrieval bool    `json:"rewrite_for_retrieval"` // Optimize for retrieval

	// 分解设置
	EnableDecomposition bool    `json:"enable_decomposition"`
	MaxSubQueries       int     `json:"max_sub_queries"`       // Max sub-queries (2-5)
	DecomposeThreshold  float64 `json:"decompose_threshold"`   // Complexity threshold

	// 有意检测
	EnableIntentDetection bool   `json:"enable_intent_detection"`

	// HyDE( 嵌入式文档)
	EnableHyDE          bool    `json:"enable_hyde"`
	HyDEDocumentCount   int     `json:"hyde_document_count"`   // Number of hypothetical docs

	// 后退提示
	EnableStepBack      bool    `json:"enable_step_back"`

	// 缓存
	EnableCache         bool          `json:"enable_cache"`
	CacheTTL            time.Duration `json:"cache_ttl"`

	// LLM 设置
	UseLLM              bool    `json:"use_llm"`               // Use LLM for transformations
	Temperature         float64 `json:"temperature"`           // LLM temperature
}

// 默认查询 TransformConfig 返回默认配置
func DefaultQueryTransformConfig() QueryTransformConfig {
	return QueryTransformConfig{
		EnableExpansion:       true,
		MaxExpansions:         3,
		ExpansionDiversity:    0.5,
		EnableRewriting:       true,
		RewriteForRetrieval:   true,
		EnableDecomposition:   true,
		MaxSubQueries:         3,
		DecomposeThreshold:    0.6,
		EnableIntentDetection: true,
		EnableHyDE:            false,
		HyDEDocumentCount:     3,
		EnableStepBack:        false,
		EnableCache:           true,
		CacheTTL:              30 * time.Minute,
		UseLLM:                true,
		Temperature:           0.3,
	}
}

// * LLM 提供者接口 {}

// 基于 LLM 的查询界面
type QueryLLMProvider interface {
	// 完整生成给定快件的补全
	Complete(ctx context.Context, prompt string) (string, error)
}

// QQ 查询变换器

// Query Transfer 为更好的检索而转换查询
type QueryTransformer struct {
	config      QueryTransformConfig
	llmProvider QueryLLMProvider
	cache       *transformCache
	logger      *zap.Logger
}

// 切换缓存转换结果
type transformCache struct {
	entries map[string]*cacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

type cacheEntry struct {
	result    *TransformedQuery
	expiresAt time.Time
}

func newTransformCache(ttl time.Duration) *transformCache {
	return &transformCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *transformCache) get(key string) (*TransformedQuery, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.result, true
}

func (c *transformCache) set(key string, result *TransformedQuery) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// 新建查询转换器创建新查询转换器
func NewQueryTransformer(
	config QueryTransformConfig,
	llmProvider QueryLLMProvider,
	logger *zap.Logger,
) *QueryTransformer {
	if logger == nil {
		logger = zap.NewNop()
	}

	var cache *transformCache
	if config.EnableCache {
		cache = newTransformCache(config.CacheTTL)
	}

	return &QueryTransformer{
		config:      config,
		llmProvider: llmProvider,
		cache:       cache,
		logger:      logger.With(zap.String("component", "query_transformer")),
	}
}

// 将所有启用的转换应用到查询中
func (t *QueryTransformer) Transform(ctx context.Context, query string) (*TransformedQuery, error) {
	// 检查缓存
	if t.cache != nil {
		if cached, ok := t.cache.get(query); ok {
			t.logger.Debug("cache hit", zap.String("query", query))
			return cached, nil
		}
	}

	result := &TransformedQuery{
		Original:   query,
		Transformed: query,
		Confidence: 1.0,
		Metadata:   make(map[string]any),
	}

	// 1. 侦测意图
	if t.config.EnableIntentDetection {
		intent, confidence := t.detectIntent(ctx, query)
		result.Intent = intent
		result.Confidence = confidence
		result.Metadata["intent_confidence"] = confidence
	}

	// 2. 摘录关键词和实体
	result.Keywords = t.extractKeywords(query)
	result.Entities = t.extractEntities(query)

	// 3. 确定是否需要分解
	if t.config.EnableDecomposition && t.shouldDecompose(query, result.Intent) {
		subQueries, err := t.decompose(ctx, query)
		if err != nil {
			t.logger.Warn("decomposition failed", zap.Error(err))
		} else {
			result.SubQueries = subQueries
			result.Type = TransformDecomposition
		}
	}

	// 4. 重写检索查询
	if t.config.EnableRewriting {
		rewritten, err := t.rewrite(ctx, query, result.Intent)
		if err != nil {
			t.logger.Warn("rewriting failed", zap.Error(err))
		} else {
			result.Transformed = rewritten
			if result.Type == "" {
				result.Type = TransformRewrite
			}
		}
	}

	// 5. 如果启用, 生成 HyDE
	if t.config.EnableHyDE {
		hydeDoc, err := t.generateHyDE(ctx, query)
		if err != nil {
			t.logger.Warn("HyDE generation failed", zap.Error(err))
		} else {
			result.Metadata["hyde_document"] = hydeDoc
		}
	}

	// 6. 启用后退提示
	if t.config.EnableStepBack {
		stepBackQuery, err := t.stepBack(ctx, query)
		if err != nil {
			t.logger.Warn("step-back failed", zap.Error(err))
		} else {
			result.Metadata["step_back_query"] = stepBackQuery
		}
	}

	// 缓存结果
	if t.cache != nil {
		t.cache.set(query, result)
	}

	t.logger.Info("query transformed",
		zap.String("original", query),
		zap.String("transformed", result.Transformed),
		zap.String("intent", string(result.Intent)),
		zap.Int("sub_queries", len(result.SubQueries)))

	return result, nil
}

// 扩展生成多个相关查询以更好地召回
func (t *QueryTransformer) Expand(ctx context.Context, query string) ([]string, error) {
	if !t.config.EnableExpansion {
		return []string{query}, nil
	}

	if t.llmProvider == nil || !t.config.UseLLM {
		return t.expandWithRules(query), nil
	}

	prompt := fmt.Sprintf(`Generate %d alternative search queries for the following query.
Each alternative should capture different aspects or phrasings of the same information need.
Return only the queries, one per line.

Original query: %s

Alternative queries:`, t.config.MaxExpansions, query)

	response, err := t.llmProvider.Complete(ctx, prompt)
	if err != nil {
		t.logger.Warn("LLM expansion failed, using rule-based", zap.Error(err))
		return t.expandWithRules(query), nil
	}

	// 解析响应
	lines := strings.Split(strings.TrimSpace(response), "\n")
	expansions := []string{query} // Include original

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 删除当前编号
		line = regexp.MustCompile(`^\d+[\.\)]\s*`).ReplaceAllString(line, "")
		if line != "" && line != query {
			expansions = append(expansions, line)
		}
		if len(expansions) >= t.config.MaxExpansions+1 {
			break
		}
	}

	return expansions, nil
}

// With Rules 使用基于规则的方法生成扩展
func (t *QueryTransformer) expandWithRules(query string) []string {
	expansions := []string{query}

	// 添加以同义词为基础的扩展
	words := strings.Fields(strings.ToLower(query))
	synonymMap := map[string][]string{
		"how":        {"what way", "method"},
		"why":        {"reason", "cause"},
		"what":       {"which", "describe"},
		"best":       {"top", "optimal", "recommended"},
		"difference": {"comparison", "contrast", "versus"},
		"example":    {"instance", "sample", "demonstration"},
		"explain":    {"describe", "clarify", "elaborate"},
		"implement":  {"create", "build", "develop"},
		"use":        {"utilize", "apply", "employ"},
		"problem":    {"issue", "challenge", "difficulty"},
	}

	for _, word := range words {
		if synonyms, ok := synonymMap[word]; ok {
			for _, syn := range synonyms {
				newQuery := strings.Replace(query, word, syn, 1)
				if newQuery != query {
					expansions = append(expansions, newQuery)
				}
				if len(expansions) >= t.config.MaxExpansions+1 {
					return expansions
				}
			}
		}
	}

	return expansions
}

// 检测意图识别查询背后的意图
func (t *QueryTransformer) detectIntent(ctx context.Context, query string) (QueryIntent, float64) {
	queryLower := strings.ToLower(query)

	// 根据规则侦测意图
	patterns := map[QueryIntent][]string{
		IntentFactual:      {"what is", "who is", "when was", "where is", "define"},
		IntentComparison:   {"compare", "difference between", "versus", "vs", "better than", "or"},
		IntentExplanation:  {"explain", "why", "how does", "what causes", "describe"},
		IntentProcedural:   {"how to", "how do i", "steps to", "guide", "tutorial"},
		IntentAnalytical:   {"analyze", "evaluate", "assess", "impact of", "effect of"},
		IntentCreative:     {"create", "generate", "write", "design", "suggest"},
		IntentAggregation:  {"list", "all", "summary", "overview", "collection"},
		IntentTemporal:     {"latest", "recent", "history", "timeline", "when"},
		IntentCausal:       {"because", "result of", "leads to", "consequence"},
		IntentHypothetical: {"what if", "suppose", "imagine", "hypothetically"},
	}

	for intent, keywords := range patterns {
		for _, keyword := range keywords {
			if strings.Contains(queryLower, keyword) {
				return intent, 0.8
			}
		}
	}

	// 对复杂案件使用 LLM
	if t.llmProvider != nil && t.config.UseLLM {
		intent, confidence := t.detectIntentWithLLM(ctx, query)
		if confidence > 0.5 {
			return intent, confidence
		}
	}

	return IntentUnknown, 0.3
}

// 检测intentWithLLM 使用 LLM 进行意图检测
func (t *QueryTransformer) detectIntentWithLLM(ctx context.Context, query string) (QueryIntent, float64) {
	prompt := fmt.Sprintf(`Classify the following query into one of these intents:
- factual: Simple fact lookup
- comparison: Compare multiple items
- explanation: Explain a concept
- procedural: How-to questions
- analytical: Analysis/reasoning required
- creative: Creative/generative tasks
- aggregation: Aggregate information
- temporal: Time-based queries
- causal: Cause-effect relationships
- hypothetical: What-if scenarios

Query: %s

Respond with only the intent name and confidence (0-1), separated by comma.
Example: factual, 0.9`, query)

	response, err := t.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return IntentUnknown, 0.0
	}

	parts := strings.Split(strings.TrimSpace(response), ",")
	if len(parts) >= 2 {
		intent := QueryIntent(strings.TrimSpace(parts[0]))
		var confidence float64
		fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &confidence)
		return intent, confidence
	}

	return IntentUnknown, 0.0
}

// 应解析是否要解析查询
func (t *QueryTransformer) shouldDecompose(query string, intent QueryIntent) bool {
	// 复杂的意图受益于分解
	complexIntents := map[QueryIntent]bool{
		IntentComparison:  true,
		IntentAnalytical:  true,
		IntentAggregation: true,
		IntentCausal:      true,
	}

	if complexIntents[intent] {
		return true
	}

	// 检查查询的复杂度
	words := strings.Fields(query)
	if len(words) > 15 {
		return true
	}

	// 检查显示多个部件的连接
	conjunctions := []string{" and ", " or ", " also ", " as well as ", " both "}
	queryLower := strings.ToLower(query)
	for _, conj := range conjunctions {
		if strings.Contains(queryLower, conj) {
			return true
		}
	}

	return false
}

// 将一个复杂的查询分解为更简单的子序列
func (t *QueryTransformer) decompose(ctx context.Context, query string) ([]string, error) {
	if t.llmProvider == nil || !t.config.UseLLM {
		return t.decomposeWithRules(query), nil
	}

	prompt := fmt.Sprintf(`Break down the following complex query into %d simpler, independent sub-queries.
Each sub-query should be self-contained and answerable independently.
Return only the sub-queries, one per line.

Complex query: %s

Sub-queries:`, t.config.MaxSubQueries, query)

	response, err := t.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return t.decomposeWithRules(query), nil
	}

	lines := strings.Split(strings.TrimSpace(response), "\n")
	subQueries := make([]string, 0, t.config.MaxSubQueries)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = regexp.MustCompile(`^\d+[\.\)]\s*`).ReplaceAllString(line, "")
		if line != "" {
			subQueries = append(subQueries, line)
		}
		if len(subQueries) >= t.config.MaxSubQueries {
			break
		}
	}

	return subQueries, nil
}

// 使用规则分解
func (t *QueryTransformer) decomposeWithRules(query string) []string {
	subQueries := []string{}

	// 组合分割
	separators := []string{" and ", " or ", " also ", " as well as "}
	parts := []string{query}

	for _, sep := range separators {
		newParts := []string{}
		for _, part := range parts {
			split := strings.Split(part, sep)
			for _, s := range split {
				s = strings.TrimSpace(s)
				if s != "" {
					newParts = append(newParts, s)
				}
			}
		}
		parts = newParts
	}

	// 确保每个部分都是有效的查询
	for _, part := range parts {
		if len(strings.Fields(part)) >= 2 {
			subQueries = append(subQueries, part)
		}
	}

	if len(subQueries) == 0 {
		return []string{query}
	}

	return subQueries
}

// 重写更改查询以更好地检索
func (t *QueryTransformer) rewrite(ctx context.Context, query string, intent QueryIntent) (string, error) {
	if t.llmProvider == nil || !t.config.UseLLM {
		return t.rewriteWithRules(query), nil
	}

	prompt := fmt.Sprintf(`Rewrite the following query to be more effective for semantic search retrieval.
- Remove filler words and conversational elements
- Focus on key concepts and entities
- Make it more specific and searchable
- Keep the core meaning intact

Original query: %s
Query intent: %s

Rewritten query:`, query, intent)

	response, err := t.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return t.rewriteWithRules(query), nil
	}

	return strings.TrimSpace(response), nil
}

// 重写“ 规则” 应用基于规则的查询重写
func (t *QueryTransformer) rewriteWithRules(query string) string {
	// 删除常用的填充词
	fillers := []string{
		"can you tell me",
		"i want to know",
		"please explain",
		"i need help with",
		"could you help me",
		"i'm looking for",
		"i would like to",
		"can you help me",
	}

	result := strings.ToLower(query)
	for _, filler := range fillers {
		result = strings.Replace(result, filler, "", 1)
	}

	// 删除问题标记并清理
	result = strings.TrimSpace(result)
	result = strings.TrimSuffix(result, "?")
	result = strings.TrimSpace(result)

	// 将第一个字母大写
	if len(result) > 0 {
		result = strings.ToUpper(string(result[0])) + result[1:]
	}

	return result
}

// 生成HyDE 为查询创建一个假设文档
func (t *QueryTransformer) generateHyDE(ctx context.Context, query string) (string, error) {
	if t.llmProvider == nil {
		return "", fmt.Errorf("LLM provider required for HyDE")
	}

	prompt := fmt.Sprintf(`Generate a hypothetical document passage that would perfectly answer the following query.
The passage should be informative, factual, and directly relevant to the query.
Write as if this is an excerpt from a real document.

Query: %s

Hypothetical document passage:`, query)

	response, err := t.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

// stepBack 生成一个更一般性的后退查询,以提示
func (t *QueryTransformer) stepBack(ctx context.Context, query string) (string, error) {
	if t.llmProvider == nil {
		return "", fmt.Errorf("LLM provider required for step-back")
	}

	prompt := fmt.Sprintf(`Given the following specific query, generate a more general "step-back" query that captures the broader concept or principle.
This helps retrieve foundational knowledge that can help answer the specific query.

Specific query: %s

Step-back query:`, query)

	response, err := t.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

// 从查询中提取关键字
func (t *QueryTransformer) extractKeywords(query string) []string {
	// 简单关键字提取
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true,
		"i": true, "you": true, "he": true, "she": true, "it": true,
		"we": true, "they": true, "what": true, "which": true, "who": true,
		"whom": true, "this": true, "that": true, "these": true, "those": true,
		"am": true, "can": true, "to": true, "of": true, "in": true,
		"for": true, "on": true, "with": true, "at": true, "by": true,
		"from": true, "as": true, "into": true, "through": true, "during": true,
		"before": true, "after": true, "above": true, "below": true, "between": true,
		"and": true, "or": true, "but": true, "if": true, "then": true,
		"because": true, "while": true, "although": true, "how": true, "why": true,
		"when": true, "where": true, "there": true, "here": true,
	}

	words := strings.Fields(strings.ToLower(query))
	keywords := make([]string, 0)

	for _, word := range words {
		// 删除标点
		word = regexp.MustCompile(`[^\w]`).ReplaceAllString(word, "")
		if word != "" && !stopWords[word] && len(word) > 2 {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// 从查询中提取实体
func (t *QueryTransformer) extractEntities(query string) []string {
	// 基于资本化的简单实体提取
	words := strings.Fields(query)
	entities := make([]string, 0)

	for i, word := range words {
		// 跳过第一个单词( 通常为资本化)
		if i == 0 {
			continue
		}

		// 检查单词是否以大写开头
		if len(word) > 0 && word[0] >= 'A' && word[0] <= 'Z' {
			// 删除后接点
			word = regexp.MustCompile(`[^\w]$`).ReplaceAllString(word, "")
			if len(word) > 1 {
				entities = append(entities, word)
			}
		}
	}

	return entities
}

// QQ 查询扩展结果 QQ

// 扩展Result包含带有元数据的扩展查询
type ExpansionResult struct {
	Original   string   `json:"original"`
	Expansions []string `json:"expansions"`
	Keywords   []string `json:"keywords"`
	Intent     QueryIntent `json:"intent"`
}

// 展开WithMetadata 扩展查询并返回详细结果
func (t *QueryTransformer) ExpandWithMetadata(ctx context.Context, query string) (*ExpansionResult, error) {
	expansions, err := t.Expand(ctx, query)
	if err != nil {
		return nil, err
	}

	intent, _ := t.detectIntent(ctx, query)
	keywords := t.extractKeywords(query)

	return &ExpansionResult{
		Original:   query,
		Expansions: expansions,
		Keywords:   keywords,
		Intent:     intent,
	}, nil
}

// 批处理

// TransformBatch 同步转换多个查询
func (t *QueryTransformer) TransformBatch(ctx context.Context, queries []string) ([]*TransformedQuery, error) {
	results := make([]*TransformedQuery, len(queries))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, query := range queries {
		wg.Add(1)
		go func(idx int, q string) {
			defer wg.Done()

			result, err := t.Transform(ctx, q)
			mu.Lock()
			defer mu.Unlock()

			if err != nil && firstErr == nil {
				firstErr = err
			}
			results[idx] = result
		}(i, query)
	}

	wg.Wait()
	return results, firstErr
}

// JSON 序列化

// ToJSON 将变形查询序列化为 JSON
func (tq *TransformedQuery) ToJSON() ([]byte, error) {
	return json.Marshal(tq)
}

// 从 JSON 解析出一个变形查询
func (tq *TransformedQuery) FromJSON(data []byte) error {
	return json.Unmarshal(data, tq)
}
