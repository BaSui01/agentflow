package image

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// OpenAIConfig 配置 OpenAI DALL-E 提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、BaseURL、Model、Timeout 字段。
type OpenAIConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// FluxConfig 配置 Black Forest Labs Flux 提供者.
type FluxConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// StabilityConfig 配置 Stability AI 提供者.
type StabilityConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultOpenAIConfig 返回默认 OpenAI 图像配置.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com",
			Model:   "dall-e-3",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultFluxConfig 返回默认 Flux 配置.
func DefaultFluxConfig() FluxConfig {
	return FluxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.bfl.ml",
			Model:   "flux-1.1-pro",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultStabilityConfig 返回默认 Stability AI 配置.
func DefaultStabilityConfig() StabilityConfig {
	return StabilityConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.stability.ai",
			Model:   "stable-diffusion-3.5-large",
			Timeout: 120 * time.Second,
		},
	}
}

// GeminiConfig 配置 Google Gemini 图像生成提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、Model、Timeout 字段。
type GeminiConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultGeminiConfig 返回默认 Gemini 图像配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "gemini-3-pro-image-preview",
			Timeout: 120 * time.Second,
		},
	}
}

// Imagen4Config 配置 Google Imagen 4 提供者.
type Imagen4Config struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultImagen4Config 返回默认 Imagen 4 配置.
func DefaultImagen4Config() Imagen4Config {
	return Imagen4Config{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "imagen-4.0-generate-preview",
			Timeout: 120 * time.Second,
		},
	}
}

