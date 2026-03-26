package compose

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type countingProvider struct {
	content         string
	lastRequest     *llm.ChatRequest
	completionCalls int
}

func (p *countingProvider) Completion(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.completionCalls++
	if req != nil {
		cloned := *req
		cloned.Metadata = cloneStringMap(req.Metadata)
		cloned.Tags = cloneStrings(req.Tags)
		p.lastRequest = &cloned
	}
	return &llm.ChatResponse{
		Provider: "counting-provider",
		Model:    req.Model,
		Choices: []llm.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: p.content,
			},
		}},
	}, nil
}

func (p *countingProvider) Stream(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	out := make(chan llm.StreamChunk, 1)
	out <- llm.StreamChunk{
		Provider: "counting-provider",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
	}
	close(out)
	return out, nil
}

func (*countingProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (*countingProvider) Name() string { return "counting-provider" }

func (*countingProvider) SupportsNativeFunctionCalling() bool { return true }

func (*countingProvider) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

func (*countingProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

func TestBuild_ReusesMainProviderAndSharedAssembly(t *testing.T) {
	t.Parallel()

	provider := &countingProvider{content: "hello"}
	runtime, err := Build(Config{
		Timeout:    2 * time.Second,
		MaxRetries: 1,
		Budget: BudgetConfig{
			Enabled:            true,
			MaxTokensPerMinute: 1000,
			MaxTokensPerDay:    10000,
			MaxCostPerDay:      10,
			AlertThreshold:     0.8,
		},
		Cache: CacheConfig{
			Enabled:      true,
			LocalMaxSize: 32,
			LocalTTL:     time.Minute,
			KeyStrategy:  "hash",
		},
	}, provider, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Provider)
	require.NotNil(t, runtime.ToolProvider)
	require.Same(t, runtime.Provider, runtime.ToolProvider)
	require.NotNil(t, runtime.BudgetManager)
	require.NotNil(t, runtime.Cache)
	require.NotNil(t, runtime.PolicyManager)

	firstReq := &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello",
		}},
		Tools:      []types.ToolSchema{},
		ToolChoice: "auto",
	}

	resp, err := runtime.Provider.Completion(context.Background(), firstReq)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 1, provider.completionCalls)
	require.NotNil(t, provider.lastRequest)
	require.Nil(t, provider.lastRequest.ToolChoice)

	provider.lastRequest = nil
	secondReq := &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []types.Message{{
			Role:    types.RoleUser,
			Content: "hello",
		}},
		Tools:      []types.ToolSchema{},
		ToolChoice: "auto",
	}

	_, err = runtime.Provider.Completion(context.Background(), secondReq)
	require.NoError(t, err)
	require.Equal(t, 1, provider.completionCalls)
	require.Nil(t, provider.lastRequest)
}

func TestBuild_RequiresMainProvider(t *testing.T) {
	t.Parallel()

	runtime, err := Build(Config{}, nil, zap.NewNop())
	require.Error(t, err)
	require.Nil(t, runtime)
	require.ErrorContains(t, err, "main provider is required")
}

func TestBuild_UsesExplicitToolProviderOverride(t *testing.T) {
	t.Parallel()

	mainProvider := &countingProvider{content: "hello"}
	runtime, err := Build(Config{
		Tool: ToolProviderConfig{
			Provider: "openai",
			APIKey:   "sk-tool-provider",
		},
	}, mainProvider, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Provider)
	require.NotNil(t, runtime.ToolProvider)
	require.NotSame(t, runtime.Provider, runtime.ToolProvider)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
