package types

import "time"

// MemoryKind is the cross-layer memory category contract.
type MemoryKind string

const (
	MemoryShortTerm  MemoryKind = "working"
	MemoryWorking    MemoryKind = "working"
	MemoryLongTerm   MemoryKind = "semantic"
	MemoryEpisodic   MemoryKind = "episodic"
	MemorySemantic   MemoryKind = "semantic"
	MemoryProcedural MemoryKind = "procedural"
)

// MemoryRecord is the cross-layer memory payload contract.
type MemoryRecord struct {
	ID        string                 `json:"id"`
	AgentID   string                 `json:"agent_id"`
	Kind      MemoryKind             `json:"kind"`
	Content   string                 `json:"content"`
	Metadata  map[string]any         `json:"metadata,omitempty"`
	VectorID  string                 `json:"vector_id,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
}

