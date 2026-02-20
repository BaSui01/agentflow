package agent

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// MemoryKind 记忆类型
// 折旧:使用类型。 用于新代码的内存类型 。
// 此别名用于后向相容性 。
type MemoryKind = types.MemoryCategory

// 记忆类型常数 - 映射到统一类型. 内存类型
// 折旧:使用类型。 记忆工作,类型。 记忆 Essodic等.
const (
	MemoryShortTerm = types.MemoryWorking   // 短期记忆 -> Working
	MemoryWorking   = types.MemoryWorking   // 工作记忆
	MemoryLongTerm  = types.MemorySemantic  // 长期记忆 -> Semantic
	MemoryEpisodic  = types.MemoryEpisodic  // 情节记忆
	MemorySemantic  = types.MemorySemantic  // 语义记忆 (新增)
	MemoryProcedural = types.MemoryProcedural // 程序记忆 (新增)
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
	ExpiresAt *time.Time           `json:"expires_at,omitempty"` // 短期记忆过期时间
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
