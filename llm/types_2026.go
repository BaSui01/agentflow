package llm

import (
	"time"
)

// ============================================================
// 多对多模型-提供商关系（2026 新增）
// ============================================================

// LLMProviderModel 提供商提供的模型实例（多对多中间表）
// 一个模型（如 gpt-4）可以被多个提供商提供（OpenAI、Azure、Cloudflare）
type LLMProviderModel struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ModelID         uint      `gorm:"not null;index:idx_model_provider" json:"model_id"`                // 关联 LLMModel
	ProviderID      uint      `gorm:"not null;index:idx_model_provider;index:idx_provider" json:"provider_id"` // 关联 LLMProvider
	RemoteModelName string    `gorm:"size:100;not null" json:"remote_model_name"`                       // 提供商侧的模型名（可能不同）
	PriceInput      float64   `gorm:"type:decimal(10,6);default:0" json:"price_input"`                  // 输入价格（$/1M tokens）
	PriceCompletion float64   `gorm:"type:decimal(10,6);default:0" json:"price_completion"`             // 输出价格（$/1M tokens）
	MaxTokens       int       `gorm:"default:0" json:"max_tokens"`                                      // 最大 tokens
	Priority        int       `gorm:"default:100" json:"priority"`                                      // 优先级（数字越小优先级越高）
	Enabled         bool      `gorm:"default:true" json:"enabled"`                                      // 是否启用
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// 关联
	Model    *LLMModel    `gorm:"foreignKey:ModelID" json:"model,omitempty"`
	Provider *LLMProvider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

func (LLMProviderModel) TableName() string {
	return "sc_llm_provider_models"
}

// ============================================================
// API Key 池（2026 新增）
// ============================================================

// LLMProviderAPIKey API Key 池
// 支持一个提供商配置多个 API Key，用于负载均衡和容灾
type LLMProviderAPIKey struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProviderID uint      `gorm:"not null;index:idx_provider" json:"provider_id"` // 关联 LLMProvider
	APIKey     string    `gorm:"size:500;not null" json:"api_key"`               // 加密存储的 API Key
	Label      string    `gorm:"size:100" json:"label"`                          // 标签（如 "主账号"、"备用账号"）
	Priority   int       `gorm:"default:100" json:"priority"`                    // 优先级（数字越小优先级越高）
	Weight     int       `gorm:"default:100" json:"weight"`                      // 权重（用于加权轮询）
	Enabled    bool      `gorm:"default:true" json:"enabled"`                    // 是否启用
	
	// 使用统计
	TotalRequests   int64     `gorm:"default:0" json:"total_requests"`    // 总请求数
	FailedRequests  int64     `gorm:"default:0" json:"failed_requests"`   // 失败请求数
	LastUsedAt      *time.Time `json:"last_used_at"`                      // 最后使用时间
	LastErrorAt     *time.Time `json:"last_error_at"`                     // 最后错误时间
	LastError       string    `gorm:"type:text" json:"last_error"`        // 最后错误信息
	
	// 限流配置
	RateLimitRPM    int       `gorm:"default:0" json:"rate_limit_rpm"`    // 每分钟请求数限制（0 表示无限制）
	RateLimitRPD    int       `gorm:"default:0" json:"rate_limit_rpd"`    // 每天请求数限制（0 表示无限制）
	CurrentRPM      int       `gorm:"default:0" json:"current_rpm"`       // 当前分钟请求数
	CurrentRPD      int       `gorm:"default:0" json:"current_rpd"`       // 当前天请求数
	RPMResetAt      time.Time `json:"rpm_reset_at"`                       // RPM 重置时间
	RPDResetAt      time.Time `json:"rpd_reset_at"`                       // RPD 重置时间
	
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// 关联
	Provider *LLMProvider `gorm:"foreignKey:ProviderID" json:"provider,omitempty"`
}

func (LLMProviderAPIKey) TableName() string {
	return "sc_llm_provider_api_keys"
}

// IsHealthy 检查 Key 是否健康
func (k *LLMProviderAPIKey) IsHealthy() bool {
	if !k.Enabled {
		return false
	}
	
	// 检查是否超过限流
	now := time.Now()
	if k.RateLimitRPM > 0 {
		if now.Before(k.RPMResetAt) && k.CurrentRPM >= k.RateLimitRPM {
			return false
		}
	}
	if k.RateLimitRPD > 0 {
		if now.Before(k.RPDResetAt) && k.CurrentRPD >= k.RateLimitRPD {
			return false
		}
	}
	
	// 检查错误率（最近 100 次请求失败率 > 50%）
	if k.TotalRequests > 100 {
		recentRequests := k.TotalRequests
		if recentRequests > 100 {
			recentRequests = 100
		}
		failRate := float64(k.FailedRequests) / float64(recentRequests)
		if failRate > 0.5 {
			return false
		}
	}
	
	return true
}

// IncrementUsage 增加使用计数
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
// 扩展现有模型
// ============================================================

// LLMModel 扩展：支持多对多关系
// 注意：原有字段保持不变，只添加新的关联
type LLMModelExtended struct {
	LLMModel
	
	// 多对多关联：该模型被哪些提供商提供
	ProviderModels []LLMProviderModel `gorm:"foreignKey:ModelID" json:"provider_models,omitempty"`
}

// LLMProvider 扩展：支持 API Key 池
type LLMProviderExtended struct {
	LLMProvider
	
	// API Key 池
	APIKeys []LLMProviderAPIKey `gorm:"foreignKey:ProviderID" json:"api_keys,omitempty"`
	
	// 多对多关联：该提供商提供哪些模型
	ProviderModels []LLMProviderModel `gorm:"foreignKey:ProviderID" json:"provider_models,omitempty"`
}
