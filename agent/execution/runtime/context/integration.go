package context

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// AgentContextManager is the standard context orchestration component used by agents.
type AgentContextManager struct {
	runtime   *Assembler
	logger    *zap.Logger
	tokenizer types.Tokenizer
	mu        sync.RWMutex
	stats     Stats
}

type Strategy string

const (
	StrategyAdaptive   Strategy = "adaptive"
	StrategySummary    Strategy = "summary"
	StrategyPruning    Strategy = "pruning"
	StrategySliding    Strategy = "sliding"
	StrategyAggressive Strategy = "aggressive"
)

type Level int

const (
	LevelNone Level = iota
	LevelNormal
	LevelAggressive
	LevelEmergency
)

func (l Level) String() string {
	switch l {
	case LevelNone:
		return "none"
	case LevelNormal:
		return "normal"
	case LevelAggressive:
		return "aggressive"
	case LevelEmergency:
		return "emergency"
	default:
		return fmt.Sprintf("Level(%d)", l)
	}
}

type Stats struct {
	TotalCompressions   int64   `json:"total_compressions"`
	EmergencyCount      int64   `json:"emergency_count"`
	AvgCompressionRatio float64 `json:"avg_compression_ratio"`
	TokensSaved         int64   `json:"tokens_saved"`
}

type Status struct {
	CurrentTokens  int     `json:"current_tokens"`
	MaxTokens      int     `json:"max_tokens"`
	UsageRatio     float64 `json:"usage_ratio"`
	Level          Level   `json:"level"`
	Recommendation string  `json:"recommendation"`
}

// AgentContextConfig configures the context runtime.
type AgentContextConfig struct {
	Enabled              bool     `json:"enabled"`
	MaxContextTokens     int      `json:"max_context_tokens"`
	ReserveForOutput     int      `json:"reserve_for_output"`
	SoftLimit            float64  `json:"soft_limit"`
	WarnLimit            float64  `json:"warn_limit"`
	HardLimit            float64  `json:"hard_limit"`
	TargetUsage          float64  `json:"target_usage"`
	KeepSystem           bool     `json:"keep_system"`
	KeepLastN            int      `json:"keep_last_n"`
	EnableSummarize      bool     `json:"enable_summarize"`
	EnableMetrics        bool     `json:"enable_metrics"`
	MemoryBudgetRatio    float64  `json:"memory_budget_ratio"`
	RetrievalBudgetRatio float64  `json:"retrieval_budget_ratio"`
	ToolStateBudgetRatio float64  `json:"tool_state_budget_ratio"`
	Strategy             Strategy `json:"strategy"`
}

func DefaultAgentContextConfig(modelFamily string) AgentContextConfig {
	cfg := AgentContextConfig{
		Enabled:              true,
		MaxContextTokens:     32000,
		ReserveForOutput:     4096,
		SoftLimit:            0.7,
		WarnLimit:            0.85,
		HardLimit:            0.95,
		TargetUsage:          0.5,
		KeepSystem:           true,
		KeepLastN:            2,
		EnableSummarize:      true,
		EnableMetrics:        true,
		MemoryBudgetRatio:    0.2,
		RetrievalBudgetRatio: 0.2,
		ToolStateBudgetRatio: 0.2,
		Strategy:             StrategyAdaptive,
	}
	switch {
	case strings.Contains(modelFamily, "gpt-4"), strings.Contains(modelFamily, "gpt-4o"):
		cfg.MaxContextTokens = 128000
	case strings.Contains(modelFamily, "claude-3"), strings.Contains(modelFamily, "claude-4"):
		cfg.MaxContextTokens = 200000
		cfg.ReserveForOutput = 8192
	case strings.Contains(modelFamily, "gemini-1.5"), strings.Contains(modelFamily, "gemini-2"):
		cfg.MaxContextTokens = 1000000
		cfg.ReserveForOutput = 8192
	}
	return cfg
}

func NewAgentContextManager(cfg AgentContextConfig, logger *zap.Logger) *AgentContextManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentContextManager{
		runtime:   newAssembler(cfg, logger),
		logger:    logger,
		tokenizer: types.NewEstimateTokenizer(),
	}
}

func (m *AgentContextManager) SetSummaryProvider(fn func(context.Context, []types.Message) (string, error)) {
	if fn == nil {
		m.runtime.summarizer = nil
		return
	}
	m.runtime.summarizer = summaryFuncAdapter{fn: fn}
}

func (m *AgentContextManager) Assemble(ctx context.Context, req *AssembleRequest) (*AssembleResult, error) {
	return m.runtime.Assemble(ctx, req)
}

func (m *AgentContextManager) PrepareMessages(ctx context.Context, messages []types.Message, currentQuery string) ([]types.Message, error) {
	before := m.EstimateTokens(messages)
	result, err := m.runtime.Assemble(ctx, &AssembleRequest{
		Conversation: messages,
		Query:        currentQuery,
	})
	if err != nil {
		return nil, err
	}
	m.recordStats(before, m.EstimateTokens(result.Messages), m.GetStatus(messages).Level)
	return result.Messages, nil
}

func (m *AgentContextManager) GetStatus(messages []types.Message) Status {
	currentTokens := m.EstimateTokens(messages)
	effectiveMax := m.runtime.config.MaxContextTokens - m.runtime.config.ReserveForOutput
	usage := 0.0
	if effectiveMax > 0 {
		usage = float64(currentTokens) / float64(effectiveMax)
	}
	status := Status{
		CurrentTokens: currentTokens,
		MaxTokens:     effectiveMax,
		UsageRatio:    usage,
		Level:         m.getLevel(usage),
	}
	switch {
	case usage >= m.runtime.config.HardLimit:
		status.Recommendation = "CRITICAL: immediate compression required"
	case usage >= m.runtime.config.WarnLimit:
		status.Recommendation = "WARNING: compression recommended"
	case usage >= m.runtime.config.SoftLimit:
		status.Recommendation = "INFO: monitoring context usage"
	default:
		status.Recommendation = "OK: context usage normal"
	}
	return status
}

func (m *AgentContextManager) CanAddMessage(messages []types.Message, newMsg types.Message) bool {
	currentTokens := m.EstimateTokens(messages)
	newTokens := m.tokenizer.CountMessageTokens(newMsg)
	maxTokens := m.runtime.config.MaxContextTokens - m.runtime.config.ReserveForOutput
	return currentTokens+newTokens <= maxTokens
}

func (m *AgentContextManager) EstimateTokens(messages []types.Message) int {
	return m.tokenizer.CountMessagesTokens(messages)
}

func (m *AgentContextManager) GetStats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

func (m *AgentContextManager) ShouldCompress(messages []types.Message) bool {
	return m.GetStatus(messages).Level >= LevelNormal
}

func (m *AgentContextManager) GetRecommendation(messages []types.Message) string {
	return m.GetStatus(messages).Recommendation
}

func (m *AgentContextManager) getLevel(usage float64) Level {
	switch {
	case usage >= m.runtime.config.HardLimit:
		return LevelEmergency
	case usage >= m.runtime.config.WarnLimit:
		return LevelAggressive
	case usage >= m.runtime.config.SoftLimit:
		return LevelNormal
	default:
		return LevelNone
	}
}

func (m *AgentContextManager) recordStats(originalTokens, finalTokens int, level Level) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if originalTokens <= 0 {
		return
	}
	m.stats.TotalCompressions++
	if level == LevelEmergency {
		m.stats.EmergencyCount++
	}
	m.stats.TokensSaved += int64(originalTokens - finalTokens)
	ratio := float64(finalTokens) / float64(originalTokens)
	m.stats.AvgCompressionRatio = (m.stats.AvgCompressionRatio*float64(m.stats.TotalCompressions-1) + ratio) / float64(m.stats.TotalCompressions)
}
