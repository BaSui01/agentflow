package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
)

func TestWriteSSEErrorEvent_UsesCanonicalEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	errInfo := api.ErrorInfoFromTypesError(
		types.NewError(types.ErrInternalError, "stream broke"),
		http.StatusInternalServerError,
	)

	require.NoError(t, writeSSEErrorEvent(rec, errInfo, "req-sse-1"))

	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, "event: error\n"), body)
	require.True(t, strings.HasSuffix(body, "\n\n"), body)

	dataLine := strings.TrimPrefix(strings.TrimSuffix(body, "\n\n"), "event: error\ndata: ")
	var payload struct {
		Error     api.ErrorInfo `json:"error"`
		RequestID string        `json:"request_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(dataLine), &payload))
	require.Equal(t, "INTERNAL_ERROR", payload.Error.Code)
	require.Equal(t, "stream broke", payload.Error.Message)
	require.Equal(t, "req-sse-1", payload.RequestID)
}
