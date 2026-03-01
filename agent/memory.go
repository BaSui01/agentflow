package agent

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// MemoryKind 记忆类型。
type MemoryKind string

// 记忆类型常数 - 映射到统一类型. 内存类型
// 折旧:使用类型。 记忆工作,类型。 记忆 Essodic等.
const (
	MemoryShortTerm  MemoryKind = MemoryKind(types.MemoryWorking)    // 短期记忆 -> Working
	MemoryWorking    MemoryKind = MemoryKind(types.MemoryWorking)    // 工作记忆
	MemoryLongTerm   MemoryKind = MemoryKind(types.MemorySemantic)   // 长期记忆 -> Semantic
	MemoryEpisodic   MemoryKind = MemoryKind(types.MemoryEpisodic)   // 情节记忆
	MemorySemantic   MemoryKind = MemoryKind(types.MemorySemantic)   // 语义记忆
	MemoryProcedural MemoryKind = MemoryKind(types.MemoryProcedural) // 程序记忆
)

// MemoryRecord 统一记忆结构
// 用途类型。 用于 Kind 字段的内存类型,以确保一致性 。
type MemoryRecord struct {
	ID        string               `json:"id"`
	AgentID   string               `json:"agent_id"`
	Kind      types.MemoryCategory `json:"kind"`
	Content   string               `json:"content"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
	VectorID  string               `json:"vector_id,omitempty"` // Qdrant 向量 ID
	CreatedAt time.Time            `json:"created_at"`
	// Deprecated: ExpiresAt is reserved for future TTL-based expiration.
	// Currently unused; memory expiration is handled by the MemoryManager implementation.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
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

