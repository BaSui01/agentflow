package types

// TokenUsage represents token consumption statistics.
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
}

// Add adds another TokenUsage to this one.
func (u *TokenUsage) Add(other TokenUsage) {
	u.PromptTokens += other.PromptTokens
	u.CompletionTokens += other.CompletionTokens
	u.TotalTokens += other.TotalTokens
	u.Cost += other.Cost
}

// Tokenizer defines the interface for token counting.
//
// Note: Three Tokenizer interfaces exist in the project, each serving a different layer:
//   - types.Tokenizer (this)    — Framework-level, Message/ToolSchema-aware, no error returns
//   - llm/tokenizer.Tokenizer   — LLM-level, full encode/decode with errors, model-aware
//   - rag.Tokenizer             — RAG chunking, minimal (CountTokens + Encode), no errors
//
// These cannot be unified without introducing circular dependencies (rag -> types.Message)
// or forcing incompatible method signatures (error vs no-error returns).
// Use rag.NewLLMTokenizerAdapter() to bridge llm/tokenizer.Tokenizer to rag.Tokenizer.
type Tokenizer interface {
	// CountTokens counts tokens in a text string.
	CountTokens(text string) int
	// CountMessageTokens counts tokens in a single message.
	CountMessageTokens(msg Message) int
	// CountMessagesTokens counts total tokens in a message slice.
	CountMessagesTokens(msgs []Message) int
	// EstimateToolTokens estimates tokens for tool schemas.
	EstimateToolTokens(tools []ToolSchema) int
}

// EstimateTokenizer provides a simple character-based token estimation.
type EstimateTokenizer struct {
	charsPerToken float64
	msgOverhead   int
}

// NewEstimateTokenizer creates a new EstimateTokenizer.
func NewEstimateTokenizer() *EstimateTokenizer {
	return &EstimateTokenizer{
		charsPerToken: 4.0,
		msgOverhead:   4,
	}
}

// CountTokens counts tokens in text.
func (t *EstimateTokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	var chineseCount, otherCount int
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FA5 {
			chineseCount++
		} else {
			otherCount++
		}
	}
	tokens := float64(chineseCount)/1.5 + float64(otherCount)/4.0
	if tokens < 1 {
		return 1
	}
	return int(tokens)
}

// CountMessageTokens counts tokens in a message.
func (t *EstimateTokenizer) CountMessageTokens(msg Message) int {
	tokens := t.msgOverhead
	tokens += t.CountTokens(msg.Content)
	if msg.Name != "" {
		tokens += t.CountTokens(msg.Name)
	}
	for _, tc := range msg.ToolCalls {
		tokens += t.CountTokens(tc.Name)
		tokens += len(tc.Arguments) / 4
	}
	return tokens
}

// CountMessagesTokens counts tokens in messages.
func (t *EstimateTokenizer) CountMessagesTokens(msgs []Message) int {
	total := 0
	for _, msg := range msgs {
		total += t.CountMessageTokens(msg)
	}
	return total
}

// EstimateToolTokens estimates tokens for tools.
func (t *EstimateTokenizer) EstimateToolTokens(tools []ToolSchema) int {
	total := 0
	for _, tool := range tools {
		total += t.CountTokens(tool.Name)
		total += t.CountTokens(tool.Description)
		total += len(tool.Parameters) / 4
		total += 10
	}
	return total
}
