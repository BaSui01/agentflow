package cache

import (
	llmpkg "github.com/yourusername/agentflow/llm"
)

// KeyStrategy 缓存键生成策略接口
type KeyStrategy interface {
	// GenerateKey 生成缓存键
	GenerateKey(req *llmpkg.ChatRequest) string

	// Name 返回策略名称（用于日志和调试）
	Name() string
}
