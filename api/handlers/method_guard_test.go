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

func TestRequireMethod(t *testing.T) {
	logger := zap.NewNop()

	t.Run("matching method passes through", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)

		ok := requireMethod(w, r, http.MethodGet, logger)

		assert.True(t, ok)
		assert.Equal(t, "", w.Header().Get("Allow"))
		assert.Equal(t, 200, w.Code)
		assert.Equal(t, 0, w.Body.Len())
	})

	t.Run("mismatched method writes 405 and allow header", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)

		ok := requireMethod(w, r, http.MethodGet, logger)

		assert.False(t, ok)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, http.MethodGet, w.Header().Get("Allow"))

		var resp Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		assert.False(t, resp.Success)
		assert.Equal(t, string(types.ErrInvalidRequest), resp.Error.Code)
		assert.Equal(t, "method not allowed", resp.Error.Message)
	})
}
