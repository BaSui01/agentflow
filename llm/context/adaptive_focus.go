package context

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// AdaptiveFocusMemory Adaptive Focus Memory（2025 最新）
// 三级保真度：Full / Compressed / Placeholder
// 基于语义相关性 + 时间衰减动态调整
type AdaptiveFocusMemory struct {
	config    AdaptiveFocusConfig
	compressor *SummaryCompressor
	logger    *zap.Logger
}

// AdaptiveFocusConfig Adaptive Focus 配置
type AdaptiveFocusConfig struct {
	// 保真度阈值
	FullThreshold        float64 `json:"full_threshold"`        // Full 保真度阈值（0.7-0.9）
	CompressedThreshold  float64 `json:"compressed_threshold"`  // Compressed 阈值（0.3-0.6）
	
	// 时间衰减
	TimeDecayFactor      float64 `json:"time_decay_factor"`     // 时间衰减因子（0.1-0.5）
	RecentWindowMinutes  int     `json:"recent_window_minutes"` // 最近窗口（分钟）
	
	// 压缩参数
	CompressionRatio     float64 `json:"compression_ratio"`     // 压缩比（0.2-0.5）
	PlaceholderTemplate  string  `json:"placeholder_template"`  // 占位符模板
}

// DefaultAdaptiveFocusConfig 默认配置
func DefaultAdaptiveFocusConfig() AdaptiveFocusConfig {
	return AdaptiveFocusConfig{
		FullThreshold:       0.75,
		CompressedThreshold: 0.45,
		TimeDecayFactor:     0.3,
		RecentWindowMinutes: 30,
		CompressionRatio:    0.3,
		PlaceholderTemplate: "[Earlier: {{count}} messages about {{topic}}]",
	}
}

// FidelityLevel 保真度级别
type FidelityLevel string

const (
	FidelityFull        FidelityLevel = "full"        // 完整保留
	FidelityCompressed  FidelityLevel = "compressed"  // 压缩摘要
	FidelityPlaceholder FidelityLevel = "placeholder" // 占位符
)

// MessageWithFidelity 带保真度的消息
type MessageWithFidelity struct {
	Message       Message
	Fidelity      FidelityLevel
	RelevanceScore float64
	Timestamp     time.Time
	Compressed    string // 压缩后的内容
}

// NewAdaptiveFocusMemory 创建 Adaptive Focus Memory
func NewAdaptiveFocusMemory(
	config AdaptiveFocusConfig,
	compressor *SummaryCompressor,
	logger *zap.Logger,
) *AdaptiveFocusMemory {
	return &AdaptiveFocusMemory{
		config:     config,
		compressor: compressor,
		logger:     logger,
	}
}

// ManageContext 管理上下文（动态调整保真度）
func (m *AdaptiveFocusMemory) ManageContext(
	ctx context.Context,
	messages []Message,
	currentQuery string,
	maxTokens int,
) ([]Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	
	m.logger.Info("adaptive focus memory managing context",
		zap.Int("messages", len(messages)),
		zap.Int("max_tokens", maxTokens))
	
	// 1. 为每条消息分配保真度级别
	withFidelity := m.assignFidelityLevels(messages, currentQuery)
	
	// 2. 应用保真度转换
	transformed, err := m.applyFidelity(ctx, withFidelity)
	if err != nil {
		return nil, err
	}
	
	// 3. 检查 token 限制
	// 如果仍超过限制，降级更多消息
	for {
		tokenCount := m.estimateTokens(transformed)
		if tokenCount <= maxTokens {
			break
		}
		
		// 降级一个 Full 消息到 Compressed
		downgraded := false
		for i := range withFidelity {
			if withFidelity[i].Fidelity == FidelityFull {
				withFidelity[i].Fidelity = FidelityCompressed
				downgraded = true
				break
			}
		}
		
		if !downgraded {
			// 降级一个 Compressed 到 Placeholder
			for i := range withFidelity {
				if withFidelity[i].Fidelity == FidelityCompressed {
					withFidelity[i].Fidelity = FidelityPlaceholder
					downgraded = true
					break
				}
			}
		}
		
		if !downgraded {
			break // 无法进一步降级
		}
		
		transformed, err = m.applyFidelity(ctx, withFidelity)
		if err != nil {
			return nil, err
		}
	}
	
	m.logger.Info("adaptive focus completed",
		zap.Int("full", m.countFidelity(withFidelity, FidelityFull)),
		zap.Int("compressed", m.countFidelity(withFidelity, FidelityCompressed)),
		zap.Int("placeholder", m.countFidelity(withFidelity, FidelityPlaceholder)))
	
	return transformed, nil
}

// assignFidelityLevels 分配保真度级别
func (m *AdaptiveFocusMemory) assignFidelityLevels(messages []Message, currentQuery string) []MessageWithFidelity {
	now := time.Now()
	result := make([]MessageWithFidelity, len(messages))
	
	for i, msg := range messages {
		// 计算相关性分数
		relevance := m.calculateRelevance(msg, currentQuery)
		
		// 计算时间衰减
		// 假设消息按时间顺序，越新的消息越重要
		recency := float64(i) / float64(len(messages))
		
		// 检查是否在最近窗口内
		isRecent := i >= len(messages)-10 // 简化：最近 10 条
		
		// 时间衰减
		timeDecay := 1.0
		if !isRecent {
			timeDecay = 1.0 - m.config.TimeDecayFactor*(1.0-recency)
		}
		
		// 重要性分类（基于角色）
		// System > User > Tool/Assistant
		importance := 0.5 // 默认
		switch msg.Role {
		case RoleSystem:
			importance = 1.0
		case RoleUser:
			importance = 0.8
		case RoleAssistant:
			importance = 0.5
		case RoleTool:
			importance = 0.4
		}
		
		// 综合分数（基于 2025 最佳实践）
		// 相关性 50% + 时间新近度 30% + 重要性 20%
		score := relevance*0.5 + recency*0.3 + importance*0.2
		score *= timeDecay
		
		// 分配保真度
		var fidelity FidelityLevel
		if score >= m.config.FullThreshold || isRecent {
			fidelity = FidelityFull
		} else if score >= m.config.CompressedThreshold {
			fidelity = FidelityCompressed
		} else {
			fidelity = FidelityPlaceholder
		}
		
		// System 消息始终保持 Full
		if msg.Role == RoleSystem {
			fidelity = FidelityFull
		}
		
		result[i] = MessageWithFidelity{
			Message:        msg,
			Fidelity:       fidelity,
			RelevanceScore: score,
			Timestamp:      now.Add(-time.Duration(len(messages)-i) * time.Minute),
		}
	}
	
	return result
}

// calculateRelevance 计算相关性
func (m *AdaptiveFocusMemory) calculateRelevance(msg Message, query string) float64 {
	if query == "" {
		return 0.5
	}
	
	// 简化实现：词重叠
	queryWords := extractWords(query)
	msgWords := extractWords(msg.Content)
	
	if len(queryWords) == 0 {
		return 0.5
	}
	
	matchCount := 0
	for _, qw := range queryWords {
		for _, mw := range msgWords {
			if qw == mw {
				matchCount++
				break
			}
		}
	}
	
	return float64(matchCount) / float64(len(queryWords))
}

// applyFidelity 应用保真度转换
func (m *AdaptiveFocusMemory) applyFidelity(ctx context.Context, withFidelity []MessageWithFidelity) ([]Message, error) {
	result := []Message{}
	
	// 分组连续的 Placeholder 消息
	i := 0
	for i < len(withFidelity) {
		msg := withFidelity[i]
		
		switch msg.Fidelity {
		case FidelityFull:
			// 完整保留
			result = append(result, msg.Message)
			i++
			
		case FidelityCompressed:
			// 压缩
			if msg.Compressed == "" {
				// 生成压缩版本
				compressed, err := m.compressMessage(ctx, msg.Message)
				if err != nil {
					m.logger.Warn("compression failed, using original",
						zap.Error(err))
					result = append(result, msg.Message)
				} else {
					compressedMsg := msg.Message
					compressedMsg.Content = compressed
					result = append(result, compressedMsg)
				}
			} else {
				compressedMsg := msg.Message
				compressedMsg.Content = msg.Compressed
				result = append(result, compressedMsg)
			}
			i++
			
		case FidelityPlaceholder:
			// 收集连续的 Placeholder
			placeholderGroup := []MessageWithFidelity{msg}
			j := i + 1
			for j < len(withFidelity) && withFidelity[j].Fidelity == FidelityPlaceholder {
				placeholderGroup = append(placeholderGroup, withFidelity[j])
				j++
			}
			
			// 创建占位符
			placeholder := m.createPlaceholder(placeholderGroup)
			result = append(result, placeholder)
			i = j
		}
	}
	
	return result, nil
}

// compressMessage 压缩消息
func (m *AdaptiveFocusMemory) compressMessage(ctx context.Context, msg Message) (string, error) {
	if m.compressor == nil {
		// 简单压缩：截断
		maxLen := int(float64(len(msg.Content)) * m.config.CompressionRatio)
		if maxLen < 50 {
			maxLen = 50
		}
		if len(msg.Content) > maxLen {
			return msg.Content[:maxLen] + "...", nil
		}
		return msg.Content, nil
	}
	
	// 使用 LLM 压缩
	compressed, err := m.compressor.Compress(ctx, []Message{msg})
	if err != nil {
		return "", err
	}
	
	if len(compressed) > 0 {
		return compressed[0].Content, nil
	}
	
	return msg.Content, nil
}

// createPlaceholder 创建占位符
func (m *AdaptiveFocusMemory) createPlaceholder(group []MessageWithFidelity) Message {
	count := len(group)
	
	// 提取主题（简化：使用第一条消息的前几个词）
	topic := "conversation"
	if len(group) > 0 && len(group[0].Message.Content) > 0 {
		words := extractWords(group[0].Message.Content)
		if len(words) > 0 {
			topic = words[0]
			if len(words) > 1 {
				topic += " " + words[1]
			}
		}
	}
	
	// 渲染占位符
	placeholder := m.config.PlaceholderTemplate
	placeholder = fmt.Sprintf("[Earlier: %d messages about %s]", count, topic)
	
	return Message{
		Role:    RoleSystem,
		Content: placeholder,
	}
}

// estimateTokens 估算 token 数
func (m *AdaptiveFocusMemory) estimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4 // 简化估算
	}
	return total
}

// countFidelity 统计保真度级别数量
func (m *AdaptiveFocusMemory) countFidelity(messages []MessageWithFidelity, level FidelityLevel) int {
	count := 0
	for _, msg := range messages {
		if msg.Fidelity == level {
			count++
		}
	}
	return count
}

// extractWords 提取单词
func extractWords(text string) []string {
	words := []string{}
	current := ""
	
	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\t' || r == ',' || r == '.' {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	
	if current != "" {
		words = append(words, current)
	}
	
	return words
}
