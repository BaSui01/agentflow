package image

import "time"

// OpenAIConfig配置了OpenAI DALL-E供应商.
type OpenAIConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // dall-e-3, gpt-image-1
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// FluxConfig配置了黑森林实验室Flux供应商.
type FluxConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // flux-1.1-pro
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 稳定Config配置稳定AI提供者.
type StabilityConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // stable-diffusion-3.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 OpenAIConfig 返回默认 OpenAI 图像配置 。
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseURL: "https://api.openai.com",
		Model:   "dall-e-3",
		Timeout: 120 * time.Second,
	}
}

// 默认FluxConfig 返回默认Flux配置 。
func DefaultFluxConfig() FluxConfig {
	return FluxConfig{
		BaseURL: "https://api.bfl.ml",
		Model:   "flux-1.1-pro",
		Timeout: 120 * time.Second,
	}
}

// 默认稳定 Config 返回默认稳定 AI 配置 。
func DefaultStabilityConfig() StabilityConfig {
	return StabilityConfig{
		BaseURL: "https://api.stability.ai",
		Model:   "stable-diffusion-3.5-large",
		Timeout: 120 * time.Second,
	}
}

// 双子座Config配置了谷歌双子座图像生成提供者.
type GeminiConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // gemini-3-pro-image-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认GeminiConfig返回默认双子星图像配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		Model:   "gemini-3-pro-image-preview",
		Timeout: 120 * time.Second,
	}
}

// Imagen4Config配置了Google Imagen 4供应商.
type Imagen4Config struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // imagen-4.0-generate-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 Imagen4Config 返回默认 Imisn 4 config 。
func DefaultImagen4Config() Imagen4Config {
	return Imagen4Config{
		Model:   "imagen-4.0-generate-preview",
		Timeout: 120 * time.Second,
	}
}
