package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPreflightPolicy_EstimatesChatTokens(t *testing.T) {
	service := newPolicyTestService(t, 30, &policyNativeTokenProvider{
		tokenResp: &llmcore.TokenCountResponse{InputTokens: 16},
	})

	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llmcore.ChatRequest{
			Model: "test-model",
			Messages: []llmcore.Message{
				{Role: llmcore.RoleUser, Content: "hello world"},
			},
			MaxTokens: 20,
		},
	}

	err := service.preflightPolicy(context.Background(), req)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "per-request")
	require.Equal(t, "36", req.Metadata["estimated_tokens"])
}

func TestPreflightPolicy_UsesMetadataEstimatedTokens(t *testing.T) {
	service := newPolicyTestService(t, 5, &policyNativeTokenProvider{
		tokenResp: &llmcore.TokenCountResponse{InputTokens: 100},
	})

	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Metadata: map[string]string{
			"estimated_tokens": "1",
		},
		Payload: &llmcore.ChatRequest{
			Model: "test-model",
			Messages: []llmcore.Message{
				{Role: llmcore.RoleUser, Content: "very long message that should be ignored by override"},
			},
			MaxTokens: 100,
		},
	}

	err := service.preflightPolicy(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "1", req.Metadata["estimated_tokens"])
}

func TestPreflightPolicy_ChatRequiresNativeTokenCounter(t *testing.T) {
	service := newPolicyTestService(t, 30, &policyNativeTokenProvider{})

	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llmcore.ChatRequest{
			Model:    "test-model",
			Messages: []llmcore.Message{{Role: llmcore.RoleUser, Content: "hello world"}},
		},
	}

	err := service.preflightPolicy(context.Background(), req)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "native token counting")
}

func TestPreflightPolicy_EmbeddingNoLongerEstimatesViaTokenizer(t *testing.T) {
	service := newPolicyTestService(t, 3, &policyNativeTokenProvider{
		tokenResp: &llmcore.TokenCountResponse{InputTokens: 10},
	})

	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityEmbedding,
		Payload: &EmbeddingInput{
			Request: &embedding.EmbeddingRequest{
				Model: "test-embedding-model",
				Input: []string{"foo", "bar"},
			},
		},
	}

	err := service.preflightPolicy(context.Background(), req)
	require.NoError(t, err)
	require.Empty(t, req.Metadata["estimated_tokens"])
}

func newPolicyTestService(t *testing.T, maxTokensPerRequest int, provider llmcore.Provider) *Service {
	t.Helper()

	cfg := llmpolicy.DefaultBudgetConfig()
	cfg.MaxTokensPerRequest = maxTokensPerRequest
	cfg.MaxTokensPerMinute = 1_000_000
	cfg.MaxTokensPerHour = 1_000_000
	cfg.MaxTokensPerDay = 1_000_000
	cfg.MaxCostPerRequest = 1_000_000
	cfg.MaxCostPerDay = 1_000_000

	budget := llmpolicy.NewTokenBudgetManager(cfg, zap.NewNop())
	manager := llmpolicy.NewManager(llmpolicy.ManagerConfig{Budget: budget})

	return New(Config{
		ChatProvider:  provider,
		PolicyManager: manager,
		Logger:        zap.NewNop(),
	})
}

type policyNativeTokenProvider struct {
	tokenResp *llmcore.TokenCountResponse
	tokenErr  error
}

func (p *policyNativeTokenProvider) Completion(context.Context, *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	return nil, nil
}
func (p *policyNativeTokenProvider) Stream(context.Context, *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	return nil, nil
}
func (p *policyNativeTokenProvider) HealthCheck(context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true}, nil
}
func (p *policyNativeTokenProvider) Name() string                        { return "native-token-provider" }
func (p *policyNativeTokenProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *policyNativeTokenProvider) ListModels(context.Context) ([]llmcore.Model, error) {
	return nil, nil
}
func (p *policyNativeTokenProvider) Endpoints() llmcore.ProviderEndpoints {
	return llmcore.ProviderEndpoints{}
}
func (p *policyNativeTokenProvider) CountTokens(context.Context, *llmcore.ChatRequest) (*llmcore.TokenCountResponse, error) {
	if p.tokenErr != nil {
		return nil, p.tokenErr
	}
	if p.tokenResp == nil {
		return nil, types.NewInternalError("native token counting unavailable")
	}
	return p.tokenResp, nil
}
