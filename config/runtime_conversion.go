package config

import (
	"strconv"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

// ToRuntimeConfig converts deployment-time config.AgentConfig into runtime
// types.AgentConfig. This is the single explicit conversion path between layers.
func (c AgentConfig) ToRuntimeConfig() types.AgentConfig {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = "default-agent"
	}

	runtimeCfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:          name,
			Name:        name,
			Type:        "generic",
			Description: c.Description,
		},
		LLM: types.LLMConfig{
			Model:       c.Model,
			MaxTokens:   c.MaxTokens,
			Temperature: float32(c.Temperature),
		},
		Metadata: map[string]string{
			"system_prompt":  c.SystemPrompt,
			"stream_enabled": strconv.FormatBool(c.StreamEnabled),
		},
	}

	if c.Memory.Enabled {
		runtimeCfg.Features.Memory = &types.MemoryConfig{
			Enabled:          true,
			MaxShortTermSize: c.Memory.MaxMessages,
		}
	}

	return runtimeCfg
}

// ToRuntimeConfig converts the root deployment config to runtime agent config.
func (c *Config) ToRuntimeConfig() types.AgentConfig {
	if c == nil {
		return types.AgentConfig{}
	}
	return c.Agent.ToRuntimeConfig()
}

