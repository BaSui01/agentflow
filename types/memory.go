package types

import "time"

// MemoryEntry 表示记忆存储条目。
// 由 agent/memory 层使用，在 agent 层的接口中引用。
type MemoryEntry struct {
	Key       string         `json:"key"`
	AgentID   string         `json:"agent_id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// EpisodicEvent 表示情节记忆事件。
type EpisodicEvent struct {
	ID        string         `json:"id"`
	AgentID   string         `json:"agent_id"`
	Type      string         `json:"type"`
	Content   string         `json:"content"`
	Context   map[string]any `json:"context"`
	Timestamp time.Time      `json:"timestamp"`
	Duration  time.Duration  `json:"duration"`
}

// MemoryKind is the cross-layer memory category contract.
type MemoryKind string

const (
	MemoryShortTerm  MemoryKind = "working"
	MemoryWorking    MemoryKind = "working"
	MemoryLongTerm   MemoryKind = "semantic"
	MemoryEpisodic   MemoryKind = "episodic"
	MemorySemantic   MemoryKind = "semantic"
	MemoryProcedural     MemoryKind = "procedural"
	MemoryObservational  MemoryKind = "observational"
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

