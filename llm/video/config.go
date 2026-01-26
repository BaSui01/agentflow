package video

import "time"

// GeminiConfig configures Google Gemini video understanding provider.
type GeminiConfig struct {
	APIKey    string        `json:"api_key" yaml:"api_key"`
	ProjectID string        `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Location  string        `json:"location,omitempty" yaml:"location,omitempty"`
	Model     string        `json:"model,omitempty" yaml:"model,omitempty"` // gemini-3-flash-preview, gemini-3-pro-preview
	Timeout   time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// VeoConfig configures Google Veo video generation provider.
type VeoConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // veo-3.1-generate-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// RunwayConfig configures Runway ML video generation provider.
type RunwayConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // gen-4, gen-4.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultGeminiConfig returns default Gemini video config.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		Model:    "gemini-3-flash-preview",
		Location: "us-central1",
		Timeout:  180 * time.Second,
	}
}

// DefaultVeoConfig returns default Veo config.
func DefaultVeoConfig() VeoConfig {
	return VeoConfig{
		Model:   "veo-3.1-generate-preview",
		Timeout: 300 * time.Second,
	}
}

// DefaultRunwayConfig returns default Runway config.
func DefaultRunwayConfig() RunwayConfig {
	return RunwayConfig{
		BaseURL: "https://api.runwayml.com",
		Model:   "gen-4.5",
		Timeout: 300 * time.Second,
	}
}
