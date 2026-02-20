package rerank

import "time"

// Cohere Config 配置 Cohere 重排提供者 。
type CohereConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // rerank-v3.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// JinaConfig配置了Jina AI重排提供者.
type JinaConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // jina-reranker-v2-base-multilingual
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// VoyageConfig 配置 Voyage AI 重排提供者 。
type VoyageConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // rerank-2
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 Cohere Config 返回默认 Cohere 重排配置 。
func DefaultCohereConfig() CohereConfig {
	return CohereConfig{
		BaseURL: "https://api.cohere.ai",
		Model:   "rerank-v3.5",
		Timeout: 30 * time.Second,
	}
}

// 默认 JinaConfig 返回默认 Jina 重排配置 。
func DefaultJinaConfig() JinaConfig {
	return JinaConfig{
		BaseURL: "https://api.jina.ai",
		Model:   "jina-reranker-v2-base-multilingual",
		Timeout: 30 * time.Second,
	}
}

// 默认 VoyageConfig 返回默认 Voyage 重排配置 。
func DefaultVoyageConfig() VoyageConfig {
	return VoyageConfig{
		BaseURL: "https://api.voyageai.com",
		Model:   "rerank-2",
		Timeout: 30 * time.Second,
	}
}
