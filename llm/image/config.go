package image

import "time"

// OpenAIConfig configures OpenAI DALL-E provider.
type OpenAIConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // dall-e-3, gpt-image-1
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// FluxConfig configures Black Forest Labs Flux provider.
type FluxConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // flux-1.1-pro
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// StabilityConfig configures Stability AI provider.
type StabilityConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // stable-diffusion-3.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultOpenAIConfig returns default OpenAI image config.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseURL: "https://api.openai.com",
		Model:   "dall-e-3",
		Timeout: 120 * time.Second,
	}
}

// DefaultFluxConfig returns default Flux config.
func DefaultFluxConfig() FluxConfig {
	return FluxConfig{
		BaseURL: "https://api.bfl.ml",
		Model:   "flux-1.1-pro",
		Timeout: 120 * time.Second,
	}
}

// DefaultStabilityConfig returns default Stability AI config.
func DefaultStabilityConfig() StabilityConfig {
	return StabilityConfig{
		BaseURL: "https://api.stability.ai",
		Model:   "stable-diffusion-3.5-large",
		Timeout: 120 * time.Second,
	}
}

// GeminiConfig configures Google Gemini image generation provider.
type GeminiConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // gemini-3-pro-image-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultGeminiConfig returns default Gemini image config.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		Model:   "gemini-3-pro-image-preview",
		Timeout: 120 * time.Second,
	}
}

// Imagen4Config configures Google Imagen 4 provider.
type Imagen4Config struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // imagen-4.0-generate-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultImagen4Config returns default Imagen 4 config.
func DefaultImagen4Config() Imagen4Config {
	return Imagen4Config{
		Model:   "imagen-4.0-generate-preview",
		Timeout: 120 * time.Second,
	}
}
