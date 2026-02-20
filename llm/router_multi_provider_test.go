package llm

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type mockProvider struct {
	name string
}

func (p *mockProvider) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
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

func (p *mockProvider) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk)
	close(ch)
	return ch, nil
}

func (p *mockProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (p *mockProvider) Name() string { return p.name }

func (p *mockProvider) SupportsNativeFunctionCalling() bool { return false }

func (p *mockProvider) ListModels(ctx context.Context) ([]Model, error) {
	return nil, nil
}

func TestMultiProviderRouter_SelectProviderWithModel(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := InitDatabase(db); err != nil {
		t.Fatalf("InitDatabase: %v", err)
	}

	// 种子提供者/模型。
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

	// 供应商模式制图(成本不同)。
	pmA := LLMProviderModel{
		ModelID:         model.ID,
		ProviderID:      pA.ID,
		RemoteModelName: "gpt-4o",
		BaseURL:         "http://example-a",
		PriceInput:      0.001,
		PriceCompletion: 0.002,
		Priority:        10,
		Enabled:         true,
	}
	pmB := LLMProviderModel{
		ModelID:         model.ID,
		ProviderID:      pB.ID,
		RemoteModelName: "gpt-4o",
		BaseURL:         "http://example-b",
		PriceInput:      0.010,
		PriceCompletion: 0.020,
		Priority:        20,
		Enabled:         true,
	}
	if err := db.Create(&pmA).Error; err != nil {
		t.Fatalf("create provider model A: %v", err)
	}
	if err := db.Create(&pmB).Error; err != nil {
		t.Fatalf("create provider model B: %v", err)
	}

	// API 密钥 (启用).
	keyA := LLMProviderAPIKey{ProviderID: pA.ID, APIKey: "kA", Enabled: true, Weight: 100, Priority: 10}
	keyB := LLMProviderAPIKey{ProviderID: pB.ID, APIKey: "kB", Enabled: true, Weight: 100, Priority: 10}
	if err := db.Create(&keyA).Error; err != nil {
		t.Fatalf("create api key A: %v", err)
	}
	if err := db.Create(&keyB).Error; err != nil {
		t.Fatalf("create api key B: %v", err)
	}

	factory := NewDefaultProviderFactory()
	factory.RegisterProvider("mockA", func(apiKey, baseURL string) (Provider, error) { return &mockProvider{name: "mockA"}, nil })
	factory.RegisterProvider("mockB", func(apiKey, baseURL string) (Provider, error) { return &mockProvider{name: "mockB"}, nil })

	router := NewMultiProviderRouter(db, factory, RouterOptions{Logger: logger})
	t.Cleanup(router.healthMonitor.Stop)

	if err := router.InitAPIKeyPools(context.Background()); err != nil {
		t.Fatalf("InitAPIKeyPools: %v", err)
	}

	// 基于成本的应选择模拟A(便宜)。
	selection, err := router.SelectProviderWithModel(context.Background(), "gpt-4o", StrategyCostBased)
	if err != nil {
		t.Fatalf("SelectProviderWithModel: %v", err)
	}
	if selection.ProviderCode != "mockA" {
		t.Fatalf("expected provider mockA, got %s", selection.ProviderCode)
	}
	if selection.ModelName != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", selection.ModelName)
	}

	// 以健康为基础,在打平分时应选择最低优先。
	selection2, err := router.SelectProviderWithModel(context.Background(), "gpt-4o", StrategyHealthBased)
	if err != nil {
		t.Fatalf("SelectProviderWithModel (health): %v", err)
	}
	if selection2.ProviderCode != "mockA" {
		t.Fatalf("expected provider mockA, got %s", selection2.ProviderCode)
	}

	// 未知的模型应该返回已输入错误 。
	if _, err := router.SelectProviderWithModel(context.Background(), "no-such-model", StrategyCostBased); err == nil {
		t.Fatalf("expected error for unknown model")
	}
}
