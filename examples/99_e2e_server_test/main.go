// Package main implements an E2E integration test that starts a full HTTP server
// with mock providers and exercises the key API endpoints without requiring
// real API keys.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// =========================================================================
// mock ChatService
// =========================================================================

type mockChatService struct{}

func (m *mockChatService) Complete(_ context.Context, req *usecase.ChatRequest) (*usecase.ChatCompletionResult, *types.Error) {
	// If the request carries tools, return a tool_calls response.
	if len(req.Tools) > 0 {
		return &usecase.ChatCompletionResult{
			Response: &usecase.ChatResponse{
				ID:       "chatcmpl-tool-001",
				Provider: "mock",
				Model:    req.Model,
				Choices: []usecase.ChatChoice{
					{
						Index:        0,
						FinishReason: "tool_calls",
						Message: usecase.Message{
							Role:    "assistant",
							Content: "",
							ToolCalls: []types.ToolCall{
								{
									ID:        "call_abc123",
									Name:      req.Tools[0].Name,
									Arguments: json.RawMessage(`{"location":"Beijing"}`),
								},
							},
						},
					},
				},
				Usage:     usecase.ChatUsage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
				CreatedAt: time.Now(),
			},
			Raw: &llmcore.ChatResponse{
				ID:    "chatcmpl-tool-001",
				Model: req.Model,
				Usage: llmcore.ChatUsage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
			},
			Duration: 30 * time.Millisecond,
		}, nil
	}

	return &usecase.ChatCompletionResult{
		Response: &usecase.ChatResponse{
			ID:       "chatcmpl-mock-001",
			Provider: "mock",
			Model:    req.Model,
			Choices: []usecase.ChatChoice{
				{
					Index:        0,
					FinishReason: "stop",
					Message:      usecase.Message{Role: "assistant", Content: "Hello from mock provider!"},
				},
			},
			Usage:     usecase.ChatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			CreatedAt: time.Now(),
		},
		Raw: &llmcore.ChatResponse{
			ID:    "chatcmpl-mock-001",
			Model: req.Model,
			Usage: llmcore.ChatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
		Duration: 50 * time.Millisecond,
	}, nil
}

func (m *mockChatService) Stream(_ context.Context, req *usecase.ChatRequest) (<-chan usecase.ChatStreamEvent, *types.Error) {
	ch := make(chan usecase.ChatStreamEvent, 4)
	go func() {
		defer close(ch)
		// Send two content chunks then a finish chunk.
		for _, text := range []string{"Hello", " world"} {
			ch <- usecase.ChatStreamEvent{
				Chunk: &usecase.ChatStreamChunk{
					ID:    "chatcmpl-stream-001",
					Model: req.Model,
					Delta: usecase.Message{Role: string(types.RoleAssistant), Content: text},
				},
			}
		}
		ch <- usecase.ChatStreamEvent{
			Chunk: &usecase.ChatStreamChunk{
				ID:           "chatcmpl-stream-001",
				Model:        req.Model,
				Delta:        usecase.Message{Role: string(types.RoleAssistant)},
				FinishReason: "stop",
			},
		}
	}()
	return ch, nil
}

func (m *mockChatService) SupportedRoutePolicies() []string { return []string{"balanced"} }
func (m *mockChatService) DefaultRoutePolicy() string       { return "balanced" }

// =========================================================================
// server setup
// =========================================================================

func buildServer() *httptest.Server {
	logger := zap.NewNop()
	svc := &mockChatService{}
	chatHandler, err := handlers.NewChatHandler(svc, logger)
	if err != nil {
		panic(err)
	}
	healthHandler := handlers.NewHealthHandler(logger)

	mux := http.NewServeMux()
	routes.RegisterSystem(mux, healthHandler, "e2e-test", "2026-03-22", "e2eabc")
	routes.RegisterChat(mux, chatHandler, logger)
	return httptest.NewServer(mux)
}

// =========================================================================
// test infrastructure
// =========================================================================

type testResult struct {
	name   string
	passed bool
	detail string
}

func (r testResult) String() string {
	mark := "PASS"
	if !r.passed {
		mark = "FAIL"
	}
	s := fmt.Sprintf("[%s] %s", mark, r.name)
	if r.detail != "" {
		s += " — " + r.detail
	}
	return s
}

func doJSON(method, url string, body any) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp, b, err
}

// =========================================================================
// individual tests
// =========================================================================

func testHealthEndpoint(base string) testResult {
	name := "GET /health"
	resp, body, err := doJSON("GET", base+"/health", nil)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		return testResult{name, false, fmt.Sprintf("status=%d, body=%s", resp.StatusCode, body)}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return testResult{name, false, fmt.Sprintf("Content-Type=%q", ct)}
	}
	var envelope api.Response
	if err := json.Unmarshal(body, &envelope); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if !envelope.Success {
		return testResult{name, false, "success=false"}
	}
	dataMap, ok := envelope.Data.(map[string]any)
	if !ok {
		return testResult{name, false, fmt.Sprintf("data type=%T", envelope.Data)}
	}
	if status, _ := dataMap["status"].(string); status != "healthy" {
		return testResult{name, false, fmt.Sprintf("status=%q", status)}
	}
	return testResult{name, true, ""}
}

func testChatCompletions(base string) testResult {
	name := "POST /api/v1/chat/completions (sync)"
	payload := map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "Hi"}},
	}
	resp, body, err := doJSON("POST", base+"/api/v1/chat/completions", payload)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		return testResult{name, false, fmt.Sprintf("status=%d, body=%s", resp.StatusCode, body)}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return testResult{name, false, fmt.Sprintf("Content-Type=%q", ct)}
	}
	var envelope api.Response
	if err := json.Unmarshal(body, &envelope); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if !envelope.Success {
		return testResult{name, false, fmt.Sprintf("success=false, error=%+v", envelope.Error)}
	}
	dataBytes, _ := json.Marshal(envelope.Data)
	var chatResp api.ChatResponse
	if err := json.Unmarshal(dataBytes, &chatResp); err != nil {
		return testResult{name, false, fmt.Sprintf("ChatResponse decode: %v", err)}
	}
	if chatResp.Model != "gpt-4" {
		return testResult{name, false, fmt.Sprintf("model=%q", chatResp.Model)}
	}
	if len(chatResp.Choices) == 0 {
		return testResult{name, false, "no choices"}
	}
	if chatResp.Choices[0].Message.Role != "assistant" {
		return testResult{name, false, fmt.Sprintf("role=%q", chatResp.Choices[0].Message.Role)}
	}
	if chatResp.Choices[0].Message.Content != "Hello from mock provider!" {
		return testResult{name, false, fmt.Sprintf("content=%q", chatResp.Choices[0].Message.Content)}
	}
	return testResult{name, true, ""}
}

func testOpenAICompatEndpoint(base string) testResult {
	name := "POST /v1/chat/completions (OpenAI compat)"
	payload := map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
	}
	resp, body, err := doJSON("POST", base+"/v1/chat/completions", payload)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		return testResult{name, false, fmt.Sprintf("status=%d, body=%s", resp.StatusCode, body)}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return testResult{name, false, fmt.Sprintf("Content-Type=%q", ct)}
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
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if chatResp.Object != "chat.completion" {
		return testResult{name, false, fmt.Sprintf("object=%q", chatResp.Object)}
	}
	if chatResp.Model != "gpt-4" {
		return testResult{name, false, fmt.Sprintf("model=%q", chatResp.Model)}
	}
	if len(chatResp.Choices) == 0 {
		return testResult{name, false, "no choices"}
	}
	if chatResp.Choices[0].Message.Role != "assistant" {
		return testResult{name, false, fmt.Sprintf("role=%q", chatResp.Choices[0].Message.Role)}
	}
	if chatResp.Choices[0].Message.Content != "Hello from mock provider!" {
		return testResult{name, false, fmt.Sprintf("content=%q", chatResp.Choices[0].Message.Content)}
	}
	if chatResp.Usage.TotalTokens != 15 {
		return testResult{name, false, fmt.Sprintf("total_tokens=%d", chatResp.Usage.TotalTokens)}
	}
	return testResult{name, true, ""}
}

func testChatCompletionsWithTools(base string) testResult {
	name := "POST /api/v1/chat/completions (with tools)"
	payload := map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "What is the weather in Beijing?"}},
		"tools": []map[string]any{
			{
				"name":        "get_weather",
				"description": "Get current weather for a location",
				"parameters":  map[string]any{"type": "object", "properties": map[string]any{"location": map[string]string{"type": "string"}}},
			},
		},
	}
	resp, body, err := doJSON("POST", base+"/api/v1/chat/completions", payload)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		return testResult{name, false, fmt.Sprintf("status=%d, body=%s", resp.StatusCode, body)}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return testResult{name, false, fmt.Sprintf("Content-Type=%q", ct)}
	}
	var envelope api.Response
	if err := json.Unmarshal(body, &envelope); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if !envelope.Success {
		return testResult{name, false, fmt.Sprintf("success=false, error=%+v", envelope.Error)}
	}
	dataBytes, _ := json.Marshal(envelope.Data)
	var chatResp api.ChatResponse
	if err := json.Unmarshal(dataBytes, &chatResp); err != nil {
		return testResult{name, false, fmt.Sprintf("ChatResponse decode: %v", err)}
	}
	if len(chatResp.Choices) == 0 {
		return testResult{name, false, "no choices"}
	}
	choice := chatResp.Choices[0]
	if choice.FinishReason != "tool_calls" {
		return testResult{name, false, fmt.Sprintf("finish_reason=%q, want tool_calls", choice.FinishReason)}
	}
	if len(choice.Message.ToolCalls) == 0 {
		return testResult{name, false, "no tool_calls in response"}
	}
	tc := choice.Message.ToolCalls[0]
	if tc.Name != "get_weather" {
		return testResult{name, false, fmt.Sprintf("tool name=%q", tc.Name)}
	}
	if tc.ID != "call_abc123" {
		return testResult{name, false, fmt.Sprintf("tool call id=%q", tc.ID)}
	}
	var args map[string]string
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		return testResult{name, false, fmt.Sprintf("tool args decode: %v", err)}
	}
	if args["location"] != "Beijing" {
		return testResult{name, false, fmt.Sprintf("tool args location=%q", args["location"])}
	}
	return testResult{name, true, ""}
}

func testOpenAICompatWithTools(base string) testResult {
	name := "POST /v1/chat/completions (OpenAI compat + tools)"
	payload := map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "What is the weather?"}},
		"tools": []map[string]any{
			{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get weather",
					"parameters":  map[string]any{"type": "object", "properties": map[string]any{"location": map[string]string{"type": "string"}}},
				},
			},
		},
	}
	resp, body, err := doJSON("POST", base+"/v1/chat/completions", payload)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		return testResult{name, false, fmt.Sprintf("status=%d, body=%s", resp.StatusCode, body)}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		return testResult{name, false, fmt.Sprintf("Content-Type=%q", ct)}
	}

	var chatResp struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int    `json:"index"`
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if chatResp.Object != "chat.completion" {
		return testResult{name, false, fmt.Sprintf("object=%q", chatResp.Object)}
	}
	if len(chatResp.Choices) == 0 {
		return testResult{name, false, "no choices"}
	}
	choice := chatResp.Choices[0]
	if choice.FinishReason != "tool_calls" {
		return testResult{name, false, fmt.Sprintf("finish_reason=%q", choice.FinishReason)}
	}
	if len(choice.Message.ToolCalls) == 0 {
		return testResult{name, false, "no tool_calls"}
	}
	tc := choice.Message.ToolCalls[0]
	if tc.Function.Name != "get_weather" {
		return testResult{name, false, fmt.Sprintf("function.name=%q", tc.Function.Name)}
	}
	if tc.Type != "function" {
		return testResult{name, false, fmt.Sprintf("type=%q", tc.Type)}
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return testResult{name, false, fmt.Sprintf("args decode: %v", err)}
	}
	if args["location"] != "Beijing" {
		return testResult{name, false, fmt.Sprintf("location=%q", args["location"])}
	}
	return testResult{name, true, ""}
}

func testVersionEndpoint(base string) testResult {
	name := "GET /version"
	resp, body, err := doJSON("GET", base+"/version", nil)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		return testResult{name, false, fmt.Sprintf("status=%d", resp.StatusCode)}
	}
	var envelope api.Response
	if err := json.Unmarshal(body, &envelope); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if !envelope.Success {
		return testResult{name, false, "success=false"}
	}
	dataMap, ok := envelope.Data.(map[string]any)
	if !ok {
		return testResult{name, false, fmt.Sprintf("data type=%T", envelope.Data)}
	}
	if v, _ := dataMap["version"].(string); v != "e2e-test" {
		return testResult{name, false, fmt.Sprintf("version=%q", v)}
	}
	return testResult{name, true, ""}
}

// =========================================================================
// streaming SSE tests
// =========================================================================

// readSSELines reads the full SSE body and returns non-empty lines.
func readSSELines(resp *http.Response) ([]string, error) {
	defer resp.Body.Close()
	var lines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func testStreamSSE(base string) testResult {
	name := "POST /api/v1/chat/completions/stream (SSE format)"
	payload, _ := json.Marshal(map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "Hi"}},
	})
	req, err := http.NewRequest("POST", base+"/api/v1/chat/completions/stream", bytes.NewReader(payload))
	if err != nil {
		return testResult{name, false, fmt.Sprintf("new request: %v", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return testResult{name, false, fmt.Sprintf("status=%d", resp.StatusCode)}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		resp.Body.Close()
		return testResult{name, false, fmt.Sprintf("Content-Type=%q", ct)}
	}

	lines, err := readSSELines(resp)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("read body: %v", err)}
	}

	// Verify at least one "data: " prefixed line and a "data: [DONE]" terminator.
	hasDataPrefix := false
	hasDone := false
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
			hasDataPrefix = true
		}
		if line == "data: [DONE]" {
			hasDone = true
		}
	}
	if !hasDataPrefix {
		return testResult{name, false, fmt.Sprintf("no data: prefix lines found, lines=%v", lines)}
	}
	if !hasDone {
		return testResult{name, false, "missing data: [DONE] terminator"}
	}
	return testResult{name, true, fmt.Sprintf("%d SSE lines", len(lines))}
}

func testOpenAICompatStreamSSE(base string) testResult {
	name := "POST /v1/chat/completions stream=true (OpenAI compat SSE)"
	payload, _ := json.Marshal(map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "Hi"}},
		"stream":   true,
	})
	req, err := http.NewRequest("POST", base+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return testResult{name, false, fmt.Sprintf("new request: %v", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return testResult{name, false, fmt.Sprintf("status=%d", resp.StatusCode)}
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		resp.Body.Close()
		return testResult{name, false, fmt.Sprintf("Content-Type=%q", ct)}
	}

	lines, err := readSSELines(resp)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("read body: %v", err)}
	}

	hasChunk := false
	hasDone := false
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
			hasChunk = true
			// Verify each data line is valid JSON with object=chat.completion.chunk
			jsonStr := strings.TrimPrefix(line, "data: ")
			var chunk map[string]any
			if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
				return testResult{name, false, fmt.Sprintf("chunk JSON decode: %v, raw=%q", err, jsonStr)}
			}
			if obj, _ := chunk["object"].(string); obj != "chat.completion.chunk" {
				return testResult{name, false, fmt.Sprintf("chunk object=%q", obj)}
			}
		}
		if line == "data: [DONE]" {
			hasDone = true
		}
	}
	if !hasChunk {
		return testResult{name, false, "no data chunks found"}
	}
	if !hasDone {
		return testResult{name, false, "missing data: [DONE] terminator"}
	}
	return testResult{name, true, fmt.Sprintf("%d SSE lines", len(lines))}
}

// =========================================================================
// error scenario tests
// =========================================================================

func testEmptyBody(base string) testResult {
	name := "POST /api/v1/chat/completions (empty body → 400)"
	req, _ := http.NewRequest("POST", base+"/api/v1/chat/completions", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		body, _ := io.ReadAll(resp.Body)
		return testResult{name, false, fmt.Sprintf("status=%d, body=%s", resp.StatusCode, body)}
	}
	body, _ := io.ReadAll(resp.Body)
	var envelope api.Response
	if err := json.Unmarshal(body, &envelope); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if envelope.Success {
		return testResult{name, false, "expected success=false"}
	}
	return testResult{name, true, ""}
}

func testInvalidJSON(base string) testResult {
	name := "POST /api/v1/chat/completions (invalid JSON → 400)"
	req, _ := http.NewRequest("POST", base+"/api/v1/chat/completions", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		body, _ := io.ReadAll(resp.Body)
		return testResult{name, false, fmt.Sprintf("status=%d, body=%s", resp.StatusCode, body)}
	}
	body, _ := io.ReadAll(resp.Body)
	var envelope api.Response
	if err := json.Unmarshal(body, &envelope); err != nil {
		return testResult{name, false, fmt.Sprintf("JSON decode: %v", err)}
	}
	if envelope.Success {
		return testResult{name, false, "expected success=false"}
	}
	return testResult{name, true, ""}
}

func testNotFound(base string) testResult {
	name := "GET /nonexistent (→ 404)"
	resp, _, err := doJSON("GET", base+"/nonexistent", nil)
	if err != nil {
		return testResult{name, false, fmt.Sprintf("request error: %v", err)}
	}
	if resp.StatusCode != 404 {
		return testResult{name, false, fmt.Sprintf("status=%d", resp.StatusCode)}
	}
	return testResult{name, true, ""}
}

// =========================================================================
// concurrency stress test
// =========================================================================

func testConcurrentRequests(base string) testResult {
	name := "Concurrent 10x POST /api/v1/chat/completions"
	const concurrency = 10
	payload := map[string]any{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "Hi"}},
	}

	var wg sync.WaitGroup
	type result struct {
		status   int
		duration time.Duration
		err      error
	}
	results := make([]result, concurrency)

	start := time.Now()
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			t0 := time.Now()
			resp, _, err := doJSON("POST", base+"/api/v1/chat/completions", payload)
			d := time.Since(t0)
			if err != nil {
				results[idx] = result{err: err, duration: d}
				return
			}
			results[idx] = result{status: resp.StatusCode, duration: d}
		}(i)
	}
	wg.Wait()
	totalDuration := time.Since(start)

	var totalLatency time.Duration
	for i, r := range results {
		if r.err != nil {
			return testResult{name, false, fmt.Sprintf("request[%d] error: %v", i, r.err)}
		}
		if r.status != 200 {
			return testResult{name, false, fmt.Sprintf("request[%d] status=%d", i, r.status)}
		}
		totalLatency += r.duration
	}
	avgLatency := totalLatency / concurrency
	detail := fmt.Sprintf("total=%v, avg_latency=%v", totalDuration, avgLatency)
	return testResult{name, true, detail}
}

// =========================================================================
// main
// =========================================================================

func main() {
	srv := buildServer()
	defer srv.Close()
	base := srv.URL

	fmt.Println("=== E2E Server Integration Tests ===")
	fmt.Printf("Server: %s\n\n", base)

	results := []testResult{
		// existing tests
		testHealthEndpoint(base),
		testVersionEndpoint(base),
		testChatCompletions(base),
		testOpenAICompatEndpoint(base),
		testChatCompletionsWithTools(base),
		testOpenAICompatWithTools(base),
		// streaming SSE tests
		testStreamSSE(base),
		testOpenAICompatStreamSSE(base),
		// error scenario tests
		testEmptyBody(base),
		testInvalidJSON(base),
		testNotFound(base),
		// concurrency stress test
		testConcurrentRequests(base),
	}

	passed, failed := 0, 0
	for _, r := range results {
		fmt.Println(r)
		if r.passed {
			passed++
		} else {
			failed++
		}
	}

	fmt.Printf("\n=== Summary: %d passed, %d failed, %d total ===\n", passed, failed, len(results))

	if failed > 0 {
		os.Exit(1)
	}
}
