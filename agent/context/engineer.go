// Package context provides unified context management for agents.
package context

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Engineer is the unified context management component for agents.
// It handles compression, pruning, and adaptive focus strategies.
type Engineer struct {
	config    Config
	tokenizer types.Tokenizer
	logger    *zap.Logger

	mu    sync.RWMutex
	stats Stats
}

// Config defines context engineering configuration.
type Config struct {
	MaxContextTokens int      `json:"max_context_tokens"`
	ReserveForOutput int      `json:"reserve_for_output"`
	SoftLimit        float64  `json:"soft_limit"`
	WarnLimit        float64  `json:"warn_limit"`
	HardLimit        float64  `json:"hard_limit"`
	TargetUsage      float64  `json:"target_usage"`
	Strategy         Strategy `json:"strategy"`
}

// Strategy defines the context management strategy.
type Strategy string

const (
	StrategyAdaptive   Strategy = "adaptive"
	StrategySummary    Strategy = "summary"
	StrategyPruning    Strategy = "pruning"
	StrategySliding    Strategy = "sliding"
	StrategyAggressive Strategy = "aggressive"
)

// Level represents compression urgency level.
type Level int

const (
	LevelNone Level = iota
	LevelNormal
	LevelAggressive
	LevelEmergency
)

// Stats tracks context engineering statistics.
type Stats struct {
	TotalCompressions   int64   `json:"total_compressions"`
	EmergencyCount      int64   `json:"emergency_count"`
	AvgCompressionRatio float64 `json:"avg_compression_ratio"`
	TokensSaved         int64   `json:"tokens_saved"`
}

// Status represents current context status.
type Status struct {
	CurrentTokens  int     `json:"current_tokens"`
	MaxTokens      int     `json:"max_tokens"`
	UsageRatio     float64 `json:"usage_ratio"`
	Level          Level   `json:"level"`
	Recommendation string  `json:"recommendation"`
}

// DefaultConfig returns sensible defaults for 200k context models.
func DefaultConfig() Config {
	return Config{
		MaxContextTokens: 200000,
		ReserveForOutput: 8000,
		SoftLimit:        0.7,
		WarnLimit:        0.85,
		HardLimit:        0.95,
		TargetUsage:      0.5,
		Strategy:         StrategyAdaptive,
	}
}

// New creates a new context Engineer.
func New(config Config, logger *zap.Logger) *Engineer {
	return &Engineer{
		config:    config,
		tokenizer: types.NewEstimateTokenizer(),
		logger:    logger,
	}
}

// GetStatus returns current context status.
func (e *Engineer) GetStatus(msgs []types.Message) Status {
	currentTokens := e.tokenizer.CountMessagesTokens(msgs)
	effectiveMax := e.config.MaxContextTokens - e.config.ReserveForOutput
	usage := float64(currentTokens) / float64(effectiveMax)

	status := Status{
		CurrentTokens: currentTokens,
		MaxTokens:     effectiveMax,
		UsageRatio:    usage,
		Level:         e.getLevel(usage),
	}

	switch {
	case usage >= e.config.HardLimit:
		status.Recommendation = "CRITICAL: immediate compression required"
	case usage >= e.config.WarnLimit:
		status.Recommendation = "WARNING: compression recommended"
	case usage >= e.config.SoftLimit:
		status.Recommendation = "INFO: monitoring context usage"
	default:
		status.Recommendation = "OK: context usage normal"
	}

	return status
}

func (e *Engineer) getLevel(usage float64) Level {
	switch {
	case usage >= e.config.HardLimit:
		return LevelEmergency
	case usage >= e.config.WarnLimit:
		return LevelAggressive
	case usage >= e.config.SoftLimit:
		return LevelNormal
	default:
		return LevelNone
	}
}

// Manage processes messages based on current context usage.
func (e *Engineer) Manage(ctx context.Context, msgs []types.Message, query string) ([]types.Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}

	status := e.GetStatus(msgs)
	e.logger.Debug("context engineering",
		zap.Int("tokens", status.CurrentTokens),
		zap.Float64("usage", status.UsageRatio),
		zap.Int("level", int(status.Level)))

	switch status.Level {
	case LevelEmergency:
		return e.emergencyCompress(msgs)
	case LevelAggressive:
		return e.aggressiveCompress(msgs, query)
	case LevelNormal:
		return e.normalCompress(msgs)
	default:
		return msgs, nil
	}
}

// emergencyCompress handles critical context overflow.
func (e *Engineer) emergencyCompress(msgs []types.Message) ([]types.Message, error) {
	e.logger.Warn("EMERGENCY compression triggered")

	e.mu.Lock()
	e.stats.EmergencyCount++
	e.mu.Unlock()

	originalTokens := e.tokenizer.CountMessagesTokens(msgs)
	targetTokens := int(float64(e.config.MaxContextTokens-e.config.ReserveForOutput) * e.config.TargetUsage)

	// Separate system and other messages
	var systemMsgs, otherMsgs []types.Message
	for _, msg := range msgs {
		if msg.Role == types.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	// Truncate system messages if too long
	systemMsgs = e.truncateMessages(systemMsgs, 4000)
	systemTokens := e.tokenizer.CountMessagesTokens(systemMsgs)

	remainingBudget := targetTokens - systemTokens
	if remainingBudget < 1000 {
		remainingBudget = 1000
	}

	// Keep only last 2 messages
	preserveCount := 2
	if preserveCount > len(otherMsgs) {
		preserveCount = len(otherMsgs)
	}

	var recentMsgs, toCompress []types.Message
	if preserveCount > 0 && len(otherMsgs) > 0 {
		recentMsgs = otherMsgs[len(otherMsgs)-preserveCount:]
		toCompress = otherMsgs[:len(otherMsgs)-preserveCount]
	}

	recentMsgs = e.truncateMessages(recentMsgs, 4000)

	// Create summary
	summary := e.createEmergencySummary(toCompress)
	summaryMsg := types.Message{
		Role:    types.RoleSystem,
		Content: fmt.Sprintf("[Emergency Summary - %d messages]\n%s", len(toCompress), summary),
	}

	result := make([]types.Message, 0, len(systemMsgs)+1+len(recentMsgs))
	result = append(result, systemMsgs...)
	result = append(result, summaryMsg)
	result = append(result, recentMsgs...)

	e.updateStats(originalTokens, e.tokenizer.CountMessagesTokens(result))
	return result, nil
}

// aggressiveCompress handles high context usage (85-95%).
func (e *Engineer) aggressiveCompress(msgs []types.Message, _ string) ([]types.Message, error) {
	e.logger.Info("aggressive compression triggered")

	originalTokens := e.tokenizer.CountMessagesTokens(msgs)

	// First do normal compression
	result, err := e.normalCompress(msgs)
	if err != nil {
		return nil, err
	}

	// If still over target, truncate further
	targetTokens := int(float64(e.config.MaxContextTokens-e.config.ReserveForOutput) * e.config.TargetUsage)
	if e.tokenizer.CountMessagesTokens(result) > targetTokens {
		result = e.truncateMessages(result, 2000)

		// Remove middle messages if still over
		for e.tokenizer.CountMessagesTokens(result) > targetTokens && len(result) > 3 {
			result = append(result[:1], result[len(result)-2:]...)
		}
	}

	e.updateStats(originalTokens, e.tokenizer.CountMessagesTokens(result))
	return result, nil
}

// normalCompress handles moderate context usage (70-85%).
func (e *Engineer) normalCompress(msgs []types.Message) ([]types.Message, error) {
	e.logger.Debug("normal compression triggered")

	// Compress tool results
	result := make([]types.Message, len(msgs))
	maxToolLen := 2000

	for i, msg := range msgs {
		result[i] = msg
		if msg.Role == types.RoleTool && len(msg.Content) > maxToolLen {
			result[i].Content = msg.Content[:maxToolLen] + "\n...[truncated]"
		}
	}

	return result, nil
}

// truncateMessages truncates messages that exceed maxTokens per message.
func (e *Engineer) truncateMessages(msgs []types.Message, maxTokens int) []types.Message {
	result := make([]types.Message, len(msgs))
	for i, msg := range msgs {
		result[i] = msg
		msgTokens := e.tokenizer.CountMessageTokens(msg)
		if msgTokens > maxTokens {
			ratio := float64(maxTokens) / float64(msgTokens) * 0.9
			targetLen := int(float64(len(msg.Content)) * ratio)
			if targetLen < 100 {
				targetLen = 100
			}
			if targetLen < len(msg.Content) {
				result[i].Content = msg.Content[:targetLen] + "\n...[truncated]"
			}
		}
	}
	return result
}

// createEmergencySummary creates a minimal summary without LLM.
func (e *Engineer) createEmergencySummary(msgs []types.Message) string {
	if len(msgs) == 0 {
		return "No previous messages"
	}

	var sb strings.Builder
	userCount, assistantCount, toolCount := 0, 0, 0

	for _, msg := range msgs {
		switch msg.Role {
		case types.RoleUser:
			userCount++
		case types.RoleAssistant:
			assistantCount++
		case types.RoleTool:
			toolCount++
		}
	}

	fmt.Fprintf(&sb, "Stats: user=%d, assistant=%d", userCount, assistantCount)
	if toolCount > 0 {
		fmt.Fprintf(&sb, ", tools=%d", toolCount)
	}
	sb.WriteString("\nKey fragments:\n")

	// Show last few non-tool messages
	shown := 0
	for i := len(msgs) - 1; i >= 0 && shown < 5; i-- {
		msg := msgs[i]
		if msg.Role == types.RoleTool {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if len(content) > 80 {
			content = content[:80] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")
		fmt.Fprintf(&sb, "- [%s] %s\n", msg.Role, content)
		shown++
	}

	return sb.String()
}

func (e *Engineer) updateStats(originalTokens, finalTokens int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stats.TotalCompressions++
	e.stats.TokensSaved += int64(originalTokens - finalTokens)

	ratio := float64(finalTokens) / float64(originalTokens)
	e.stats.AvgCompressionRatio = (e.stats.AvgCompressionRatio*float64(e.stats.TotalCompressions-1) + ratio) / float64(e.stats.TotalCompressions)
}

// GetStats returns compression statistics.
func (e *Engineer) GetStats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.stats
}

// MustFit ensures messages fit within context window.
func (e *Engineer) MustFit(ctx context.Context, msgs []types.Message, query string) ([]types.Message, error) {
	maxTokens := e.config.MaxContextTokens - e.config.ReserveForOutput

	for i := 0; i < 5; i++ {
		if e.tokenizer.CountMessagesTokens(msgs) <= maxTokens {
			return msgs, nil
		}

		var err error
		msgs, err = e.Manage(ctx, msgs, query)
		if err != nil {
			return nil, err
		}
	}

	// Last resort: hard truncate
	return e.hardTruncate(msgs, maxTokens), nil
}

func (e *Engineer) hardTruncate(msgs []types.Message, maxTokens int) []types.Message {
	e.logger.Warn("hard truncate triggered")

	var systemMsgs, otherMsgs []types.Message
	for _, msg := range msgs {
		if msg.Role == types.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	// Truncate system messages
	for i := range systemMsgs {
		if len(systemMsgs[i].Content) > 1000 {
			systemMsgs[i].Content = systemMsgs[i].Content[:1000] + "\n...[truncated]"
		}
	}

	// Keep only last 2 other messages
	if len(otherMsgs) > 2 {
		otherMsgs = otherMsgs[len(otherMsgs)-2:]
	}

	result := append(systemMsgs, otherMsgs...)
	if len(result) == 0 {
		return result
	}

	// Ensure we actually fit within the requested budget.
	for tries := 0; tries < 20 && e.tokenizer.CountMessagesTokens(result) > maxTokens; tries++ {
		// Prefer dropping older non-system messages first.
		if len(result) > 1 {
			dropIdx := -1
			for i := 0; i < len(result); i++ {
				if result[i].Role != types.RoleSystem {
					dropIdx = i
					break
				}
			}
			if dropIdx < 0 {
				dropIdx = 0
			}
			result = append(result[:dropIdx], result[dropIdx+1:]...)
			continue
		}

		// Single message left: truncate aggressively to fit.
		result = e.truncateMessages(result, maxTokens)
	}

	// If we still overflow, try truncating with a per-message budget.
	for tries := 0; tries < 10 && e.tokenizer.CountMessagesTokens(result) > maxTokens && len(result) > 0; tries++ {
		perMsg := maxTokens / len(result)
		if perMsg < 1 {
			perMsg = 1
		}
		result = e.truncateMessages(result, perMsg)
		if e.tokenizer.CountMessagesTokens(result) <= maxTokens {
			break
		}
		if len(result) > 1 {
			result = result[1:]
		}
	}

	return result
}

// EstimateTokens returns token count for messages.
func (e *Engineer) EstimateTokens(msgs []types.Message) int {
	return e.tokenizer.CountMessagesTokens(msgs)
}

// CanAddMessage checks if a new message can be added.
func (e *Engineer) CanAddMessage(msgs []types.Message, newMsg types.Message) bool {
	currentTokens := e.tokenizer.CountMessagesTokens(msgs)
	newTokens := e.tokenizer.CountMessageTokens(newMsg)
	maxTokens := e.config.MaxContextTokens - e.config.ReserveForOutput
	return currentTokens+newTokens <= maxTokens
}
