package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	llmpkg "github.com/yourusername/agentflow/llm"
)

// HierarchicalKeyStrategy 层次化缓存键策略
// 格式：llm:cache:{tenantID}:{model}:{msgHash}
// msgHash 只包含系统消息 + 历史消息（不含最后一条用户消息）
// 这样多轮对话的前 N-1 轮可以共享缓存前缀
type HierarchicalKeyStrategy struct{}

// Name 返回策略名称
func (s *HierarchicalKeyStrategy) Name() string {
	return "hierarchical"
}

// GenerateKey 生成层次化缓存键
func (s *HierarchicalKeyStrategy) GenerateKey(req *llmpkg.ChatRequest) string {
	// 基础键：tenant:model
	baseKey := fmt.Sprintf("llm:cache:%s:%s", req.TenantID, req.Model)

	// 提取历史消息（不含最后一条）
	var msgSlice []llmpkg.Message
	if len(req.Messages) > 0 {
		// 只包含前 N-1 条消息
		msgSlice = req.Messages[:len(req.Messages)-1]
	}

	// 如果没有历史消息，使用特殊标记
	if len(msgSlice) == 0 {
		return baseKey + ":initial"
	}

	// 计算消息 Hash
	msgHash := hashMessages(msgSlice)

	return fmt.Sprintf("%s:%s", baseKey, msgHash)
}

// hashMessages 计算消息列表的 Hash
func hashMessages(msgs []llmpkg.Message) string {
	data, _ := json.Marshal(msgs)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:12]) // 使用前 12 字节
}

// NewHierarchicalKeyStrategy 创建层次化策略
func NewHierarchicalKeyStrategy() *HierarchicalKeyStrategy {
	return &HierarchicalKeyStrategy{}
}
