package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// Mock Agent for resolver tests
// =============================================================================

type mockAgent struct {
	id        string
	name      string
	agentType agent.AgentType
	state     agent.State
	executeFn func(ctx context.Context, input *agent.Input) (*agent.Output, error)
	planFn    func(ctx context.Context, input *agent.Input) (*agent.PlanResult, error)
}

func (m *mockAgent) ID() string                                         { return m.id }
func (m *mockAgent) Name() string                                       { return m.name }
func (m *mockAgent) Type() agent.AgentType                              { return m.agentType }
func (m *mockAgent) State() agent.State                                 { return m.state }
func (m *mockAgent) Init(_ context.Context) error                       { return nil }
func (m *mockAgent) Teardown(_ context.Context) error                   { return nil }
func (m *mockAgent) Observe(_ context.Context, _ *agent.Feedback) error { return nil }

func (m *mockAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, input)
	}
	return &agent.Output{Content: "default response"}, nil
}

func (m *mockAgent) Plan(ctx context.Context, input *agent.Input) (*agent.PlanResult, error) {
	if m.planFn != nil {
		return m.planFn(ctx, input)
	}
	return &agent.PlanResult{Steps: []string{"step1"}}, nil
}

// =============================================================================
// HandleExecuteAgent with resolver — success
// =============================================================================

func TestAgentHandler_HandleExecuteAgent_WithResolver_Success(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("test-agent", discovery.AgentStatusOnline))

	ma := &mockAgent{
		id:   "test-agent",
		name: "test-agent",
		executeFn: func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{
				TraceID:      "trace-1",
				Content:      "hello back",
				TokensUsed:   42,
				FinishReason: "stop",
			}, nil
		},
	}

	resolver := func(ctx context.Context, agentID string) (agent.Agent, error) {
		if agentID == "test-agent" {
			return ma, nil
		}
		return nil, fmt.Errorf("not found")
	}

	handler := newTestHandlerWithResolver(reg, resolver)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "test-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecuteAgent(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.Success)
}

// =============================================================================
// HandleExecuteAgent with resolver — agent execution error
// =============================================================================

func TestAgentHandler_HandleExecuteAgent_WithResolver_ExecutionError(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("err-agent", discovery.AgentStatusOnline))

	ma := &mockAgent{
		executeFn: func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			return nil, errors.New("execution failed")
		},
	}

	resolver := func(ctx context.Context, agentID string) (agent.Agent, error) {
		return ma, nil
	}

	handler := newTestHandlerWithResolver(reg, resolver)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "err-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecuteAgent(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// =============================================================================
// HandleExecuteAgent with resolver — resolver returns not found
// =============================================================================

func TestAgentHandler_HandleExecuteAgent_WithResolver_NotFound(t *testing.T) {
	reg := newMockRegistry()

	resolver := func(ctx context.Context, agentID string) (agent.Agent, error) {
		return nil, fmt.Errorf("agent not found")
	}

	handler := newTestHandlerWithResolver(reg, resolver)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "missing-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecuteAgent(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// PLACEHOLDER_PART2

// =============================================================================
// HandleAgentStream — no resolver, agent exists (501)
// =============================================================================

func TestAgentHandler_HandleAgentStream_NoResolver_AgentExists(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("stream-agent", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "stream-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

// =============================================================================
// HandleAgentStream — no resolver, agent not found
// =============================================================================

func TestAgentHandler_HandleAgentStream_NoResolver_NotFound(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "nonexistent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// =============================================================================
// HandleAgentStream — missing body
// =============================================================================

func TestAgentHandler_HandleAgentStream_MissingBody(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", nil)

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleAgentStream — invalid agent ID
// =============================================================================

func TestAgentHandler_HandleAgentStream_InvalidAgentID(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "../../../etc/passwd",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleAgentStream — missing required fields
// =============================================================================

func TestAgentHandler_HandleAgentStream_MissingFields(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "test",
		Content: "",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleAgentStream — with resolver, agent not found
// =============================================================================

func TestAgentHandler_HandleAgentStream_WithResolver_NotFound(t *testing.T) {
	reg := newMockRegistry()
	resolver := func(ctx context.Context, agentID string) (agent.Agent, error) {
		return nil, fmt.Errorf("not found")
	}
	handler := newTestHandlerWithResolver(reg, resolver)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "missing",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// =============================================================================
// HandleAgentHealth — invalid agent ID format
// =============================================================================

func TestAgentHandler_HandleAgentHealth_InvalidID(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/health?id=../../../etc", nil)

	handler.HandleAgentHealth(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleListAgents — registry error
// =============================================================================

func TestAgentHandler_HandleListAgents_Error(t *testing.T) {
	reg := &mockRegistry{
		agents: make(map[string]*discovery.AgentInfo),
		err:    errors.New("registry error"),
	}
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)

	handler.HandleListAgents(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// =============================================================================
// extractAgentID — with PathValue
// =============================================================================

func TestExtractAgentID_WithPathValue(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/my-agent-1", nil)
	r.SetPathValue("id", "my-agent-1")

	id := extractAgentID(r)
	assert.Equal(t, "my-agent-1", id)
}

func TestExtractAgentID_InvalidPathValue(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/bad<id>", nil)
	r.SetPathValue("id", "bad<id>")

	id := extractAgentID(r)
	assert.Equal(t, "", id)
}

func TestExtractAgentID_EmptyPath(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/", nil)

	id := extractAgentID(r)
	assert.Equal(t, "", id)
}

// =============================================================================
// DatabaseHealthCheck and RedisHealthCheck
// =============================================================================

func TestDatabaseHealthCheck(t *testing.T) {
	check := NewDatabaseHealthCheck("postgres", func(ctx context.Context) error {
		return nil
	})
	assert.Equal(t, "postgres", check.Name())
	assert.NoError(t, check.Check(context.Background()))
}

func TestDatabaseHealthCheck_Error(t *testing.T) {
	check := NewDatabaseHealthCheck("postgres", func(ctx context.Context) error {
		return errors.New("connection refused")
	})
	assert.Error(t, check.Check(context.Background()))
}

func TestRedisHealthCheck(t *testing.T) {
	check := NewRedisHealthCheck("redis", func(ctx context.Context) error {
		return nil
	})
	assert.Equal(t, "redis", check.Name())
	assert.NoError(t, check.Check(context.Background()))
}

func TestRedisHealthCheck_Error(t *testing.T) {
	check := NewRedisHealthCheck("redis", func(ctx context.Context) error {
		return errors.New("timeout")
	})
	assert.Error(t, check.Check(context.Background()))
}

// PLACEHOLDER_PART3

// =============================================================================
// ChatHandler — handleProviderError
// =============================================================================

func TestChatHandler_HandleProviderError_TypedError(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	w := httptest.NewRecorder()
	typedErr := types.NewError(types.ErrRateLimit, "too many requests")
	handler.handleProviderError(w, typedErr)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestChatHandler_HandleProviderError_GenericError(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	w := httptest.NewRecorder()
	handler.handleProviderError(w, errors.New("unknown error"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// =============================================================================
// ChatHandler — convertTypesMessageToAPI with images
// =============================================================================

func TestConvertTypesMessageToAPI_WithImages(t *testing.T) {
	msg := types.Message{
		Role:    types.RoleAssistant,
		Content: "Here is the image",
		Images: []types.ImageContent{
			{Type: "url", URL: "https://example.com/img.png"},
			{Type: "base64", Data: "aGVsbG8="},
		},
	}

	result := convertTypesMessageToAPI(msg)
	assert.Equal(t, "assistant", result.Role)
	assert.Equal(t, "Here is the image", result.Content)
	require.Len(t, result.Images, 2)
	assert.Equal(t, "url", result.Images[0].Type)
	assert.Equal(t, "https://example.com/img.png", result.Images[0].URL)
	assert.Equal(t, "base64", result.Images[1].Type)
	assert.Equal(t, "aGVsbG8=", result.Images[1].Data)
}

func TestConvertTypesMessageToAPI_WithMetadata(t *testing.T) {
	ts := time.Now()
	msg := types.Message{
		Role:       types.RoleUser,
		Content:    "test",
		Name:       "user1",
		ToolCallID: "tc-1",
		Metadata:   map[string]string{"key": "val"},
		Timestamp:  ts,
	}

	result := convertTypesMessageToAPI(msg)
	assert.Equal(t, "user", result.Role)
	assert.Equal(t, "user1", result.Name)
	assert.Equal(t, "tc-1", result.ToolCallID)
	assert.Equal(t, map[string]string{"key": "val"}, result.Metadata)
	assert.Equal(t, ts, result.Timestamp)
}

func TestConvertTypesMessageToAPI_WithExtendedFields(t *testing.T) {
	reasoning := "internal reasoning"
	refusal := "cannot comply"
	msg := types.Message{
		Role:             types.RoleAssistant,
		Content:          "final answer",
		ReasoningContent: &reasoning,
		ReasoningSummaries: []types.ReasoningSummary{
			{Provider: "openai", ID: "rs_1", Kind: "summary_text", Text: "short summary"},
		},
		OpaqueReasoning: []types.OpaqueReasoning{
			{Provider: "openai", ID: "rs_1", Kind: "encrypted_content", State: "enc_123"},
		},
		ThinkingBlocks: []types.ThinkingBlock{
			{Thinking: "step 1"},
		},
		Refusal:     &refusal,
		IsToolError: true,
		Videos: []types.VideoContent{
			{URL: "https://example.com/video.mp4"},
		},
		Annotations: []types.Annotation{
			{Type: "url_citation", URL: "https://example.com", Title: "example"},
		},
	}

	result := convertTypesMessageToAPI(msg)
	require.NotNil(t, result.ReasoningContent)
	require.NotNil(t, result.Refusal)
	assert.Equal(t, reasoning, *result.ReasoningContent)
	require.Len(t, result.ReasoningSummaries, 1)
	assert.Equal(t, "short summary", result.ReasoningSummaries[0].Text)
	require.Len(t, result.OpaqueReasoning, 1)
	assert.Equal(t, "enc_123", result.OpaqueReasoning[0].State)
	assert.Len(t, result.ThinkingBlocks, 1)
	assert.Equal(t, "step 1", result.ThinkingBlocks[0].Thinking)
	assert.Equal(t, refusal, *result.Refusal)
	assert.True(t, result.IsToolError)
	assert.Len(t, result.Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", result.Videos[0].URL)
	assert.Len(t, result.Annotations, 1)
	assert.Equal(t, "url_citation", result.Annotations[0].Type)
	assert.Equal(t, "https://example.com", result.Annotations[0].URL)
}

// =============================================================================
// ChatHandler — convertStreamUsage
// =============================================================================

func TestConvertStreamUsage_Nil(t *testing.T) {
	result := convertStreamUsage(nil)
	assert.Nil(t, result)
}

func TestConvertStreamUsage_NonNil(t *testing.T) {
	usage := &llm.ChatUsage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}
	result := convertStreamUsage(usage)
	require.NotNil(t, result)
	assert.Equal(t, 10, result.PromptTokens)
	assert.Equal(t, 20, result.CompletionTokens)
	assert.Equal(t, 30, result.TotalTokens)
}

// =============================================================================
// ChatHandler — convertToLLMRequest with images
// =============================================================================

func TestChatHandler_ConvertToLLMRequest_WithImages(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	apiReq := &api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{
				Role:    "user",
				Content: "Look at this",
				Images: []api.ImageContent{
					{Type: "url", URL: "https://example.com/img.png"},
				},
			},
		},
	}

	llmReq := handler.convertToLLMRequest(apiReq)
	require.Len(t, llmReq.Messages, 1)
	require.Len(t, llmReq.Messages[0].Images, 1)
	assert.Equal(t, "url", llmReq.Messages[0].Images[0].Type)
	assert.Equal(t, "https://example.com/img.png", llmReq.Messages[0].Images[0].URL)
}

func TestChatHandler_ConvertToLLMRequest_InvalidTimeout(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	apiReq := &api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: "Hello"},
		},
		Timeout: "not-a-duration",
	}

	llmReq := handler.convertToLLMRequest(apiReq)
	// Should fall back to default 30s
	assert.Equal(t, 30*time.Second, llmReq.Timeout)
}

func TestChatHandler_ConvertToLLMRequest_WithExtendedMessageFields(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	reasoning := "internal reasoning"
	refusal := "cannot comply"
	apiReq := &api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{
				Role:             "assistant",
				Content:          "final answer",
				ReasoningContent: &reasoning,
				ReasoningSummaries: []types.ReasoningSummary{
					{Provider: "gemini", ID: "part_0", Kind: "thought_summary", Text: "summary"},
				},
				OpaqueReasoning: []types.OpaqueReasoning{
					{Provider: "gemini", Kind: "thought_signature", State: "sig_1", PartIndex: 0},
				},
				ThinkingBlocks: []types.ThinkingBlock{
					{Thinking: "step 1"},
				},
				Refusal:     &refusal,
				IsToolError: true,
				Videos: []types.VideoContent{
					{URL: "https://example.com/video.mp4"},
				},
				Annotations: []types.Annotation{
					{Type: "url_citation", URL: "https://example.com", Title: "example"},
				},
			},
		},
	}

	llmReq := handler.convertToLLMRequest(apiReq)
	require.Len(t, llmReq.Messages, 1)
	msg := llmReq.Messages[0]
	require.NotNil(t, msg.ReasoningContent)
	require.NotNil(t, msg.Refusal)
	assert.Equal(t, reasoning, *msg.ReasoningContent)
	require.Len(t, msg.ReasoningSummaries, 1)
	assert.Equal(t, "summary", msg.ReasoningSummaries[0].Text)
	require.Len(t, msg.OpaqueReasoning, 1)
	assert.Equal(t, "sig_1", msg.OpaqueReasoning[0].State)
	assert.Len(t, msg.ThinkingBlocks, 1)
	assert.Equal(t, "step 1", msg.ThinkingBlocks[0].Thinking)
	assert.Equal(t, refusal, *msg.Refusal)
	assert.True(t, msg.IsToolError)
	assert.Len(t, msg.Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", msg.Videos[0].URL)
	assert.Len(t, msg.Annotations, 1)
	assert.Equal(t, "url_citation", msg.Annotations[0].Type)
	assert.Equal(t, "https://example.com", msg.Annotations[0].URL)
}

// =============================================================================
// ChatHandler — HandleCompletion with provider error
// =============================================================================

func TestChatHandler_HandleCompletion_ProviderError(t *testing.T) {
	provider := &mockProvider{
		completionFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, types.NewError(types.ErrServiceUnavailable, "service down")
		},
	}

	handler := newChatHandlerForProvider(provider, zap.NewNop())

	request := api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	body, _ := json.Marshal(request)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleCompletion(w, r)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// =============================================================================
// ChatHandler — HandleCompletion with wrong content type
// =============================================================================

func TestChatHandler_HandleCompletion_WrongContentType(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Content-Type", "text/plain")

	handler.HandleCompletion(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// ChatHandler — HandleStream with provider error
// =============================================================================

func TestChatHandler_HandleStream_ProviderError(t *testing.T) {
	provider := &mockProvider{
		streamFunc: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			return nil, errors.New("stream init failed")
		},
	}

	handler := newChatHandlerForProvider(provider, zap.NewNop())

	request := api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	body, _ := json.Marshal(request)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleStream(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// =============================================================================
// ChatHandler — HandleStream with stream error chunk
// =============================================================================

func TestChatHandler_HandleStream_StreamErrorChunk(t *testing.T) {
	provider := &mockProvider{
		streamFunc: func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			ch := make(chan llm.StreamChunk, 2)
			ch <- llm.StreamChunk{
				ID:    "test-id",
				Delta: types.Message{Content: "partial"},
			}
			ch <- llm.StreamChunk{
				Err: types.NewError(types.ErrInternalError, "stream broke"),
			}
			close(ch)
			return ch, nil
		},
	}

	handler := newChatHandlerForProvider(provider, zap.NewNop())

	request := api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	body, _ := json.Marshal(request)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleStream(w, r)

	// Should contain the error event
	assert.Contains(t, w.Body.String(), "event: error")
	assert.Contains(t, w.Body.String(), "stream broke")
}

// =============================================================================
// ChatHandler — validateChatRequest with invalid role
// =============================================================================

func TestChatHandler_ValidateChatRequest_InvalidRole(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	req := &api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "invalid_role", Content: "Hello"},
		},
	}

	err := handler.validateChatRequest(req)
	assert.NotNil(t, err)
	assert.Contains(t, err.Message, "role must be one of")
}

func TestChatHandler_ValidateChatRequest_DeveloperRole(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	req := &api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "developer", Content: "You must output JSON"},
		},
	}

	err := handler.validateChatRequest(req)
	assert.Nil(t, err)
}

// =============================================================================
// ChatHandler — validateChatRequest with negative max_tokens
// =============================================================================

func TestChatHandler_ValidateChatRequest_NegativeMaxTokens(t *testing.T) {
	handler := NewChatHandler(nil, zap.NewNop())

	req := &api.ChatRequest{
		Model: "gpt-4",
		Messages: []api.Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: -1,
	}

	err := handler.validateChatRequest(req)
	assert.NotNil(t, err)
	assert.Contains(t, err.Message, "max_tokens")
}

// =============================================================================
// Common — ValidateURL
// =============================================================================

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"valid https", "https://example.com", true},
		{"valid http", "http://example.com", true},
		{"no scheme", "example.com", false},
		{"ftp scheme", "ftp://example.com", false},
		{"empty", "", false},
		{"just scheme", "https://", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidateURL(tt.url))
		})
	}
}

// =============================================================================
// Common — ValidateNonNegative
// =============================================================================

func TestValidateNonNegative(t *testing.T) {
	assert.True(t, ValidateNonNegative(0))
	assert.True(t, ValidateNonNegative(1.5))
	assert.False(t, ValidateNonNegative(-0.1))
}

// =============================================================================
// Common — WriteErrorMessage
// =============================================================================

func TestWriteErrorMessage(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorMessage(w, http.StatusForbidden, types.ErrForbidden, "access denied", zap.NewNop())

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Success)
	assert.Equal(t, string(types.ErrForbidden), resp.Error.Code)
}

// =============================================================================
// Common — mapErrorCodeToHTTPStatus additional codes
// =============================================================================

func TestMapErrorCodeToHTTPStatus_AdditionalCodes(t *testing.T) {
	tests := []struct {
		code       types.ErrorCode
		wantStatus int
	}{
		{types.ErrQuotaExceeded, http.StatusPaymentRequired},
		{types.ErrContextTooLong, http.StatusRequestEntityTooLarge},
		{types.ErrContentFiltered, http.StatusUnprocessableEntity},
		{types.ErrToolValidation, http.StatusBadRequest},
		{types.ErrGuardrailsViolated, http.StatusForbidden},
		{types.ErrUpstreamTimeout, http.StatusGatewayTimeout},
		{types.ErrModelOverloaded, http.StatusServiceUnavailable},
		{types.ErrProviderUnavailable, http.StatusServiceUnavailable},
		{types.ErrUpstreamError, http.StatusBadGateway},
		{types.ErrUnauthorized, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			assert.Equal(t, tt.wantStatus, mapErrorCodeToHTTPStatus(tt.code))
		})
	}
}

// =============================================================================
// Common — DecodeJSONBody with nil body
// =============================================================================

func TestDecodeJSONBody_NilBody(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	r.Body = nil

	var result map[string]string
	err := DecodeJSONBody(w, r, &result, zap.NewNop())
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// Common — ResponseWriter Write without explicit WriteHeader
// =============================================================================

func TestResponseWriter_WriteWithoutHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	// Write without calling WriteHeader first
	n, err := rw.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.True(t, rw.Written)
	assert.Equal(t, http.StatusOK, rw.StatusCode)
}

// =============================================================================
// HandleAgentStream — with resolver, success
// =============================================================================

func TestAgentHandler_HandleAgentStream_WithResolver_Success(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("stream-agent", discovery.AgentStatusOnline))

	ma := &mockAgent{
		executeFn: func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			return &agent.Output{Content: "streamed result"}, nil
		},
	}

	resolver := func(ctx context.Context, agentID string) (agent.Agent, error) {
		return ma, nil
	}

	handler := newTestHandlerWithResolver(reg, resolver)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "stream-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "[DONE]")
}

// =============================================================================
// HandleAgentStream — with resolver, execution error
// =============================================================================

func TestAgentHandler_HandleAgentStream_WithResolver_ExecutionError(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("err-stream", discovery.AgentStatusOnline))

	ma := &mockAgent{
		executeFn: func(ctx context.Context, input *agent.Input) (*agent.Output, error) {
			return nil, errors.New("stream execution failed")
		},
	}

	resolver := func(ctx context.Context, agentID string) (agent.Agent, error) {
		return ma, nil
	}

	handler := newTestHandlerWithResolver(reg, resolver)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "err-stream",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	// Should contain error event and DONE
	assert.Contains(t, w.Body.String(), "event: error")
	assert.Contains(t, w.Body.String(), "\"code\":\"INTERNAL_ERROR\"")
	assert.Contains(t, w.Body.String(), "\"request_id\":")
	assert.Contains(t, w.Body.String(), "[DONE]")
}

// =============================================================================
// HandleGetAgent — registry error
// =============================================================================

func TestAgentHandler_HandleGetAgent_RegistryError(t *testing.T) {
	reg := &mockRegistry{
		agents: make(map[string]*discovery.AgentInfo),
		err:    errors.New("registry error"),
	}
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/some-agent", nil)

	handler.HandleGetAgent(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// =============================================================================
// HandleListAPIKeys — invalid provider ID
// =============================================================================

func TestHandleListAPIKeys_InvalidProviderID(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormAPIKeyStore(db)
	h := NewAPIKeyHandler(usecase.NewDefaultAPIKeyService(store), zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/abc/api-keys", nil)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()
	h.HandleListAPIKeys(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleCreateAPIKey — missing content type
// =============================================================================

func TestHandleCreateAPIKey_MissingContentType(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormAPIKeyStore(db)
	h := NewAPIKeyHandler(usecase.NewDefaultAPIKeyService(store), zap.NewNop())

	body, _ := json.Marshal(createAPIKeyRequest{APIKey: "sk-test", Label: "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/1/api-keys", bytes.NewReader(body))
	// No Content-Type header
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.HandleCreateAPIKey(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleUpdateAPIKey — invalid provider ID
// =============================================================================

func TestHandleUpdateAPIKey_InvalidProviderID(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormAPIKeyStore(db)
	h := NewAPIKeyHandler(usecase.NewDefaultAPIKeyService(store), zap.NewNop())

	newLabel := "updated"
	body, _ := json.Marshal(updateAPIKeyRequest{Label: &newLabel})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/providers/abc/api-keys/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "abc")
	req.SetPathValue("keyId", "1")
	w := httptest.NewRecorder()
	h.HandleUpdateAPIKey(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleUpdateAPIKey — invalid key ID
// =============================================================================

func TestHandleUpdateAPIKey_InvalidKeyID(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormAPIKeyStore(db)
	h := NewAPIKeyHandler(usecase.NewDefaultAPIKeyService(store), zap.NewNop())

	newLabel := "updated"
	body, _ := json.Marshal(updateAPIKeyRequest{Label: &newLabel})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/providers/1/api-keys/abc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "1")
	req.SetPathValue("keyId", "abc")
	w := httptest.NewRecorder()
	h.HandleUpdateAPIKey(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleDeleteAPIKey — invalid provider ID
// =============================================================================

func TestHandleDeleteAPIKey_InvalidProviderID(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormAPIKeyStore(db)
	h := NewAPIKeyHandler(usecase.NewDefaultAPIKeyService(store), zap.NewNop())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/providers/abc/api-keys/1", nil)
	req.SetPathValue("id", "abc")
	req.SetPathValue("keyId", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteAPIKey(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// =============================================================================
// HandleAPIKeyStats — with keys
// =============================================================================

func TestHandleAPIKeyStats_WithKeys(t *testing.T) {
	db := setupTestDB(t)
	store := NewGormAPIKeyStore(db)
	h := NewAPIKeyHandler(usecase.NewDefaultAPIKeyService(store), zap.NewNop())

	db.Create(&llm.LLMProviderAPIKey{
		ProviderID: 1, APIKey: "sk-stats-1", BaseURL: "https://api.test.com",
		Label: "key1", Priority: 10, Weight: 100, Enabled: true,
		TotalRequests: 200, FailedRequests: 10,
	})
	db.Create(&llm.LLMProviderAPIKey{
		ProviderID: 1, APIKey: "sk-stats-2", BaseURL: "https://api.test.com",
		Label: "key2", Priority: 20, Weight: 50, Enabled: false,
		TotalRequests: 50, FailedRequests: 0,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/1/api-keys/stats", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	h.HandleAPIKeyStats(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
}
