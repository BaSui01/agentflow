package music

import "time"

// SunoConfig配置了Suno音乐提供者.
type SunoConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认SunoConfig 返回默认的 Suno 配置 。
func DefaultSunoConfig() SunoConfig {
	return SunoConfig{
		BaseURL: "https://api.sunoapi.com/v1",
		Model:   "suno-v5",
		Timeout: 300 * time.Second,
	}
}

// MiniMaxMusicConfig配置了MiniMax音乐提供者.
type MiniMaxMusicConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认MiniMaxMusicConfig返回默认的MiniMax音乐配置.
func DefaultMiniMaxMusicConfig() MiniMaxMusicConfig {
	return MiniMaxMusicConfig{
		BaseURL: "https://api.minimax.io",
		Model:   "music-01",
		Timeout: 300 * time.Second,
	}
}
