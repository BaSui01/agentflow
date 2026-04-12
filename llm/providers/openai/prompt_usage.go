package openai

import (
	"encoding/json"
	"strings"

	"github.com/BaSui01/agentflow/llm/tokenizer"
)

func countResponsesPromptTokens(body openAIResponsesRequest) int {
	model := strings.TrimSpace(body.Model)
	tok := tokenizer.GetTokenizerOrEstimator(model)
	if tok == nil {
		return 0
	}

	total := 0
	total += countResponsesTokenField(tok, body.Input)
	total += countResponsesTokenField(tok, body.Tools)
	total += countResponsesTokenField(tok, body.ToolChoice)
	if body.Text != nil {
		total += countResponsesTokenField(tok, body.Text.Format)
	}
	if total < 0 {
		return 0
	}
	return total
}

func countResponsesTokenField(tok tokenizer.Tokenizer, value any) int {
	if tok == nil || value == nil {
		return 0
	}
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" || text == "[]" || text == "{}" || text == "\"\"" {
		return 0
	}
	count, err := tok.CountTokens(text)
	if err != nil || count < 0 {
		return len(text) / 4
	}
	return count
}
