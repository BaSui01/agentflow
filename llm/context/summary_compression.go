package context

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// SummaryCompressor 摘要压缩器（基于 LLM）
type SummaryCompressor struct {
	// LLM Provider（用于生成摘要）
	summaryProvider func(context.Context, []Message) (string, error)
	
	// 压缩配置
	config SummaryCompressionConfig
	
	logger *zap.Logger
}

// SummaryCompressionConfig 摘要压缩配置
type SummaryCompressionConfig struct {
	// 触发压缩的阈值
	TriggerTokenCount int     `json:"trigger_token_count"` // 超过此 token 数触发压缩
	TriggerMessageCount int   `json:"trigger_message_count"` // 超过此消息数触发压缩
	
	// 压缩策略
	CompressionRatio float64 `json:"compression_ratio"` // 目标压缩比（0.1-0.5）
	PreserveRecent   int     `json:"preserve_recent"`   // 保留最近 N 条消息不压缩
	PreserveSystem   bool    `json:"preserve_system"`   // 保留 System 消息
	
	// 摘要模板
	SummaryPrompt string `json:"summary_prompt"`
}

// DefaultSummaryCompressionConfig 默认配置
func DefaultSummaryCompressionConfig() SummaryCompressionConfig {
	return SummaryCompressionConfig{
		TriggerTokenCount:   8000,
		TriggerMessageCount: 20,
		CompressionRatio:    0.3,
		PreserveRecent:      5,
		PreserveSystem:      true,
		SummaryPrompt: `请将以下对话历史压缩为简洁的摘要，保留关键信息：

{{messages}}

要求：
1. 提取核心主题和关键决策
2. 保留重要的上下文信息
3. 使用简洁的语言
4. 按时间顺序组织`,
	}
}


// NewSummaryCompressor 创建摘要压缩器
func NewSummaryCompressor(
	summaryProvider func(context.Context, []Message) (string, error),
	config SummaryCompressionConfig,
	logger *zap.Logger,
) *SummaryCompressor {
	return &SummaryCompressor{
		summaryProvider: summaryProvider,
		config:          config,
		logger:          logger,
	}
}

// CompressIfNeeded 根据需要压缩消息
func (c *SummaryCompressor) CompressIfNeeded(ctx context.Context, msgs []Message, tokenizer Tokenizer) ([]Message, error) {
	// 检查是否需要压缩
	if !c.shouldCompress(msgs, tokenizer) {
		return msgs, nil
	}

	c.logger.Info("triggering message compression",
		zap.Int("message_count", len(msgs)))

	return c.Compress(ctx, msgs)
}

// Compress 压缩消息
func (c *SummaryCompressor) Compress(ctx context.Context, msgs []Message) ([]Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}

	// 1. 分离需要保留的消息
	systemMsgs, recentMsgs, toCompress := c.partitionMessages(msgs)

	if len(toCompress) == 0 {
		c.logger.Debug("no messages to compress")
		return msgs, nil
	}

	// 2. 生成摘要
	summary, err := c.generateSummary(ctx, toCompress)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	// 3. 创建摘要消息
	summaryMsg := Message{
		Role:    RoleSystem,
		Content: fmt.Sprintf("[对话摘要]\n%s", summary),
	}

	// 4. 合并消息
	result := []Message{}
	result = append(result, systemMsgs...)
	result = append(result, summaryMsg)
	result = append(result, recentMsgs...)

	c.logger.Info("compression completed",
		zap.Int("original", len(msgs)),
		zap.Int("compressed", len(result)),
		zap.Int("compressed_messages", len(toCompress)))

	return result, nil
}

// shouldCompress 判断是否应该压缩
func (c *SummaryCompressor) shouldCompress(msgs []Message, tokenizer Tokenizer) bool {
	// 检查消息数量
	if len(msgs) > c.config.TriggerMessageCount {
		return true
	}

	// 检查 token 数量
	if tokenizer != nil {
		tokenCount := tokenizer.CountMessagesTokens(msgs)
		if tokenCount > c.config.TriggerTokenCount {
			return true
		}
	}

	return false
}

// partitionMessages 分区消息
func (c *SummaryCompressor) partitionMessages(msgs []Message) (system, recent, toCompress []Message) {
	system = []Message{}
	recent = []Message{}
	toCompress = []Message{}

	// 1. 提取 System 消息
	if c.config.PreserveSystem {
		for _, msg := range msgs {
			if msg.Role == RoleSystem {
				system = append(system, msg)
			}
		}
	}

	// 2. 保留最近的消息
	recentStart := len(msgs) - c.config.PreserveRecent
	if recentStart < 0 {
		recentStart = 0
	}

	for i := recentStart; i < len(msgs); i++ {
		if msgs[i].Role != RoleSystem || !c.config.PreserveSystem {
			recent = append(recent, msgs[i])
		}
	}

	// 3. 需要压缩的消息
	for i := 0; i < recentStart; i++ {
		if msgs[i].Role != RoleSystem || !c.config.PreserveSystem {
			toCompress = append(toCompress, msgs[i])
		}
	}

	return system, recent, toCompress
}

// generateSummary 生成摘要
func (c *SummaryCompressor) generateSummary(ctx context.Context, msgs []Message) (string, error) {
	if c.summaryProvider == nil {
		return c.simpleSummary(msgs), nil
	}

	// 使用 LLM 生成摘要
	summary, err := c.summaryProvider(ctx, msgs)
	if err != nil {
		c.logger.Warn("LLM summary failed, using simple summary", zap.Error(err))
		return c.simpleSummary(msgs), nil
	}

	return summary, nil
}

// simpleSummary 简单摘要（不使用 LLM）
func (c *SummaryCompressor) simpleSummary(msgs []Message) string {
	var parts []string

	// 统计消息类型
	userCount := 0
	assistantCount := 0
	toolCount := 0

	for _, msg := range msgs {
		switch msg.Role {
		case RoleUser:
			userCount++
		case RoleAssistant:
			assistantCount++
		case RoleTool:
			toolCount++
		}
	}

	parts = append(parts, fmt.Sprintf("压缩了 %d 条消息", len(msgs)))
	parts = append(parts, fmt.Sprintf("- 用户消息: %d 条", userCount))
	parts = append(parts, fmt.Sprintf("- 助手回复: %d 条", assistantCount))
	if toolCount > 0 {
		parts = append(parts, fmt.Sprintf("- 工具调用: %d 次", toolCount))
	}

	// 提取关键词
	keywords := c.extractKeywords(msgs)
	if len(keywords) > 0 {
		parts = append(parts, fmt.Sprintf("主要话题: %s", strings.Join(keywords[:min(5, len(keywords))], ", ")))
	}

	return strings.Join(parts, "\n")
}

// extractKeywords 提取关键词
func (c *SummaryCompressor) extractKeywords(msgs []Message) []string {
	// 简化实现：统计词频
	wordCount := make(map[string]int)

	for _, msg := range msgs {
		words := strings.Fields(strings.ToLower(msg.Content))
		for _, word := range words {
			if len(word) > 3 { // 过滤短词
				wordCount[word]++
			}
		}
	}

	// 排序
	type wordFreq struct {
		word  string
		count int
	}

	freqs := []wordFreq{}
	for word, count := range wordCount {
		freqs = append(freqs, wordFreq{word, count})
	}

	sort.Slice(freqs, func(i, j int) bool {
		return freqs[i].count > freqs[j].count
	})

	// 返回 Top-K
	result := []string{}
	for i := 0; i < min(10, len(freqs)); i++ {
		result = append(result, freqs[i].word)
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
