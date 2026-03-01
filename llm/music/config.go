package music

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// SunoConfig 配置 Suno 音乐提供者.
type SunoConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultSunoConfig 返回默认 Suno 配置.
func DefaultSunoConfig() SunoConfig {
	return SunoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.sunoapi.com/v1",
			Model:   "suno-v5",
			Timeout: 300 * time.Second,
		},
	}
}

// MiniMaxMusicConfig 配置 MiniMax 音乐提供者.
type MiniMaxMusicConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultMiniMaxMusicConfig 返回默认 MiniMax 音乐配置.
func DefaultMiniMaxMusicConfig() MiniMaxMusicConfig {
	return MiniMaxMusicConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.minimax.io",
			Model:   "music-01",
			Timeout: 300 * time.Second,
		},
	}
}

