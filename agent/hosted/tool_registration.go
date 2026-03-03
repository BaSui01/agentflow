package hosted

import (
	"encoding/json"
	"time"
)

// ToolRegistration stores DB-managed tool alias configuration.
// It maps an exposed tool name to an existing runtime target tool.
type ToolRegistration struct {
	ID          uint            `gorm:"primaryKey" json:"id"`
	Name        string          `gorm:"size:120;not null;uniqueIndex" json:"name"`
	Description string          `gorm:"type:text" json:"description,omitempty"`
	Target      string          `gorm:"size:120;not null" json:"target"`
	Parameters  json.RawMessage `gorm:"type:json" json:"parameters,omitempty"`
	Enabled     bool            `gorm:"default:true;index" json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

func (ToolRegistration) TableName() string {
	return "sc_tool_registrations"
}
