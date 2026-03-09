package image

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// ProviderType 标识 image provider 类型。
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderFlux      ProviderType = "flux"
	ProviderGemini    ProviderType = "gemini"
	ProviderStability ProviderType = "stability"
	ProviderIdeogram  ProviderType = "ideogram"
	ProviderTongyi    ProviderType = "tongyi"
	ProviderZhipu     ProviderType = "zhipu"
	ProviderBaidu     ProviderType = "baidu"
	ProviderDoubao    ProviderType = "doubao"
	ProviderTencent   ProviderType = "tencent"
	ProviderKling     ProviderType = "kling"
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
	case ProviderStability:
		return NewStabilityProvider(StabilityConfig{BaseProviderConfig: base}), nil
	case ProviderIdeogram:
		return NewIdeogramProvider(IdeogramConfig{BaseProviderConfig: base}), nil
	case ProviderTongyi:
		return NewTongyiProvider(TongyiConfig{BaseProviderConfig: base}), nil
	case ProviderZhipu:
		return NewZhipuProvider(ZhipuConfig{BaseProviderConfig: base}), nil
	case ProviderBaidu:
		return NewBaiduProvider(BaiduConfig{BaseProviderConfig: base}), nil
	case ProviderDoubao:
		return NewDoubaoProvider(DoubaoConfig{BaseProviderConfig: base}), nil
	case ProviderTencent:
		return NewTencentHunyuanProvider(TencentHunyuanConfig{BaseProviderConfig: base}), nil
	case ProviderKling:
		return NewKlingProvider(KlingConfig{BaseProviderConfig: base}), nil
	default:
		return nil, fmt.Errorf("unsupported image provider type: %s", t)
	}
}

