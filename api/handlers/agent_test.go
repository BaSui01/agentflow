package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// Mock Registry for testing
// =============================================================================

type mockRegistry struct {
	agents map[string]*discovery.AgentInfo
	err    error
}

func newMockRegistry() *mockRegistry {
	return &mockRegistry{
		agents: make(map[string]*discovery.AgentInfo),
	}
}

func (m *mockRegistry) withAgent(info *discovery.AgentInfo) *mockRegistry {
	m.agents[info.Card.Name] = info
	return m
}

func (m *mockRegistry) RegisterAgent(_ context.Context, info *discovery.AgentInfo) error {
	if m.err != nil {
		return m.err
	}
	m.agents[info.Card.Name] = info
	return nil
}

func (m *mockRegistry) UnregisterAgent(_ context.Context, agentID string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.agents, agentID)
	return nil
}

func (m *mockRegistry) UpdateAgent(_ context.Context, info *discovery.AgentInfo) error {
	return m.err
}

func (m *mockRegistry) GetAgent(_ context.Context, agentID string) (*discovery.AgentInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	info, ok := m.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}
	return info, nil
}

func (m *mockRegistry) ListAgents(_ context.Context) ([]*discovery.AgentInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*discovery.AgentInfo, 0, len(m.agents))
	for _, info := range m.agents {
		result = append(result, info)
	}
	return result, nil
}

func (m *mockRegistry) RegisterCapability(_ context.Context, _ string, _ *discovery.CapabilityInfo) error {
	return nil
}
func (m *mockRegistry) UnregisterCapability(_ context.Context, _, _ string) error { return nil }
func (m *mockRegistry) UpdateCapability(_ context.Context, _ string, _ *discovery.CapabilityInfo) error {
	return nil
}
func (m *mockRegistry) GetCapability(_ context.Context, _, _ string) (*discovery.CapabilityInfo, error) {
	return nil, nil
}
func (m *mockRegistry) ListCapabilities(_ context.Context, _ string) ([]discovery.CapabilityInfo, error) {
	return nil, nil
}

func (m *mockRegistry) FindCapabilities(_ context.Context, _ string) ([]discovery.CapabilityInfo, error) {
	return nil, nil
}
func (m *mockRegistry) UpdateAgentStatus(_ context.Context, _ string, _ discovery.AgentStatus) error {
	return nil
}
func (m *mockRegistry) UpdateAgentLoad(_ context.Context, _ string, _ float64) error { return nil }
func (m *mockRegistry) RecordExecution(_ context.Context, _ string, _ string, _ bool, _ time.Duration) error {
	return nil
}
func (m *mockRegistry) Subscribe(_ discovery.DiscoveryEventHandler) string { return "" }
func (m *mockRegistry) Unsubscribe(_ string)                               {}
func (m *mockRegistry) Close() error                                       { return nil }

// Verify mockRegistry implements discovery.Registry
var _ discovery.Registry = (*mockRegistry)(nil)

type stubAgentService struct {
	resolveForOperationFn func(ctx context.Context, agentID string, op usecase.AgentOperation) (agent.Agent, *types.Error)
	listAgentsFn          func(ctx context.Context) ([]*discovery.AgentInfo, *types.Error)
	getAgentFn            func(ctx context.Context, agentID string) (*discovery.AgentInfo, *types.Error)
	executeAgentFn        func(ctx context.Context, req usecase.AgentExecuteRequest, traceID string) (*usecase.AgentExecuteResponse, time.Duration, *types.Error)
	executeAgentStreamFn  func(ctx context.Context, req usecase.AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error
}

func (s *stubAgentService) ResolveForOperation(ctx context.Context, agentID string, op usecase.AgentOperation) (agent.Agent, *types.Error) {
	if s.resolveForOperationFn != nil {
		return s.resolveForOperationFn(ctx, agentID, op)
	}
	return nil, nil
}

func (s *stubAgentService) ListAgents(ctx context.Context) ([]*discovery.AgentInfo, *types.Error) {
	if s.listAgentsFn != nil {
		return s.listAgentsFn(ctx)
	}
	return nil, nil
}

func (s *stubAgentService) GetAgent(ctx context.Context, agentID string) (*discovery.AgentInfo, *types.Error) {
	if s.getAgentFn != nil {
		return s.getAgentFn(ctx, agentID)
	}
	return nil, nil
}

func (s *stubAgentService) ExecuteAgent(ctx context.Context, req usecase.AgentExecuteRequest, traceID string) (*usecase.AgentExecuteResponse, time.Duration, *types.Error) {
	if s.executeAgentFn != nil {
		return s.executeAgentFn(ctx, req, traceID)
	}
	return nil, 0, nil
}

func (s *stubAgentService) ExecuteAgentStream(ctx context.Context, req usecase.AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error {
	if s.executeAgentStreamFn != nil {
		return s.executeAgentStreamFn(ctx, req, traceID, emitter)
	}
	return nil
}

// =============================================================================
// Test helpers
// =============================================================================

func newTestAgentInfo(name string, status discovery.AgentStatus) *discovery.AgentInfo {
	return &discovery.AgentInfo{
		Card: &a2a.AgentCard{
			Name:        name,
			Description: "test agent " + name,
		},
		Status:       status,
		RegisteredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func newTestHandler(reg *mockRegistry) *AgentHandler {
	return NewAgentHandler(reg, nil, zap.NewNop())
}

// =============================================================================
// AgentHandler Tests
// =============================================================================

func TestAgentHandler_HandleListAgents_Empty(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)

	handler.HandleListAgents(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	dataMap, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	items, ok := dataMap["items"].([]any)
	require.True(t, ok)
	assert.Empty(t, items)
	assert.Equal(t, float64(0), dataMap["total"])
}

func TestAgentHandler_HandleListAgents_WithAgents(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("agent-1", discovery.AgentStatusOnline)).
		withAgent(newTestAgentInfo("agent-2", discovery.AgentStatusBusy))
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)

	handler.HandleListAgents(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	dataMap, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	items, ok := dataMap["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
	assert.Equal(t, float64(2), dataMap["total"])
}

func TestAgentHandler_HandleGetAgent_Found(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("test-id", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/test-id", nil)

	handler.HandleGetAgent(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	dataBytes, err := json.Marshal(resp.Data)
	require.NoError(t, err)
	var info AgentInfo
	err = json.Unmarshal(dataBytes, &info)
	require.NoError(t, err)
	assert.Equal(t, "test-id", info.ID)
	assert.Equal(t, "online", info.State)
}

func TestAgentHandler_HandleGetAgent_NotFound(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/nonexistent", nil)

	handler.HandleGetAgent(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "MODEL_NOT_FOUND", resp.Error.Code)
}

func TestAgentHandler_HandleExecuteAgent_MissingBody(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", nil)

	handler.HandleExecuteAgent(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentHandler_HandleExecuteAgent_AgentNotFound(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "nonexistent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecuteAgent(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAgentHandler_HandleExecuteAgent_LocalAgent(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("local-agent", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "local-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecuteAgent(w, r)

	// Local agent execution returns 501 (not yet wired)
	assert.Equal(t, http.StatusNotImplemented, w.Code)
}

func TestAgentHandler_HandleAgentHealth_Online(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("healthy-agent", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/health?id=healthy-agent", nil)

	handler.HandleAgentHealth(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}

func TestAgentHandler_HandleAgentHealth_Unhealthy(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("sick-agent", discovery.AgentStatusUnhealthy))
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/health?id=sick-agent", nil)

	handler.HandleAgentHealth(w, r)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestAgentHandler_HandleAgentHealth_NotFound(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/health?id=nonexistent", nil)

	handler.HandleAgentHealth(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAgentHandler_HandleAgentHealth_MissingID(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/health", nil)

	handler.HandleAgentHealth(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentHandler_HandleAgentError(t *testing.T) {
	handler := newTestHandler(newMockRegistry())

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "types.Error",
			err:            types.NewError(types.ErrInvalidRequest, "invalid input"),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "INVALID_REQUEST",
		},
		{
			name:           "generic error",
			err:            assert.AnError,
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.handleAgentError(w, tt.err)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp Response
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err)

			assert.False(t, resp.Success)
			assert.NotNil(t, resp.Error)
			assert.Equal(t, tt.expectedCode, resp.Error.Code)
		})
	}
}

// =============================================================================
// AgentID Validation Tests
// =============================================================================

func TestAgentHandler_HandleExecuteAgent_InvalidAgentID(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	tests := []struct {
		name    string
		agentID string
	}{
		{"SQL injection", "agent'; DROP TABLE--"},
		{"path traversal", "../../../etc/passwd"},
		{"too long", string(make([]byte, 200))},
		{"special chars", "agent<script>alert(1)</script>"},
		{"starts with dot", ".hidden-agent"},
		{"empty after trim", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(usecase.AgentExecuteRequest{
				AgentID: tt.agentID,
				Content: "hello",
			})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")

			handler.HandleExecuteAgent(w, r)

			assert.Equal(t, http.StatusBadRequest, w.Code, "agentID=%q should be rejected", tt.agentID)
		})
	}
}

func TestAgentHandler_HandleExecuteAgent_ValidAgentID(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("valid-agent-1", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "valid-agent-1",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecuteAgent(w, r)

	// Should pass validation (501 = not yet implemented, but not 400)
	assert.NotEqual(t, http.StatusBadRequest, w.Code)
}

func TestAgentHandler_HandleExecuteAgent_MultiAgentValidation(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	tests := []struct {
		name string
		req  usecase.AgentExecuteRequest
	}{
		{
			name: "too many agent ids",
			req: usecase.AgentExecuteRequest{
				AgentIDs: []string{"a1", "a2", "a3", "a4", "a5", "a6"},
				Content:  "hello",
			},
		},
		{
			name: "mixed agent_id and agent_ids",
			req: usecase.AgentExecuteRequest{
				AgentID:  "a1",
				AgentIDs: []string{"a2", "a3"},
				Content:  "hello",
			},
		},
		{
			name: "invalid mode",
			req: usecase.AgentExecuteRequest{
				AgentIDs: []string{"a1", "a2"},
				Content:  "hello",
				Mode:     "fanout",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.req)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")

			handler.HandleExecuteAgent(w, r)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestAgentHandler_HandleExecuteAgent_MultiAgentValid(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("agent-1", discovery.AgentStatusOnline)).
		withAgent(newTestAgentInfo("agent-2", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentIDs: []string{"agent-1", "agent-2"},
		Content:  "hello",
		Mode:     "parallel",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleExecuteAgent(w, r)

	assert.NotEqual(t, http.StatusBadRequest, w.Code)
}

func TestAgentHandler_HandleAgentStream_EmbedsExecutionFieldsInPayload(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("stream-agent", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)
	handler.service = &stubAgentService{
		resolveForOperationFn: func(ctx context.Context, agentID string, op usecase.AgentOperation) (agent.Agent, *types.Error) {
			return nil, nil
		},
		executeAgentStreamFn: func(ctx context.Context, req usecase.AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error {
			emitter(agent.RuntimeStreamEvent{
				Type:  agent.RuntimeStreamToken,
				Delta: "hello",
			})
			return nil
		},
	}

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "stream-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	lines := strings.Split(w.Body.String(), "\n")
	var tokenPayload map[string]any
	for i := 0; i < len(lines); i++ {
		if lines[i] == "event: token" && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
			err := json.Unmarshal([]byte(strings.TrimPrefix(lines[i+1], "data: ")), &tokenPayload)
			require.NoError(t, err)
			break
		}
	}
	require.NotNil(t, tokenPayload)
	assert.Equal(t, "hello", tokenPayload["content"])
	assert.Contains(t, tokenPayload, "current_stage")
	assert.Contains(t, tokenPayload, "iteration_count")
	assert.Contains(t, tokenPayload, "selected_reasoning_mode")
	assert.Contains(t, tokenPayload, "stop_reason")
	assert.Contains(t, tokenPayload, "checkpoint_id")
	assert.Contains(t, tokenPayload, "resumable")
	assert.Equal(t, "", tokenPayload["current_stage"])
	assert.Equal(t, float64(0), tokenPayload["iteration_count"])
	assert.Equal(t, false, tokenPayload["resumable"])
}

func TestAgentHandler_HandleAgentStream_EmitsStatusEventsWithStableExecutionFields(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("stream-agent", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)
	handler.service = &stubAgentService{
		resolveForOperationFn: func(ctx context.Context, agentID string, op usecase.AgentOperation) (agent.Agent, *types.Error) {
			return nil, nil
		},
		executeAgentStreamFn: func(ctx context.Context, req usecase.AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error {
			emitter(agent.RuntimeStreamEvent{
				Type:           agent.RuntimeStreamStatus,
				CurrentStage:   "evaluate",
				IterationCount: 2,
				SelectedMode:   "react",
				StopReason:     "solved",
				Data: map[string]any{
					"status":   "completion_judge_decision",
					"decision": "done",
				},
			})
			emitter(agent.RuntimeStreamEvent{
				Type:           agent.RuntimeStreamToken,
				Delta:          "hello",
				CurrentStage:   "reasoning",
				IterationCount: 2,
				SelectedMode:   "react",
				StopReason:     "solved",
			})
			return nil
		},
	}

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "stream-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	lines := strings.Split(w.Body.String(), "\n")
	var statusPayload map[string]any
	var tokenPayload map[string]any
	for i := 0; i < len(lines); i++ {
		if lines[i] == "event: status" && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
			err := json.Unmarshal([]byte(strings.TrimPrefix(lines[i+1], "data: ")), &statusPayload)
			require.NoError(t, err)
		}
		if lines[i] == "event: token" && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
			err := json.Unmarshal([]byte(strings.TrimPrefix(lines[i+1], "data: ")), &tokenPayload)
			require.NoError(t, err)
		}
	}
	require.NotNil(t, statusPayload)
	require.NotNil(t, tokenPayload)
	assert.Equal(t, "completion_judge_decision", statusPayload["status"])
	assert.Equal(t, "evaluate", statusPayload["current_stage"])
	assert.Equal(t, float64(2), statusPayload["iteration_count"])
	assert.Equal(t, "react", statusPayload["selected_reasoning_mode"])
	assert.Equal(t, "solved", statusPayload["stop_reason"])
	assert.Equal(t, "done", statusPayload["decision"])

	assert.Equal(t, "hello", tokenPayload["content"])
	assert.Equal(t, "reasoning", tokenPayload["current_stage"])
	assert.Equal(t, float64(2), tokenPayload["iteration_count"])
	assert.Equal(t, "react", tokenPayload["selected_reasoning_mode"])
	assert.Equal(t, "solved", tokenPayload["stop_reason"])
}

func TestAgentHandler_HandleAgentStream_StatusEventPayload(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("stream-agent", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)
	handler.service = &stubAgentService{
		resolveForOperationFn: func(ctx context.Context, agentID string, op usecase.AgentOperation) (agent.Agent, *types.Error) {
			return nil, nil
		},
		executeAgentStreamFn: func(ctx context.Context, req usecase.AgentExecuteRequest, traceID string, emitter agent.RuntimeStreamEmitter) *types.Error {
			emitter(agent.RuntimeStreamEvent{
				Type:           agent.RuntimeStreamStatus,
				CurrentStage:   "evaluate",
				IterationCount: 2,
				SelectedMode:   "dynamic_planner",
				StopReason:     "blocked",
				CheckpointID:   "cp-2",
				Resumable:      true,
				Data: map[string]any{
					"status":   "loop_stopped",
					"decision": "replan",
				},
			})
			return nil
		},
	}

	body, _ := json.Marshal(usecase.AgentExecuteRequest{
		AgentID: "stream-agent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute/stream", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleAgentStream(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	lines := strings.Split(w.Body.String(), "\n")
	var statusPayload map[string]any
	for i := 0; i < len(lines); i++ {
		if lines[i] == "event: status" && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
			err := json.Unmarshal([]byte(strings.TrimPrefix(lines[i+1], "data: ")), &statusPayload)
			require.NoError(t, err)
			break
		}
	}
	require.NotNil(t, statusPayload)
	assert.Equal(t, "loop_stopped", statusPayload["status"])
	assert.Equal(t, "evaluate", statusPayload["current_stage"])
	assert.Equal(t, float64(2), statusPayload["iteration_count"])
	assert.Equal(t, "dynamic_planner", statusPayload["selected_reasoning_mode"])
	assert.Equal(t, "blocked", statusPayload["stop_reason"])
	assert.Equal(t, "cp-2", statusPayload["checkpoint_id"])
	assert.Equal(t, true, statusPayload["resumable"])
}

func TestAgentHandler_HandleExecuteAgent_InvalidRoutingParams(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	tests := []struct {
		name string
		req  usecase.AgentExecuteRequest
	}{
		{
			name: "invalid provider",
			req: usecase.AgentExecuteRequest{
				AgentID:  "agent-1",
				Content:  "hello",
				Provider: "bad/provider",
			},
		},
		{
			name: "invalid route policy",
			req: usecase.AgentExecuteRequest{
				AgentID:     "agent-1",
				Content:     "hello",
				RoutePolicy: "fastest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.req)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")

			handler.HandleExecuteAgent(w, r)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestAgentHandler_HandleCapabilities(t *testing.T) {
	handler := newTestHandler(newMockRegistry())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capabilities", nil)

	handler.HandleCapabilities(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}
