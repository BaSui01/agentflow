package rag

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// SimpleContextProvider 基于模板的简单上下文提供器。
// 不依赖 LLM，通过提取文档元数据来生成上下文摘要，
// 适用于本地开发、测试和不需要 LLM 调用的场景。
type SimpleContextProvider struct {
	mu     sync.RWMutex
	cache  map[string]string // docID+chunk -> context
	logger *zap.Logger
}

// NewSimpleContextProvider 创建简单上下文提供器。
func NewSimpleContextProvider(logger *zap.Logger) *SimpleContextProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SimpleContextProvider{
		cache:  make(map[string]string),
		logger: logger.With(zap.String("component", "context_provider_simple")),
	}
}

// GenerateContext 为 chunk 生成上下文。
// 基于文档元数据（title、section）和 chunk 内容生成简要上下文描述。
func (p *SimpleContextProvider) GenerateContext(ctx context.Context, doc Document, chunk string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// 构造缓存 key
	cacheKey := fmt.Sprintf("%s:%d", doc.ID, hashString(chunk))

	p.mu.RLock()
	if cached, ok := p.cache[cacheKey]; ok {
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	// 提取文档元数据
	title := getMetadataString(doc.Metadata, "title")
	section := getMetadataString(doc.Metadata, "section")

	// 构建上下文描述
	var parts []string
	if title != "" {
		parts = append(parts, fmt.Sprintf("This chunk is from the document titled %q", title))
	}
	if section != "" {
		parts = append(parts, fmt.Sprintf("under the section %q", section))
	}

	// 提取 chunk 的前几个词作为内容提示
	preview := truncateText(chunk, 80)
	if preview != "" {
		parts = append(parts, fmt.Sprintf("covering: %s", preview))
	}

	result := strings.Join(parts, ", ")
	if result == "" {
		result = "General content chunk"
	}

	// 写入缓存
	p.mu.Lock()
	p.cache[cacheKey] = result
	p.mu.Unlock()

	p.logger.Debug("context generated",
		zap.String("doc_id", doc.ID),
		zap.Int("context_len", len(result)))

	return result, nil
}

// hashString 简单字符串哈希，用于缓存 key。
func hashString(s string) uint64 {
	var h uint64
	for _, c := range s {
		h = h*31 + uint64(c)
	}
	return h
}

// truncateText 截断文本到指定长度，在词边界处截断。
func truncateText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	// 在词边界处截断
	truncated := s[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "..."
}
