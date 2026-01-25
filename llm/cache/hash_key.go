package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	llmpkg "github.com/BaSui01/agentflow/llm"
)

// HashKeyStrategy Hash 缓存键策略
// 使用全请求 Hash 生成缓存键（原有实现）
type HashKeyStrategy struct{}

// Name 返回策略名称
func (s *HashKeyStrategy) Name() string {
	return "hash"
}

// GenerateKey 生成 Hash 缓存键
func (s *HashKeyStrategy) GenerateKey(req *llmpkg.ChatRequest) string {
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return "llm:cache:" + hex.EncodeToString(hash[:16]) // 使用前 16 字节
}

// NewHashKeyStrategy 创建 Hash 策略
func NewHashKeyStrategy() *HashKeyStrategy {
	return &HashKeyStrategy{}
}
