package agent

import (
	"context"
	"time"
)

// MemoryKind 记忆类型
type MemoryKind string

const (
	MemoryShortTerm MemoryKind = "short_term" // 短期记忆（Redis）
	MemoryWorking   MemoryKind = "working"    // 工作记忆（ReAct traces）
	MemoryLongTerm  MemoryKind = "long_term"  // 长期记忆（PostgreSQL/Qdrant）
	MemoryEpisodic  MemoryKind = "episodic"   // 情节记忆（Vector Store）
)

// MemoryRecord 统一记忆结构
type MemoryRecord struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agent_id"`
	Kind      MemoryKind     `json:"kind"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	VectorID  string         `json:"vector_id,omitempty"` // Qdrant 向量 ID
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"` // 短期记忆过期时间
}

// MemoryWriter 记忆写入接口
type MemoryWriter interface {
	// Save 保存记忆
	Save(ctx context.Context, rec MemoryRecord) error
	// Delete 删除记忆
	Delete(ctx context.Context, id string) error
	// Clear 清空 Agent 的所有记忆
	Clear(ctx context.Context, agentID string, kind MemoryKind) error
}

// MemoryReader 记忆读取接口
type MemoryReader interface {
	// LoadRecent 加载最近的记忆（按时间倒序）
	LoadRecent(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error)
	// Search 语义检索（长期记忆）
	Search(ctx context.Context, agentID string, query string, topK int) ([]MemoryRecord, error)
	// Get 获取单条记忆
	Get(ctx context.Context, id string) (*MemoryRecord, error)
}

// MemoryManager 组合读写接口
type MemoryManager interface {
	MemoryWriter
	MemoryReader
}
