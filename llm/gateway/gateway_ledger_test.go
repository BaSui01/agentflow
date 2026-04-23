package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_Invoke_RecordsLedger(t *testing.T) {
	ledger := &recordingLedger{}
	calc := observability.NewCostCalculator()
	calc.SetPrice("ledger-provider", "ledger-model", 1.0, 2.0)

	service := New(Config{
		ChatProvider:   &ledgerProvider{},
		CostCalculator: calc,
		Ledger:         ledger,
	})

	resp, err := service.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability:  llmcore.CapabilityChat,
		RoutePolicy: llmcore.RoutePolicyCostFirst,
		TraceID:     "trace-invoke",
		Metadata: map[string]string{
			"user_id": "u-1",
		},
		Payload: &llmcore.ChatRequest{
			Model: "ledger-model",
			Messages: []types.Message{
				{Role: types.RoleUser, Content: "hello"},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "USD", resp.Cost.Currency)
	require.InDelta(t, 0.016, resp.Cost.AmountUSD, 0.000001)

	entries := ledger.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, "trace-invoke", entries[0].TraceID)
	require.Equal(t, string(llmcore.CapabilityChat), entries[0].Capability)
	require.Equal(t, "ledger-provider", entries[0].Provider)
	require.Equal(t, "ledger-model", entries[0].Model)
	require.Equal(t, resp.Cost.Currency, entries[0].Cost.Currency)
	require.InDelta(t, resp.Cost.AmountUSD, entries[0].Cost.AmountUSD, 0.000001)
	require.Equal(t, "u-1", entries[0].Metadata["user_id"])
}

func TestService_Stream_RecordsLedger(t *testing.T) {
	ledger := &recordingLedger{}
	calc := observability.NewCostCalculator()
	calc.SetPrice("ledger-provider", "ledger-model", 1.0, 2.0)

	service := New(Config{
		ChatProvider:   &ledgerProvider{},
		CostCalculator: calc,
		Ledger:         ledger,
	})

	ch, err := service.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		TraceID:    "trace-stream",
		Payload: &llmcore.ChatRequest{
			Model: "ledger-model",
			Messages: []types.Message{
				{Role: types.RoleUser, Content: "stream"},
			},
		},
	})
	require.NoError(t, err)

	for range ch {
		// drain stream to ensure goroutine exits
	}

	require.Eventually(t, func() bool {
		return len(ledger.Entries()) == 1
	}, time.Second, 10*time.Millisecond)

	entry := ledger.Entries()[0]
	require.Equal(t, "trace-stream", entry.TraceID)
	require.Equal(t, "USD", entry.Cost.Currency)
	require.InDelta(t, 0.005, entry.Cost.AmountUSD, 0.000001)
	require.Equal(t, 3, entry.Usage.TotalTokens)
}

func TestService_Stream_RecordsUsageOnceWithFinalChunk(t *testing.T) {
	ledger := &recordingLedger{}
	calc := observability.NewCostCalculator()
	calc.SetPrice("ledger-provider", "ledger-model", 1.0, 2.0)

	budgetCfg := llmpolicy.DefaultBudgetConfig()
	budgetCfg.MaxTokensPerRequest = 1_000_000
	budgetCfg.MaxTokensPerMinute = 1_000_000
	budgetCfg.MaxTokensPerHour = 1_000_000
	budgetCfg.MaxTokensPerDay = 1_000_000
	budgetCfg.MaxCostPerRequest = 1_000_000
	budgetCfg.MaxCostPerDay = 1_000_000
	budget := llmpolicy.NewTokenBudgetManager(budgetCfg, zap.NewNop())
	manager := llmpolicy.NewManager(llmpolicy.ManagerConfig{Budget: budget})

	service := New(Config{
		ChatProvider:   &ledgerMultiUsageProvider{},
		CostCalculator: calc,
		Ledger:         ledger,
		PolicyManager:  manager,
	})

	ch, err := service.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		TraceID:    "trace-stream-final",
		Payload: &llmcore.ChatRequest{
			Model: "ledger-model",
			Messages: []types.Message{
				{Role: types.RoleUser, Content: "stream"},
			},
		},
	})
	require.NoError(t, err)

	for range ch {
		// drain stream to ensure final usage aggregation runs
	}

	status := budget.GetStatus()
	require.Equal(t, int64(5), status.TokensUsedDay)

	require.Eventually(t, func() bool {
		return len(ledger.Entries()) == 1
	}, time.Second, 10*time.Millisecond)
	entry := ledger.Entries()[0]
	require.Equal(t, 5, entry.Usage.TotalTokens)
	require.InDelta(t, 0.008, entry.Cost.AmountUSD, 0.000001)
}

func TestService_Invoke_WithoutLedgerDoesNotFail(t *testing.T) {
	service := New(Config{
		ChatProvider: &ledgerProvider{},
	})

	resp, err := service.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llmcore.ChatRequest{
			Model: "ledger-model",
			Messages: []types.Message{
				{Role: types.RoleUser, Content: "hello"},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

type recordingLedger struct {
	mu      sync.Mutex
	entries []observability.LedgerEntry
}

func (l *recordingLedger) Record(_ context.Context, entry observability.LedgerEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
	return nil
}

func (l *recordingLedger) Entries() []observability.LedgerEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]observability.LedgerEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

type ledgerProvider struct{}

func (p *ledgerProvider) Completion(ctx context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	return &llmcore.ChatResponse{
		ID:       "resp-ledger",
		Provider: "ledger-provider",
		Model:    req.Model,
		Choices: []llmcore.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: "ok",
				},
			},
		},
		Usage: llmcore.ChatUsage{
			PromptTokens:     4,
			CompletionTokens: 6,
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *ledgerProvider) Stream(ctx context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	out := make(chan llmcore.StreamChunk, 1)
	out <- llmcore.StreamChunk{
		ID:       "chunk-ledger",
		Provider: "ledger-provider",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: "part",
		},
		Usage: &llmcore.ChatUsage{
			PromptTokens:     1,
			CompletionTokens: 2,
		},
	}
	close(out)
	return out, nil
}

func (p *ledgerProvider) HealthCheck(context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true}, nil
}

func (p *ledgerProvider) Name() string { return "ledger-provider" }

func (p *ledgerProvider) SupportsNativeFunctionCalling() bool { return false }

func (p *ledgerProvider) ListModels(context.Context) ([]llmcore.Model, error) { return nil, nil }

func (p *ledgerProvider) Endpoints() llmcore.ProviderEndpoints { return llmcore.ProviderEndpoints{} }
func (p *ledgerProvider) CountTokens(context.Context, *llmcore.ChatRequest) (*llmcore.TokenCountResponse, error) {
	return &llmcore.TokenCountResponse{InputTokens: 1, TotalTokens: 1}, nil
}

type ledgerMultiUsageProvider struct{}

func (p *ledgerMultiUsageProvider) Completion(context.Context, *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	return nil, nil
}

func (p *ledgerMultiUsageProvider) Stream(ctx context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	out := make(chan llmcore.StreamChunk, 2)
	out <- llmcore.StreamChunk{
		ID:       "chunk-1",
		Provider: "ledger-provider",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: "a",
		},
		Usage: &llmcore.ChatUsage{
			PromptTokens:     1,
			CompletionTokens: 1,
			TotalTokens:      2,
		},
	}
	out <- llmcore.StreamChunk{
		ID:       "chunk-2",
		Provider: "ledger-provider",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: "b",
		},
		Usage: &llmcore.ChatUsage{
			PromptTokens:     2,
			CompletionTokens: 3,
			TotalTokens:      5,
		},
	}
	close(out)
	return out, nil
}

func (p *ledgerMultiUsageProvider) HealthCheck(context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true}, nil
}

func (p *ledgerMultiUsageProvider) Name() string { return "ledger-provider" }

func (p *ledgerMultiUsageProvider) SupportsNativeFunctionCalling() bool { return false }

func (p *ledgerMultiUsageProvider) ListModels(context.Context) ([]llmcore.Model, error) {
	return nil, nil
}

func (p *ledgerMultiUsageProvider) Endpoints() llmcore.ProviderEndpoints {
	return llmcore.ProviderEndpoints{}
}
func (p *ledgerMultiUsageProvider) CountTokens(context.Context, *llmcore.ChatRequest) (*llmcore.TokenCountResponse, error) {
	return &llmcore.TokenCountResponse{InputTokens: 1, TotalTokens: 1}, nil
}
