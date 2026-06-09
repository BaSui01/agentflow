package tokenizer

import (
	"testing"

	pkgtokenizer "github.com/BaSui01/agentflow/pkg/tokenizer"
	"github.com/BaSui01/agentflow/pkg/tokenizer/contracttest"
)

func TestTiktokenTokenizerSatisfiesSharedContract(t *testing.T) {
	tok, err := NewTiktokenTokenizer("gpt-4o-mini")
	if err != nil {
		t.Fatalf("new tokenizer: %v", err)
	}

	contracttest.Validate(t, tok, pkgtokenizer.ContractCases{
		Texts: []string{"hello", "你好", "hello 你好"},
	})
}
