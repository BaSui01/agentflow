package context

import (
	"encoding/json"
	"fmt"
	"sort"

	"go.uber.org/zap"
)

// Role 消息角色（本地定义避免循环依赖）
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall 工具调用
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Message 消息（本地定义避免循环依赖）
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Metadata   any        `json:"metadata,omitempty"`
}

// ContextManager 定义上下文管理器接口。
type ContextManager interface {
	// TrimMessages 裁剪消息列表以适应 token 限制
	TrimMessages(msgs []Message, maxTokens int) ([]Message, error)

	// PruneByStrategy 根据策略裁剪消息
	PruneByStrategy(msgs []Message, maxTokens int, strategy PruneStrategy) ([]Message, error)

	// EstimateTokens 估算消息列表的 token 数
	EstimateTokens(msgs []Message) int
}

// PruneStrategy 定义裁剪策略。
type PruneStrategy string

const (
	// PruneOldest 优先删除最旧的消息（保留 System 和最近的消息）
	PruneOldest PruneStrategy = "oldest"

	// PruneByRole 按角色优先级裁剪（User/Assistant > Tool > System）
	PruneByRole PruneStrategy = "by_role"

	// PruneLeastImportant 按重要性裁剪（需要消息带 Metadata 中的 importance 字段）
	PruneLeastImportant PruneStrategy = "least_important"

	// PruneSlidingWindow 滑动窗口（保留最近 N 条消息）
	PruneSlidingWindow PruneStrategy = "sliding_window"

	// PruneToolCalls 优先删除工具调用和结果
	PruneToolCalls PruneStrategy = "tool_calls"
)

// ====== 实现：DefaultContextManager ======

type DefaultContextManager struct {
	tokenizer Tokenizer
	logger    *zap.Logger
}

// NewDefaultContextManager 创建默认的上下文管理器。
func NewDefaultContextManager(tokenizer Tokenizer, logger *zap.Logger) *DefaultContextManager {
	return &DefaultContextManager{
		tokenizer: tokenizer,
		logger:    logger,
	}
}

func (m *DefaultContextManager) EstimateTokens(msgs []Message) int {
	return m.tokenizer.CountMessagesTokens(msgs)
}

func (m *DefaultContextManager) TrimMessages(msgs []Message, maxTokens int) ([]Message, error) {
	// 默认使用 PruneOldest 策略
	return m.PruneByStrategy(msgs, maxTokens, PruneOldest)
}

func (m *DefaultContextManager) PruneByStrategy(msgs []Message, maxTokens int, strategy PruneStrategy) ([]Message, error) {
	currentTokens := m.EstimateTokens(msgs)

	// 如果已经在限制内，直接返回
	if currentTokens <= maxTokens {
		m.logger.Debug("messages within token limit",
			zap.Int("current", currentTokens),
			zap.Int("max", maxTokens))
		return msgs, nil
	}

	m.logger.Info("pruning messages",
		zap.Int("current", currentTokens),
		zap.Int("max", maxTokens),
		zap.String("strategy", string(strategy)))

	switch strategy {
	case PruneOldest:
		return m.pruneOldest(msgs, maxTokens)
	case PruneByRole:
		return m.pruneByRole(msgs, maxTokens)
	case PruneLeastImportant:
		return m.pruneLeastImportant(msgs, maxTokens)
	case PruneSlidingWindow:
		return m.pruneSlidingWindow(msgs, maxTokens)
	case PruneToolCalls:
		return m.pruneToolCalls(msgs, maxTokens)
	default:
		return nil, fmt.Errorf("unknown prune strategy: %s", strategy)
	}
}

// ====== 裁剪策略实现 ======

// pruneOldest 删除最旧的消息，保留 System 消息和最近的消息。
func (m *DefaultContextManager) pruneOldest(msgs []Message, maxTokens int) ([]Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}

	// 分离 System 消息和其他消息
	var systemMsgs []Message
	var otherMsgs []Message

	for _, msg := range msgs {
		if msg.Role == RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	// 计算 System 消息的 token 数
	systemTokens := m.EstimateTokens(systemMsgs)
	if systemTokens >= maxTokens {
		// System 消息本身就超过限制，需要裁剪 System 消息
		return m.trimSystemMessages(systemMsgs, maxTokens)
	}

	// 剩余 token 预算
	remainingTokens := maxTokens - systemTokens

	// 从后往前保留消息
	result := make([]Message, 0)
	currentTokens := 0

	for i := len(otherMsgs) - 1; i >= 0; i-- {
		msgTokens := m.tokenizer.CountMessageTokens(otherMsgs[i])
		if currentTokens+msgTokens <= remainingTokens {
			result = append(result, otherMsgs[i])
			currentTokens += msgTokens
		} else {
			break
		}
	}

	// 反转结果（因为是从后往前添加的）
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	// 合并 System 消息和其他消息
	final := append(systemMsgs, result...)

	m.logger.Info("pruned messages",
		zap.Int("original", len(msgs)),
		zap.Int("pruned", len(final)),
		zap.Int("tokens", m.EstimateTokens(final)))

	return final, nil
}

// pruneByRole 按角色优先级裁剪。
func (m *DefaultContextManager) pruneByRole(msgs []Message, maxTokens int) ([]Message, error) {
	// 角色优先级：System > User/Assistant > Tool
	rolesPriority := map[Role]int{
		RoleSystem:    3,
		RoleUser:      2,
		RoleAssistant: 2,
		RoleTool:      1,
	}

	// 按优先级排序
	type msgWithPriority struct {
		msg      Message
		priority int
		index    int // 原始索引
	}

	weighted := make([]msgWithPriority, len(msgs))
	for i, msg := range msgs {
		weighted[i] = msgWithPriority{
			msg:      msg,
			priority: rolesPriority[msg.Role],
			index:    i,
		}
	}

	// 按优先级降序排序，优先级相同时保持原顺序
	sort.Slice(weighted, func(i, j int) bool {
		if weighted[i].priority != weighted[j].priority {
			return weighted[i].priority > weighted[j].priority
		}
		return weighted[i].index < weighted[j].index
	})

	// 选择消息直到达到 token 限制
	selected := make([]msgWithPriority, 0)
	currentTokens := 0

	for _, w := range weighted {
		msgTokens := m.tokenizer.CountMessageTokens(w.msg)
		if currentTokens+msgTokens <= maxTokens {
			selected = append(selected, w)
			currentTokens += msgTokens
		}
	}

	// 恢复原始顺序
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].index < selected[j].index
	})

	result := make([]Message, len(selected))
	for i, w := range selected {
		result[i] = w.msg
	}

	return result, nil
}

// pruneLeastImportant 按重要性裁剪（需要 Metadata.importance）。
func (m *DefaultContextManager) pruneLeastImportant(msgs []Message, maxTokens int) ([]Message, error) {
	// 按 importance 排序
	type msgWithImportance struct {
		msg        Message
		importance float64
		index      int
	}

	weighted := make([]msgWithImportance, len(msgs))
	for i, msg := range msgs {
		importance := 1.0 // 默认重要性
		if msg.Metadata != nil {
			if imp, ok := msg.Metadata.(map[string]interface{})["importance"].(float64); ok {
				importance = imp
			}
		}

		weighted[i] = msgWithImportance{
			msg:        msg,
			importance: importance,
			index:      i,
		}
	}

	// 按重要性降序排序
	sort.Slice(weighted, func(i, j int) bool {
		if weighted[i].importance != weighted[j].importance {
			return weighted[i].importance > weighted[j].importance
		}
		return weighted[i].index < weighted[j].index
	})

	// 选择消息
	selected := make([]msgWithImportance, 0)
	currentTokens := 0

	for _, w := range weighted {
		msgTokens := m.tokenizer.CountMessageTokens(w.msg)
		if currentTokens+msgTokens <= maxTokens {
			selected = append(selected, w)
			currentTokens += msgTokens
		}
	}

	// 恢复原始顺序
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].index < selected[j].index
	})

	result := make([]Message, len(selected))
	for i, w := range selected {
		result[i] = w.msg
	}

	return result, nil
}

// pruneSlidingWindow 滑动窗口策略（保留最近 N 条消息）。
func (m *DefaultContextManager) pruneSlidingWindow(msgs []Message, maxTokens int) ([]Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}

	// 从后往前累积消息，直到超过 token 限制
	result := make([]Message, 0)
	currentTokens := 0

	for i := len(msgs) - 1; i >= 0; i-- {
		msgTokens := m.tokenizer.CountMessageTokens(msgs[i])
		if currentTokens+msgTokens <= maxTokens {
			result = append(result, msgs[i])
			currentTokens += msgTokens
		} else {
			break
		}
	}

	// 反转结果
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}

// pruneToolCalls 优先删除工具调用和结果。
func (m *DefaultContextManager) pruneToolCalls(msgs []Message, maxTokens int) ([]Message, error) {
	// 分离工具相关消息和其他消息
	var toolMsgs []Message
	var otherMsgs []Message

	for _, msg := range msgs {
		if msg.Role == RoleTool || len(msg.ToolCalls) > 0 {
			toolMsgs = append(toolMsgs, msg)
		} else {
			otherMsgs = append(otherMsgs, msg)
		}
	}

	// 优先保留非工具消息
	otherTokens := m.EstimateTokens(otherMsgs)
	if otherTokens <= maxTokens {
		// 非工具消息已经在限制内，尝试添加一些工具消息
		remainingTokens := maxTokens - otherTokens
		result := append([]Message{}, otherMsgs...)

		for _, toolMsg := range toolMsgs {
			msgTokens := m.tokenizer.CountMessageTokens(toolMsg)
			if msgTokens <= remainingTokens {
				result = append(result, toolMsg)
				remainingTokens -= msgTokens
			}
		}

		return result, nil
	}

	// 非工具消息本身超过限制，裁剪非工具消息
	return m.pruneOldest(otherMsgs, maxTokens)
}

// trimSystemMessages 裁剪 System 消息（当 System 消息本身超过限制时）。
func (m *DefaultContextManager) trimSystemMessages(systemMsgs []Message, maxTokens int) ([]Message, error) {
	if len(systemMsgs) == 0 {
		return systemMsgs, nil
	}

	// 只保留第一条 System 消息，并尝试裁剪内容
	firstMsg := systemMsgs[0]
	firstTokens := m.tokenizer.CountMessageTokens(firstMsg)

	if firstTokens <= maxTokens {
		return []Message{firstMsg}, nil
	}

	// 裁剪 System 消息的内容
	// 简单策略：按字符比例裁剪
	ratio := float64(maxTokens) / float64(firstTokens)
	targetLen := int(float64(len(firstMsg.Content)) * ratio)

	if targetLen < 100 {
		return nil, fmt.Errorf("system message too long to fit in context")
	}

	firstMsg.Content = firstMsg.Content[:targetLen] + "...[truncated]"
	return []Message{firstMsg}, nil
}

// ====== 高级功能 ======

// SummarizeOldMessages 对旧消息进行摘要（需要调用 LLM）。
// 这是一个占位函数，实际实现需要调用 LLM 生成摘要。
func (m *DefaultContextManager) SummarizeOldMessages(msgs []Message, summaryProvider func([]Message) (string, error)) (Message, error) {
	if len(msgs) == 0 {
		return Message{}, fmt.Errorf("no messages to summarize")
	}

	summary, err := summaryProvider(msgs)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Role:    RoleSystem,
		Content: fmt.Sprintf("[Summary of previous conversation]\n%s", summary),
	}, nil
}
