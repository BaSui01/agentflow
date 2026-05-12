package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConsumeDirectStreamChunkAccumulatesContentReasoningAndProviderState(t *testing.T) {
	reasoning := "think"
	events := make([]RuntimeStreamEvent, 0, 2)
	result := &directStreamingAttemptResult{}
	var reasoningBuf strings.Builder

	consumeDirectStreamChunk(func(event RuntimeStreamEvent) {
		events = append(events, event)
	}, result, &reasoningBuf, types.StreamChunk{
		ID:           "chunk-1",
		Provider:     "openai",
		Model:        "gpt-5.4",
		FinishReason: "stop",
		Usage:        &types.ChatUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
		Delta: types.Message{
			Content:          "hello",
			ReasoningContent: &reasoning,
			ReasoningSummaries: []types.ReasoningSummary{{
				Provider: "openai",
				Text:     "summary",
			}},
			OpaqueReasoning: []types.OpaqueReasoning{{
				Provider: "openai",
				Kind:     "encrypted",
				State:    "state",
			}},
			ThinkingBlocks: []types.ThinkingBlock{{
				Thinking:  "hidden",
				Signature: "sig",
			}},
		},
	})

	assert.Equal(t, "chunk-1", result.lastID)
	assert.Equal(t, "openai", result.lastProvider)
	assert.Equal(t, "gpt-5.4", result.lastModel)
	assert.Equal(t, "stop", result.lastFinishReason)
	require.NotNil(t, result.lastUsage)
	assert.Equal(t, 3, result.lastUsage.TotalTokens)
	assert.Equal(t, "hello", result.assembled.Content)
	assert.Equal(t, "think", reasoningBuf.String())
	require.Len(t, result.assembled.ReasoningSummaries, 1)
	require.Len(t, result.assembled.OpaqueReasoning, 1)
	require.Len(t, result.assembled.ThinkingBlocks, 1)
	require.Len(t, events, 2)
	assert.Equal(t, RuntimeStreamToken, events[0].Type)
	assert.Equal(t, RuntimeStreamReasoning, events[1].Type)
}

func TestFinalizeDirectStreamingResponseEmitsMessageAndLoopStop(t *testing.T) {
	events := make([]RuntimeStreamEvent, 0, 3)
	resp := finalizeDirectStreamingResponse(func(event RuntimeStreamEvent) {
		events = append(events, event)
	}, &directStreamingAttemptResult{
		assembled:        types.Message{Content: "done"},
		lastID:           "chatcmpl-1",
		lastProvider:     "openai",
		lastModel:        "gpt-5.4",
		lastFinishReason: "stop",
		reasoning:        "reason",
	}, types.ChatUsage{TotalTokens: 7})

	require.NotNil(t, resp)
	assert.Equal(t, "chatcmpl-1", resp.ID)
	assert.Equal(t, "openai", resp.Provider)
	assert.Equal(t, "gpt-5.4", resp.Model)
	assert.Equal(t, 7, resp.Usage.TotalTokens)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, types.RoleAssistant, resp.Choices[0].Message.Role)
	assert.Equal(t, "done", resp.Choices[0].Message.Content)
	require.NotNil(t, resp.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "reason", *resp.Choices[0].Message.ReasoningContent)

	require.Len(t, events, 3)
	assert.Equal(t, SDKMessageOutputCreated, events[0].SDKEventName)
	assert.Equal(t, "completion_judge_decision", events[1].Data.(map[string]any)["status"])
	assert.Equal(t, "loop_stopped", events[2].Data.(map[string]any)["status"])
}

func TestReactToolLoopBudgetDefaultsToExecutorBudget(t *testing.T) {
	assert.Equal(t, 10, reactToolLoopBudget(nil))
	assert.Equal(t, 10, reactToolLoopBudget(&preparedRequest{}))
}

func TestReactToolLoopBudgetUsesPreparedOverride(t *testing.T) {
	assert.Equal(t, 3, reactToolLoopBudget(&preparedRequest{maxReActIter: 3}))
}

func TestChatCompletionWithToolsPassesPreparedToolRiskToAuthorization(t *testing.T) {
	provider := &toolCallingProvider{
		responses: []types.ChatResponse{
			{
				Model: "gpt-4",
				Choices: []types.ChatChoice{{
					Index: 0,
					Message: types.Message{
						Role: types.RoleAssistant,
						ToolCalls: []types.ToolCall{{
							ID:        "call-1",
							Name:      "read_file",
							Arguments: json.RawMessage(`{"path":"README.md"}`),
						}},
					},
				}},
			},
			{
				Model: "gpt-4",
				Choices: []types.ChatChoice{{
					Index:   0,
					Message: types.Message{Role: types.RoleAssistant, Content: "done"},
				}},
			},
		},
	}
	manager := &recordingToolManager{
		schemas: []types.ToolSchema{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}},
		results: []types.ToolResult{{
			ToolCallID: "call-1",
			Name:       "read_file",
			Result:     json.RawMessage(`{"ok":true}`),
		}},
	}
	var captured []types.AuthorizationRequest
	authorize := func(_ context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
		captured = append(captured, req)
		return &types.AuthorizationDecision{Decision: types.DecisionAllow, Reason: "ok"}, nil
	}

	ag, err := BuildBaseAgent(
		types.AgentConfig{
			Core:    types.CoreConfig{ID: "agent-a", Name: "Agent A", Type: "assistant"},
			LLM:     types.LLMConfig{Model: "gpt-4"},
			Runtime: types.RuntimeConfig{MaxReActIterations: 2, Tools: []string{"read_file"}},
		},
		testGateway(provider),
		nil,
		manager,
		nil,
		zap.NewNop(),
		nil,
	)
	require.NoError(t, err)
	ag.SetAuthorizeFunc(authorize)

	require.NotEmpty(t, manager.GetAllowedTools("agent-a"))
	resp, err := ag.ChatCompletion(context.Background(), []types.Message{{
		Role:    types.RoleUser,
		Content: "read it",
	}})
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Len(t, captured, 1)
	assert.Equal(t, "read_file", captured[0].ResourceID)
	assert.Equal(t, types.RiskSafeRead, captured[0].RiskTier)
	assert.Equal(t, "agent-a", captured[0].Context["agent_id"])
	require.Len(t, manager.calls, 1)
	assert.Equal(t, "read_file", manager.calls[0].Name)
}

type toolCallingProvider struct {
	responses []types.ChatResponse
	calls     int
}

func (p *toolCallingProvider) Completion(_ context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	if p.calls >= len(p.responses) {
		return &llmcore.ChatResponse{
			Model: req.Model,
			Choices: []llmcore.ChatChoice{{
				Index:   0,
				Message: types.Message{Role: types.RoleAssistant, Content: "fallback"},
			}},
		}, nil
	}
	resp := p.responses[p.calls]
	p.calls++
	return &resp, nil
}

func (p *toolCallingProvider) Stream(_ context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	ch := make(chan llmcore.StreamChunk, 1)
	ch <- llmcore.StreamChunk{Model: req.Model, Delta: types.Message{Role: types.RoleAssistant, Content: "stream"}}
	close(ch)
	return ch, nil
}

func (p *toolCallingProvider) HealthCheck(context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true, Latency: time.Millisecond}, nil
}

func (p *toolCallingProvider) Name() string                        { return "tool-calling-provider" }
func (p *toolCallingProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *toolCallingProvider) ListModels(context.Context) ([]llmcore.Model, error) {
	return []llmcore.Model{{ID: "gpt-4"}}, nil
}
func (p *toolCallingProvider) Endpoints() llmcore.ProviderEndpoints {
	return llmcore.ProviderEndpoints{}
}

type recordingToolManager struct {
	schemas []types.ToolSchema
	results []types.ToolResult
	calls   []types.ToolCall
}

func (m *recordingToolManager) GetAllowedTools(string) []types.ToolSchema {
	return append([]types.ToolSchema(nil), m.schemas...)
}

func (m *recordingToolManager) ExecuteForAgent(_ context.Context, _ string, calls []types.ToolCall) []llmtools.ToolResult {
	m.calls = append(m.calls, calls...)
	return append([]types.ToolResult(nil), m.results...)
}
