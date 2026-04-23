package vendor

import (
	"fmt"
	"strings"
	"time"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/providers"
	claude "github.com/BaSui01/agentflow/llm/providers/anthropic"
	"github.com/BaSui01/agentflow/llm/providers/gemini"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
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

	providerCode, cfg := canonicalizeChatProviderConfig(name, cfg)
	switch providerCode {
	case "openai":
		return newOpenAIChatProvider(cfg, logger), nil
	case "anthropic", "claude":
		return newAnthropicChatProvider(cfg, logger), nil
	case "gemini", "gemini-vertex":
		return newGeminiChatProvider(providerCode, cfg, logger), nil
	case "qwen":
		return newCompatBuiltInChatProvider(providerCode, cfg, logger)
	case "deepseek", "glm", "grok", "kimi", "mistral", "minimax", "hunyuan", "doubao", "llama":
		return newCompatBuiltInChatProvider(providerCode, cfg, logger)
	default:
		return newOpenAICompatChatProvider(providerCode, cfg, logger)
	}
}

func canonicalizeChatProviderConfig(name string, cfg ChatProviderConfig) (string, ChatProviderConfig) {
	providerCode := strings.ToLower(strings.TrimSpace(name))
	if cfg.Extra == nil {
		cfg.Extra = map[string]any{}
	}
	switch providerCode {
	case "openai-responses", "openai-responses-api":
		cfg.Extra["use_responses_api"] = true
		return "openai", cfg
	case "anthropic-sdk", "anthropic-sdk-go", "claude-sdk":
		return "anthropic", cfg
	case "google", "google-genai":
		return "gemini", cfg
	case "vertex-ai", "vertexai":
		if _, exists := cfg.Extra["auth_type"]; !exists {
			cfg.Extra["auth_type"] = "oauth"
		}
		return "gemini-vertex", cfg
	default:
		return providerCode, cfg
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
