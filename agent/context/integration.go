// Package context provides agent integration helpers.
package context

import (
	"context"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// AgentContextManager is the standard context management component for agents.
// It wraps Engineer with agent-specific functionality.
type AgentContextManager struct {
	engineer      *Engineer
	summaryFunc   func(context.Context, []types.Message) (string, error)
	logger        *zap.Logger
	enableMetrics bool
}

// AgentContextConfig configures the agent context manager.
type AgentContextConfig struct {
	// MaxContextTokens is the model's context window size.
	MaxContextTokens int `json:"max_context_tokens"`

	// ReserveForOutput reserves tokens for model output.
	ReserveForOutput int `json:"reserve_for_output"`

	// Strategy determines compression behavior.
	Strategy Strategy `json:"strategy"`

	// EnableMetrics enables compression metrics collection.
	EnableMetrics bool `json:"enable_metrics"`
}

// DefaultAgentContextConfig returns defaults for common models.
func DefaultAgentContextConfig(modelFamily string) AgentContextConfig {
	switch modelFamily {
	case "gpt-4", "gpt-4o":
		return AgentContextConfig{
			MaxContextTokens: 128000,
			ReserveForOutput: 4096,
			Strategy:         StrategyAdaptive,
			EnableMetrics:    true,
		}
	case "claude-3", "claude-3.5":
		return AgentContextConfig{
			MaxContextTokens: 200000,
			ReserveForOutput: 8192,
			Strategy:         StrategyAdaptive,
			EnableMetrics:    true,
		}
	case "gemini-1.5", "gemini-2":
		return AgentContextConfig{
			MaxContextTokens: 1000000,
			ReserveForOutput: 8192,
			Strategy:         StrategyAdaptive,
			EnableMetrics:    true,
		}
	default:
		return AgentContextConfig{
			MaxContextTokens: 32000,
			ReserveForOutput: 4096,
			Strategy:         StrategyAdaptive,
			EnableMetrics:    true,
		}
	}
}

// NewAgentContextManager creates a context manager for an agent.
func NewAgentContextManager(cfg AgentContextConfig, logger *zap.Logger) *AgentContextManager {
	engineerCfg := Config{
		MaxContextTokens: cfg.MaxContextTokens,
		ReserveForOutput: cfg.ReserveForOutput,
		SoftLimit:        0.7,
		WarnLimit:        0.85,
		HardLimit:        0.95,
		TargetUsage:      0.5,
		Strategy:         cfg.Strategy,
	}

	return &AgentContextManager{
		engineer:      New(engineerCfg, logger),
		logger:        logger,
		enableMetrics: cfg.EnableMetrics,
	}
}

// SetSummaryProvider sets the LLM-based summary function.
func (m *AgentContextManager) SetSummaryProvider(fn func(context.Context, []types.Message) (string, error)) {
	m.summaryFunc = fn
}

// PrepareMessages optimizes messages before sending to LLM.
func (m *AgentContextManager) PrepareMessages(
	ctx context.Context,
	messages []types.Message,
	currentQuery string,
) ([]types.Message, error) {
	return m.engineer.MustFit(ctx, messages, currentQuery)
}

// GetStatus returns current context status.
func (m *AgentContextManager) GetStatus(messages []types.Message) Status {
	return m.engineer.GetStatus(messages)
}

// CanAddMessage checks if a message can be added without overflow.
func (m *AgentContextManager) CanAddMessage(messages []types.Message, newMsg types.Message) bool {
	return m.engineer.CanAddMessage(messages, newMsg)
}

// EstimateTokens returns token count for messages.
func (m *AgentContextManager) EstimateTokens(messages []types.Message) int {
	return m.engineer.EstimateTokens(messages)
}

// GetStats returns compression statistics.
func (m *AgentContextManager) GetStats() Stats {
	return m.engineer.GetStats()
}

// ShouldCompress checks if compression is recommended.
func (m *AgentContextManager) ShouldCompress(messages []types.Message) bool {
	status := m.engineer.GetStatus(messages)
	return status.Level >= LevelNormal
}

// GetRecommendation returns a human-readable recommendation.
func (m *AgentContextManager) GetRecommendation(messages []types.Message) string {
	status := m.engineer.GetStatus(messages)
	return status.Recommendation
}
