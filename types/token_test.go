package types

import (
	"encoding/json"
	"testing"
)

func TestTokenUsage_Add(t *testing.T) {
	t.Parallel()

	u := TokenUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3, Cost: 0.5}
	u.Add(TokenUsage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 5, Cost: 1.25})

	if u.PromptTokens != 4 || u.CompletionTokens != 6 || u.TotalTokens != 8 {
		t.Fatalf("unexpected tokens: %+v", u)
	}
	if u.Cost != 1.75 {
		t.Fatalf("unexpected cost: %v", u.Cost)
	}
}

func TestEstimateTokenizer_Counting(t *testing.T) {
	t.Parallel()

	tok := NewEstimateTokenizer()

	if got := tok.CountTokens(""); got != 0 {
		t.Fatalf("expected 0 tokens for empty, got %d", got)
	}
	if got := tok.CountTokens("a"); got != 1 {
		t.Fatalf("expected minimum 1 token for non-empty, got %d", got)
	}

	msg := Message{
		Role:    RoleUser,
		Content: "hello world",
		Name:    "user",
		ToolCalls: []ToolCall{{
			ID:        "tc1",
			Name:      "get_weather",
			Arguments: json.RawMessage(`{"city":"SF"}`),
		}},
	}

	if got := tok.CountMessageTokens(msg); got <= 0 {
		t.Fatalf("expected positive message tokens, got %d", got)
	}
	if got := tok.CountMessagesTokens([]Message{msg, msg}); got <= tok.CountMessageTokens(msg) {
		t.Fatalf("expected messages tokens > single message tokens, got %d", got)
	}

	tools := []ToolSchema{{
		Name:        "get_weather",
		Description: "Get weather",
		Parameters:  []byte(`{"type":"object","properties":{"city":{"type":"string"}}}`),
	}}
	if got := tok.EstimateToolTokens(tools); got <= 0 {
		t.Fatalf("expected positive tool tokens, got %d", got)
	}
}
