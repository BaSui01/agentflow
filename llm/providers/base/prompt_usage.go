package providerbase

import (
	"encoding/json"
	"strings"

	"github.com/BaSui01/agentflow/llm/tokenizer"
)

// CountOpenAICompatPromptTokens estimates prompt tokens from a fully-built
// OpenAI-compatible request body, immediately before it is sent upstream.
func CountOpenAICompatPromptTokens(model string, body OpenAICompatRequest) int {
	tok := tokenizer.GetTokenizerOrEstimator(strings.TrimSpace(model))
	if tok == nil {
		return 0
	}

	messages := make([]tokenizer.Message, 0, len(body.Messages))
	for _, message := range body.Messages {
		messages = append(messages, tokenizer.Message{
			Role:    strings.TrimSpace(message.Role),
			Content: flattenOpenAICompatMessageForPromptCounting(message),
		})
	}

	promptTokens, err := tok.CountMessages(messages)
	if err != nil || promptTokens < 0 {
		promptTokens = fallbackCountMessages(messages)
	}

	promptTokens += countJSONTokenField(tok, body.Tools)
	promptTokens += countJSONTokenField(tok, body.ToolChoice)
	promptTokens += countJSONTokenField(tok, body.ResponseFormat)

	if promptTokens < 0 {
		return 0
	}
	return promptTokens
}

func flattenOpenAICompatMessageForPromptCounting(message OpenAICompatMessage) string {
	parts := make([]string, 0, 10)
	if content := strings.TrimSpace(message.Content); content != "" {
		parts = append(parts, content)
	}
	if message.ReasoningContent != nil {
		if reasoning := strings.TrimSpace(*message.ReasoningContent); reasoning != "" {
			parts = append(parts, reasoning)
		}
	}
	if message.Refusal != nil {
		if refusal := strings.TrimSpace(*message.Refusal); refusal != "" {
			parts = append(parts, refusal)
		}
	}
	for _, call := range message.ToolCalls {
		if fn := call.Function; fn != nil {
			if name := strings.TrimSpace(fn.Name); name != "" {
				parts = append(parts, name)
			}
			if args := strings.TrimSpace(string(fn.Arguments)); args != "" {
				parts = append(parts, args)
			}
		}
		if custom := call.Custom; custom != nil {
			if name := strings.TrimSpace(custom.Name); name != "" {
				parts = append(parts, name)
			}
			if input := strings.TrimSpace(custom.Input); input != "" {
				parts = append(parts, input)
			}
		}
	}
	if message.ToolCallID != "" {
		parts = append(parts, strings.TrimSpace(message.ToolCallID))
	}
	if message.Name != "" {
		parts = append(parts, strings.TrimSpace(message.Name))
	}
	if len(message.MultiContent) > 0 {
		if data, err := json.Marshal(message.MultiContent); err == nil {
			if text := strings.TrimSpace(string(data)); text != "" && text != "null" {
				parts = append(parts, text)
			}
		}
	}
	if len(message.Annotations) > 0 {
		if data, err := json.Marshal(message.Annotations); err == nil {
			if text := strings.TrimSpace(string(data)); text != "" && text != "null" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func countJSONTokenField(tok tokenizer.Tokenizer, value any) int {
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

func fallbackCountMessages(messages []tokenizer.Message) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content)/4 + 4
	}
	if total > 0 {
		total += 3
	}
	return total
}
