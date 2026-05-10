package memory

import "time"

// VectorItem 向量项
type VectorItem struct {
	ID       string
	Vector   []float64
	Metadata map[string]any
}

// Entity 实体
type Entity struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Relation 关系
type Relation struct {
	ID         string         `json:"id"`
	FromID     string         `json:"from_id"`
	ToID       string         `json:"to_id"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Weight     float64        `json:"weight"`
	CreatedAt  time.Time      `json:"created_at"`
}
