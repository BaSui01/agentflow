// Package router provides intelligent routing for LLM requests.
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

// IntentType represents a classified intent.
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

// IntentClassification represents the result of intent classification.
type IntentClassification struct {
	Intent     IntentType        `json:"intent"`
	Confidence float64           `json:"confidence"`
	SubIntents []IntentType      `json:"sub_intents,omitempty"`
	Entities   map[string]string `json:"entities,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// RouteConfig defines routing configuration for an intent.
type RouteConfig struct {
	Intent           IntentType `json:"intent"`
	PreferredModels  []string   `json:"preferred_models"`
	FallbackModels   []string   `json:"fallback_models,omitempty"`
	MaxTokens        int        `json:"max_tokens,omitempty"`
	Temperature      float32    `json:"temperature,omitempty"`
	RequiredFeatures []string   `json:"required_features,omitempty"` // e.g., "function_calling", "vision"
}

// SemanticRouterConfig configures the semantic router.
type SemanticRouterConfig struct {
	ClassifierModel      string                     `json:"classifier_model"`
	DefaultRoute         RouteConfig                `json:"default_route"`
	Routes               map[IntentType]RouteConfig `json:"routes"`
	CacheClassifications bool                       `json:"cache_classifications"`
	CacheTTL             time.Duration              `json:"cache_ttl"`
}

// DefaultSemanticRouterConfig returns sensible defaults.
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

// SemanticRouter routes requests based on intent classification.
type SemanticRouter struct {
	classifier llm.Provider
	providers  map[string]llm.Provider
	config     SemanticRouterConfig
	cache      *classificationCache
	logger     *zap.Logger
	mu         sync.RWMutex
}

// NewSemanticRouter creates a new semantic router.
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

// Route classifies the request and routes to appropriate provider.
func (r *SemanticRouter) Route(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Classify intent
	classification, err := r.ClassifyIntent(ctx, req)
	if err != nil {
		r.logger.Warn("intent classification failed, using default route", zap.Error(err))
		classification = &IntentClassification{Intent: IntentUnknown, Confidence: 0}
	}

	r.logger.Debug("classified intent",
		zap.String("intent", string(classification.Intent)),
		zap.Float64("confidence", classification.Confidence))

	// Get route config
	routeConfig := r.getRouteConfig(classification.Intent)

	// Apply route config to request
	if routeConfig.MaxTokens > 0 && req.MaxTokens == 0 {
		req.MaxTokens = routeConfig.MaxTokens
	}
	if routeConfig.Temperature > 0 && req.Temperature == 0 {
		req.Temperature = routeConfig.Temperature
	}

	// Try preferred models in order
	var lastErr error
	for _, modelName := range routeConfig.PreferredModels {
		provider := r.findProviderForModel(modelName)
		if provider == nil {
			continue
		}

		// Check required features
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

	// Try fallback models
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

// ClassifyIntent classifies the intent of a request.
func (r *SemanticRouter) ClassifyIntent(ctx context.Context, req *llm.ChatRequest) (*IntentClassification, error) {
	// Check cache
	cacheKey := r.buildCacheKey(req)
	if r.config.CacheClassifications {
		if cached := r.cache.get(cacheKey); cached != nil {
			return cached, nil
		}
	}

	// Build classification prompt
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

	// Parse classification
	var classification IntentClassification
	content := resp.Choices[0].Message.Content
	content = extractJSONFromResponse(content)
	if err := json.Unmarshal([]byte(content), &classification); err != nil {
		// Default to unknown
		classification = IntentClassification{Intent: IntentUnknown, Confidence: 0.5}
	}

	// Cache result
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

	// Direct match
	if p, ok := r.providers[model]; ok {
		return p
	}

	// Check if any provider supports this model
	for _, p := range r.providers {
		// Simple heuristic: check provider name prefix
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

// AddRoute adds or updates a route configuration.
func (r *SemanticRouter) AddRoute(intent IntentType, config RouteConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.Routes[intent] = config
}

// AddProvider adds a provider.
func (r *SemanticRouter) AddProvider(name string, provider llm.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = provider
}

// Classification cache
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

// Helper functions
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
