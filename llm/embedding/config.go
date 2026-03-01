package embedding

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// OpenAIConfig 配置 OpenAI 嵌入提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、BaseURL、Model、Timeout 字段。
type OpenAIConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
	Dimensions                  int `json:"dimensions,omitempty" yaml:"dimensions,omitempty"` // 256, 1024, 3072
}

// VoyageConfig 配置 Voyage AI 嵌入提供者.
type VoyageConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// CohereConfig 配置 Cohere 嵌入提供者.
type CohereConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// JinaConfig 配置 Jina AI 嵌入提供者.
type JinaConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// 默认 OpenAIConfig 返回默认 OpenAI 嵌入配置.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com",
			Model:   "text-embedding-3-large",
			Timeout: 30 * time.Second,
		},
		Dimensions: 3072,
	}
}

// 默认 VoyageConfig 返回默认 Voyage AI 配置.
func DefaultVoyageConfig() VoyageConfig {
	return VoyageConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.voyageai.com",
			Model:   "voyage-3-large",
			Timeout: 30 * time.Second,
		},
	}
}

// 默认 CohereConfig 返回默认 Cohere 配置.
func DefaultCohereConfig() CohereConfig {
	return CohereConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.cohere.ai",
			Model:   "embed-v3.5",
			Timeout: 30 * time.Second,
		},
	}
}

// 默认 JinaConfig 返回默认 Jina AI 配置.
func DefaultJinaConfig() JinaConfig {
	return JinaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.jina.ai",
			Model:   "jina-embeddings-v3",
			Timeout: 30 * time.Second,
		},
	}
}

