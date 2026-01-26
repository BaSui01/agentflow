package threed

import "time"

// MeshyConfig configures the Meshy 3D provider.
type MeshyConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultMeshyConfig returns default Meshy config.
func DefaultMeshyConfig() MeshyConfig {
	return MeshyConfig{
		BaseURL: "https://api.meshy.ai/v2",
		Model:   "meshy-4",
		Timeout: 600 * time.Second,
	}
}

// TripoConfig configures the Tripo3D provider.
type TripoConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultTripoConfig returns default Tripo3D config.
func DefaultTripoConfig() TripoConfig {
	return TripoConfig{
		BaseURL: "https://api.tripo3d.ai/v2",
		Model:   "tripo-v2.5",
		Timeout: 600 * time.Second,
	}
}
