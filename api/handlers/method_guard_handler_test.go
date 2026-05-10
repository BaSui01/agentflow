package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestAPIKeyHandler_MethodGuardSetsAllowHeader(t *testing.T) {
	h := NewAPIKeyHandler(nil, zap.NewNop())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/providers/1/api-keys", nil)

	h.HandleCreateAPIKey(w, r)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, http.MethodPost, w.Header().Get("Allow"))
}
