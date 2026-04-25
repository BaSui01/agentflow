package routes

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestCompatibilityEndpointRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterChat(mux, handlers.NewChatHandler(nil, zap.NewNop()), zap.NewNop())

	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "openai chat completions",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5.2","messages":[{"role":"user","content":"hi"}]}`,
		},
		{
			name: "openai responses",
			path: "/v1/responses",
			body: `{"model":"gpt-5.2","input":"hi"}`,
		},
		{
			name: "anthropic messages",
			path: "/v1/messages",
			body: `{"model":"claude-sonnet-4-20250514","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			assert.NotEqual(t, http.StatusNotFound, rec.Code)
		})
	}
}
