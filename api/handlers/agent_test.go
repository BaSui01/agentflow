package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
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

	dataBytes, err := json.Marshal(resp.Data)
	require.NoError(t, err)
	var agents []AgentInfo
	err = json.Unmarshal(dataBytes, &agents)
	require.NoError(t, err)
	assert.Empty(t, agents)
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

	dataBytes, err := json.Marshal(resp.Data)
	require.NoError(t, err)
	var agents []AgentInfo
	err = json.Unmarshal(dataBytes, &agents)
	require.NoError(t, err)
	assert.Len(t, agents, 2)
}

func TestAgentHandler_HandleGetAgent_Found(t *testing.T) {
	reg := newMockRegistry().
		withAgent(newTestAgentInfo("test-id", discovery.AgentStatusOnline))
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/test-id", nil)

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
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/nonexistent", nil)

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

	body, _ := json.Marshal(AgentExecuteRequest{
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

	body, _ := json.Marshal(AgentExecuteRequest{
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

func TestAgentHandler_HandlePlanAgent_MissingBody(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/plan", nil)

	handler.HandlePlanAgent(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentHandler_HandlePlanAgent_AgentNotFound(t *testing.T) {
	reg := newMockRegistry()
	handler := newTestHandler(reg)

	body, _ := json.Marshal(AgentExecuteRequest{
		AgentID: "nonexistent",
		Content: "hello",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/plan", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandlePlanAgent(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)
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
