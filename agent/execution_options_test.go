package agent

import (
	"context"
	"testing"
	"time"

	agentadapters "github.com/BaSui01/agentflow/agent/adapters"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type resolverStub struct {
	options types.ExecutionOptions
}

func (s resolverStub) Resolve(_ context.Context, _ types.AgentConfig, _ *Input) types.ExecutionOptions {
	return s.options.Clone()
}

type adapterStub struct {
	req  *types.ChatRequest
	seen types.ExecutionOptions
}

func (s *adapterStub) Build(options types.ExecutionOptions, messages []types.Message) (*types.ChatRequest, error) {
	s.seen = options.Clone()
	req := *s.req
	req.Messages = append([]types.Message(nil), messages...)
	return &req, nil
}

func TestExecutionOptionsResolver(t *testing.T) {
	cfg := testAgentConfig("agent-1", "Agent", "base-model")
	cfg.LLM.Provider = "anthropic"
	cfg.Runtime.Tools = []string{"search", "calc"}
	cfg.Runtime.MaxReActIterations = 4
	cfg.Runtime.MaxLoopIterations = 2

	ctx := types.WithLLMModel(context.Background(), "override-model")
	ctx = types.WithLLMProvider(ctx, "openai")
	ctx = types.WithLLMRoutePolicy(ctx, "latency_first")
	ctx = WithRunConfig(ctx, &RunConfig{
		MaxTokens:          IntPtr(3072),
		ToolChoice:         StringPtr("required"),
		ToolWhitelist:      []string{"search"},
		Timeout:            DurationPtr(45 * time.Second),
		MaxReActIterations: IntPtr(7),
		Tags:               []string{"unit"},
	})

	options := NewDefaultExecutionOptionsResolver().Resolve(ctx, cfg, &Input{
		Context: map[string]any{
			"disable_planner": true,
		},
	})
	assert.Equal(t, "override-model", options.Model.Model)
	assert.Equal(t, "openai", options.Model.Provider)
	assert.Equal(t, "latency_first", options.Model.RoutePolicy)
	assert.Equal(t, 3072, options.Model.MaxTokens)
	assert.Equal(t, 45*time.Second, options.Control.Timeout)
	assert.Equal(t, 7, options.Control.MaxReActIterations)
	assert.True(t, options.Control.DisablePlanner)
	assert.Equal(t, []string{"search"}, options.Tools.ToolWhitelist)
	require.NotNil(t, options.Tools.ToolChoice)
	assert.Equal(t, types.ToolChoiceModeRequired, options.Tools.ToolChoice.Mode)
	assert.Equal(t, []string{"search", "calc"}, options.Tools.AllowedTools)
	assert.Equal(t, []string{"unit"}, options.Tags)
}

func TestExecutionOptionsResolver_ResolvesControlFlagsFromInputContext(t *testing.T) {
	cfg := testAgentConfig("agent-1", "Agent", "base-model")
	options := NewDefaultExecutionOptionsResolver().Resolve(context.Background(), cfg, &Input{
		Context: map[string]any{
			"disable_planner":       true,
			"top_level_loop_budget": 1,
		},
	})
	assert.True(t, options.Control.DisablePlanner)
	assert.Equal(t, 1, options.Control.MaxLoopIterations)
}

func TestChatRequestAdapter(t *testing.T) {
	parallel := false
	disableParallel := true
	options := types.ExecutionOptions{
		Model: types.ModelOptions{
			Model:       "gpt-4o",
			Provider:    "openai",
			RoutePolicy: "balanced",
			MaxTokens:   2048,
			Temperature: 0.2,
			TopP:        0.8,
			Stop:        []string{"STOP"},
		},
		Control: types.AgentControlOptions{
			Timeout: 30 * time.Second,
		},
		Tools: types.ToolProtocolOptions{
			ToolChoice: &types.ToolChoice{
				Mode:                   types.ToolChoiceModeAllowed,
				AllowedTools:           []string{"search", "calc"},
				DisableParallelToolUse: &disableParallel,
			},
			ParallelToolCalls: &parallel,
			ToolCallMode:      types.ToolCallModeNative,
		},
		Metadata: map[string]string{"tenant": "t1"},
		Tags:     []string{"unit"},
	}

	req, err := agentadapters.NewDefaultChatRequestAdapter().Build(options, []types.Message{{Role: llm.RoleUser, Content: "hello"}})
	require.NoError(t, err)
	require.NotNil(t, req)
	assert.Equal(t, "gpt-4o", req.Model)
	assert.Equal(t, "balanced", req.RoutePolicy)
	assert.Equal(t, 30*time.Second, req.Timeout)
	assert.Equal(t, types.ToolCallModeNative, req.ToolCallMode)
	assert.Equal(t, map[string]string{
		"tenant":        "t1",
		"chat_provider": "openai",
	}, req.Metadata)
	require.NotNil(t, req.ParallelToolCalls)
	assert.False(t, *req.ParallelToolCalls)
	require.NotNil(t, req.ToolChoice)
	assert.Equal(t, types.ToolChoiceModeAllowed, req.ToolChoice.Mode)
	assert.Equal(t, []string{"search", "calc"}, req.ToolChoice.AllowedTools)
	require.NotNil(t, req.ToolChoice.DisableParallelToolUse)
	assert.True(t, *req.ToolChoice.DisableParallelToolUse)
}

func TestPrepareChatRequest_UsesResolvedExecutionOptions(t *testing.T) {
	agent := BuildBaseAgent(
		testAgentConfig("agent-1", "Agent", "base-model"),
		testGatewayFromProvider(&testProvider{name: "main", supportsNative: true}),
		nil,
		&testToolManager{
			getAllowedToolsFn: func(string) []types.ToolSchema {
				return []types.ToolSchema{
					{
						Type:        types.ToolTypeFunction,
						Name:        "search",
						Description: "search",
						Parameters:  []byte(`{"type":"object"}`),
					},
					{
						Type:        types.ToolTypeFunction,
						Name:        "calc",
						Description: "calc",
						Parameters:  []byte(`{"type":"object"}`),
					},
				}
			},
		},
		nil,
		zap.NewNop(),
		nil,
	)
	agent.config.Runtime.Tools = []string{"search", "calc"}
	agent.config.Runtime.MaxReActIterations = 5

	ctx := types.WithLLMModel(context.Background(), "override-model")
	ctx = WithRunConfig(ctx, &RunConfig{
		MaxTokens:          IntPtr(4096),
		ToolChoice:         StringPtr("search"),
		ToolWhitelist:      []string{"search"},
		MaxReActIterations: IntPtr(8),
	})

	prepared, err := agent.prepareChatRequest(ctx, []types.Message{{Role: llm.RoleUser, Content: "hello"}})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.Equal(t, "override-model", prepared.req.Model)
	assert.Equal(t, 4096, prepared.req.MaxTokens)
	assert.Equal(t, 1, len(prepared.req.Tools))
	assert.Equal(t, "search", prepared.req.Tools[0].Name)
	require.NotNil(t, prepared.req.ToolChoice)
	assert.Equal(t, types.ToolChoiceModeSpecific, prepared.req.ToolChoice.Mode)
	assert.Equal(t, "search", prepared.req.ToolChoice.ToolName)
	assert.Equal(t, 8, prepared.maxReActIter)
	assert.Equal(t, "override-model", prepared.options.Model.Model)
}

func TestPrepareChatRequest_UsesInjectedResolverAndAdapter(t *testing.T) {
	agent := BuildBaseAgent(
		testAgentConfig("agent-1", "Agent", "base-model"),
		testGatewayFromProvider(&testProvider{name: "main", supportsNative: true}),
		nil,
		nil,
		nil,
		zap.NewNop(),
		nil,
	)

	adapter := &adapterStub{req: &types.ChatRequest{Model: "adapter-model"}}
	agent.SetExecutionOptionsResolver(resolverStub{
		options: types.ExecutionOptions{
			Model: types.ModelOptions{Model: "resolved-model"},
		},
	})
	agent.SetChatRequestAdapter(adapter)

	prepared, err := agent.prepareChatRequest(context.Background(), []types.Message{{Role: llm.RoleUser, Content: "hello"}})
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.Equal(t, "adapter-model", prepared.req.Model)
	assert.Equal(t, "resolved-model", adapter.seen.Model.Model)
}
