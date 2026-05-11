package providerbase

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func TestNormalizeFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"STOP", "stop"},
		{"stop", "stop"},
		{"COMPLETED", "stop"},
		{"completed", "stop"},
		{"CANCELLED", "stop"},
		{"MAX_TOKENS", "length"},
		{"INCOMPLETE", "length"},
		{"LENGTH", "length"},
		{"TOOL_CALLS", "tool_calls"},
		{"SAFETY", "content_filter"},
		{"RECITATION", "content_filter"},
		{"BLOCKLIST", "content_filter"},
		{"PROHIBITED_CONTENT", "content_filter"},
		{"SPII", "content_filter"},
		{"LANGUAGE", "content_filter"},
		{"FAILED", "error"},
		{"", ""},
		{"end_turn", "end_turn"},
		{"STOP", "stop"},
	}
	for _, tt := range tests {
		got := NormalizeFinishReason(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeFinishReason(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMapSDKError(t *testing.T) {
	extractOpenAI := func(err error) (int, string, bool) {
		return 401, "unauthorized", true
	}
	extractNone := func(err error) (int, string, bool) {
		return 0, "", false
	}

	t.Run("nil error returns nil", func(t *testing.T) {
		got := MapSDKError(nil, "test", extractOpenAI)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("api error extracts status and message", func(t *testing.T) {
		err := errors.New("api failed")
		got := MapSDKError(err, "test", extractOpenAI)
		te, ok := got.(*types.Error)
		if !ok {
			t.Fatalf("expected *types.Error, got %T", got)
		}
		if te.Code != llm.ErrUnauthorized {
			t.Errorf("expected ErrUnauthorized, got %v", te.Code)
		}
		if te.Provider != "test" {
			t.Errorf("expected provider 'test', got %q", te.Provider)
		}
	})

	t.Run("non-api error returns upstream error", func(t *testing.T) {
		err := errors.New("network failure")
		got := MapSDKError(err, "test", extractNone)
		te, ok := got.(*types.Error)
		if !ok {
			t.Fatalf("expected *types.Error, got %T", got)
		}
		if te.Code != llm.ErrUpstreamError {
			t.Errorf("expected ErrUpstreamError, got %v", te.Code)
		}
		if te.HTTPStatus != http.StatusBadGateway {
			t.Errorf("expected 502, got %d", te.HTTPStatus)
		}
		if !te.Retryable {
			t.Error("expected retryable")
		}
	})
}

func TestStreamSSE_AggregatesFunctionToolCallDeltasBeforeEmitting(t *testing.T) {
	body := io.NopCloser(strings.NewReader(strings.Join([]string{
		`data: {"id":"resp-1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\""}}]}}]}`,
		`data: {"id":"resp-1","model":"gpt-4o-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"type":"function","function":{"arguments":"agent"}}]}}]}`,
		`data: {"id":"resp-1","model":"gpt-4o-mini","choices":[{"index":0,"finish_reason":"tool_calls","delta":{"tool_calls":[{"index":0,"type":"function","function":{"arguments":"flow\"}"}}]}}]}`,
		`data: [DONE]`,
	}, "\n")))

	ch := StreamSSE(context.Background(), body, "openai-compat")

	var chunks []llm.StreamChunk
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if len(chunks[0].Delta.ToolCalls) != 0 || len(chunks[1].Delta.ToolCalls) != 0 {
		t.Fatalf("expected fragmented deltas to stay buffered until tool_calls stop, got %#v %#v", chunks[0].Delta.ToolCalls, chunks[1].Delta.ToolCalls)
	}
	if len(chunks[2].Delta.ToolCalls) != 1 {
		t.Fatalf("expected exactly one assembled tool call, got %#v", chunks[2].Delta.ToolCalls)
	}
	call := chunks[2].Delta.ToolCalls[0]
	if call.ID != "call_1" {
		t.Fatalf("unexpected call id: %s", call.ID)
	}
	if call.Name != "search" {
		t.Fatalf("unexpected call name: %s", call.Name)
	}
	if got, want := string(call.Arguments), `{"q":"agentflow"}`; got != want {
		t.Fatalf("arguments mismatch: got=%s want=%s", got, want)
	}
	if chunks[2].FinishReason != "tool_calls" {
		t.Fatalf("unexpected finish reason: %s", chunks[2].FinishReason)
	}
}

func TestRewriteChainError(t *testing.T) {
	err := errors.New("rewrite boom")
	got := RewriteChainError(err, "myprov")
	if got.Code != llm.ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %v", got.Code)
	}
	if got.HTTPStatus != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", got.HTTPStatus)
	}
	if got.Provider != "myprov" {
		t.Errorf("expected provider 'myprov', got %q", got.Provider)
	}
}

func TestValidateTemperatureTopPMutualExclusion(t *testing.T) {
	if err := ValidateTemperatureTopPMutualExclusion(0, 0, "test"); err != nil {
		t.Errorf("both zero should pass: %v", err)
	}
	if err := ValidateTemperatureTopPMutualExclusion(0.5, 0, "test"); err != nil {
		t.Errorf("temperature only should pass: %v", err)
	}
	if err := ValidateTemperatureTopPMutualExclusion(0, 0.8, "test"); err != nil {
		t.Errorf("topP only should pass: %v", err)
	}
	if err := ValidateTemperatureTopPMutualExclusion(0.5, 0.8, "test"); err == nil {
		t.Error("both non-zero should fail")
	}
}

func TestValidateMaxTokensRange(t *testing.T) {
	if err := ValidateMaxTokensRange(0, 1, 8192, "test"); err != nil {
		t.Errorf("zero maxTokens should pass: %v", err)
	}
	if err := ValidateMaxTokensRange(100, 1, 8192, "test"); err != nil {
		t.Errorf("in range should pass: %v", err)
	}
	if err := ValidateMaxTokensRange(0, 1, 0, "test"); err != nil {
		t.Errorf("no max limit should pass: %v", err)
	}
	if err := ValidateMaxTokensRange(99999, 1, 8192, "test"); err == nil {
		t.Error("exceeds max should fail")
	}
	if err := ValidateMaxTokensRange(0, 1, 8192, "test"); err != nil {
		t.Errorf("zero should pass: %v", err)
	}
}

func TestValidateTemperatureRange(t *testing.T) {
	if err := ValidateTemperatureRange(0, 0, 2, "test"); err != nil {
		t.Errorf("zero temperature should pass: %v", err)
	}
	if err := ValidateTemperatureRange(1.0, 0, 2, "test"); err != nil {
		t.Errorf("in range should pass: %v", err)
	}
	if err := ValidateTemperatureRange(3.0, 0, 2, "test"); err == nil {
		t.Error("exceeds max should fail")
	}
}

func TestValidateModelName(t *testing.T) {
	if err := ValidateModelName("", []string{"gpt"}, "test"); err != nil {
		t.Errorf("empty model should pass: %v", err)
	}
	if err := ValidateModelName("gpt-4", []string{"gpt"}, "test"); err != nil {
		t.Errorf("matching prefix should pass: %v", err)
	}
	if err := ValidateModelName("claude-3", []string{"gpt"}, "test"); err == nil {
		t.Error("non-matching prefix should fail")
	}
	if err := ValidateModelName("anything", nil, "test"); err != nil {
		t.Errorf("empty allowed should pass: %v", err)
	}
}
