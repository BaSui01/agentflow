package context

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// 工程师是代理商的统一上下文管理部分.
// 它处理压缩,打压,以及适应性聚焦策略.
type Engineer struct {
	config    Config
	tokenizer types.Tokenizer
	logger    *zap.Logger

	mu    sync.RWMutex
	stats Stats
}

// 配置定义上下文工程配置 。
type Config struct {
	MaxContextTokens int      `json:"max_context_tokens"`
	ReserveForOutput int      `json:"reserve_for_output"`
	SoftLimit        float64  `json:"soft_limit"`
	WarnLimit        float64  `json:"warn_limit"`
	HardLimit        float64  `json:"hard_limit"`
	TargetUsage      float64  `json:"target_usage"`
	Strategy         Strategy `json:"strategy"`
}

// 战略确定了背景管理战略。
type Strategy string

const (
	StrategyAdaptive   Strategy = "adaptive"
	StrategySummary    Strategy = "summary"
	StrategyPruning    Strategy = "pruning"
	StrategySliding    Strategy = "sliding"
	StrategyAggressive Strategy = "aggressive"
)

// 关卡代表压缩紧急关卡.
type Level int

const (
	LevelNone Level = iota
	LevelNormal
	LevelAggressive
	LevelEmergency
)

// String returns the string representation of Level.
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

// Stats跟踪上下文工程统计.
type Stats struct {
	TotalCompressions   int64   `json:"total_compressions"`
	EmergencyCount      int64   `json:"emergency_count"`
	AvgCompressionRatio float64 `json:"avg_compression_ratio"`
	TokensSaved         int64   `json:"tokens_saved"`
}

// 状况代表当前背景状况。
type Status struct {
	CurrentTokens  int     `json:"current_tokens"`
	MaxTokens      int     `json:"max_tokens"`
	UsageRatio     float64 `json:"usage_ratio"`
	Level          Level   `json:"level"`
	Recommendation string  `json:"recommendation"`
}

// 默认Config返回200k上下文模型的合理默认值.
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

// 新创建了新的上下文工程师.
func New(config Config, logger *zap.Logger) *Engineer {
	return &Engineer{
		config:    config,
		tokenizer: types.NewEstimateTokenizer(),
		logger:    logger,
	}
}

// GetState 返回当前上下文状态 。
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

// 根据当前上下文使用管理处理信件 。
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

// EmergencyCompress处理紧急环境溢出。
func (e *Engineer) emergencyCompress(msgs []types.Message) ([]types.Message, error) {
	e.logger.Warn("EMERGENCY compression triggered")

	e.mu.Lock()
	e.stats.EmergencyCount++
	e.mu.Unlock()

	originalTokens := e.tokenizer.CountMessagesTokens(msgs)
	targetTokens := int(float64(e.config.MaxContextTokens-e.config.ReserveForOutput) * e.config.TargetUsage)

	// 单独的系统和其他信息
	var systemMsgs, otherMsgs []types.Message
	for _, msg := range msgs {
		if msg.Role == types.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	// 如果系统消息时间太长, 则中断
	systemMsgs = e.truncateMessages(systemMsgs, 4000)
	systemTokens := e.tokenizer.CountMessagesTokens(systemMsgs)

	remainingBudget := targetTokens - systemTokens
	if remainingBudget < 1000 {
		remainingBudget = 1000
	}

	// 只保留上两个消息
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

	// 创建摘要
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

// 积极的Compress处理高上下文使用率(85-95%).
func (e *Engineer) aggressiveCompress(msgs []types.Message, _ string) ([]types.Message, error) {
	e.logger.Info("aggressive compression triggered")

	originalTokens := e.tokenizer.CountMessagesTokens(msgs)

	// 首先做正常压缩
	result, err := e.normalCompress(msgs)
	if err != nil {
		return nil, err
	}

	// 如果还超过目标,再往前划
	targetTokens := int(float64(e.config.MaxContextTokens-e.config.ReserveForOutput) * e.config.TargetUsage)
	if e.tokenizer.CountMessagesTokens(result) > targetTokens {
		result = e.truncateMessages(result, 2000)

		// 如果仍然结束, 删除中间信件
		for e.tokenizer.CountMessagesTokens(result) > targetTokens && len(result) > 3 {
			result = append(result[:1], result[len(result)-2:]...)
		}
	}

	e.updateStats(originalTokens, e.tokenizer.CountMessagesTokens(result))
	return result, nil
}

// 普通Compress处理中度上下文使用(70-85%).
func (e *Engineer) normalCompress(msgs []types.Message) ([]types.Message, error) {
	e.logger.Debug("normal compression triggered")

	// 压缩工具结果
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

// 切换Messages 切换信件, 超过每封信的最大切换量 。
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

// 创建 Emergency 概要创建一个没有 LLM 的最小摘要。
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

	// 显示最后几个非工具消息
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

// GetStats 返回压缩统计.
func (e *Engineer) GetStats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.stats
}

// 必须 适合可确保消息符合上下文窗口。
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

	// 最后手段:硬截断
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

	// 截断系统消息
	for i := range systemMsgs {
		if len(systemMsgs[i].Content) > 1000 {
			systemMsgs[i].Content = systemMsgs[i].Content[:1000] + "\n...[truncated]"
		}
	}

	// 只保留最后两个其他信件
	if len(otherMsgs) > 2 {
		otherMsgs = otherMsgs[len(otherMsgs)-2:]
	}

	result := append(systemMsgs, otherMsgs...)
	if len(result) == 0 {
		return result
	}

	// 确保我们实际符合所要求的预算。
	for tries := 0; tries < 20 && e.tokenizer.CountMessagesTokens(result) > maxTokens; tries++ {
		// 最好先扔出旧的非系统消息
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

		// 左边的单条信息: 快速切入以适应。
		result = e.truncateMessages(result, maxTokens)
	}

	// 如果我们仍然溢出,尝试缩短 与每个消息的预算。
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

// 估计Tokens返回消息的符号数 。
func (e *Engineer) EstimateTokens(msgs []types.Message) int {
	return e.tokenizer.CountMessagesTokens(msgs)
}

// CanAddMessage 检查是否可以添加新信件 。
func (e *Engineer) CanAddMessage(msgs []types.Message, newMsg types.Message) bool {
	currentTokens := e.tokenizer.CountMessagesTokens(msgs)
	newTokens := e.tokenizer.CountMessageTokens(newMsg)
	maxTokens := e.config.MaxContextTokens - e.config.ReserveForOutput
	return currentTokens+newTokens <= maxTokens
}
