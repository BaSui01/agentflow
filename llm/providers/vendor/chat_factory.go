package vendor

import (
	"fmt"
	"strings"
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

// ChatProviderConfig 定义 chat provider 构造配置。
type ChatProviderConfig struct {
	APIKey  string
	APIKeys []providers.APIKeyEntry
	BaseURL string
	Model   string
	Timeout time.Duration
	Extra   map[string]any
}

// NewChatProviderFromConfig 是 chat provider 的唯一配置构造入口。
func NewChatProviderFromConfig(name string, cfg ChatProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	providerCode := strings.ToLower(strings.TrimSpace(name))
	switch providerCode {
	case "openai":
		return newOpenAIChatProvider(cfg, logger), nil
	case "anthropic", "claude":
		return newAnthropicChatProvider(cfg, logger), nil
	case "gemini", "gemini-vertex":
		return newGeminiChatProvider(providerCode, cfg, logger), nil
	case "deepseek":
		return deepseek.NewDeepSeekProvider(providers.DeepSeekConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "qwen":
		return qwen.NewQwenProvider(providers.QwenConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "glm":
		return glm.NewGLMProvider(providers.GLMConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "grok":
		return grok.NewGrokProvider(providers.GrokConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "kimi":
		return kimi.NewKimiProvider(providers.KimiConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "mistral":
		return mistral.NewMistralProvider(providers.MistralConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "minimax":
		return minimax.NewMiniMaxProvider(providers.MiniMaxConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "hunyuan":
		return hunyuan.NewHunyuanProvider(providers.HunyuanConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "doubao":
		return doubao.NewDoubaoProvider(providers.DoubaoConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}, logger), nil
	case "llama":
		return newLlamaChatProvider(cfg, logger), nil
	default:
		return newOpenAICompatChatProvider(providerCode, cfg, logger)
	}
}

func toBaseProviderConfig(cfg ChatProviderConfig) providers.BaseProviderConfig {
	return providers.BaseProviderConfig{
		APIKey:  cfg.APIKey,
		APIKeys: cfg.APIKeys,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}
}

func newOpenAIChatProvider(cfg ChatProviderConfig, logger *zap.Logger) llm.Provider {
	openaiCfg := providers.OpenAIConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["organization"].(string); ok {
			openaiCfg.Organization = v
		}
		if v, ok := cfg.Extra["use_responses_api"].(bool); ok {
			openaiCfg.UseResponsesAPI = v
		}
	}
	return openai.NewOpenAIProvider(openaiCfg, logger)
}

func newAnthropicChatProvider(cfg ChatProviderConfig, logger *zap.Logger) llm.Provider {
	anthropicCfg := providers.ClaudeConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["anthropic_version"].(string); ok {
			anthropicCfg.AnthropicVersion = v
		}
	}
	return claude.NewClaudeProvider(anthropicCfg, logger)
}

func newGeminiChatProvider(providerCode string, cfg ChatProviderConfig, logger *zap.Logger) llm.Provider {
	geminiCfg := providers.GeminiConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["project_id"].(string); ok {
			geminiCfg.ProjectID = v
		}
		if v, ok := cfg.Extra["region"].(string); ok {
			geminiCfg.Region = v
		}
		if v, ok := cfg.Extra["auth_type"].(string); ok {
			geminiCfg.AuthType = v
		}
	}
	if providerCode == "gemini-vertex" && geminiCfg.AuthType == "" {
		geminiCfg.AuthType = "oauth"
	}
	return gemini.NewGeminiProvider(geminiCfg, logger)
}

func newLlamaChatProvider(cfg ChatProviderConfig, logger *zap.Logger) llm.Provider {
	llamaCfg := providers.LlamaConfig{BaseProviderConfig: toBaseProviderConfig(cfg)}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["provider"].(string); ok {
			llamaCfg.Provider = v
		}
	}
	return llama.NewLlamaProvider(llamaCfg, logger)
}

func newOpenAICompatChatProvider(providerCode string, cfg ChatProviderConfig, logger *zap.Logger) (llm.Provider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("unknown provider %q: built-in provider not found, and base_url is required for generic OpenAI-compatible provider", providerCode)
	}

	compatCfg := openaicompat.Config{
		ProviderName: providerCode,
		APIKey:       cfg.APIKey,
		APIKeys:      cfg.APIKeys,
		BaseURL:      cfg.BaseURL,
		DefaultModel: cfg.Model,
	}
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["endpoint_path"].(string); ok {
			compatCfg.EndpointPath = v
		}
		if v, ok := cfg.Extra["models_endpoint"].(string); ok {
			compatCfg.ModelsEndpoint = v
		}
		if v, ok := cfg.Extra["auth_header"].(string); ok {
			compatCfg.AuthHeaderName = v
		}
		if v, ok := cfg.Extra["supports_tools"].(bool); ok {
			compatCfg.SupportsTools = &v
		}
		if v, ok := cfg.Extra["api_keys"].([]any); ok {
			for _, key := range v {
				if s, ok := key.(string); ok {
					compatCfg.APIKeys = append(compatCfg.APIKeys, providers.APIKeyEntry{Key: s})
				}
			}
		}
	}

	logger.Info("creating generic OpenAI-compatible chat provider",
		zap.String("provider", providerCode),
		zap.String("base_url", cfg.BaseURL))
	return openaicompat.New(compatCfg, logger), nil
}
