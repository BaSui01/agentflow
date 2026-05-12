// Package contracttest provides reusable tokenizer contract assertions for tests.
package contracttest

import (
	"testing"

	"github.com/BaSui01/agentflow/pkg/tokenizer"
)

// Validate verifies that a tokenizer keeps CountTokens, Encode, and Decode
// behavior aligned for representative text cases.
func Validate(t *testing.T, tok tokenizer.Tokenizer, cases tokenizer.ContractCases) {
	t.Helper()
	if tok == nil {
		t.Fatal("tokenizer cannot be nil")
	}
	if tok.Name() == "" {
		t.Fatal("tokenizer name cannot be empty")
	}
	if tok.MaxTokens() < 0 {
		t.Fatalf("max tokens cannot be negative: %d", tok.MaxTokens())
	}
	texts := cases.Texts
	if len(texts) == 0 {
		texts = []string{"hello", "你好", "hello 你好"}
	}
	for _, text := range texts {
		t.Run(text, func(t *testing.T) {
			count, err := tok.CountTokens(text)
			if err != nil {
				t.Fatalf("CountTokens(%q): %v", text, err)
			}
			if count < 0 {
				t.Fatalf("CountTokens(%q) returned negative count %d", text, count)
			}
			tokens, err := tok.Encode(text)
			if err != nil {
				t.Fatalf("Encode(%q): %v", text, err)
			}
			if len(tokens) != count {
				t.Fatalf("Encode(%q) length=%d does not match CountTokens=%d", text, len(tokens), count)
			}
			decoded, err := tok.Decode(tokens)
			if err != nil {
				t.Fatalf("Decode(%q tokens): %v", text, err)
			}
			if decoded != text {
				t.Fatalf("Decode(Encode(%q))=%q", text, decoded)
			}
		})
	}
}
