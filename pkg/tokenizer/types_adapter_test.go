package tokenizer

import (
	"testing"

	"github.com/BaSui01/agentflow/types"
)

type fakeSharedTokenizer struct {
	countErr  bool
	encodeErr bool
}

func (f fakeSharedTokenizer) CountTokens(text string) (int, error) {
	if f.countErr {
		return 0, errFakeTokenizer
	}
	return len([]rune(text)), nil
}

func (f fakeSharedTokenizer) Encode(text string) ([]int, error) {
	if f.countErr || f.encodeErr {
		return nil, errFakeTokenizer
	}
	return make([]int, len([]rune(text))), nil
}
func (f fakeSharedTokenizer) Decode([]int) (string, error) { return "", nil }
func (f fakeSharedTokenizer) MaxTokens() int               { return 4096 }
func (f fakeSharedTokenizer) Name() string                 { return "fake" }

var errFakeTokenizer = &fakeTokenizerError{}

type fakeTokenizerError struct{}

func (*fakeTokenizerError) Error() string { return "fake tokenizer error" }

func TestTypesAdapterImplementsTypesTokenizer(t *testing.T) {
	var _ types.Tokenizer = NewTypesAdapter(fakeSharedTokenizer{})

	adapter := NewTypesAdapter(fakeSharedTokenizer{})
	if got := adapter.CountTokens("你好"); got != 2 {
		t.Fatalf("CountTokens = %d, want 2", got)
	}
	msgs := []types.Message{{Content: "hi"}, {Content: "你好"}}
	if got := adapter.CountMessagesTokens(msgs); got != 4 {
		t.Fatalf("CountMessagesTokens = %d, want 4", got)
	}
	tools := []types.ToolSchema{{Name: "search", Description: "find docs", Parameters: []byte(`{"type":"object"}`)}}
	if got := adapter.EstimateToolTokens(tools); got <= 10 {
		t.Fatalf("EstimateToolTokens = %d, want tool overhead plus text/params", got)
	}
}

func TestTypesAdapterCountErrorFallsBackToZero(t *testing.T) {
	adapter := NewTypesAdapter(fakeSharedTokenizer{countErr: true})
	if got := adapter.CountTokens("hello"); got != 0 {
		t.Fatalf("CountTokens error fallback = %d, want 0", got)
	}
}
