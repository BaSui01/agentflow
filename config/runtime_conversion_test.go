package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConfig_ToRuntimeConfig(t *testing.T) {
	cfg := AgentConfig{
		Name:          "planner",
		Description:   "planning agent",
		Model:         "gpt-4o",
		SystemPrompt:  "You are a planner",
		MaxTokens:     2048,
		Temperature:   0.5,
		StreamEnabled: true,
		Memory: MemoryConfig{
			Enabled:     true,
			MaxMessages: 64,
		},
	}

	runtimeCfg := cfg.ToRuntimeConfig()
	assert.Equal(t, "planner", runtimeCfg.Core.ID)
	assert.Equal(t, "planner", runtimeCfg.Core.Name)
	assert.Equal(t, "generic", runtimeCfg.Core.Type)
	assert.Equal(t, "planning agent", runtimeCfg.Core.Description)
	assert.Equal(t, "gpt-4o", runtimeCfg.LLM.Model)
	assert.Equal(t, 2048, runtimeCfg.LLM.MaxTokens)
	assert.InDelta(t, 0.5, runtimeCfg.LLM.Temperature, 0.001)
	require.NotNil(t, runtimeCfg.Features.Memory)
	assert.Equal(t, 64, runtimeCfg.Features.Memory.MaxShortTermSize)
	assert.Equal(t, "You are a planner", runtimeCfg.Metadata["system_prompt"])
	assert.Equal(t, "true", runtimeCfg.Metadata["stream_enabled"])
}

func TestConfig_ToRuntimeConfig_NilReceiver(t *testing.T) {
	var cfg *Config
	runtimeCfg := cfg.ToRuntimeConfig()
	assert.Equal(t, "", runtimeCfg.Core.ID)
}
