package config

import (
	"time"
)

// FallbackPolicy 降级策略配置
type FallbackPolicy struct {
	ID               string       `json:"id" yaml:"id"`
	Name             string       `json:"name" yaml:"name"`
	Priority         int          `json:"priority" yaml:"priority"`
	TriggerProvider  string       `json:"trigger_provider,omitempty" yaml:"trigger_provider,omitempty"`
	TriggerModel     string       `json:"trigger_model,omitempty" yaml:"trigger_model,omitempty"`
	TriggerErrors    []string     `json:"trigger_errors" yaml:"trigger_errors"`
	FallbackType     FallbackType `json:"fallback_type" yaml:"fallback_type"`
	FallbackTarget   string       `json:"fallback_target,omitempty" yaml:"fallback_target,omitempty"`
	FallbackTemplate string       `json:"fallback_template,omitempty" yaml:"fallback_template,omitempty"`
	RetryMax         int          `json:"retry_max" yaml:"retry_max"`
	RetryDelayMs     int          `json:"retry_delay_ms" yaml:"retry_delay_ms"`
	RetryMultiplier  float64      `json:"retry_multiplier" yaml:"retry_multiplier"`
	Enabled          bool         `json:"enabled" yaml:"enabled"`
	Version          int          `json:"version" yaml:"version"`
}

type FallbackType string

const (
	FallbackModel        FallbackType = "model"
	FallbackProvider     FallbackType = "provider"
	FallbackDisableTools FallbackType = "disable_tools"
	FallbackTemplate     FallbackType = "template"
)

// RoutingWeight 路由权重配置
type RoutingWeight struct {
	ID             string  `json:"id"`
	ModelID        string  `json:"model_id"`
	TaskType       string  `json:"task_type,omitempty"`
	Weight         int     `json:"weight"`
	CostWeight     float64 `json:"cost_weight"`
	LatencyWeight  float64 `json:"latency_weight"`
	QualityWeight  float64 `json:"quality_weight"`
	MaxCostPerReq  float64 `json:"max_cost_per_req,omitempty"`
	MaxLatencyMs   int     `json:"max_latency_ms,omitempty"`
	MinSuccessRate float64 `json:"min_success_rate,omitempty"`
	Enabled        bool    `json:"enabled"`
	Version        int     `json:"version"`
}

// LLMConfig 完整的 LLM 配置
type LLMConfig struct {
	Version          int64                      `json:"version"`
	UpdatedAt        time.Time                  `json:"updated_at"`
	FallbackPolicies []FallbackPolicy           `json:"fallback_policies"`
	RoutingWeights   map[string][]RoutingWeight `json:"routing_weights"` // key: task_type
	Providers        map[string]ProviderConfig  `json:"providers"`       // key: provider_code
	PrefixRules      []PrefixRule               `json:"prefix_rules" yaml:"prefix_rules"`
	Caching          CacheConfig                `json:"caching" yaml:"caching"`
}

// PrefixRule 前缀路由规则
type PrefixRule struct {
	Prefix   string `json:"prefix" yaml:"prefix"`     // 模型 ID 前缀
	Provider string `json:"provider" yaml:"provider"` // Provider 代码
}

// CacheConfig 缓存配置
type CacheConfig struct {
	LocalMaxSize    int           `json:"local_max_size" yaml:"local_max_size"`
	LocalTTL        time.Duration `json:"local_ttl" yaml:"local_ttl"`
	RedisTTL        time.Duration `json:"redis_ttl" yaml:"redis_ttl"`
	EnableLocal     bool          `json:"enable_local" yaml:"enable_local"`
	EnableRedis     bool          `json:"enable_redis" yaml:"enable_redis"`
	KeyStrategyType string        `json:"key_strategy" yaml:"key_strategy"` // hash | hierarchical
}

// ProviderConfig Provider 配置
type ProviderConfig struct {
	Code          string        `json:"code"`
	Name          string        `json:"name"`
	BaseURL       string        `json:"base_url"`
	AuthMode      string        `json:"auth_mode"`
	Enabled       bool          `json:"enabled"`
	FailoverGroup string        `json:"failover_group,omitempty"`
	Models        []ModelConfig `json:"models"`
	Version       int           `json:"version"`
}

// ModelConfig 模型配置
type ModelConfig struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
	PriceInput  float64  `json:"price_input"`
	PriceOutput float64  `json:"price_output"`
	Tags        []string `json:"tags"`
	Enabled     bool     `json:"enabled"`
}
