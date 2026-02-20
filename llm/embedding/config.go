package embedding

import "time"

// OpenAIConfig 配置 OpenAI 嵌入提供者.
type OpenAIConfig struct {
	APIKey     string        `json:"api_key" yaml:"api_key"`
	BaseURL    string        `json:"base_url" yaml:"base_url"`
	Model      string        `json:"model,omitempty" yaml:"model,omitempty"`           // text-embedding-3-large
	Dimensions int           `json:"dimensions,omitempty" yaml:"dimensions,omitempty"` // 256, 1024, 3072
	Timeout    time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// VoyageConfig 配置 Voyage AI 嵌入提供者 。
type VoyageConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // voyage-3-large, voyage-code-3
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// CohereConfig 配置 Cohere 嵌入提供者 。
type CohereConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // embed-v3.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// JinaConfig配置了Jina AI嵌入提供者.
type JinaConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // jina-embeddings-v3
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 OpenAIConfig 返回默认 OpenAI 嵌入配置 。
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseURL:    "https://api.openai.com",
		Model:      "text-embedding-3-large",
		Dimensions: 3072,
		Timeout:    30 * time.Second,
	}
}

// 默认 VoyageConfig 返回默认 Voyage AI 配置 。
func DefaultVoyageConfig() VoyageConfig {
	return VoyageConfig{
		BaseURL: "https://api.voyageai.com",
		Model:   "voyage-3-large",
		Timeout: 30 * time.Second,
	}
}

// 默认 Cohere Config 返回默认 Cohere 配置 。
func DefaultCohereConfig() CohereConfig {
	return CohereConfig{
		BaseURL: "https://api.cohere.ai",
		Model:   "embed-v3.5",
		Timeout: 30 * time.Second,
	}
}

// 默认 JinaConfig 返回默认 Jina AI 配置 。
func DefaultJinaConfig() JinaConfig {
	return JinaConfig{
		BaseURL: "https://api.jina.ai",
		Model:   "jina-embeddings-v3",
		Timeout: 30 * time.Second,
	}
}
