package embedding

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// ProviderType 标识 embedding provider 类型。
type ProviderType string

const (
	ProviderOpenAI ProviderType = "openai"
	ProviderCohere ProviderType = "cohere"
	ProviderVoyage ProviderType = "voyage"
	ProviderJina   ProviderType = "jina"
	ProviderGemini ProviderType = "gemini"
)

// FactoryConfig 是 embedding 统一工厂输入。
type FactoryConfig struct {
	Type       ProviderType
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	Dimensions int
}

// NewProviderFromConfig 是 embedding 包唯一构建入口。
// 当 Type 为空时默认创建 OpenAI provider。
func NewProviderFromConfig(cfg FactoryConfig) (Provider, error) {
	providerType := cfg.Type
	if providerType == "" {
		providerType = ProviderOpenAI
	}

	base := providers.BaseProviderConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	switch providerType {
	case ProviderOpenAI:
		return NewOpenAIProvider(OpenAIConfig{
			BaseProviderConfig: base,
			Dimensions:         cfg.Dimensions,
		}), nil
	case ProviderCohere:
		return NewCohereProvider(CohereConfig{BaseProviderConfig: base}), nil
	case ProviderVoyage:
		return NewVoyageProvider(VoyageConfig{BaseProviderConfig: base}), nil
	case ProviderJina:
		return NewJinaProvider(JinaConfig{BaseProviderConfig: base}), nil
	case ProviderGemini:
		return NewGeminiProvider(GeminiConfig{BaseProviderConfig: base}), nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider type: %s", providerType)
	}
}
