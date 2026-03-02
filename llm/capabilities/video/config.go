package video

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// GeminiConfig configures the Google Gemini video understanding provider.
// It embeds providers.BaseProviderConfig to reuse APIKey, Model, and Timeout.
// ProjectID and Location remain Gemini-specific fields.
type GeminiConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
	ProjectID                    string `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Location                     string `json:"location,omitempty" yaml:"location,omitempty"`
}

// VeoConfig configures the Google Veo video generation provider.
type VeoConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// RunwayConfig configures the Runway ML video generation provider.
type RunwayConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// SoraConfig configures the OpenAI Sora video generation provider.
type SoraConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultGeminiConfig returns the default Gemini video configuration.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "gemini-3-flash-preview",
			Timeout: 180 * time.Second,
		},
		Location: "us-central1",
	}
}

// DefaultVeoConfig returns the default Veo configuration.
func DefaultVeoConfig() VeoConfig {
	return VeoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "veo-3.1-generate-preview",
			Timeout: defaultVideoTimeout,
		},
	}
}

// DefaultRunwayConfig returns the default Runway configuration.
func DefaultRunwayConfig() RunwayConfig {
	return RunwayConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.runwayml.com",
			Model:   "gen-4.5",
			Timeout: defaultVideoTimeout,
		},
	}
}

// DefaultSoraConfig returns the default Sora configuration.
func DefaultSoraConfig() SoraConfig {
	return SoraConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com",
			Model:   "sora-2",
			Timeout: defaultVideoTimeout,
		},
	}
}

// KlingConfig configures the Kling AI video generation provider.
type KlingConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultKlingConfig returns the default Kling configuration.
func DefaultKlingConfig() KlingConfig {
	return KlingConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.klingai.com",
			Model:   "kling-v3-pro",
			Timeout: defaultVideoTimeout,
		},
	}
}

// LumaConfig configures the Luma AI Dream Machine video generation provider.
type LumaConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultLumaConfig returns the default Luma configuration.
func DefaultLumaConfig() LumaConfig {
	return LumaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.lumalabs.ai",
			Model:   "ray-2",
			Timeout: defaultVideoTimeout,
		},
	}
}

// MiniMaxVideoConfig configures the MiniMax Hailuo video generation provider.
type MiniMaxVideoConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultMiniMaxVideoConfig returns the default MiniMax video configuration.
func DefaultMiniMaxVideoConfig() MiniMaxVideoConfig {
	return MiniMaxVideoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.minimax.chat",
			Model:   "video-01",
			Timeout: defaultVideoTimeout,
		},
	}
}
