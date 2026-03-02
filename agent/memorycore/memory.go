package memorycore

import (
	"context"
	"time"
)

// MaxRecentMemory is the upper bound for in-process recent-memory cache.
const MaxRecentMemory = 50

// MemoryKind is the memory category.
type MemoryKind string

const (
	MemoryShortTerm  MemoryKind = "working"
	MemoryWorking    MemoryKind = "working"
	MemoryLongTerm   MemoryKind = "semantic"
	MemoryEpisodic   MemoryKind = "episodic"
	MemorySemantic   MemoryKind = "semantic"
	MemoryProcedural MemoryKind = "procedural"
)

// MemoryRecord is the unified memory record.
type MemoryRecord struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	Kind      MemoryKind `json:"kind"`
	Content   string               `json:"content"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
	VectorID  string               `json:"vector_id,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
	ExpiresAt *time.Time           `json:"expires_at,omitempty"`
}

// MemoryWriter writes memory.
type MemoryWriter interface {
	Save(ctx context.Context, rec MemoryRecord) error
	Delete(ctx context.Context, id string) error
	Clear(ctx context.Context, agentID string, kind MemoryKind) error
}

// MemoryReader reads memory.
type MemoryReader interface {
	LoadRecent(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error)
	Search(ctx context.Context, agentID string, query string, topK int) ([]MemoryRecord, error)
	Get(ctx context.Context, id string) (*MemoryRecord, error)
}

// MemoryManager combines read/write operations.
type MemoryManager interface {
	MemoryWriter
	MemoryReader
}
