package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// 🧪 AgentHandler 测试
// =============================================================================

func TestAgentHandler_HandleListAgents(t *testing.T) {
	logger := zap.NewNop()
	handler := NewAgentHandler(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)

	handler.HandleListAgents(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)

	// 当前返回空列表
	dataBytes, err := json.Marshal(resp.Data)
	require.NoError(t, err)

	var agents []AgentInfo
	err = json.Unmarshal(dataBytes, &agents)
	require.NoError(t, err)

	assert.Empty(t, agents)
}

func TestAgentHandler_HandleGetAgent(t *testing.T) {
	logger := zap.NewNop()
	handler := NewAgentHandler(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/test-id", nil)

	handler.HandleGetAgent(w, r)

	// 当前实现返回 404
	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "MODEL_NOT_FOUND", resp.Error.Code)
}

func TestAgentHandler_HandleExecuteAgent(t *testing.T) {
	logger := zap.NewNop()
	handler := NewAgentHandler(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/execute", nil)

	handler.HandleExecuteAgent(w, r)

	// 当前实现返回 500 (not implemented)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

func TestAgentHandler_HandlePlanAgent(t *testing.T) {
	logger := zap.NewNop()
	handler := NewAgentHandler(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/agents/plan", nil)

	handler.HandlePlanAgent(w, r)

	// 当前实现返回 500 (not implemented)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

func TestAgentHandler_HandleAgentHealth(t *testing.T) {
	logger := zap.NewNop()
	handler := NewAgentHandler(logger)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/agents/health?id=test-id", nil)

	handler.HandleAgentHealth(w, r)

	// 当前实现返回 404
	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp Response
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)

	assert.False(t, resp.Success)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "MODEL_NOT_FOUND", resp.Error.Code)
}

func TestAgentHandler_HandleAgentError(t *testing.T) {
	logger := zap.NewNop()
	handler := NewAgentHandler(logger)

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
