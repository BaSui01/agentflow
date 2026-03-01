package threed

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// MeshyConfig 配置 Meshy 3D 提供者.
type MeshyConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultMeshyConfig 返回默认 Meshy 配置.
func DefaultMeshyConfig() MeshyConfig {
	return MeshyConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.meshy.ai/v2",
			Model:   "meshy-4",
			Timeout: 600 * time.Second,
		},
	}
}

// TripoConfig 配置 Tripo3D 提供者.
type TripoConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultTripoConfig 返回默认 Tripo3D 配置.
func DefaultTripoConfig() TripoConfig {
	return TripoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.tripo3d.ai/v2",
			Model:   "tripo-v2.5",
			Timeout: 600 * time.Second,
		},
	}
}
