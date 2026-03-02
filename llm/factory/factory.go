// Package factory provides a centralized, registration-based factory for
// creating LLM Provider instances by name.
package factory

import (
	"fmt"
	"sort"
	"sync"
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
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"github.com/BaSui01/agentflow/llm/providers/qwen"
	"go.uber.org/zap"
)

// ProviderConfig is the generic configuration accepted by the factory function.
// It uses a flat structure with an Extra map for provider-specific fields.
type ProviderConfig struct {
	APIKey  string                  `json:"api_key" yaml:"api_key"`
	APIKeys []providers.APIKeyEntry `json:"api_keys,omitempty" yaml:"api_keys,omitempty"`
	BaseURL string                  `json:"base_url" yaml:"base_url"`
	Model   string                  `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration           `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Extra   map[string]any          `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// ProviderConstructor builds a provider for a given name and config.
//
// The name parameter allows alias-specific behavior (e.g. gemini-vertex).
type ProviderConstructor func(name string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error)

var (
	providerConstructors   = make(map[string]ProviderConstructor)
	providerConstructorsMu sync.RWMutex
)

func init() {
	registerBuiltInConstructors()
}

func registerBuiltInConstructors() {
	mustRegisterProviderConstructor("openai", newOpenAIProvider)
	mustRegisterProviderConstructor("anthropic", newAnthropicProvider)
	mustRegisterProviderConstructor("claude", newAnthropicProvider)
	mustRegisterProviderConstructor("gemini", newGeminiProvider)
	mustRegisterProviderConstructor("gemini-vertex", newGeminiProvider)
	mustRegisterProviderConstructor("deepseek", newDeepSeekProvider)
	mustRegisterProviderConstructor("qwen", newQwenProvider)
	mustRegisterProviderConstructor("glm", newGLMProvider)
	mustRegisterProviderConstructor("grok", newGrokProvider)
	mustRegisterProviderConstructor("kimi", newKimiProvider)
	mustRegisterProviderConstructor("mistral", newMistralProvider)
	mustRegisterProviderConstructor("minimax", newMiniMaxProvider)
	mustRegisterProviderConstructor("hunyuan", newHunyuanProvider)
	mustRegisterProviderConstructor("doubao", newDoubaoProvider)
	mustRegisterProviderConstructor("llama", newLlamaProvider)
}

func mustRegisterProviderConstructor(name string, constructor ProviderConstructor) {
	if err := RegisterProviderConstructor(name, constructor); err != nil {
		panic(err)
	}
}

// RegisterProviderConstructor registers a named provider constructor.
// Returns an error if the name is empty, constructor is nil, or the name
// is already registered.
func RegisterProviderConstructor(name string, constructor ProviderConstructor) error {
	if name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}
	if constructor == nil {
		return fmt.Errorf("provider constructor cannot be nil")
	}

	providerConstructorsMu.Lock()
	defer providerConstructorsMu.Unlock()

	if _, exists := providerConstructors[name]; exists {
		return fmt.Errorf("provider constructor already registered for %q", name)
	}

	providerConstructors[name] = constructor
	return nil
}

func getProviderConstructor(name string) (ProviderConstructor, bool) {
	providerConstructorsMu.RLock()
	defer providerConstructorsMu.RUnlock()

	constructor, ok := providerConstructors[name]
	return constructor, ok
}

func makeBaseProviderConfig(cfg ProviderConfig) providers.BaseProviderConfig {
	return providers.BaseProviderConfig{
		APIKey:  cfg.APIKey,
		APIKeys: cfg.APIKeys,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}
}

func newOpenAIProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	oc := providers.OpenAIConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["organization"].(string); ok {
			oc.Organization = v
		}
		if v, ok := cfg.Extra["use_responses_api"].(bool); ok {
			oc.UseResponsesAPI = v
		}
	}
	return openai.NewOpenAIProvider(oc, logger), nil
}

func newAnthropicProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	cc := providers.ClaudeConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["anthropic_version"].(string); ok {
			cc.AnthropicVersion = v
		}
	}
	return claude.NewClaudeProvider(cc, logger), nil
}

func newGeminiProvider(name string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	gc := providers.GeminiConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["project_id"].(string); ok {
			gc.ProjectID = v
		}
		if v, ok := cfg.Extra["region"].(string); ok {
			gc.Region = v
		}
		if v, ok := cfg.Extra["auth_type"].(string); ok {
			gc.AuthType = v
		}
	}
	if name == "gemini-vertex" && gc.AuthType == "" {
		gc.AuthType = "oauth"
	}
	return gemini.NewGeminiProvider(gc, logger), nil
}

func newDeepSeekProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return deepseek.NewDeepSeekProvider(providers.DeepSeekConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newQwenProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return qwen.NewQwenProvider(providers.QwenConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newGLMProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return glm.NewGLMProvider(providers.GLMConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newGrokProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return grok.NewGrokProvider(providers.GrokConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newKimiProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return kimi.NewKimiProvider(providers.KimiConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newMistralProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return mistral.NewMistralProvider(providers.MistralConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newMiniMaxProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return minimax.NewMiniMaxProvider(providers.MiniMaxConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newHunyuanProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return hunyuan.NewHunyuanProvider(providers.HunyuanConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newDoubaoProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	return doubao.NewDoubaoProvider(providers.DoubaoConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}, logger), nil
}

func newLlamaProvider(_ string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	lc := providers.LlamaConfig{BaseProviderConfig: makeBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["provider"].(string); ok {
			lc.Provider = v
		}
	}
	return llama.NewLlamaProvider(lc, logger), nil
}

func newOpenAICompatProvider(name string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("unknown provider %q: built-in provider not found, and base_url is required for generic OpenAI-compatible provider", name)
	}

	oc := openaicompat.Config{
		ProviderName: name,
		APIKey:       cfg.APIKey,
		APIKeys:      cfg.APIKeys,
		BaseURL:      cfg.BaseURL,
		DefaultModel: cfg.Model,
	}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["endpoint_path"].(string); ok {
			oc.EndpointPath = v
		}
		if v, ok := cfg.Extra["models_endpoint"].(string); ok {
			oc.ModelsEndpoint = v
		}
		if v, ok := cfg.Extra["auth_header"].(string); ok {
			oc.AuthHeaderName = v
		}
		if v, ok := cfg.Extra["supports_tools"].(bool); ok {
			oc.SupportsTools = &v
		}
		if v, ok := cfg.Extra["api_keys"].([]any); ok {
			for _, k := range v {
				if s, ok := k.(string); ok {
					oc.APIKeys = append(oc.APIKeys, providers.APIKeyEntry{Key: s})
				}
			}
		}
	}

	logger.Info("creating generic OpenAI-compatible provider",
		zap.String("provider", name),
		zap.String("base_url", cfg.BaseURL))
	return openaicompat.New(oc, logger), nil
}

// NewProviderFromConfig creates a Provider instance based on the provider name
// and a generic ProviderConfig.
//
// For registered names, it dispatches to the corresponding constructor.
// For unregistered names, it falls back to a generic OpenAI-compatible provider
// and requires base_url.
func NewProviderFromConfig(name string, cfg ProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	if constructor, ok := getProviderConstructor(name); ok {
		return constructor(name, cfg, logger)
	}
	return newOpenAICompatProvider(name, cfg, logger)
}

// SupportedProviders returns all registered provider names.
func SupportedProviders() []string {
	providerConstructorsMu.RLock()
	defer providerConstructorsMu.RUnlock()

	names := make([]string, 0, len(providerConstructors))
	for name := range providerConstructors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
