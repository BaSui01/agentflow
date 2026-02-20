// Package factory provides a centralized factory for creating LLM Provider
// instances by name. It imports all provider sub-packages and maps string
// names to their constructors, breaking the import cycle that would occur
// if this logic lived in the llm package directly.
package factory

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	claude "github.com/BaSui01/agentflow/llm/providers/anthropic"
	"github.com/BaSui01/agentflow/llm/providers/deepseek"
	"github.com/BaSui01/agentflow/llm/providers/doubao"
	"github.com/BaSui01/agentflow/llm/providers/gemini"
	"github.com/BaSui01/agentflow/llm/providers/glm"
	"github.com/BaSui01/agentflow/llm/providers/grok"
	"github.com/BaSui01/agentflow/llm/providers/hunyuan"
	"github.com/BaSui01/agentflow/llm/providers/kimi"
	"github.com/BaSui01/agentflow/llm/providers/llama"
	"github.com/BaSui01/agentflow/llm/providers/minimax"
	"github.com/BaSui01/agentflow/llm/providers/mistral"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/llm/providers/qwen"
	"go.uber.org/zap"
)

// ProviderConfig is the generic configuration accepted by the factory function.
// It uses a flat structure with an Extra map for provider-specific fields.
type ProviderConfig struct {
	APIKey  string         `json:"api_key" yaml:"api_key"`
	BaseURL string         `json:"base_url" yaml:"base_url"`
	Model   string         `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration  `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Extra   map[string]any `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// NewProviderFromConfig creates a Provider instance based on the provider name
// and a generic ProviderConfig. It maps the name to the appropriate constructor.
//
// Supported names: openai, anthropic, claude, gemini, deepseek, qwen, glm,
// grok, kimi, mistral, minimax, hunyuan, doubao, llama.
func NewProviderFromConfig(name string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	base := providers.BaseProviderConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	switch name {
	case "openai":
		oc := providers.OpenAIConfig{BaseProviderConfig: base}
		if cfg.Extra != nil {
			if v, ok := cfg.Extra["organization"].(string); ok {
				oc.Organization = v
			}
			if v, ok := cfg.Extra["use_responses_api"].(bool); ok {
				oc.UseResponsesAPI = v
			}
		}
		return openai.NewOpenAIProvider(oc, logger), nil

	case "anthropic", "claude":
		return claude.NewClaudeProvider(providers.ClaudeConfig{BaseProviderConfig: base}, logger), nil

	case "gemini":
		return gemini.NewGeminiProvider(providers.GeminiConfig{BaseProviderConfig: base}, logger), nil

	case "deepseek":
		return deepseek.NewDeepSeekProvider(providers.DeepSeekConfig{BaseProviderConfig: base}, logger), nil

	case "qwen":
		return qwen.NewQwenProvider(providers.QwenConfig{BaseProviderConfig: base}, logger), nil

	case "glm":
		return glm.NewGLMProvider(providers.GLMConfig{BaseProviderConfig: base}, logger), nil

	case "grok":
		return grok.NewGrokProvider(providers.GrokConfig{BaseProviderConfig: base}, logger), nil

	case "kimi":
		return kimi.NewKimiProvider(providers.KimiConfig{BaseProviderConfig: base}, logger), nil

	case "mistral":
		return mistral.NewMistralProvider(providers.MistralConfig{BaseProviderConfig: base}, logger), nil

	case "minimax":
		return minimax.NewMiniMaxProvider(providers.MiniMaxConfig{BaseProviderConfig: base}, logger), nil

	case "hunyuan":
		return hunyuan.NewHunyuanProvider(providers.HunyuanConfig{BaseProviderConfig: base}, logger), nil

	case "doubao":
		return doubao.NewDoubaoProvider(providers.DoubaoConfig{BaseProviderConfig: base}, logger), nil

	case "llama":
		lc := providers.LlamaConfig{BaseProviderConfig: base}
		if cfg.Extra != nil {
			if v, ok := cfg.Extra["provider"].(string); ok {
				lc.Provider = v
			}
		}
		return llama.NewLlamaProvider(lc, logger), nil

	default:
		return nil, fmt.Errorf("unknown provider: %q", name)
	}
}

// SupportedProviders returns the list of provider names supported by the factory.
func SupportedProviders() []string {
	return []string{
		"openai", "anthropic", "claude", "gemini", "deepseek",
		"qwen", "glm", "grok", "kimi", "mistral",
		"minimax", "hunyuan", "doubao", "llama",
	}
}

// RegistryConfig describes multiple providers and which one is the default.
// Use this with NewRegistryFromConfig to build a ProviderRegistry in one call.
type RegistryConfig struct {
	// Default is the name of the default provider (must match a key in Providers).
	Default string `json:"default" yaml:"default"`
	// Providers maps provider names to their configurations.
	Providers map[string]ProviderConfig `json:"providers" yaml:"providers"`
}

// NewRegistryFromConfig creates a ProviderRegistry populated with all providers
// defined in the RegistryConfig. It sets the default provider if specified.
// Any provider that fails to initialize is logged as a warning and skipped.
func NewRegistryFromConfig(cfg RegistryConfig, logger *zap.Logger) (*llm.ProviderRegistry, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	reg := llm.NewProviderRegistry()

	for name, pcfg := range cfg.Providers {
		p, err := NewProviderFromConfig(name, pcfg, logger)
		if err != nil {
			logger.Warn("skipping provider: initialization failed",
				zap.String("provider", name),
				zap.Error(err))
			continue
		}
		reg.Register(name, p)
		logger.Info("provider registered", zap.String("provider", name))
	}

	if cfg.Default != "" {
		if err := reg.SetDefault(cfg.Default); err != nil {
			return reg, fmt.Errorf("failed to set default provider %q: %w", cfg.Default, err)
		}
	}

	return reg, nil
}
