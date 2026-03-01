package image

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// ProviderType 标识 image provider 类型。
type ProviderType string

const (
	ProviderOpenAI ProviderType = "openai"
	ProviderFlux   ProviderType = "flux"
	ProviderGemini ProviderType = "gemini"
)

// FactoryConfig 是 image 统一工厂输入。
type FactoryConfig struct {
	Type    ProviderType
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewProviderFromConfig 是 image 包唯一构建入口。
func NewProviderFromConfig(cfg FactoryConfig) (Provider, error) {
	t := cfg.Type
	if t == "" {
		t = ProviderOpenAI
	}

	base := providers.BaseProviderConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	switch t {
	case ProviderOpenAI:
		return NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: base}), nil
	case ProviderFlux:
		return NewFluxProvider(FluxConfig{BaseProviderConfig: base}), nil
	case ProviderGemini:
		return NewGeminiProvider(GeminiConfig{BaseProviderConfig: base}), nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", t)
	}
}

