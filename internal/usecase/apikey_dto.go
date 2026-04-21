package usecase

import "time"

type CreateAPIKeyInput struct {
	APIKey       string
	BaseURL      string
	Label        string
	Priority     int
	Weight       int
	Enabled      *bool
	RateLimitRPM int
	RateLimitRPD int
}

type UpdateAPIKeyInput struct {
	BaseURL      *string
	Label        *string
	Priority     *int
	Weight       *int
	Enabled      *bool
	RateLimitRPM *int
	RateLimitRPD *int
}

type APIKeyView struct {
	ID             uint   `json:"id"`
	ProviderID     uint   `json:"provider_id"`
	APIKeyMasked   string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Label          string `json:"label"`
	Priority       int    `json:"priority"`
	Weight         int    `json:"weight"`
	Enabled        bool   `json:"enabled"`
	TotalRequests  int64  `json:"total_requests"`
	FailedRequests int64  `json:"failed_requests"`
	RateLimitRPM   int    `json:"rate_limit_rpm"`
	RateLimitRPD   int    `json:"rate_limit_rpd"`
}

type APIKeyStatsView struct {
	KeyID          uint       `json:"key_id"`
	Label          string     `json:"label"`
	BaseURL        string     `json:"base_url"`
	Enabled        bool       `json:"enabled"`
	IsHealthy      bool       `json:"is_healthy"`
	TotalRequests  int64      `json:"total_requests"`
	FailedRequests int64      `json:"failed_requests"`
	SuccessRate    float64    `json:"success_rate"`
	CurrentRPM     int        `json:"current_rpm"`
	CurrentRPD     int        `json:"current_rpd"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	LastErrorAt    *time.Time `json:"last_error_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}
