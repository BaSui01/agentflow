package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newChatServiceUnderTest(
	gateway llmcore.Gateway,
	chatProvider llm.Provider,
	toolManager agent.ToolManager,
) usecase.ChatService {
	return usecase.NewDefaultChatService(
		usecase.ChatRuntime{
			Gateway:      gateway,
			ChatProvider: chatProvider,
			ToolManager:  toolManager,
		},
		newUsecaseChatConverter(NewDefaultChatConverter(defaultStreamTimeout)),
		zap.NewNop(),
	)
}

type chatGatewayStub struct {
	invokeReq *llmcore.UnifiedRequest
	streamReq *llmcore.UnifiedRequest
}

type chatProviderStub struct {
	completionCalls int
	streamCalls     int
	completionFunc  func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFunc      func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
}

func (s *chatProviderStub) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	s.completionCalls++
	if s.completionFunc == nil {
		return nil, errors.New("completion not configured")
	}
	return s.completionFunc(ctx, req)
}

func (s *chatProviderStub) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	s.streamCalls++
	if s.streamFunc == nil {
		return nil, errors.New("stream not configured")
	}
	return s.streamFunc(ctx, req)
}

func (s *chatProviderStub) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (s *chatProviderStub) Name() string { return "chat-provider-stub" }

func (s *chatProviderStub) SupportsNativeFunctionCalling() bool { return true }

func (s *chatProviderStub) ListModels(_ context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (s *chatProviderStub) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{}
}

type chatToolManagerStub struct {
	allowed  []types.ToolSchema
	executed []types.ToolCall
	execFn   func(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult
}

func (s *chatToolManagerStub) GetAllowedTools(agentID string) []types.ToolSchema {
	_ = agentID
	return s.allowed
}

func (s *chatToolManagerStub) ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
	_ = agentID
	s.executed = append(s.executed, calls...)
	if s.execFn != nil {
		return s.execFn(ctx, agentID, calls)
	}
	results := make([]llmtools.ToolResult, len(calls))
	for i, call := range calls {
		results[i] = llmtools.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Result:     json.RawMessage(`{"ok":true}`),
		}
	}
	return results
}

func (s *chatGatewayStub) Invoke(_ context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	s.invokeReq = req
	return &llmcore.UnifiedResponse{
		Output: &llm.ChatResponse{
			ID:       "chat-1",
			Provider: "openai",
			Model:    "gpt-4o",
			Choices: []llm.ChatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: types.Message{
						Role:    types.RoleAssistant,
						Content: "ok",
					},
				},
			},
			Usage: llm.ChatUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
			CreatedAt: time.Now(),
		},
	}, nil
}

func (s *chatGatewayStub) Stream(_ context.Context, req *llmcore.UnifiedRequest) (<-chan llmcore.UnifiedChunk, error) {
	s.streamReq = req
	ch := make(chan llmcore.UnifiedChunk, 1)
	ch <- llmcore.UnifiedChunk{
		Output: &llm.StreamChunk{
			ID:       "stream-1",
			Provider: "openai",
			Model:    "gpt-4o",
			Delta: types.Message{
				Role:    types.RoleAssistant,
				Content: "hello",
			},
		},
	}
	close(ch)
	return ch, nil
}

func TestChatService_Complete_RoutesByParams(t *testing.T) {
	gw := &chatGatewayStub{}
	svc := newChatServiceUnderTest(gw, nil, nil)

	result, err := svc.Complete(context.Background(), NewDefaultChatConverter(defaultStreamTimeout).ToUsecaseRequest(&api.ChatRequest{
		Model:        "gpt-4o",
		Provider:     "openai",
		RoutePolicy:  "cost_first",
		EndpointMode: "responses",
		Messages: []api.Message{
			{Role: "user", Content: "hello"},
		},
		Metadata: map[string]string{"tenant": "t1"},
		Tags:     []string{"prod", "prod", "chat"},
	}))
	require.Nil(t, err)
	require.NotNil(t, result)
	require.NotNil(t, gw.invokeReq)

	assert.Equal(t, "openai", gw.invokeReq.ProviderHint)
	assert.Equal(t, llmcore.RoutePolicyCostFirst, gw.invokeReq.RoutePolicy)
	assert.Equal(t, "openai", gw.invokeReq.Metadata[llmcore.MetadataKeyChatProvider])
	assert.Equal(t, "cost_first", gw.invokeReq.Metadata["route_policy"])
	assert.Equal(t, "responses", gw.invokeReq.Metadata["endpoint_mode"])
	assert.Equal(t, "t1", gw.invokeReq.Metadata["tenant"])
	assert.Equal(t, []string{"prod", "chat"}, gw.invokeReq.Tags)
}

func TestChatService_Complete_InvalidRoutePolicy(t *testing.T) {
	gw := &chatGatewayStub{}
	svc := newChatServiceUnderTest(gw, nil, nil)

	result, err := svc.Complete(context.Background(), NewDefaultChatConverter(defaultStreamTimeout).ToUsecaseRequest(&api.ChatRequest{
		Model:       "gpt-4o",
		RoutePolicy: "fastest",
		Messages: []api.Message{
			{Role: "user", Content: "hello"},
		},
	}))
	require.Nil(t, result)
	require.NotNil(t, err)
	assert.Equal(t, types.ErrInvalidRequest, err.Code)
}

func TestChatService_Complete_InvalidEndpointMode(t *testing.T) {
	gw := &chatGatewayStub{}
	svc := newChatServiceUnderTest(gw, nil, nil)

	result, err := svc.Complete(context.Background(), NewDefaultChatConverter(defaultStreamTimeout).ToUsecaseRequest(&api.ChatRequest{
		Model:        "gpt-4o",
		EndpointMode: "responses_api",
		Messages: []api.Message{
			{Role: "user", Content: "hello"},
		},
	}))
	require.Nil(t, result)
	require.NotNil(t, err)
	assert.Equal(t, types.ErrInvalidRequest, err.Code)
}

func TestChatService_Stream_RoutesByParams(t *testing.T) {
	gw := &chatGatewayStub{}
	svc := newChatServiceUnderTest(gw, nil, nil)

	stream, err := svc.Stream(context.Background(), NewDefaultChatConverter(defaultStreamTimeout).ToUsecaseRequest(&api.ChatRequest{
		Model:        "gpt-4o",
		Provider:     "openai",
		RoutePolicy:  "balanced",
		EndpointMode: "chat_completions",
		Messages: []api.Message{
			{Role: "user", Content: "hello"},
		},
	}))
	require.Nil(t, err)
	require.NotNil(t, stream)
	require.NotNil(t, gw.streamReq)
	assert.Equal(t, "openai", gw.streamReq.ProviderHint)
	assert.Equal(t, llmcore.RoutePolicyBalanced, gw.streamReq.RoutePolicy)
	assert.Equal(t, "chat_completions", gw.streamReq.Metadata["endpoint_mode"])
}

func TestChatService_Complete_UsesLocalToolLoopWhenAvailable(t *testing.T) {
	gw := &chatGatewayStub{}

	provider := &chatProviderStub{}
	provider.completionFunc = func(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
		if provider.completionCalls == 1 {
			require.Len(t, req.Tools, 1)
			assert.Equal(t, "retrieval", req.Tools[0].Name)
			return &llm.ChatResponse{
				ID:       "step-1",
				Provider: "gateway",
				Model:    req.Model,
				Choices: []llm.ChatChoice{
					{
						Index:        0,
						FinishReason: "tool_calls",
						Message: types.Message{
							Role: types.RoleAssistant,
							ToolCalls: []types.ToolCall{
								{
									ID:        "call-1",
									Name:      "retrieval",
									Arguments: json.RawMessage(`{"query":"agentflow"}`),
								},
							},
						},
					},
				},
			}, nil
		}

		require.GreaterOrEqual(t, len(req.Messages), 3)
		assert.Equal(t, types.RoleTool, req.Messages[len(req.Messages)-1].Role)
		return &llm.ChatResponse{
			ID:       "step-2",
			Provider: "gateway",
			Model:    req.Model,
			Choices: []llm.ChatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message: types.Message{
						Role:    types.RoleAssistant,
						Content: "final from tool loop",
					},
				},
			},
		}, nil
	}

	toolManager := &chatToolManagerStub{
		allowed: []types.ToolSchema{
			{Name: "retrieval", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
		execFn: func(_ context.Context, _ string, calls []types.ToolCall) []llmtools.ToolResult {
			require.Len(t, calls, 1)
			assert.Equal(t, "retrieval", calls[0].Name)
			return []llmtools.ToolResult{
				{
					ToolCallID: calls[0].ID,
					Name:       calls[0].Name,
					Result:     json.RawMessage(`{"doc":"hit"}`),
				},
			}
		},
	}

	svc := newChatServiceUnderTest(gw, provider, toolManager)

	result, err := svc.Complete(context.Background(), NewDefaultChatConverter(defaultStreamTimeout).ToUsecaseRequest(&api.ChatRequest{
		Model: "gpt-4o",
		Messages: []api.Message{
			{Role: "user", Content: "请检索并回答"},
		},
		Tools: []api.ToolSchema{
			{Name: "retrieval", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "not_registered", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
		ToolChoice: "auto",
	}))
	require.Nil(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Raw)
	assert.Equal(t, "final from tool loop", result.Raw.Choices[0].Message.Content)
	assert.Equal(t, 2, provider.completionCalls)
	assert.Len(t, toolManager.executed, 1)
	assert.Nil(t, gw.invokeReq)
}

func TestChatService_Complete_FallbackGatewayWhenNoToolManager(t *testing.T) {
	gw := &chatGatewayStub{}
	provider := &chatProviderStub{
		completionFunc: func(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, errors.New("should not call provider completion")
		},
	}
	svc := newChatServiceUnderTest(gw, provider, nil)

	result, err := svc.Complete(context.Background(), NewDefaultChatConverter(defaultStreamTimeout).ToUsecaseRequest(&api.ChatRequest{
		Model: "gpt-4o",
		Messages: []api.Message{
			{Role: "user", Content: "hello"},
		},
		Tools: []api.ToolSchema{
			{Name: "retrieval", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	}))
	require.Nil(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, provider.completionCalls)
	require.NotNil(t, gw.invokeReq)
}

func TestChatService_Stream_UsesLocalToolLoopWhenAvailable(t *testing.T) {
	gw := &chatGatewayStub{}

	provider := &chatProviderStub{}
	provider.streamFunc = func(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
		ch := make(chan llm.StreamChunk, 2)
		if provider.streamCalls == 1 {
			require.Len(t, req.Tools, 1)
			ch <- llm.StreamChunk{
				ID:       "step-1",
				Provider: "gateway",
				Model:    req.Model,
				Delta: types.Message{
					Role: types.RoleAssistant,
					ToolCalls: []types.ToolCall{
						{
							ID:        "call-1",
							Name:      "retrieval",
							Arguments: json.RawMessage(`{"query":"agentflow"}`),
						},
					},
				},
				FinishReason: "tool_calls",
			}
		} else {
			ch <- llm.StreamChunk{
				ID:       "step-2",
				Provider: "gateway",
				Model:    req.Model,
				Delta: types.Message{
					Role:    types.RoleAssistant,
					Content: "final stream answer",
				},
				FinishReason: "stop",
			}
		}
		close(ch)
		return ch, nil
	}

	toolManager := &chatToolManagerStub{
		allowed: []types.ToolSchema{
			{Name: "retrieval", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	}

	svc := newChatServiceUnderTest(gw, provider, toolManager)
	stream, err := svc.Stream(context.Background(), NewDefaultChatConverter(defaultStreamTimeout).ToUsecaseRequest(&api.ChatRequest{
		Model: "gpt-4o",
		Messages: []api.Message{
			{Role: "user", Content: "流式检索回答"},
		},
		Tools: []api.ToolSchema{
			{Name: "retrieval", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	}))
	require.Nil(t, err)
	require.NotNil(t, stream)
	assert.Nil(t, gw.streamReq)

	var chunks []*llm.StreamChunk
	for c := range stream {
		require.Nil(t, c.Err)
		chunk, ok := c.Output.(*llm.StreamChunk)
		require.True(t, ok)
		chunks = append(chunks, chunk)
	}

	require.NotEmpty(t, chunks)
	assert.Equal(t, 2, provider.streamCalls)
	assert.Contains(t, chunks[len(chunks)-1].Delta.Content, "final stream answer")
	assert.Len(t, toolManager.executed, 1)
}
