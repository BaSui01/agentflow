package rerank

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// ProviderType 标识 rerank provider 类型。
type ProviderType string

const (
	ProviderCohere ProviderType = "cohere"
	ProviderVoyage ProviderType = "voyage"
	ProviderJina   ProviderType = "jina"
)

// FactoryConfig 是 rerank 统一工厂输入。
type FactoryConfig struct {
	Type    ProviderType
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewProviderFromConfig 是 rerank 包唯一构建入口。
func NewProviderFromConfig(cfg FactoryConfig) (Provider, error) {
	t := cfg.Type
	if t == "" {
		t = ProviderCohere
	}

	base := providers.BaseProviderConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	switch t {
	case ProviderCohere:
		return NewCohereProvider(CohereConfig{BaseProviderConfig: base}), nil
	case ProviderVoyage:
		return NewVoyageProvider(VoyageConfig{BaseProviderConfig: base}), nil
	case ProviderJina:
		return NewJinaProvider(JinaConfig{BaseProviderConfig: base}), nil
	default:
		return nil, fmt.Errorf("unsupported rerank provider type: %s", t)
	}
}

