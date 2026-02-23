package router

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== Mock Provider ======

type testProvider struct {
	name         string
	completionFn func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn     func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
	healthFn     func(ctx context.Context) (*llm.HealthStatus, error)
	supportsFC   bool
	models       []llm.Model
}

func newTestProvider(name string) *testProvider {
	return &testProvider{
		name:       name,
		supportsFC: true,
		models:     []llm.Model{{ID: name + "-model"}},
	}
}

func (p *testProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{{
			Message: llm.Message{Role: llm.RoleAssistant, Content: "response from " + p.name},
		}},
		Usage: llm.ChatUsage{TotalTokens: 10},
	}, nil
}

func (p *testProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	ch := make(chan llm.StreamChunk, 1)
	close(ch)
	return ch, nil
}

func (p *testProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	if p.healthFn != nil {
		return p.healthFn(ctx)
	}
	return &llm.HealthStatus{Healthy: true}, nil
}

func (p *testProvider) Name() string { return p.name }

func (p *testProvider) SupportsNativeFunctionCalling() bool { return p.supportsFC }

func (p *testProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return p.models, nil
}

func (p *testProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{BaseURL: "http://test/" + p.name}
}

// ====== SemanticRouter Tests ======

func TestSemanticRouter_Route_Success(t *testing.T) {
	classifier := newTestProvider("classifier")
	classifier.completionFn = func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{
			Choices: []llm.ChatChoice{{
				Message: llm.Message{Content: `{"intent":"code_generation","confidence":0.9}`},
			}},
		}, nil
	}

	codeProvider := newTestProvider("code-provider")
	providers := map[string]llm.Provider{
		"claude-3-5-sonnet": codeProvider,
	}

	cfg := DefaultSemanticRouterConfig()
	router := NewSemanticRouter(classifier, providers, cfg, nil)

	resp, err := router.Route(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Write a function"}},
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestSemanticRouter_Route_ClassifierFails_UsesDefault(t *testing.T) {
	classifier := newTestProvider("classifier")
	classifier.completionFn = func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
		return nil, fmt.Errorf("classifier down")
	}

	defaultProvider := newTestProvider("default")
	providers := map[string]llm.Provider{
		"gpt-4o": defaultProvider,
	}

	cfg := DefaultSemanticRouterConfig()
	router := NewSemanticRouter(classifier, providers, cfg, nil)

	resp, err := router.Route(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestSemanticRouter_Route_AllProvidersFail(t *testing.T) {
	classifier := newTestProvider("classifier")
	classifier.completionFn = func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{
			Choices: []llm.ChatChoice{{
				Message: llm.Message{Content: `{"intent":"chat","confidence":0.9}`},
			}},
		}, nil
	}

	failProvider := newTestProvider("fail")
	failProvider.completionFn = func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
		return nil, fmt.Errorf("provider error")
	}

	providers := map[string]llm.Provider{
		"gpt-4o": failProvider,
	}

	cfg := DefaultSemanticRouterConfig()
	router := NewSemanticRouter(classifier, providers, cfg, nil)

	_, err := router.Route(context.Background(), &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all routes failed")
}

func TestSemanticRouter_ClassifyIntent_CachesResult(t *testing.T) {
	callCount := 0
	classifier := newTestProvider("classifier")
	classifier.completionFn = func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
		callCount++
		return &llm.ChatResponse{
			Choices: []llm.ChatChoice{{
				Message: llm.Message{Content: `{"intent":"chat","confidence":0.8}`},
			}},
		}, nil
	}

	cfg := DefaultSemanticRouterConfig()
	cfg.CacheClassifications = true
	cfg.CacheTTL = time.Minute

	router := NewSemanticRouter(classifier, nil, cfg, nil)

	req := &llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	}

	// First call
	c1, err := router.ClassifyIntent(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, IntentChat, c1.Intent)

	// Second call should use cache
	c2, err := router.ClassifyIntent(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, IntentChat, c2.Intent)
	assert.Equal(t, 1, callCount)
}

func TestSemanticRouter_AddRoute(t *testing.T) {
	router := NewSemanticRouter(newTestProvider("c"), nil, DefaultSemanticRouterConfig(), nil)

	router.AddRoute(IntentDataAnalysis, RouteConfig{
		PreferredModels: []string{"data-model"},
		MaxTokens:       16384,
	})

	cfg := router.getRouteConfig(IntentDataAnalysis)
	assert.Equal(t, []string{"data-model"}, cfg.PreferredModels)
}

func TestSemanticRouter_AddProvider(t *testing.T) {
	router := NewSemanticRouter(newTestProvider("c"), map[string]llm.Provider{}, DefaultSemanticRouterConfig(), nil)

	p := newTestProvider("new-provider")
	router.AddProvider("new-model", p)

	found := router.findProviderForModel("new-model")
	assert.NotNil(t, found)
}

func TestSemanticRouter_CheckFeatures(t *testing.T) {
	router := NewSemanticRouter(newTestProvider("c"), nil, DefaultSemanticRouterConfig(), nil)

	p := newTestProvider("test")
	p.supportsFC = true
	assert.True(t, router.checkFeatures(p, []string{"function_calling"}))

	p.supportsFC = false
	assert.False(t, router.checkFeatures(p, []string{"function_calling"}))

	// No features required
	assert.True(t, router.checkFeatures(p, nil))
}

// ====== Helper Function Tests ======

func TestExtractUserMessage(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "system"},
		{Role: llm.RoleUser, Content: "first user"},
		{Role: llm.RoleAssistant, Content: "assistant"},
		{Role: llm.RoleUser, Content: "last user"},
	}
	assert.Equal(t, "last user", extractUserMessage(msgs))
}

func TestExtractUserMessage_Empty(t *testing.T) {
	assert.Equal(t, "", extractUserMessage(nil))
	assert.Equal(t, "", extractUserMessage([]llm.Message{{Role: llm.RoleSystem, Content: "sys"}}))
}

func TestExtractJSONFromResponse(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"intent":"chat"}`, `{"intent":"chat"}`},
		{`Here is the result: {"intent":"chat"} done`, `{"intent":"chat"}`},
		{`no json here`, `no json here`},
		{`{"nested":{"a":1}}`, `{"nested":{"a":1}}`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractJSONFromResponse(tt.input))
		})
	}
}

func TestMatchesProvider(t *testing.T) {
	assert.True(t, matchesProvider("gpt-4o", "openai"))
	assert.True(t, matchesProvider("claude-3", "anthropic"))
	assert.True(t, matchesProvider("gemini-2.0", "gemini"))
	assert.True(t, matchesProvider("deepseek-v3", "deepseek"))
	assert.False(t, matchesProvider("gpt-4o", "anthropic"))
	assert.False(t, matchesProvider("unknown-model", "openai"))
}

// ====== classificationCache Tests ======

func TestClassificationCache_SetGet(t *testing.T) {
	cache := newClassificationCache(time.Minute)

	c := &IntentClassification{Intent: IntentChat, Confidence: 0.9}
	cache.set("key1", c)

	got := cache.get("key1")
	require.NotNil(t, got)
	assert.Equal(t, IntentChat, got.Intent)
}

func TestClassificationCache_Expiry(t *testing.T) {
	cache := newClassificationCache(1 * time.Millisecond)

	cache.set("key1", &IntentClassification{Intent: IntentChat})
	time.Sleep(5 * time.Millisecond)

	got := cache.get("key1")
	assert.Nil(t, got)
}

func TestClassificationCache_MaxSize(t *testing.T) {
	cache := newClassificationCache(time.Minute)
	cache.maxSize = 3

	for i := 0; i < 5; i++ {
		cache.set(fmt.Sprintf("key%d", i), &IntentClassification{Intent: IntentChat})
	}

	// Should not exceed maxSize
	assert.True(t, len(cache.entries) <= 3)
}

func TestClassificationCache_Miss(t *testing.T) {
	cache := newClassificationCache(time.Minute)
	assert.Nil(t, cache.get("nonexistent"))
}
