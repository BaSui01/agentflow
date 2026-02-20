package image

import "time"

// OpenAIConfig 配置 OpenAI DALL-E 提供者.
type OpenAIConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // dall-e-3, gpt-image-1
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// FluxConfig 配置 Black Forest Labs Flux 提供者.
type FluxConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // flux-1.1-pro
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// StabilityConfig 配置 Stability AI 提供者.
type StabilityConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // stable-diffusion-3.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultOpenAIConfig 返回默认 OpenAI 图像配置.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseURL: "https://api.openai.com",
		Model:   "dall-e-3",
		Timeout: 120 * time.Second,
	}
}

// DefaultFluxConfig 返回默认 Flux 配置.
func DefaultFluxConfig() FluxConfig {
	return FluxConfig{
		BaseURL: "https://api.bfl.ml",
		Model:   "flux-1.1-pro",
		Timeout: 120 * time.Second,
	}
}

// DefaultStabilityConfig 返回默认 Stability AI 配置.
func DefaultStabilityConfig() StabilityConfig {
	return StabilityConfig{
		BaseURL: "https://api.stability.ai",
		Model:   "stable-diffusion-3.5-large",
		Timeout: 120 * time.Second,
	}
}

// GeminiConfig 配置 Google Gemini 图像生成提供者.
type GeminiConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // gemini-3-pro-image-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultGeminiConfig 返回默认 Gemini 图像配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		Model:   "gemini-3-pro-image-preview",
		Timeout: 120 * time.Second,
	}
}

// Imagen4Config 配置 Google Imagen 4 提供者.
type Imagen4Config struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // imagen-4.0-generate-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultImagen4Config 返回默认 Imagen 4 配置.
func DefaultImagen4Config() Imagen4Config {
	return Imagen4Config{
		Model:   "imagen-4.0-generate-preview",
		Timeout: 120 * time.Second,
	}
}
