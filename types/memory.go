// Package types provides unified type definitions for the AgentFlow framework.
package types

import "time"

// MemoryCategory defines the unified memory category across the framework.
// This replaces the inconsistent MemoryKind (agent/) and MemoryType (agent/memory/).
type MemoryCategory string

const (
	// MemoryWorking represents short-term working memory for current task context.
	// Storage: In-memory or Redis with TTL.
	MemoryWorking MemoryCategory = "working"

	// MemoryEpisodic represents event-based experiential memories.
	// Storage: Vector store for semantic search.
	MemoryEpisodic MemoryCategory = "episodic"

	// MemorySemantic represents factual knowledge and learned information.
	// Storage: PostgreSQL/Qdrant for long-term persistence.
	MemorySemantic MemoryCategory = "semantic"

	// MemoryProcedural represents how-to knowledge and learned procedures.
	// Storage: Structured storage for procedure definitions.
	MemoryProcedural MemoryCategory = "procedural"
)

// MemoryRecord represents a unified memory entry structure.
type MemoryRecord struct {
	ID          string            `json:"id"`
	AgentID     string            `json:"agent_id"`
	Category    MemoryCategory    `json:"category"`
	Content     string            `json:"content"`
	Embedding   []float32         `json:"embedding,omitempty"`
	Importance  float64           `json:"importance,omitempty"`
	AccessCount int               `json:"access_count,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	VectorID    string            `json:"vector_id,omitempty"`
	Relations   []string          `json:"relations,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	LastAccess  time.Time         `json:"last_access,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

// MemoryQuery represents a query for memory retrieval.
type MemoryQuery struct {
	AgentID    string         `json:"agent_id"`
	Category   MemoryCategory `json:"category,omitempty"`
	Query      string         `json:"query,omitempty"`
	TopK       int            `json:"top_k,omitempty"`
	MinScore   float64        `json:"min_score,omitempty"`
	TimeRange  *TimeRange     `json:"time_range,omitempty"`
}

// TimeRange represents a time range for filtering.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MemoryStats provides statistics about memory usage.
type MemoryStats struct {
	TotalRecords   int            `json:"total_records"`
	ByCategory     map[string]int `json:"by_category"`
	OldestRecord   time.Time      `json:"oldest_record,omitempty"`
	NewestRecord   time.Time      `json:"newest_record,omitempty"`
	TotalSizeBytes int64          `json:"total_size_bytes,omitempty"`
}
