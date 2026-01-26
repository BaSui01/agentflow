package rerank

import "time"

// CohereConfig configures the Cohere reranker provider.
type CohereConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // rerank-v3.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// JinaConfig configures the Jina AI reranker provider.
type JinaConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // jina-reranker-v2-base-multilingual
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// VoyageConfig configures the Voyage AI reranker provider.
type VoyageConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // rerank-2
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultCohereConfig returns default Cohere reranker config.
func DefaultCohereConfig() CohereConfig {
	return CohereConfig{
		BaseURL: "https://api.cohere.ai",
		Model:   "rerank-v3.5",
		Timeout: 30 * time.Second,
	}
}

// DefaultJinaConfig returns default Jina reranker config.
func DefaultJinaConfig() JinaConfig {
	return JinaConfig{
		BaseURL: "https://api.jina.ai",
		Model:   "jina-reranker-v2-base-multilingual",
		Timeout: 30 * time.Second,
	}
}

// DefaultVoyageConfig returns default Voyage reranker config.
func DefaultVoyageConfig() VoyageConfig {
	return VoyageConfig{
		BaseURL: "https://api.voyageai.com",
		Model:   "rerank-2",
		Timeout: 30 * time.Second,
	}
}
