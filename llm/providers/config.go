package providers

import "time"

// BaseProviderConfig 所有 Provider 共享的基础配置字段。
// 通过嵌入此结构体，各 Provider 的 Config 自动获得 APIKey、BaseURL、Model、Timeout 四个字段，
// 避免重复定义。
type BaseProviderConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	APIKeys []string      `json:"api_keys,omitempty" yaml:"api_keys,omitempty"` // 多 API Key 支持，轮询使用
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Models  []string      `json:"models,omitempty" yaml:"models,omitempty"` // 可用模型白名单
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// OpenAIConfig OpenAI Provider 配置
type OpenAIConfig struct {
	BaseProviderConfig `yaml:",inline"`
	Organization       string `json:"organization,omitempty" yaml:"organization,omitempty"`
	UseResponsesAPI    bool   `json:"use_responses_api,omitempty" yaml:"use_responses_api,omitempty"` // 启用新的 Responses API (2025)
}

// ClaudeConfig Claude Provider 配置
type ClaudeConfig struct {
	BaseProviderConfig `yaml:",inline"`
	AuthType          string `json:"auth_type,omitempty" yaml:"auth_type,omitempty"`           // "api_key"(默认) | "bearer"
	AnthropicVersion  string `json:"anthropic_version,omitempty" yaml:"anthropic_version,omitempty"` // 默认 "2023-06-01"
}

// GeminiConfig Gemini Provider 配置
type GeminiConfig struct {
	BaseProviderConfig `yaml:",inline"`
	ProjectID string `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Region    string `json:"region,omitempty" yaml:"region,omitempty"`
	AuthType  string `json:"auth_type,omitempty" yaml:"auth_type,omitempty"` // "api_key"(默认) | "oauth"
}

// GrokConfig xAI Grok Provider 配置
type GrokConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// GLMConfig Zhipu AI GLM Provider 配置
type GLMConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// MiniMaxConfig MiniMax Provider 配置
type MiniMaxConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// QwenConfig Alibaba Qwen Provider 配置
type QwenConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// DeepSeekConfig DeepSeek Provider 配置
type DeepSeekConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// MistralConfig Mistral AI Provider 配置
type MistralConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// HunyuanConfig Tencent Hunyuan Provider 配置
type HunyuanConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// KimiConfig Moonshot Kimi Provider 配置
type KimiConfig struct {
	BaseProviderConfig `yaml:",inline"`
}

// LlamaConfig Meta Llama Provider 配置 (via Together AI/Replicate)
type LlamaConfig struct {
	BaseProviderConfig `yaml:",inline"`
	Provider           string `json:"provider,omitempty" yaml:"provider,omitempty"` // together/replicate/openrouter
}

// DoubaoConfig ByteDance Doubao Provider 配置
type DoubaoConfig struct {
	BaseProviderConfig `yaml:",inline"`
}
