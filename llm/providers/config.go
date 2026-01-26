package providers

import "time"

// OpenAIConfig OpenAI Provider 配置
type OpenAIConfig struct {
	APIKey          string        `json:"api_key" yaml:"api_key"`
	BaseURL         string        `json:"base_url" yaml:"base_url"`
	Organization    string        `json:"organization,omitempty" yaml:"organization,omitempty"`
	Model           string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	UseResponsesAPI bool          `json:"use_responses_api,omitempty" yaml:"use_responses_api,omitempty"` // 启用新的 Responses API (2025)
}

// ClaudeConfig Claude Provider 配置
type ClaudeConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// GeminiConfig Gemini Provider 配置
type GeminiConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// GrokConfig xAI Grok Provider 配置
type GrokConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// GLMConfig Zhipu AI GLM Provider 配置
type GLMConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// MiniMaxConfig MiniMax Provider 配置
type MiniMaxConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// QwenConfig Alibaba Qwen Provider 配置
type QwenConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DeepSeekConfig DeepSeek Provider 配置
type DeepSeekConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// MistralConfig Mistral AI Provider 配置
type MistralConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// HunyuanConfig Tencent Hunyuan Provider 配置
type HunyuanConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// KimiConfig Moonshot Kimi Provider 配置
type KimiConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// LlamaConfig Meta Llama Provider 配置 (via Together AI/Replicate)
type LlamaConfig struct {
	APIKey   string        `json:"api_key" yaml:"api_key"`
	BaseURL  string        `json:"base_url" yaml:"base_url"`
	Model    string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Provider string        `json:"provider,omitempty" yaml:"provider,omitempty"` // together/replicate/openrouter
}

// DoubaoConfig ByteDance Doubao Provider 配置
type DoubaoConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"` // Default: https://ark.cn-beijing.volces.com/api/v3
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // Default: Doubao-1.5-pro-32k
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}
