package sdk

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/runtime"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	channelstore "github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockProvider struct {
	name string
}

func (m mockProvider) Name() string { return m.name }

func (m mockProvider) Completion(ctx context.Context, req *types.ChatRequest) (*types.ChatResponse, error) {
	_ = ctx
	if req == nil {
		return nil, nil
	}
	return &types.ChatResponse{
		ID:       "mock",
		Provider: m.name,
		Model:    req.Model,
		Choices: []types.ChatChoice{
			{Index: 0, FinishReason: "stop", Message: types.Message{Role: "assistant", Content: "ok"}},
		},
		CreatedAt: time.Now(),
	}, nil
}

func (m mockProvider) Stream(ctx context.Context, req *types.ChatRequest) (<-chan types.StreamChunk, error) {
	_ = ctx
	_ = req
	ch := make(chan types.StreamChunk)
	close(ch)
	return ch, nil
}

func (m mockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	_ = ctx
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m mockProvider) SupportsNativeFunctionCalling() bool { return false }

func (m mockProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	_ = ctx
	return nil, nil
}

func (m mockProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{BaseURL: "mock://"}
}

type sdkToolManager struct {
	schemas []types.ToolSchema
}

func (m *sdkToolManager) GetAllowedTools(string) []types.ToolSchema {
	return append([]types.ToolSchema(nil), m.schemas...)
}

func (m *sdkToolManager) ExecuteForAgent(context.Context, string, []types.ToolCall) []llmtools.ToolResult {
	return nil
}

func TestSDK_Build_BoundaryA(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	provider := mockProvider{name: "mock"}

	opts := runtime.DefaultBuildOptions()
	opts.EnableSkills = false
	rt, err := New(Options{
		Logger: logger,
		LLM: &LLMOptions{
			Provider: provider,
		},
		Agent: &AgentOptions{
			BuildOptions: opts,
		},
		Workflow: &WorkflowOptions{
			Enable:    true,
			EnableDSL: true,
		},
		RAG: &RAGOptions{
			Enable: true,
		},
	}).Build(ctx)
	require.NoError(t, err)
	require.NotNil(t, rt)
	require.NotNil(t, rt.Workflow)
	require.NotNil(t, rt.Workflow.Facade)
	require.NotNil(t, rt.Workflow.Parser)
	require.NotNil(t, rt.RAG)

	ag, err := rt.NewAgent(ctx, types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "sdk-agent-1",
			Name: "SDK Agent",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "mock-model",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestSDK_Build_WithLLMRouterStore(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	store := channelstore.NewStaticStore(channelstore.StaticStoreConfig{
		Channels: []channelstore.Channel{
			{ID: "ch1", Provider: "openai", BaseURL: "https://example.invalid", Weight: 100},
		},
		Keys: []channelstore.Key{
			{ID: "k1", ChannelID: "ch1", Weight: 100},
		},
		Mappings: []channelstore.ModelMapping{
			{ID: "m1", ChannelID: "ch1", PublicModel: "mock-model", RemoteModel: "mock-model", Provider: "openai", Weight: 100},
		},
		Secrets: map[string]channelstore.Secret{
			"k1": {APIKey: "sk-test"},
		},
	})

	rt, err := New(Options{
		Logger: logger,
		LLM: &LLMOptions{
			Router: &LLMRouterOptions{
				Name:  "sdk-router",
				Store: store,
			},
		},
		Agent: &AgentOptions{
			BuildOptions: runtime.DefaultBuildOptions(),
		},
	}).Build(ctx)
	require.NoError(t, err)
	require.NotNil(t, rt)
	require.NotNil(t, rt.Provider)

	ag, err := rt.NewAgent(ctx, types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "sdk-agent-router",
			Name: "SDK Agent Router",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "mock-model",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestSDK_Build_RequiresLLMOptions(t *testing.T) {
	ctx := context.Background()

	_, err := New(Options{}).Build(ctx)
	require.Error(t, err)
	require.ErrorContains(t, err, "Options.LLM")
}

func TestSDK_Build_AgentOptionsExposeToolManager(t *testing.T) {
	ctx := context.Background()
	manager := &sdkToolManager{
		schemas: []types.ToolSchema{{
			Type:       types.ToolTypeFunction,
			Name:       "lookup",
			Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	}

	opts := runtime.DefaultBuildOptions()
	opts.EnableSkills = false
	rt, err := New(Options{
		Logger: zap.NewNop(),
		LLM: &LLMOptions{
			Provider: mockProvider{name: "mock"},
		},
		Agent: &AgentOptions{
			BuildOptions: opts,
			ToolManager:  manager,
			ToolScope:    []string{"lookup"},
		},
	}).Build(ctx)
	require.NoError(t, err)

	ag, err := rt.NewAgent(ctx, types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "sdk-agent-tools",
			Name: "SDK Agent Tools",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model: "mock-model",
		},
	})
	require.NoError(t, err)
	require.Same(t, manager, ag.Tools())
	require.Equal(t, []string{"lookup"}, ag.Config().Tools.AllowedTools)
	require.Equal(t, []string{"lookup"}, ag.Config().Runtime.Tools)
}
