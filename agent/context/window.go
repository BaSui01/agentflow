package context

import (
	"context"

	"github.com/BaSui01/agentflow/types"
)

// WindowStrategy defines the context window management strategy.
type WindowStrategy string

const (
	// StrategySlidingWindow keeps the most recent N messages.
	StrategySlidingWindow WindowStrategy = "sliding_window"
	// StrategyTokenBudget trims messages by token budget.
	StrategyTokenBudget WindowStrategy = "token_budget"
	// StrategySummarize compresses old messages via LLM summarization.
	StrategySummarize WindowStrategy = "summarize"
)

// WindowConfig configures the window manager.
type WindowConfig struct {
	Strategy      WindowStrategy `json:"strategy"`
	MaxTokens     int            `json:"max_tokens"`      // Token budget ceiling
	MaxMessages   int            `json:"max_messages"`    // Maximum message count
	ReserveTokens int            `json:"reserve_tokens"`  // Tokens reserved for new reply
	SummaryModel  string         `json:"summary_model"`   // Model for summarization (optional)
	KeepSystemMsg bool           `json:"keep_system_msg"` // Always preserve system messages
	KeepLastN     int            `json:"keep_last_n"`     // Always preserve last N messages
}

// Summarizer compresses messages into a summary string via LLM.
// Optional — when nil, the Summarize strategy falls back to TokenBudget.
type Summarizer interface {
	Summarize(ctx context.Context, messages []types.Message) (string, error)
}

// WindowStatus reports the current state of the context window.
type WindowStatus struct {
	TotalTokens  int  `json:"total_tokens"`
	MessageCount int  `json:"message_count"`
	MaxTokens    int  `json:"max_tokens"`
	Trimmed      bool `json:"trimmed"`
}

// WindowManager implements automatic context window management.
// It satisfies the agent.ContextManager interface.
type WindowManager struct {
	config       WindowConfig
	tokenCounter types.TokenCounter
	summarizer   Summarizer
}

// NewWindowManager creates a WindowManager.
// tokenCounter may be nil (defaults to len/4 estimation).
// summarizer may be nil (Summarize strategy falls back to TokenBudget).
func NewWindowManager(config WindowConfig, tokenCounter types.TokenCounter, summarizer Summarizer) *WindowManager {
	return &WindowManager{
		config:       config,
		tokenCounter: tokenCounter,
		summarizer:   summarizer,
	}
}

// countTokens counts tokens for a string, falling back to len/4 if no counter.
func (w *WindowManager) countTokens(text string) int {
	if w.tokenCounter != nil {
		return w.tokenCounter.CountTokens(text)
	}
	n := len(text) / 4
	if n == 0 && len(text) > 0 {
		return 1
	}
	return n
}

// messageTokens estimates tokens for a single message.
func (w *WindowManager) messageTokens(msg types.Message) int {
	tokens := w.countTokens(msg.Content)
	if msg.Name != "" {
		tokens += w.countTokens(msg.Name)
	}
	for _, tc := range msg.ToolCalls {
		tokens += w.countTokens(tc.Name)
		tokens += len(tc.Arguments) / 4
	}
	// per-message overhead
	tokens += 4
	return tokens
}

// EstimateTokens returns the total token count across all messages.
func (w *WindowManager) EstimateTokens(messages []types.Message) int {
	total := 0
	for _, m := range messages {
		total += w.messageTokens(m)
	}
	return total
}

// GetStatus returns the current window state.
func (w *WindowManager) GetStatus(messages []types.Message) any {
	budget := w.config.MaxTokens - w.config.ReserveTokens
	if budget < 0 {
		budget = 0
	}
	return WindowStatus{
		TotalTokens:  w.EstimateTokens(messages),
		MessageCount: len(messages),
		MaxTokens:    budget,
		Trimmed:      false,
	}
}

// PrepareMessages trims messages according to the configured strategy.
func (w *WindowManager) PrepareMessages(ctx context.Context, messages []types.Message, _ string) ([]types.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	switch w.config.Strategy {
	case StrategySlidingWindow:
		return w.slidingWindow(messages), nil
	case StrategyTokenBudget:
		return w.tokenBudget(messages), nil
	case StrategySummarize:
		return w.summarize(ctx, messages)
	default:
		return w.tokenBudget(messages), nil
	}
}

// splitSystemAndOther separates system messages from the rest.
func splitSystemAndOther(msgs []types.Message, keepSystem bool) (system, other []types.Message) {
	for _, m := range msgs {
		if keepSystem && m.Role == types.RoleSystem {
			system = append(system, m)
		} else {
			other = append(other, m)
		}
	}
	return
}

// slidingWindow keeps system messages + the last N non-system messages.
// Bug fix: 修正边界条件 —— MaxMessages 作为非系统消息的上限，
// KeepLastN 作为下限但不超过可用消息数，防止重复或丢失消息。
func (w *WindowManager) slidingWindow(messages []types.Message) []types.Message {
	system, other := splitSystemAndOther(messages, w.config.KeepSystemMsg)

	if len(other) == 0 {
		return system
	}

	limit := w.config.MaxMessages
	if limit <= 0 {
		// MaxMessages 未设置时保留所有非系统消息
		return append(system, other...)
	}

	// KeepLastN 作为保留下限，但不能超过 MaxMessages
	if w.config.KeepLastN > 0 && limit < w.config.KeepLastN {
		limit = w.config.KeepLastN
	}

	// 确保 limit 不超过可用消息数，防止切片越界
	if limit >= len(other) {
		return append(system, other...)
	}

	kept := other[len(other)-limit:]
	return append(system, kept...)
}

// tokenBudget keeps messages from newest to oldest within the token budget.
func (w *WindowManager) tokenBudget(messages []types.Message) []types.Message {
	budget := w.config.MaxTokens - w.config.ReserveTokens
	if budget <= 0 {
		return nil
	}

	system, other := splitSystemAndOther(messages, w.config.KeepSystemMsg)

	// Account for system message tokens first.
	used := 0
	for _, m := range system {
		used += w.messageTokens(m)
	}

	// Guarantee KeepLastN from the tail.
	keepN := w.config.KeepLastN
	if keepN > len(other) {
		keepN = len(other)
	}

	// Walk backwards, accumulating tokens.
	kept := make([]types.Message, 0, len(other))
	for i := len(other) - 1; i >= 0; i-- {
		cost := w.messageTokens(other[i])
		inKeepZone := (len(other) - 1 - i) < keepN
		if inKeepZone || used+cost <= budget {
			kept = append(kept, other[i])
			used += cost
		}
	}

	// Reverse to restore chronological order.
	for l, r := 0, len(kept)-1; l < r; l, r = l+1, r-1 {
		kept[l], kept[r] = kept[r], kept[l]
	}

	return append(system, kept...)
}

// summarize compresses old messages via LLM summarization.
// Falls back to tokenBudget when no summarizer is configured.
func (w *WindowManager) summarize(ctx context.Context, messages []types.Message) ([]types.Message, error) {
	if w.summarizer == nil {
		return w.tokenBudget(messages), nil
	}

	budget := w.config.MaxTokens - w.config.ReserveTokens
	if budget <= 0 {
		return nil, nil
	}

	// If already within budget, no trimming needed.
	if w.EstimateTokens(messages) <= budget {
		return messages, nil
	}

	system, other := splitSystemAndOther(messages, w.config.KeepSystemMsg)

	// Determine how many tail messages to preserve.
	keepN := w.config.KeepLastN
	if keepN <= 0 {
		keepN = 2
	}
	if keepN > len(other) {
		keepN = len(other)
	}

	toSummarize := other[:len(other)-keepN]
	tail := other[len(other)-keepN:]

	if len(toSummarize) == 0 {
		return append(system, tail...), nil
	}

	summary, err := w.summarizer.Summarize(ctx, toSummarize)
	if err != nil {
		// On summarizer failure, fall back to token budget.
		return w.tokenBudget(messages), nil
	}

	summaryMsg := types.Message{
		Role:    types.RoleAssistant,
		Content: summary,
	}

	result := make([]types.Message, 0, len(system)+1+len(tail))
	result = append(result, system...)
	result = append(result, summaryMsg)
	result = append(result, tail...)
	return result, nil
}
