package embedding

import "time"

// OpenAIConfig configures the OpenAI embedding provider.
type OpenAIConfig struct {
	APIKey     string        `json:"api_key" yaml:"api_key"`
	BaseURL    string        `json:"base_url" yaml:"base_url"`
	Model      string        `json:"model,omitempty" yaml:"model,omitempty"`           // text-embedding-3-large
	Dimensions int           `json:"dimensions,omitempty" yaml:"dimensions,omitempty"` // 256, 1024, 3072
	Timeout    time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// VoyageConfig configures the Voyage AI embedding provider.
type VoyageConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // voyage-3-large, voyage-code-3
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// CohereConfig configures the Cohere embedding provider.
type CohereConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // embed-v3.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// JinaConfig configures the Jina AI embedding provider.
type JinaConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // jina-embeddings-v3
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultOpenAIConfig returns default OpenAI embedding config.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseURL:    "https://api.openai.com",
		Model:      "text-embedding-3-large",
		Dimensions: 3072,
		Timeout:    30 * time.Second,
	}
}

// DefaultVoyageConfig returns default Voyage AI config.
func DefaultVoyageConfig() VoyageConfig {
	return VoyageConfig{
		BaseURL: "https://api.voyageai.com",
		Model:   "voyage-3-large",
		Timeout: 30 * time.Second,
	}
}

// DefaultCohereConfig returns default Cohere config.
func DefaultCohereConfig() CohereConfig {
	return CohereConfig{
		BaseURL: "https://api.cohere.ai",
		Model:   "embed-v3.5",
		Timeout: 30 * time.Second,
	}
}

// DefaultJinaConfig returns default Jina AI config.
func DefaultJinaConfig() JinaConfig {
	return JinaConfig{
		BaseURL: "https://api.jina.ai",
		Model:   "jina-embeddings-v3",
		Timeout: 30 * time.Second,
	}
}
