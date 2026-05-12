package tokenizer_test

import (
	"testing"

	"github.com/BaSui01/agentflow/pkg/tokenizer"
	"github.com/BaSui01/agentflow/pkg/tokenizer/contracttest"
)

type roundTripTokenizer struct{}

func (roundTripTokenizer) CountTokens(text string) (int, error) { return len([]rune(text)), nil }
func (roundTripTokenizer) Encode(text string) ([]int, error) {
	out := make([]int, 0, len([]rune(text)))
	for _, r := range text {
		out = append(out, int(r))
	}
	return out, nil
}
func (roundTripTokenizer) Decode(tokens []int) (string, error) {
	runes := make([]rune, len(tokens))
	for i, token := range tokens {
		runes[i] = rune(token)
	}
	return string(runes), nil
}
func (roundTripTokenizer) MaxTokens() int { return 1024 }
func (roundTripTokenizer) Name() string   { return "roundtrip" }

func TestValidateContractAcceptsRoundTripTokenizer(t *testing.T) {
	contracttest.Validate(t, roundTripTokenizer{}, tokenizer.ContractCases{
		Texts: []string{"hello", "你好", "hello 你好"},
	})
}
