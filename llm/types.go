package llm

import (
	"fmt"
	"time"
)

// ============================================================
// 提供者和模型基础类型
// ============================================================

// LLMProvider 状态代表 LLM 提供者的地位
type LLMProviderStatus int16

const (
	LLMProviderStatusInactive LLMProviderStatus = 0
	LLMProviderStatusActive   LLMProviderStatus = 1
	LLMProviderStatusDisabled LLMProviderStatus = 2
)

// String returns the string representation of LLMProviderStatus.
func (s LLMProviderStatus) String() string {
	switch s {
	case LLMProviderStatusInactive:
		return "inactive"
	case LLMProviderStatusActive:
		return "active"
	case LLMProviderStatusDisabled:
		return "disabled"
	default:
		return fmt.Sprintf("LLMProviderStatus(%d)", s)
	}
}

// LLMMOdel代表抽象模型(例如gpt-4,claude-3-opus).
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

// LLMProvider 代表提供商( 如 OpenAI, Anthropic, DeepSeek)
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
// 多服务支持( 多对多)
// ============================================================

// LLMProvider Model 代表提供商的模型实例( 多人对多人映射)
// 多个提供者（OpenAI、Azure、Cloudflare）可提供同一模型（例如 gpt-4）
type LLMProviderModel struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ModelID         uint      `gorm:"not null;index:idx_model_provider" json:"model_id"`
	ProviderID      uint      `gorm:"not null;index:idx_model_provider;index:idx_provider_models_provider_id" json:"provider_id"`
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
// API 密钥池
// ============================================================

// LLMProviderAPIKey 代表池中的 API 密钥
// 每个提供者支持多个 API 密钥进行负载平衡和失效
type LLMProviderAPIKey struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	ProviderID uint   `gorm:"not null;index:idx_provider_api_keys_provider_id" json:"provider_id"`
	APIKey     string `gorm:"size:500;not null" json:"api_key"`
	BaseURL    string `gorm:"size:500" json:"base_url"`
	Label      string `gorm:"size:100" json:"label"`
	Priority   int    `gorm:"default:100" json:"priority"`
	Weight     int    `gorm:"default:100" json:"weight"`
	Enabled    bool   `gorm:"default:true" json:"enabled"`

	// 使用统计
	TotalRequests  int64      `gorm:"default:0" json:"total_requests"`
	FailedRequests int64      `gorm:"default:0" json:"failed_requests"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	LastErrorAt    *time.Time `json:"last_error_at"`
	LastError      string     `gorm:"type:text" json:"last_error"`

	// 限制费率
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

// API 键是否健康 健康检查
func (k *LLMProviderAPIKey) IsHealthy() bool {
	if !k.Enabled {
		return false
	}

	now := time.Now()

	// 检查率限制
	if k.RateLimitRPM > 0 && now.Before(k.RPMResetAt) && k.CurrentRPM >= k.RateLimitRPM {
		return false
	}
	if k.RateLimitRPD > 0 && now.Before(k.RPDResetAt) && k.CurrentRPD >= k.RateLimitRPD {
		return false
	}

	// 检查出错率( 不及格率 > 50%)
	if k.TotalRequests >= 100 {
		failRate := float64(k.FailedRequests) / float64(k.TotalRequests)
		if failRate > 0.5 {
			return false
		}
	}

	return true
}

// 递增使用计数器
func (k *LLMProviderAPIKey) IncrementUsage(success bool) {
	now := time.Now()
	k.TotalRequests++
	k.LastUsedAt = &now

	if !success {
		k.FailedRequests++
		k.LastErrorAt = &now
	}

	// 重置 RPM 计数器
	if now.After(k.RPMResetAt) {
		k.CurrentRPM = 0
		k.RPMResetAt = now.Add(time.Minute)
	}
	k.CurrentRPM++

	// 重置 RPD 计数器
	if now.After(k.RPDResetAt) {
		k.CurrentRPD = 0
		k.RPDResetAt = now.Add(24 * time.Hour)
	}
	k.CurrentRPD++
}

// ============================================================
// 扩展类型(与协会)
// ============================================================

// LLMMOdel Extended 扩展 LLMMOdel 与协会
type LLMModelExtended struct {
	LLMModel
	ProviderModels []LLMProviderModel `gorm:"foreignKey:ModelID" json:"provider_models,omitempty"`
}

// LLMProvider Extended 扩展 LLMProvider 协会
type LLMProviderExtended struct {
	LLMProvider
	APIKeys        []LLMProviderAPIKey `gorm:"foreignKey:ProviderID" json:"api_keys,omitempty"`
	ProviderModels []LLMProviderModel  `gorm:"foreignKey:ProviderID" json:"provider_models,omitempty"`
}

// ============================================================
// 审计日志
// ============================================================

// 审计日志代表审计日志条目
type AuditLog struct {
	ID           uint
	TenantID     uint
	UserID       uint
	Action       string
	ResourceType string
	ResourceID   string
	Details      map[string]any
	CreatedAt    time.Time
}
