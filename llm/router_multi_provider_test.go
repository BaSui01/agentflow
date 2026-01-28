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

	// Seed providers/models.
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

	// Provider-model mapping (different costs).
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

	// API keys (enabled).
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

	// Cost-based should pick mockA (cheaper).
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

	// Health-based should pick lowest priority when scores tie (mockA).
	selection2, err := router.SelectProviderWithModel(context.Background(), "gpt-4o", StrategyHealthBased)
	if err != nil {
		t.Fatalf("SelectProviderWithModel (health): %v", err)
	}
	if selection2.ProviderCode != "mockA" {
		t.Fatalf("expected provider mockA, got %s", selection2.ProviderCode)
	}

	// Unknown model should return a typed Error.
	if _, err := router.SelectProviderWithModel(context.Background(), "no-such-model", StrategyCostBased); err == nil {
		t.Fatalf("expected error for unknown model")
	}
}
