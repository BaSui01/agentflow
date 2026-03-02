package declarative

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// YAMLLoader tests
// ============================================================

func TestYAMLLoader_LoadFile_YAML(t *testing.T) {
	content := `
id: test-agent
name: Test Agent
description: A test agent
version: "1.0"
model: gpt-4
provider: openai
temperature: 0.7
max_tokens: 2048
system_prompt: You are a helpful assistant.
tools:
  - calculator
  - search
features:
  enable_reflection: true
  max_react_iterations: 5
metadata:
  env: test
`
	path := writeTemp(t, "agent.yaml", content)
	loader := NewYAMLLoader()

	def, err := loader.LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "test-agent", def.ID)
	assert.Equal(t, "Test Agent", def.Name)
	assert.Equal(t, "A test agent", def.Description)
	assert.Equal(t, "1.0", def.Version)
	assert.Equal(t, "gpt-4", def.Model)
	assert.Equal(t, "openai", def.Provider)
	assert.InDelta(t, 0.7, def.Temperature, 0.001)
	assert.Equal(t, 2048, def.MaxTokens)
	assert.Equal(t, "You are a helpful assistant.", def.SystemPrompt)
	assert.Equal(t, []string{"calculator", "search"}, def.Tools)
	assert.True(t, def.Features.EnableReflection)
	assert.Equal(t, 5, def.Features.MaxReActIterations)
	assert.Equal(t, "test", def.Metadata["env"])
}

func TestYAMLLoader_LoadFile_JSON(t *testing.T) {
	content := `{
  "id": "json-agent",
  "name": "JSON Agent",
  "model": "claude-3",
  "provider": "anthropic",
  "temperature": 0.5,
  "tools": ["web_search"],
  "features": {
    "enable_mcp": true
  }
}`
	path := writeTemp(t, "agent.json", content)
	loader := NewYAMLLoader()

	def, err := loader.LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "json-agent", def.ID)
	assert.Equal(t, "JSON Agent", def.Name)
	assert.Equal(t, "claude-3", def.Model)
	assert.Equal(t, "anthropic", def.Provider)
	assert.InDelta(t, 0.5, def.Temperature, 0.001)
	assert.Equal(t, []string{"web_search"}, def.Tools)
	assert.True(t, def.Features.EnableMCP)
}

func TestYAMLLoader_LoadFile_YMLExtension(t *testing.T) {
	content := `
name: YML Agent
model: gpt-4
`
	path := writeTemp(t, "agent.yml", content)
	loader := NewYAMLLoader()

	def, err := loader.LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "YML Agent", def.Name)
}

func TestYAMLLoader_LoadFile_NotFound(t *testing.T) {
	loader := NewYAMLLoader()
	_, err := loader.LoadFile(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read agent definition file")
}

func TestYAMLLoader_LoadFile_UnsupportedExtension(t *testing.T) {
	path := writeTemp(t, "agent.toml", "name = 'test'")
	loader := NewYAMLLoader()

	_, err := loader.LoadFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file extension")
}

func TestYAMLLoader_LoadFile_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "bad.yaml", "{{invalid yaml")
	loader := NewYAMLLoader()

	_, err := loader.LoadFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse YAML")
}

func TestYAMLLoader_LoadFile_InvalidJSON(t *testing.T) {
	path := writeTemp(t, "bad.json", "{invalid json}")
	loader := NewYAMLLoader()

	_, err := loader.LoadFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse JSON")
}

func TestYAMLLoader_LoadBytes_YAML(t *testing.T) {
	data := []byte(`
name: Bytes Agent
model: gpt-4
temperature: 0.3
`)
	loader := NewYAMLLoader()

	def, err := loader.LoadBytes(data, "yaml")
	require.NoError(t, err)
	assert.Equal(t, "Bytes Agent", def.Name)
	assert.Equal(t, "gpt-4", def.Model)
	assert.InDelta(t, 0.3, def.Temperature, 0.001)
}

func TestYAMLLoader_LoadBytes_JSON(t *testing.T) {
	data := []byte(`{"name": "JSON Bytes", "model": "claude-3"}`)
	loader := NewYAMLLoader()

	def, err := loader.LoadBytes(data, "json")
	require.NoError(t, err)
	assert.Equal(t, "JSON Bytes", def.Name)
	assert.Equal(t, "claude-3", def.Model)
}

func TestYAMLLoader_LoadBytes_UnsupportedFormat(t *testing.T) {
	loader := NewYAMLLoader()
	_, err := loader.LoadBytes([]byte("data"), "xml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestYAMLLoader_LoadBytes_MinimalDefinition(t *testing.T) {
	data := []byte(`name: Minimal
model: gpt-4`)
	loader := NewYAMLLoader()

	def, err := loader.LoadBytes(data, "yaml")
	require.NoError(t, err)
	assert.Equal(t, "Minimal", def.Name)
	assert.Equal(t, "gpt-4", def.Model)
	// Zero values for optional fields
	assert.Empty(t, def.ID)
	assert.Empty(t, def.Description)
	assert.Empty(t, def.Tools)
	assert.Zero(t, def.Temperature)
	assert.Zero(t, def.MaxTokens)
	assert.False(t, def.Features.EnableReflection)
}

// ============================================================
// AgentFactory tests
// ============================================================

func TestAgentFactory_Validate(t *testing.T) {
	tests := []struct {
		name    string
		def     *AgentDefinition
		wantErr string
	}{
		{
			name:    "nil definition",
			def:     nil,
			wantErr: "agent definition is nil",
		},
		{
			name:    "missing name",
			def:     &AgentDefinition{Model: "gpt-4"},
			wantErr: "name is required",
		},
		{
			name:    "missing model",
			def:     &AgentDefinition{Name: "test"},
			wantErr: "model is required",
		},
		{
			name: "temperature too high",
			def: &AgentDefinition{
				Name:        "test",
				Model:       "gpt-4",
				Temperature: 3.0,
			},
			wantErr: "temperature must be between 0 and 2",
		},
		{
			name: "negative temperature",
			def: &AgentDefinition{
				Name:        "test",
				Model:       "gpt-4",
				Temperature: -0.5,
			},
			wantErr: "temperature must be between 0 and 2",
		},
		{
			name: "negative max_tokens",
			def: &AgentDefinition{
				Name:      "test",
				Model:     "gpt-4",
				MaxTokens: -1,
			},
			wantErr: "max_tokens must be non-negative",
		},
		{
			name: "negative max_react_iterations",
			def: &AgentDefinition{
				Name:  "test",
				Model: "gpt-4",
				Features: AgentFeatures{
					MaxReActIterations: -1,
				},
			},
			wantErr: "max_react_iterations must be non-negative",
		},
		{
			name: "valid minimal",
			def: &AgentDefinition{
				Name:  "test",
				Model: "gpt-4",
			},
			wantErr: "",
		},
		{
			name: "valid full",
			def: &AgentDefinition{
				ID:           "agent-1",
				Name:         "Full Agent",
				Model:        "gpt-4",
				Provider:     "openai",
				Temperature:  1.5,
				MaxTokens:    4096,
				SystemPrompt: "You are helpful.",
				Tools:        []string{"calc"},
				Features: AgentFeatures{
					EnableReflection:   true,
					MaxReActIterations: 10,
				},
			},
			wantErr: "",
		},
	}

	factory := NewAgentFactory(zap.NewNop())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := factory.Validate(tt.def)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestAgentFactory_ToAgentConfig_Full(t *testing.T) {
	def := &AgentDefinition{
		ID:           "agent-1",
		Name:         "Full Agent",
		Description:  "A fully configured agent",
		Version:      "2.0",
		Model:        "gpt-4",
		Provider:     "openai",
		Temperature:  0.8,
		MaxTokens:    4096,
		SystemPrompt: "You are helpful.",
		Tools:        []string{"calculator", "search"},
		Features: AgentFeatures{
			EnableReflection:     true,
			EnableToolSelection:  true,
			EnablePromptEnhancer: true,
			EnableSkills:         true,
			EnableMCP:            true,
			EnableObservability:  true,
			MaxReActIterations:   15,
		},
		Metadata: map[string]string{"env": "prod", "team": "ai"},
	}

	factory := NewAgentFactory(zap.NewNop())
	cfg := factory.ToAgentConfig(def)

	assert.Equal(t, "agent-1", cfg.Core.ID)
	assert.Equal(t, "Full Agent", cfg.Core.Name)
	assert.Equal(t, "A fully configured agent", cfg.Core.Description)
	assert.Equal(t, "gpt-4", cfg.LLM.Model)
	assert.Equal(t, "openai", cfg.LLM.Provider)
	assert.Equal(t, "You are helpful.", cfg.Runtime.SystemPrompt)
	assert.InDelta(t, 0.8, cfg.LLM.Temperature, 0.001)
	assert.Equal(t, 4096, cfg.LLM.MaxTokens)
	assert.Equal(t, []string{"calculator", "search"}, cfg.Runtime.Tools)
	assert.Equal(t, 15, cfg.Runtime.MaxReActIterations)

	require.NotNil(t, cfg.Features.Reflection)
	require.NotNil(t, cfg.Features.ToolSelection)
	require.NotNil(t, cfg.Features.PromptEnhancer)
	require.NotNil(t, cfg.Extensions.Skills)
	require.NotNil(t, cfg.Extensions.MCP)
	require.NotNil(t, cfg.Extensions.Observability)
	assert.True(t, cfg.Features.Reflection.Enabled)
	assert.True(t, cfg.Features.ToolSelection.Enabled)
	assert.True(t, cfg.Features.PromptEnhancer.Enabled)
	assert.True(t, cfg.Extensions.Skills.Enabled)
	assert.True(t, cfg.Extensions.MCP.Enabled)
	assert.True(t, cfg.Extensions.Observability.Enabled)
	assert.Equal(t, "prod", cfg.Metadata["env"])
	assert.Equal(t, "ai", cfg.Metadata["team"])
}

func TestAgentFactory_ToAgentConfig_Minimal(t *testing.T) {
	def := &AgentDefinition{
		Name:  "Minimal Agent",
		Model: "gpt-4",
	}

	factory := NewAgentFactory(zap.NewNop())
	cfg := factory.ToAgentConfig(def)

	assert.Equal(t, "", cfg.Core.ID)
	assert.Equal(t, "Minimal Agent", cfg.Core.Name)
	assert.Equal(t, "gpt-4", cfg.LLM.Model)
	assert.Empty(t, cfg.Core.Description)
	assert.Empty(t, cfg.LLM.Provider)
	assert.Empty(t, cfg.Runtime.SystemPrompt)
	assert.Empty(t, cfg.Runtime.Tools)
	assert.Zero(t, cfg.Runtime.MaxReActIterations)
	assert.Nil(t, cfg.Features.Reflection)
	assert.Nil(t, cfg.Extensions.MCP)
	assert.Nil(t, cfg.Metadata)
}

func TestAgentFactory_ToAgentConfig_NilLogger(t *testing.T) {
	factory := NewAgentFactory(nil)
	def := &AgentDefinition{Name: "test", Model: "gpt-4"}
	cfg := factory.ToAgentConfig(def)
	assert.Equal(t, "test", cfg.Core.Name)
}

func TestYAMLLoader_LoadFile_WithMemoryAndGuardrails(t *testing.T) {
	content := `
name: Guarded Agent
model: gpt-4
type: react
memory:
  type: short_term
  capacity: 100
guardrails:
  max_retries: 3
  on_input_failure: reject
  on_output_failure: warn
tool_definitions:
  - name: calculator
    description: Performs arithmetic
  - name: search
    description: Web search
`
	path := writeTemp(t, "guarded.yaml", content)
	loader := NewYAMLLoader()

	def, err := loader.LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "Guarded Agent", def.Name)
	assert.Equal(t, "react", def.Type)

	require.NotNil(t, def.Memory)
	assert.Equal(t, "short_term", def.Memory.Type)
	assert.Equal(t, 100, def.Memory.Capacity)

	require.NotNil(t, def.Guardrails)
	assert.Equal(t, 3, def.Guardrails.MaxRetries)
	assert.Equal(t, "reject", def.Guardrails.OnInputFailure)
	assert.Equal(t, "warn", def.Guardrails.OnOutputFailure)

	require.Len(t, def.ToolDefinitions, 2)
	assert.Equal(t, "calculator", def.ToolDefinitions[0].Name)
	assert.Equal(t, "Performs arithmetic", def.ToolDefinitions[0].Description)
	assert.Equal(t, "search", def.ToolDefinitions[1].Name)
}

func TestAgentFactory_ToAgentConfig_WithNewFields(t *testing.T) {
	def := &AgentDefinition{
		Name:  "Full Agent",
		Model: "gpt-4",
		Type:  "plan_execute",
		Memory: &MemoryConfig{
			Type:     "both",
			Capacity: 500,
		},
		Guardrails: &GuardrailsConfig{
			MaxRetries:      2,
			OnInputFailure:  "reject",
			OnOutputFailure: "ignore",
		},
		ToolDefinitions: []ToolDefinition{
			{Name: "calc", Description: "Calculator"},
		},
	}

	factory := NewAgentFactory(zap.NewNop())
	cfg := factory.ToAgentConfig(def)

	assert.Equal(t, "plan_execute", cfg.Core.Type)
	require.NotNil(t, cfg.Features.Guardrails)
	assert.Equal(t, 2, cfg.Features.Guardrails.MaxRetries)
	assert.Equal(t, "reject", cfg.Features.Guardrails.OnInputFailure)
	assert.Equal(t, "ignore", cfg.Features.Guardrails.OnOutputFailure)
}

func TestAgentFactory_ToAgentConfig_NilMemoryAndGuardrails(t *testing.T) {
	def := &AgentDefinition{
		Name:  "Simple",
		Model: "gpt-4",
	}

	factory := NewAgentFactory(zap.NewNop())
	cfg := factory.ToAgentConfig(def)

	assert.Equal(t, "Simple", cfg.Core.Name)
	assert.Equal(t, "gpt-4", cfg.LLM.Model)
	assert.Empty(t, cfg.Core.Type)
	assert.Empty(t, cfg.Runtime.Tools)
	assert.Nil(t, cfg.Features.Guardrails)
	assert.Nil(t, cfg.Features.Reflection)
	assert.Nil(t, cfg.Extensions.MCP)
}

// ============================================================
// detectFormat tests
// ============================================================

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"agent.yaml", "yaml"},
		{"agent.YAML", "yaml"},
		{"agent.yml", "yaml"},
		{"agent.json", "json"},
		{"agent.JSON", "json"},
		{"agent.toml", ""},
		{"agent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, detectFormat(tt.path))
		})
	}
}

// ============================================================
// Helper
// ============================================================

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}
