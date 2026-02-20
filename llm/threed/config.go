package threed

import "time"

// MeshyConfig配置了 Meshy 3D提供者.
type MeshyConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 MeshyConfig 返回默认 Meshy 配置 。
func DefaultMeshyConfig() MeshyConfig {
	return MeshyConfig{
		BaseURL: "https://api.meshy.ai/v2",
		Model:   "meshy-4",
		Timeout: 600 * time.Second,
	}
}

// TripoConfig 配置 Tripo3D 提供者 。
type TripoConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 TripoConfig 返回默认 Tripo3D 配置 。
func DefaultTripoConfig() TripoConfig {
	return TripoConfig{
		BaseURL: "https://api.tripo3d.ai/v2",
		Model:   "tripo-v2.5",
		Timeout: 600 * time.Second,
	}
}
