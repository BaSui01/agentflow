package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// mock ChatService
// ---------------------------------------------------------------------------

type mockChatService struct {
	completeFunc func(ctx context.Context, req *usecase.ChatRequest) (*usecase.ChatCompletionResult, *types.Error)
	streamFunc   func(ctx context.Context, req *usecase.ChatRequest) (<-chan usecase.ChatStreamEvent, *types.Error)
}

func (m *mockChatService) Complete(ctx context.Context, req *usecase.ChatRequest) (*usecase.ChatCompletionResult, *types.Error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, req)
	}
	return &usecase.ChatCompletionResult{
		Response: &usecase.ChatResponse{
			ID:       "chatcmpl-test-123",
			Provider: "mock",
			Model:    req.Model,
			Choices: []usecase.ChatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message:      usecase.Message{Role: "assistant", Content: "Hello from mock"},
				},
			},
			Usage:     usecase.ChatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			CreatedAt: time.Now(),
		},
		Raw: &llmcore.ChatResponse{
			ID:    "chatcmpl-test-123",
			Model: req.Model,
			Usage: llmcore.ChatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
		Duration: 50 * time.Millisecond,
	}, nil
}

func (m *mockChatService) Stream(ctx context.Context, req *usecase.ChatRequest) (<-chan usecase.ChatStreamEvent, *types.Error) {
	if m.streamFunc != nil {
		return m.streamFunc(ctx, req)
	}
	ch := make(chan usecase.ChatStreamEvent, 1)
	close(ch)
	return ch, nil
}

func (m *mockChatService) SupportedRoutePolicies() []string { return []string{"balanced"} }
func (m *mockChatService) DefaultRoutePolicy() string       { return "balanced" }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestMux(t *testing.T) (*http.ServeMux, *mockChatService) {
	t.Helper()
	logger := zap.NewNop()
	svc := &mockChatService{}
	chatHandler := handlers.NewChatHandler(svc, logger)
	healthHandler := handlers.NewHealthHandler(logger)

	mux := http.NewServeMux()
	routes.RegisterSystem(mux, healthHandler, "test", "now", "abc123")
	routes.RegisterChat(mux, chatHandler, logger)
	return mux, svc
}

func mustDecodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("failed to decode JSON: %v\nbody: %s", err, string(body))
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHealthEndpoint(t *testing.T) {
	mux, _ := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json Content-Type, got %q", ct)
	}

	var envelope api.Response
	mustDecodeJSON(t, readBody(t, resp), &envelope)
	if !envelope.Success {
		t.Fatal("expected success=true")
	}

	dataMap, ok := envelope.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be map, got %T", envelope.Data)
	}
	if status, _ := dataMap["status"].(string); status != "healthy" {
		t.Fatalf("expected status=healthy, got %q", status)
	}
}

func TestChatCompletionsEndpoint(t *testing.T) {
	mux, _ := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hi"}]
	}`
	resp, err := http.Post(srv.URL+"/api/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/chat/completions failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, string(readBody(t, resp)))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var envelope api.Response
	mustDecodeJSON(t, readBody(t, resp), &envelope)
	if !envelope.Success {
		t.Fatalf("expected success=true, error=%+v", envelope.Error)
	}

	// data should contain ChatResponse fields
	dataBytes, _ := json.Marshal(envelope.Data)
	var chatResp api.ChatResponse
	mustDecodeJSON(t, dataBytes, &chatResp)

	if chatResp.Model != "gpt-4" {
		t.Fatalf("expected model=gpt-4, got %q", chatResp.Model)
	}
	if len(chatResp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if chatResp.Choices[0].Message.Role != "assistant" {
		t.Fatalf("expected role=assistant, got %q", chatResp.Choices[0].Message.Role)
	}
	if chatResp.Choices[0].Message.Content != "Hello from mock" {
		t.Fatalf("expected content='Hello from mock', got %q", chatResp.Choices[0].Message.Content)
	}
}

func TestChatCompletionsEndpoint_InvalidRequest(t *testing.T) {
	mux, _ := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// missing model
	body := `{"messages": [{"role": "user", "content": "Hi"}]}`
	resp, err := http.Post(srv.URL+"/api/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected non-200 for missing model")
	}

	var envelope api.Response
	mustDecodeJSON(t, readBody(t, resp), &envelope)
	if envelope.Success {
		t.Fatal("expected success=false")
	}
	if envelope.Error == nil {
		t.Fatal("expected error info")
	}
}

func TestOpenAICompatChatCompletionsEndpoint(t *testing.T) {
	mux, _ := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hello"}]
	}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/chat/completions failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", resp.StatusCode, string(readBody(t, resp)))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var chatResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int    `json:"index"`
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	mustDecodeJSON(t, readBody(t, resp), &chatResp)

	if chatResp.Object != "chat.completion" {
		t.Fatalf("expected object=chat.completion, got %q", chatResp.Object)
	}
	if chatResp.Model != "gpt-4" {
		t.Fatalf("expected model=gpt-4, got %q", chatResp.Model)
	}
	if len(chatResp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if chatResp.Choices[0].Message.Role != "assistant" {
		t.Fatalf("expected role=assistant, got %q", chatResp.Choices[0].Message.Role)
	}
	if chatResp.Choices[0].Message.Content != "Hello from mock" {
		t.Fatalf("expected content='Hello from mock', got %q", chatResp.Choices[0].Message.Content)
	}
	if chatResp.Usage.TotalTokens != 15 {
		t.Fatalf("expected total_tokens=15, got %d", chatResp.Usage.TotalTokens)
	}
}

func TestOpenAICompatChatCompletionsEndpoint_MethodNotAllowed(t *testing.T) {
	mux, _ := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/chat/completions")
	if err != nil {
		t.Fatalf("GET /v1/chat/completions failed: %v", err)
	}
	defer resp.Body.Close()

	// Go 1.22+ ServeMux returns 405 for method mismatch on method-specific patterns
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestOpenAICompatChatCompletionsEndpoint_InvalidJSON(t *testing.T) {
	mux, _ := newTestMux(t)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected non-200 for invalid JSON")
	}

	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	mustDecodeJSON(t, readBody(t, resp), &errResp)
	if errResp.Error.Type != "invalid_request_error" {
		t.Fatalf("expected type=invalid_request_error, got %q", errResp.Error.Type)
	}
}

func TestChatCompletionsEndpoint_ServiceError(t *testing.T) {
	mux, svc := newTestMux(t)
	svc.completeFunc = func(ctx context.Context, req *usecase.ChatRequest) (*usecase.ChatCompletionResult, *types.Error) {
		return nil, types.NewInternalError("mock service failure")
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}]}`
	resp, err := http.Post(srv.URL+"/api/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected non-200 for service error")
	}

	var envelope api.Response
	mustDecodeJSON(t, readBody(t, resp), &envelope)
	if envelope.Success {
		t.Fatal("expected success=false")
	}
}

// ---------------------------------------------------------------------------
// readBody helper
// ---------------------------------------------------------------------------

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	buf := new(strings.Builder)
	if _, err := strings.NewReader("").WriteTo(buf); err != nil {
		t.Fatal(err)
	}
	var b []byte
	b = make([]byte, 0, 4096)
	tmp := make([]byte, 512)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			b = append(b, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return b
}
