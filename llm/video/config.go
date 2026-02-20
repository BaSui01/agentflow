package video

import "time"

// 双子座Config配置了谷歌双子座视频理解提供者.
type GeminiConfig struct {
	APIKey    string        `json:"api_key" yaml:"api_key"`
	ProjectID string        `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Location  string        `json:"location,omitempty" yaml:"location,omitempty"`
	Model     string        `json:"model,omitempty" yaml:"model,omitempty"` // gemini-3-flash-preview, gemini-3-pro-preview
	Timeout   time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// VeoConfig配置了谷歌Veo视频生成供应商.
type VeoConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // veo-3.1-generate-preview
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// Runway Config 配置了 Runway ML 视频生成提供者.
type RunwayConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // gen-4, gen-4.5
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认GeminiConfig返回默认双子座视频配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		Model:    "gemini-3-flash-preview",
		Location: "us-central1",
		Timeout:  180 * time.Second,
	}
}

// 默认 VeoConfig 返回默认 Veo 配置 。
func DefaultVeoConfig() VeoConfig {
	return VeoConfig{
		Model:   "veo-3.1-generate-preview",
		Timeout: 300 * time.Second,
	}
}

// 默认 Runway Config 返回默认 Runway 配置 。
func DefaultRunwayConfig() RunwayConfig {
	return RunwayConfig{
		BaseURL: "https://api.runwayml.com",
		Model:   "gen-4.5",
		Timeout: 300 * time.Second,
	}
}
