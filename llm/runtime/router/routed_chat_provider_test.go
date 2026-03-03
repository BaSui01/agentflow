package router

import (
	"context"
	"testing"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"go.uber.org/zap"
)

type captureProvider struct {
	name      string
	lastModel string
}

func (p *captureProvider) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	p.lastModel = req.Model
	return &ChatResponse{
		Provider: p.name,
		Model:    req.Model,
		Choices: []ChatChoice{{
			Index: 0,
			Message: Message{
				Role:    RoleAssistant,
				Content: "ok",
			},
		}},
	}, nil
}

func (p *captureProvider) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	p.lastModel = req.Model
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{
		Provider: p.name,
		Model:    req.Model,
		Delta: Message{
			Role:    RoleAssistant,
			Content: "ok",
		},
	}
	close(ch)
	return ch, nil
}

func (p *captureProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (p *captureProvider) Name() string { return p.name }

func (p *captureProvider) SupportsNativeFunctionCalling() bool { return true }

func (p *captureProvider) ListModels(ctx context.Context) ([]Model, error) { return nil, nil }

func (p *captureProvider) Endpoints() ProviderEndpoints { return ProviderEndpoints{} }

func setupRouterForRoutedProviderTest(t *testing.T) (*MultiProviderRouter, map[string]*captureProvider) {
	t.Helper()

	db := openRouterTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}, &LLMModel{}, &LLMProviderModel{}, &LLMProviderAPIKey{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	pA := LLMProvider{Code: "mockA", Name: "Mock A", Status: LLMProviderStatusActive}
	pB := LLMProvider{Code: "mockB", Name: "Mock B", Status: LLMProviderStatusActive}
	if err := db.Create(&pA).Error; err != nil {
		t.Fatalf("create provider A: %v", err)
	}
	if err := db.Create(&pB).Error; err != nil {
		t.Fatalf("create provider B: %v", err)
	}
	model := LLMModel{ModelName: "gpt-4o", DisplayName: "GPT-4o", Enabled: true}
	if err := db.Create(&model).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}

	if err := db.Create(&LLMProviderModel{
		ModelID:         model.ID,
		ProviderID:      pA.ID,
		RemoteModelName: "remote-a",
		BaseURL:         "http://example-a",
		PriceInput:      0.001,
		PriceCompletion: 0.002,
		Priority:        10,
		Enabled:         true,
	}).Error; err != nil {
		t.Fatalf("create provider model A: %v", err)
	}
	if err := db.Create(&LLMProviderModel{
		ModelID:         model.ID,
		ProviderID:      pB.ID,
		RemoteModelName: "remote-b",
		BaseURL:         "http://example-b",
		PriceInput:      0.010,
		PriceCompletion: 0.020,
		Priority:        20,
		Enabled:         true,
	}).Error; err != nil {
		t.Fatalf("create provider model B: %v", err)
	}

	keyA := LLMProviderAPIKey{ProviderID: pA.ID, APIKey: "kA", Enabled: true, Weight: 100, Priority: 10}
	keyB := LLMProviderAPIKey{ProviderID: pB.ID, APIKey: "kB", Enabled: true, Weight: 100, Priority: 10}
	if err := db.Create(&keyA).Error; err != nil {
		t.Fatalf("create api key A: %v", err)
	}
	if err := db.Create(&keyB).Error; err != nil {
		t.Fatalf("create api key B: %v", err)
	}

	if err := db.Exec("CREATE TABLE IF NOT EXISTS sc_llm_usage_logs (provider TEXT, latency_ms REAL, created_at DATETIME)").Error; err != nil {
		t.Fatalf("create usage logs table: %v", err)
	}
	now := time.Now()
	if err := db.Exec("INSERT INTO sc_llm_usage_logs (provider, latency_ms, created_at) VALUES (?, ?, ?), (?, ?, ?)",
		"mockA", 1200, now,
		"mockB", 200, now).Error; err != nil {
		t.Fatalf("seed usage logs: %v", err)
	}

	providers := map[string]*captureProvider{
		"mockA": {name: "mockA"},
		"mockB": {name: "mockB"},
	}
	factory := NewDefaultProviderFactory()
	factory.RegisterProvider("mockA", func(apiKey, baseURL string) (Provider, error) { return providers["mockA"], nil })
	factory.RegisterProvider("mockB", func(apiKey, baseURL string) (Provider, error) { return providers["mockB"], nil })

	router := NewMultiProviderRouter(db, factory, RouterOptions{Logger: zap.NewNop()})
	if err := router.InitAPIKeyPools(context.Background()); err != nil {
		t.Fatalf("InitAPIKeyPools: %v", err)
	}
	t.Cleanup(router.Stop)
	return router, providers
}

func TestRoutedChatProvider_RespectsProviderHint(t *testing.T) {
	t.Parallel()

	router, providers := setupRouterForRoutedProviderTest(t)
	routed := NewRoutedChatProvider(router, RoutedChatProviderOptions{
		DefaultStrategy: StrategyCostBased,
		Logger:          zap.NewNop(),
	})

	resp, err := routed.Completion(context.Background(), &ChatRequest{
		Model: "gpt-4o",
		Metadata: map[string]string{
			llmcore.MetadataKeyChatProvider: "mockB",
			"route_policy":                  "cost_first",
		},
	})
	if err != nil {
		t.Fatalf("Completion error: %v", err)
	}
	if resp.Provider != "mockB" {
		t.Fatalf("expected mockB provider, got %s", resp.Provider)
	}
	if providers["mockB"].lastModel != "remote-b" {
		t.Fatalf("expected remote-b model, got %s", providers["mockB"].lastModel)
	}
}

func TestRoutedChatProvider_RespectsLatencyPolicy(t *testing.T) {
	t.Parallel()

	router, providers := setupRouterForRoutedProviderTest(t)
	routed := NewRoutedChatProvider(router, RoutedChatProviderOptions{
		DefaultStrategy: StrategyCostBased,
		Logger:          zap.NewNop(),
	})

	resp, err := routed.Completion(context.Background(), &ChatRequest{
		Model: "gpt-4o",
		Metadata: map[string]string{
			"route_policy": "latency_first",
		},
	})
	if err != nil {
		t.Fatalf("Completion error: %v", err)
	}
	if resp.Provider != "mockB" {
		t.Fatalf("expected latency-first to choose mockB, got %s", resp.Provider)
	}
	if providers["mockB"].lastModel != "remote-b" {
		t.Fatalf("expected remote-b model, got %s", providers["mockB"].lastModel)
	}
}
