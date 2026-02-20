// 包路由器为LLM请求提供智能路由.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// intentType代表一种分类意图.
type IntentType string

const (
	IntentCodeGeneration  IntentType = "code_generation"
	IntentCodeReview      IntentType = "code_review"
	IntentQA              IntentType = "question_answering"
	IntentSummarization   IntentType = "summarization"
	IntentTranslation     IntentType = "translation"
	IntentCreativeWriting IntentType = "creative_writing"
	IntentDataAnalysis    IntentType = "data_analysis"
	IntentMath            IntentType = "math"
	IntentReasoning       IntentType = "reasoning"
	IntentChat            IntentType = "chat"
	IntentToolUse         IntentType = "tool_use"
	IntentUnknown         IntentType = "unknown"
)

// 意向分类代表意向分类的结果.
type IntentClassification struct {
	Intent     IntentType        `json:"intent"`
	Confidence float64           `json:"confidence"`
	SubIntents []IntentType      `json:"sub_intents,omitempty"`
	Entities   map[string]string `json:"entities,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// RouteConfig定义了意图的路由配置.
type RouteConfig struct {
	Intent           IntentType `json:"intent"`
	PreferredModels  []string   `json:"preferred_models"`
	FallbackModels   []string   `json:"fallback_models,omitempty"`
	MaxTokens        int        `json:"max_tokens,omitempty"`
	Temperature      float32    `json:"temperature,omitempty"`
	RequiredFeatures []string   `json:"required_features,omitempty"` // e.g., "function_calling", "vision"
}

// 语义路由器 Config 配置语义路由器.
type SemanticRouterConfig struct {
	ClassifierModel      string                     `json:"classifier_model"`
	DefaultRoute         RouteConfig                `json:"default_route"`
	Routes               map[IntentType]RouteConfig `json:"routes"`
	CacheClassifications bool                       `json:"cache_classifications"`
	CacheTTL             time.Duration              `json:"cache_ttl"`
}

// 默认Semantic Router Config 返回合理的默认值 。
func DefaultSemanticRouterConfig() SemanticRouterConfig {
	return SemanticRouterConfig{
		ClassifierModel: "gpt-4o-mini",
		DefaultRoute: RouteConfig{
			PreferredModels: []string{"gpt-4o"},
			MaxTokens:       4096,
			Temperature:     0.7,
		},
		Routes: map[IntentType]RouteConfig{
			IntentCodeGeneration: {
				Intent:          IntentCodeGeneration,
				PreferredModels: []string{"claude-3-5-sonnet", "gpt-4o"},
				MaxTokens:       8192,
				Temperature:     0.2,
			},
			IntentReasoning: {
				Intent:          IntentReasoning,
				PreferredModels: []string{"o1", "claude-3-5-sonnet"},
				MaxTokens:       16384,
				Temperature:     0.1,
			},
			IntentMath: {
				Intent:          IntentMath,
				PreferredModels: []string{"o1", "gpt-4o"},
				MaxTokens:       4096,
				Temperature:     0.0,
			},
			IntentCreativeWriting: {
				Intent:          IntentCreativeWriting,
				PreferredModels: []string{"claude-3-5-sonnet", "gpt-4o"},
				MaxTokens:       8192,
				Temperature:     0.9,
			},
			IntentToolUse: {
				Intent:           IntentToolUse,
				PreferredModels:  []string{"gpt-4o", "claude-3-5-sonnet"},
				RequiredFeatures: []string{"function_calling"},
				MaxTokens:        4096,
				Temperature:      0.3,
			},
		},
		CacheClassifications: true,
		CacheTTL:             5 * time.Minute,
	}
}

// 语义鲁特路线请求基于意向分类.
type SemanticRouter struct {
	classifier llm.Provider
	providers  map[string]llm.Provider
	config     SemanticRouterConfig
	cache      *classificationCache
	logger     *zap.Logger
	mu         sync.RWMutex
}

// 新语义路透创造出一个新的语义路由器.
func NewSemanticRouter(classifier llm.Provider, providers map[string]llm.Provider, config SemanticRouterConfig, logger *zap.Logger) *SemanticRouter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SemanticRouter{
		classifier: classifier,
		providers:  providers,
		config:     config,
		cache:      newClassificationCache(config.CacheTTL),
		logger:     logger,
	}
}

// 路线将请求和路线分类到适当的提供者.
func (r *SemanticRouter) Route(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 分类意图
	classification, err := r.ClassifyIntent(ctx, req)
	if err != nil {
		r.logger.Warn("intent classification failed, using default route", zap.Error(err))
		classification = &IntentClassification{Intent: IntentUnknown, Confidence: 0}
	}

	r.logger.Debug("classified intent",
		zap.String("intent", string(classification.Intent)),
		zap.Float64("confidence", classification.Confidence))

	// 获取路由配置
	routeConfig := r.getRouteConfig(classification.Intent)

	// 应用路由配置以请求
	if routeConfig.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = routeConfig.MaxTokens
	}
	if routeConfig.Temperature > 0 && req.Temperature == 0 {
		req.Temperature = routeConfig.Temperature
	}

	// 按顺序尝试首选模式
	var lastErr error
	for _, modelName := range routeConfig.PreferredModels {
		provider := r.findProviderForModel(modelName)
		if provider == nil {
			continue
		}

		// 检查所需的特性
		if !r.checkFeatures(provider, routeConfig.RequiredFeatures) {
			continue
		}

		req.Model = modelName
		resp, err := provider.Completion(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		r.logger.Warn("preferred model failed", zap.String("model", modelName), zap.Error(err))
	}

	// 尝试后退模式
	for _, modelName := range routeConfig.FallbackModels {
		provider := r.findProviderForModel(modelName)
		if provider == nil {
			continue
		}

		req.Model = modelName
		resp, err := provider.Completion(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all routes failed: %w", lastErr)
	}
	return nil, fmt.Errorf("no available provider for intent: %s", classification.Intent)
}

// 分类意向对请求的意图进行分类.
func (r *SemanticRouter) ClassifyIntent(ctx context.Context, req *llm.ChatRequest) (*IntentClassification, error) {
	// 检查缓存
	cacheKey := r.buildCacheKey(req)
	if r.config.CacheClassifications {
		if cached := r.cache.get(cacheKey); cached != nil {
			return cached, nil
		}
	}

	// 构建分类提示
	userMessage := extractUserMessage(req.Messages)
	prompt := fmt.Sprintf(`Classify the intent of this user message. Choose from:
- code_generation: Writing new code
- code_review: Reviewing or improving existing code
- question_answering: Answering factual questions
- summarization: Summarizing content
- translation: Translating between languages
- creative_writing: Creative or narrative writing
- data_analysis: Analyzing data or statistics
- math: Mathematical calculations or proofs
- reasoning: Complex logical reasoning
- chat: General conversation
- tool_use: Requires external tool/API calls
- unknown: Cannot determine

User message: %s

Respond with JSON: {"intent": "intent_type", "confidence": 0.0-1.0, "entities": {}}`, userMessage)

	resp, err := r.classifier.Completion(ctx, &llm.ChatRequest{
		Model: r.config.ClassifierModel,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   200,
	})
	if err != nil {
		return nil, err
	}

	// 分析分类
	var classification IntentClassification
	content := resp.Choices[0].Message.Content
	content = extractJSONFromResponse(content)
	if err := json.Unmarshal([]byte(content), &classification); err != nil {
		// 默认为未知
		classification = IntentClassification{Intent: IntentUnknown, Confidence: 0.5}
	}

	// 缓存结果
	if r.config.CacheClassifications {
		r.cache.set(cacheKey, &classification)
	}

	return &classification, nil
}

func (r *SemanticRouter) getRouteConfig(intent IntentType) RouteConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if config, ok := r.config.Routes[intent]; ok {
		return config
	}
	return r.config.DefaultRoute
}

func (r *SemanticRouter) findProviderForModel(model string) llm.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 直接匹配
	if p, ok := r.providers[model]; ok {
		return p
	}

	// 检查是否有任何提供者支持此模式
	for _, p := range r.providers {
		// 简单的heuristic: 检查提供者名前缀
		if matchesProvider(model, p.Name()) {
			return p
		}
	}
	return nil
}

func (r *SemanticRouter) checkFeatures(provider llm.Provider, features []string) bool {
	for _, f := range features {
		if f == "function_calling" && !provider.SupportsNativeFunctionCalling() {
			return false
		}
	}
	return true
}

func (r *SemanticRouter) buildCacheKey(req *llm.ChatRequest) string {
	msg := extractUserMessage(req.Messages)
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// 添加Route 添加或更新一个路由配置 。
func (r *SemanticRouter) AddRoute(intent IntentType, config RouteConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.Routes[intent] = config
}

// 添加提供者 。
func (r *SemanticRouter) AddProvider(name string, provider llm.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = provider
}

// 分类缓存
type classificationCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	classification *IntentClassification
	expiresAt      time.Time
}

func newClassificationCache(ttl time.Duration) *classificationCache {
	return &classificationCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *classificationCache) get(key string) *IntentClassification {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.classification
}

func (c *classificationCache) set(key string, classification *IntentClassification) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		classification: classification,
		expiresAt:      time.Now().Add(c.ttl),
	}
}

// 辅助功能
func extractUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

func extractJSONFromResponse(s string) string {
	start := -1
	end := -1
	for i, c := range s {
		if c == '{' && start == -1 {
			start = i
		}
		if c == '}' {
			end = i
		}
	}
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func matchesProvider(model, providerName string) bool {
	modelPrefixes := map[string][]string{
		"openai":    {"gpt-", "o1", "o3", "davinci", "text-"},
		"anthropic": {"claude"},
		"gemini":    {"gemini"},
		"deepseek":  {"deepseek"},
	}

	if prefixes, ok := modelPrefixes[providerName]; ok {
		for _, prefix := range prefixes {
			if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}
