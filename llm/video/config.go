package video

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// GeminiConfig 配置 Google Gemini 视频理解提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、Model、Timeout 字段。
// 保留 ProjectID 和 Location 作为视频领域特有字段。
type GeminiConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
	ProjectID                   string `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Location                    string `json:"location,omitempty" yaml:"location,omitempty"`
}

// VeoConfig 配置 Google Veo 视频生成提供者.
type VeoConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// RunwayConfig 配置 Runway ML 视频生成提供者.
type RunwayConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// SoraConfig configures the OpenAI Sora video generation provider.
type SoraConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultGeminiConfig 返回默认 Gemini 视频配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "gemini-3-flash-preview",
			Timeout: 180 * time.Second,
		},
		Location: "us-central1",
	}
}

// DefaultVeoConfig 返回默认 Veo 配置.
func DefaultVeoConfig() VeoConfig {
	return VeoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "veo-3.1-generate-preview",
			Timeout: 300 * time.Second,
		},
	}
}

// DefaultRunwayConfig 返回默认 Runway 配置.
func DefaultRunwayConfig() RunwayConfig {
	return RunwayConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.runwayml.com",
			Model:   "gen-4.5",
			Timeout: 300 * time.Second,
		},
	}
}

// DefaultSoraConfig returns the default Sora configuration.
func DefaultSoraConfig() SoraConfig {
	return SoraConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com",
			Model:   "sora-2",
			Timeout: 300 * time.Second,
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
			Timeout: 300 * time.Second,
		},
	}
}

// LumaConfig 配置 Luma AI Dream Machine 视频生成提供者.
type LumaConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultLumaConfig 返回默认 Luma 配置.
func DefaultLumaConfig() LumaConfig {
	return LumaConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.lumalabs.ai",
			Model:   "ray-2",
			Timeout: 300 * time.Second,
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
			Timeout: 300 * time.Second,
		},
	}
}

