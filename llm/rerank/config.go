package rerank

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// CohereConfig 配置 Cohere 重排提供者.
type CohereConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// JinaConfig 配置 Jina AI 重排提供者.
type JinaConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// VoyageConfig 配置 Voyage AI 重排提供者.
type VoyageConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultCohereConfig 返回默认 Cohere 重排配置.
func DefaultCohereConfig() CohereConfig {
	return CohereConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.cohere.ai",
			Model:   "rerank-v3.5",
			Timeout: 30 * time.Second,
		},
	}
}

// DefaultJinaConfig 返回默认 Jina 重排配置.
func DefaultJinaConfig() JinaConfig {
	return JinaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.jina.ai",
			Model:   "jina-reranker-v2-base-multilingual",
			Timeout: 30 * time.Second,
		},
	}
}

// DefaultVoyageConfig 返回默认 Voyage 重排配置.
func DefaultVoyageConfig() VoyageConfig {
	return VoyageConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.voyageai.com",
			Model:   "rerank-2",
			Timeout: 30 * time.Second,
		},
	}
}
