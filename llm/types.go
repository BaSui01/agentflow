package llm

import "time"

// LLMProviderStatus 表示 LLM Provider 的状态
type LLMProviderStatus int16

const (
	LLMProviderStatusInactive LLMProviderStatus = 0
	LLMProviderStatusActive   LLMProviderStatus = 1
	LLMProviderStatusDisabled LLMProviderStatus = 2
)

// LLMModel 本地类型（避免导入 internal/domain）
type LLMModel struct {
	ID              uint
	ProviderID      uint
	ModelName       string
	PriceInput      float64
	PriceCompletion float64
	Tags            []string
	Enabled         bool
}

// LLMProvider 本地类型（避免导入 internal/domain）
type LLMProvider struct {
	ID        uint
	Code      string
	Name      string
	Status    LLMProviderStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AuditLog 本地类型（避免导入 internal/domain）
type AuditLog struct {
	ID           uint
	TenantID     uint
	UserID       uint
	Action       string
	ResourceType string
	ResourceID   string
	Details      map[string]interface{}
	CreatedAt    time.Time
}
