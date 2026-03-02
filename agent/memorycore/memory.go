package memorycore

import (
	"context"

	"github.com/BaSui01/agentflow/types"
)

// MaxRecentMemory is the upper bound for in-process recent-memory cache.
const MaxRecentMemory = 50

// MemoryKind is the memory category.
type MemoryKind = types.MemoryKind

const (
	MemoryShortTerm  MemoryKind = types.MemoryShortTerm
	MemoryWorking    MemoryKind = types.MemoryWorking
	MemoryLongTerm   MemoryKind = types.MemoryLongTerm
	MemoryEpisodic   MemoryKind = types.MemoryEpisodic
	MemorySemantic   MemoryKind = types.MemorySemantic
	MemoryProcedural MemoryKind = types.MemoryProcedural
)

// MemoryRecord is the unified memory record.
type MemoryRecord = types.MemoryRecord

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
