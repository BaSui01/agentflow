package llm

import "time"

// ============================================================
// Provider & Model Base Types
// ============================================================

// LLMProviderStatus represents the status of an LLM provider
type LLMProviderStatus int16

const (
	LLMProviderStatusInactive LLMProviderStatus = 0
	LLMProviderStatusActive   LLMProviderStatus = 1
	LLMProviderStatusDisabled LLMProviderStatus = 2
)

// LLMModel represents an abstract model (e.g., gpt-4, claude-3-opus)
type LLMModel struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ModelName   string    `gorm:"size:100;not null;uniqueIndex" json:"model_name"`
	DisplayName string    `gorm:"size:200" json:"display_name"`
	Description string    `gorm:"type:text" json:"description"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (LLMModel) TableName() string {
	return "sc_llm_models"
}

// LLMProvider represents a provider (e.g., OpenAI, Anthropic, DeepSeek)
type LLMProvider struct {
	ID          uint              `gorm:"primaryKey" json:"id"`
	Code        string            `gorm:"size:50;not null;uniqueIndex" json:"code"`
	Name        string            `gorm:"size:200;not null" json:"name"`
	Description string            `gorm:"type:text" json:"description"`
	Status      LLMProviderStatus `gorm:"default:1" json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func (LLMProvider) TableName() string {
	return "sc_llm_providers"
}

// ============================================================
// Multi-Provider Support (Many-to-Many)
// ============================================================

// LLMProviderModel represents a provider's model instance (many-to-many mapping)
// One model (e.g., gpt-4) can be provided by multiple providers (OpenAI, Azure, Cloudflare)
type LLMProviderModel struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ModelID         uint      `gorm:"not null;index:idx_model_provider" json:"model_id"`
	ProviderID      uint      `gorm:"not null;index:idx_model_provider;index:idx_provider" json:"provider_id"`
	RemoteModelName string    `gorm:"size:100;not null" json:"remote_model_name"`
	BaseURL         string    `gorm:"size:500" json:"base_url"`
	PriceInput      float64   `gorm:"type:decimal(10,6);default:0" json:"price_input"`
	PriceCompletion float64   `gorm:"type:decimal(10,6);default:0" json:"price_completion"`
	MaxTokens       int       `gorm:"default:0" json:"max_tokens"`
	Priority        int       `gorm:"default:100" json:"priority"`
	Enabled         bool      `gorm:"default:true" json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Model    *LLMModel    `gorm:"foreignKey:ModelID" json:"model,omitempty"`
	Provider *LLMProvider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

func (LLMProviderModel) TableName() string {
	return "sc_llm_provider_models"
}

// ============================================================
// API Key Pool
// ============================================================

// LLMProviderAPIKey represents an API key in the pool
// Supports multiple API keys per provider for load balancing and failover
type LLMProviderAPIKey struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProviderID uint      `gorm:"not null;index:idx_provider" json:"provider_id"`
	APIKey     string    `gorm:"size:500;not null" json:"api_key"`
	Label      string    `gorm:"size:100" json:"label"`
	Priority   int       `gorm:"default:100" json:"priority"`
	Weight     int       `gorm:"default:100" json:"weight"`
	Enabled    bool      `gorm:"default:true" json:"enabled"`
	
	// Usage statistics
	TotalRequests   int64      `gorm:"default:0" json:"total_requests"`
	FailedRequests  int64      `gorm:"default:0" json:"failed_requests"`
	LastUsedAt      *time.Time `json:"last_used_at"`
	LastErrorAt     *time.Time `json:"last_error_at"`
	LastError       string     `gorm:"type:text" json:"last_error"`
	
	// Rate limiting
	RateLimitRPM int       `gorm:"default:0" json:"rate_limit_rpm"`
	RateLimitRPD int       `gorm:"default:0" json:"rate_limit_rpd"`
	CurrentRPM   int       `gorm:"default:0" json:"current_rpm"`
	CurrentRPD   int       `gorm:"default:0" json:"current_rpd"`
	RPMResetAt   time.Time `json:"rpm_reset_at"`
	RPDResetAt   time.Time `json:"rpd_reset_at"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Provider *LLMProvider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

func (LLMProviderAPIKey) TableName() string {
	return "sc_llm_provider_api_keys"
}

// IsHealthy checks if the API key is healthy
func (k *LLMProviderAPIKey) IsHealthy() bool {
	if !k.Enabled {
		return false
	}
	
	now := time.Now()
	
	// Check rate limits
	if k.RateLimitRPM > 0 && now.Before(k.RPMResetAt) && k.CurrentRPM >= k.RateLimitRPM {
		return false
	}
	if k.RateLimitRPD > 0 && now.Before(k.RPDResetAt) && k.CurrentRPD >= k.RateLimitRPD {
		return false
	}
	
	// Check error rate (fail rate > 50%)
	if k.TotalRequests >= 100 {
		failRate := float64(k.FailedRequests) / float64(k.TotalRequests)
		if failRate > 0.5 {
			return false
		}
	}
	
	return true
}

// IncrementUsage increments usage counters
func (k *LLMProviderAPIKey) IncrementUsage(success bool) {
	now := time.Now()
	k.TotalRequests++
	k.LastUsedAt = &now
	
	if !success {
		k.FailedRequests++
		k.LastErrorAt = &now
	}
	
	// Reset RPM counter
	if now.After(k.RPMResetAt) {
		k.CurrentRPM = 0
		k.RPMResetAt = now.Add(time.Minute)
	}
	k.CurrentRPM++
	
	// Reset RPD counter
	if now.After(k.RPDResetAt) {
		k.CurrentRPD = 0
		k.RPDResetAt = now.Add(24 * time.Hour)
	}
	k.CurrentRPD++
}

// ============================================================
// Extended Types (with associations)
// ============================================================

// LLMModelExtended extends LLMModel with associations
type LLMModelExtended struct {
	LLMModel
	ProviderModels []LLMProviderModel `gorm:"foreignKey:ModelID" json:"provider_models,omitempty"`
}

// LLMProviderExtended extends LLMProvider with associations
type LLMProviderExtended struct {
	LLMProvider
	APIKeys        []LLMProviderAPIKey `gorm:"foreignKey:ProviderID" json:"api_keys,omitempty"`
	ProviderModels []LLMProviderModel  `gorm:"foreignKey:ProviderID" json:"provider_models,omitempty"`
}

// ============================================================
// Audit Log
// ============================================================

// AuditLog represents an audit log entry
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
