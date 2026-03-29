package vendor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewChatProviderFromConfig_ProviderContractMatrix(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		model        string
		apiKey       string
		extra        map[string]any
		handler      func(t *testing.T) http.HandlerFunc
		wantName     string
		wantText     string
		wantModel    string
	}{
		{
			name:         "openai",
			providerName: "openai",
			model:        "gpt-5.2",
			apiKey:       "sk-openai",
			extra: map[string]any{
				"organization": "org-test",
			},
			handler: func(t *testing.T) http.HandlerFunc {
				t.Helper()
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/models":
						assert.Equal(t, "Bearer sk-openai", r.Header.Get("Authorization"))
						assert.Equal(t, "org-test", r.Header.Get("OpenAI-Organization"))
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"data":[{"id":"gpt-5.2","object":"model","owned_by":"openai"}]}`)
					case "/v1/chat/completions":
						body, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						assert.Equal(t, "Bearer sk-openai", r.Header.Get("Authorization"))
						assert.Equal(t, "org-test", r.Header.Get("OpenAI-Organization"))
						if strings.Contains(string(body), `"stream":true`) {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-stream\",\"model\":\"gpt-5.2\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello OpenAI\"}}]}\n\n")
							_, _ = io.WriteString(w, "data: [DONE]\n\n")
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"chatcmpl-1","model":"gpt-5.2","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Hello OpenAI"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
					default:
						http.NotFound(w, r)
					}
				}
			},
			wantName:  "openai",
			wantText:  "Hello OpenAI",
			wantModel: "gpt-5.2",
		},
		{
			name:         "anthropic",
			providerName: "anthropic",
			model:        "claude-sonnet-4-20250514",
			apiKey:       "sk-anthropic",
			extra: map[string]any{
				"anthropic_version": "2024-01-01",
			},
			handler: func(t *testing.T) http.HandlerFunc {
				t.Helper()
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/models":
						assert.Equal(t, "sk-anthropic", r.Header.Get("x-api-key"))
						assert.Equal(t, "2024-01-01", r.Header.Get("anthropic-version"))
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"data":[{"id":"claude-sonnet-4-20250514","created_at":"2025-05-14T00:00:00Z","type":"model"}]}`)
					case "/v1/messages":
						body, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						assert.Equal(t, "sk-anthropic", r.Header.Get("x-api-key"))
						assert.Equal(t, "2024-01-01", r.Header.Get("anthropic-version"))
						if strings.Contains(string(body), `"stream":true`) {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream\",\"model\":\"claude-sonnet-4-20250514\"}}\n\n")
							_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello Anthropic\"}}\n\n")
							_, _ = io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n")
							_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"msg_1","role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"Hello Anthropic"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
					default:
						http.NotFound(w, r)
					}
				}
			},
			wantName:  "claude",
			wantText:  "Hello Anthropic",
			wantModel: "claude-sonnet-4-20250514",
		},
		{
			name:         "gemini-vertex",
			providerName: "gemini-vertex",
			model:        "gemini-2.5-flash",
			apiKey:       "oauth-token",
			extra: map[string]any{
				"project_id": "demo-project",
				"region":     "asia-east1",
			},
			handler: func(t *testing.T) http.HandlerFunc {
				t.Helper()
				return func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.Contains(r.URL.Path, ":streamGenerateContent"):
						assert.Equal(t, "Bearer oauth-token", r.Header.Get("Authorization"))
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Hello Gemini"}]}}]}`)
						_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":" Vertex"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
					case strings.Contains(r.URL.Path, ":generateContent"):
						assert.Equal(t, "Bearer oauth-token", r.Header.Get("Authorization"))
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"responseId":"resp-gemini-1","candidates":[{"index":0,"finishReason":"STOP","content":{"role":"model","parts":[{"text":"Hello Gemini Vertex"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
					case strings.Contains(r.URL.Path, "/publishers/google/models"):
						assert.Equal(t, "Bearer oauth-token", r.Header.Get("Authorization"))
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"models":[{"name":"models/gemini-2.5-flash","inputTokenLimit":1048576,"outputTokenLimit":8192,"supportedGenerationMethods":["generateContent"]}]}`)
					default:
						http.NotFound(w, r)
					}
				}
			},
			wantName:  "gemini",
			wantText:  "Hello Gemini Vertex",
			wantModel: "gemini-2.5-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler(t))
			t.Cleanup(server.Close)

			p, err := NewChatProviderFromConfig(tt.providerName, ChatProviderConfig{
				APIKey:  tt.apiKey,
				BaseURL: server.URL,
				Model:   tt.model,
				Extra:   tt.extra,
			}, zap.NewNop())
			require.NoError(t, err)
			require.NotNil(t, p)

			resp, err := p.Completion(context.Background(), &llm.ChatRequest{
				Model:    tt.model,
				Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
			})
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantName, p.Name())
			assert.Equal(t, tt.wantName, resp.Provider)
			assert.Equal(t, tt.wantModel, resp.Model)
			require.Len(t, resp.Choices, 1)
			assert.Equal(t, tt.wantText, resp.Choices[0].Message.Content)

			status, err := p.HealthCheck(context.Background())
			require.NoError(t, err)
			assert.True(t, status.Healthy)

			models, err := p.ListModels(context.Background())
			require.NoError(t, err)
			require.NotEmpty(t, models)
			assert.Equal(t, tt.wantModel, models[0].ID)

			stream, err := p.Stream(context.Background(), &llm.ChatRequest{
				Model:    tt.model,
				Messages: []types.Message{{Role: llm.RoleUser, Content: "Hi"}},
			})
			require.NoError(t, err)

			var content strings.Builder
			for chunk := range stream {
				require.Nil(t, chunk.Err)
				content.WriteString(chunk.Delta.Content)
			}
			assert.Equal(t, tt.wantText, content.String())
		})
	}
}
