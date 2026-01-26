package music

import "time"

// SunoConfig configures the Suno music provider.
type SunoConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultSunoConfig returns default Suno config.
func DefaultSunoConfig() SunoConfig {
	return SunoConfig{
		BaseURL: "https://api.sunoapi.com/v1",
		Model:   "suno-v5",
		Timeout: 300 * time.Second,
	}
}

// MiniMaxMusicConfig configures the MiniMax music provider.
type MiniMaxMusicConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultMiniMaxMusicConfig returns default MiniMax music config.
func DefaultMiniMaxMusicConfig() MiniMaxMusicConfig {
	return MiniMaxMusicConfig{
		BaseURL: "https://api.minimax.io",
		Model:   "music-01",
		Timeout: 300 * time.Second,
	}
}
