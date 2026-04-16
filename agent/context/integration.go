package context

import (
	"context"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Agent ContextManager是代理的标准上下文管理组件.
// 它将工程师包裹在具有特定代理功能的容器上.
type AgentContextManager struct {
	engineer      *Engineer
	summaryFunc   func(context.Context, []types.Message) (string, error)
	logger        *zap.Logger
	enableMetrics bool
}

// Agent ContextConfig 配置代理上下文管理器.
type AgentContextConfig struct {
	// Max ContextTokens是模型的上下文窗口大小.
	MaxContextTokens int `json:"max_context_tokens"`

	// 储备输出为模型输出保留符.
	ReserveForOutput int `json:"reserve_for_output"`

	// 策略决定了压缩行为.
	Strategy Strategy `json:"strategy"`

	// 启用度量衡允许压缩度量衡收集 。
	EnableMetrics bool `json:"enable_metrics"`
}

// 默认 Agent ContextConfig 返回常见模型的默认值。
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

// NewAgent ContextManager为代理创建上下文管理器.
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

// SetSummary Provider 设置基于 LLM 的汇总函数.
func (m *AgentContextManager) SetSummaryProvider(fn func(context.Context, []types.Message) (string, error)) {
	m.summaryFunc = fn
}

// ReadyMessages在发送到 LLM 之前优化消息.
func (m *AgentContextManager) PrepareMessages(
	ctx context.Context,
	messages []types.Message,
	currentQuery string,
) ([]types.Message, error) {
	return m.engineer.MustFit(ctx, messages, currentQuery)
}

// GetState 返回当前上下文状态 。
func (m *AgentContextManager) GetStatus(messages []types.Message) Status {
	return m.engineer.GetStatus(messages)
}

// CanAddMessage 检查是否可以不溢出添加消息 。
func (m *AgentContextManager) CanAddMessage(messages []types.Message, newMsg types.Message) bool {
	return m.engineer.CanAddMessage(messages, newMsg)
}

// 估计Tokens返回消息的符号数 。
func (m *AgentContextManager) EstimateTokens(messages []types.Message) int {
	return m.engineer.EstimateTokens(messages)
}

// GetStats 返回压缩统计.
func (m *AgentContextManager) GetStats() Stats {
	return m.engineer.GetStats()
}

// 如果建议压缩, 则应该压缩检查 。
func (m *AgentContextManager) ShouldCompress(messages []types.Message) bool {
	status := m.engineer.GetStatus(messages)
	return status.Level >= LevelNormal
}

// Get Agreement return a human可读的推荐。
func (m *AgentContextManager) GetRecommendation(messages []types.Message) string {
	status := m.engineer.GetStatus(messages)
	return status.Recommendation
}
