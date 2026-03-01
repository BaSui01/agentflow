package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultBuildOptions(t *testing.T) {
	opts := DefaultBuildOptions()
	assert.True(t, opts.EnableAll)
	assert.True(t, opts.EnableReflection)
	assert.True(t, opts.EnableToolSelection)
	assert.True(t, opts.EnablePromptEnhancer)
	assert.True(t, opts.EnableSkills)
	assert.True(t, opts.EnableMCP)
	assert.True(t, opts.EnableLSP)
	assert.True(t, opts.EnableEnhancedMemory)
	assert.True(t, opts.EnableObservability)
	assert.Equal(t, "./skills", opts.SkillsDirectory)
	assert.Equal(t, "agentflow-mcp", opts.MCPServerName)
	assert.Equal(t, "0.1.0", opts.MCPServerVersion)
	assert.Equal(t, "agentflow-lsp", opts.LSPServerName)
	assert.Equal(t, "0.1.0", opts.LSPServerVersion)
	assert.False(t, opts.InitAgent)
}

func TestEnabled(t *testing.T) {
	tests := []struct {
		name string
		all  bool
		v    bool
		want bool
	}{
		{"both true", true, true, true},
		{"all true, v false", true, false, true},
		{"all false, v true", false, true, true},
		{"both false", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, enabled(tt.all, tt.v))
		})
	}
}

func TestBuildAgent_NilProvider(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	opts := BuildOptions{} // all disabled

	// Build should fail because provider is nil
	_, err := BuildAgent(context.Background(), cfg, nil, zap.NewNop(), opts)
	require.Error(t, err)
}

func TestBuildAgent_NilLogger(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	opts := BuildOptions{}

	// Should not panic with nil logger (it defaults to nop)
	_, err := BuildAgent(context.Background(), cfg, nil, nil, opts)
	// Will still fail due to nil provider, but shouldn't panic
	require.Error(t, err)
}

func TestQuickSetup_NilAgent(t *testing.T) {
	err := QuickSetup(context.Background(), nil, BuildOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent is nil")
}

func TestBuildAgent_AllDisabled(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	provider := mocks.NewSuccessProvider("hello")
	opts := BuildOptions{} // all disabled

	ag, err := BuildAgent(context.Background(), cfg, provider, zap.NewNop(), opts)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuildAgent_WithSubsystems(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	provider := mocks.NewSuccessProvider("hello")
	opts := BuildOptions{
		EnableReflection:     true,
		EnableToolSelection:  true,
		EnablePromptEnhancer: true,
		EnableSkills:         true,
		EnableMCP:            true,
		EnableLSP:            true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
		SkillsDirectory:      t.TempDir(),
		MCPServerName:        "test-mcp",
		MCPServerVersion:     "0.1.0",
		LSPServerName:        "test-lsp",
		LSPServerVersion:     "0.1.0",
	}

	ag, err := BuildAgent(context.Background(), cfg, provider, zap.NewNop(), opts)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestBuildAgent_EnableAll(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	provider := mocks.NewSuccessProvider("hello")
	opts := DefaultBuildOptions()
	opts.InitAgent = false
	opts.SkillsDirectory = t.TempDir()

	ag, err := BuildAgent(context.Background(), cfg, provider, zap.NewNop(), opts)
	require.NoError(t, err)
	require.NotNil(t, ag)
}

func TestQuickSetup_WithAgent(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	provider := mocks.NewSuccessProvider("hello")

	// Build a minimal agent first
	ag, err := BuildAgent(context.Background(), cfg, provider, zap.NewNop(), BuildOptions{})
	require.NoError(t, err)

	// QuickSetup with subsystems
	opts := BuildOptions{
		EnableSkills:         true,
		EnableMCP:            true,
		EnableLSP:            true,
		EnableEnhancedMemory: true,
		EnableObservability:  true,
		SkillsDirectory:      t.TempDir(),
		MCPServerName:        "test-mcp",
		MCPServerVersion:     "0.1.0",
		LSPServerName:        "test-lsp",
		LSPServerVersion:     "0.1.0",
	}

	err = QuickSetup(context.Background(), ag, opts)
	require.NoError(t, err)
}

func TestQuickSetup_AllEnabled(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	provider := mocks.NewSuccessProvider("hello")

	ag, err := BuildAgent(context.Background(), cfg, provider, zap.NewNop(), BuildOptions{})
	require.NoError(t, err)

	opts := DefaultBuildOptions()
	opts.InitAgent = false
	opts.SkillsDirectory = t.TempDir()

	err = QuickSetup(context.Background(), ag, opts)
	require.NoError(t, err)
}

func TestQuickSetup_EmptyNames(t *testing.T) {
	cfg := agent.Config{
		ID:    "test-agent",
		Name:  "Test",
		Type:  agent.TypeAssistant,
		Model: "gpt-4",
	}
	provider := mocks.NewSuccessProvider("hello")

	ag, err := BuildAgent(context.Background(), cfg, provider, zap.NewNop(), BuildOptions{})
	require.NoError(t, err)

	// Empty names should use defaults
	opts := BuildOptions{
		EnableMCP:        true,
		EnableLSP:        true,
		MCPServerName:    "",
		MCPServerVersion: "",
		LSPServerName:    "",
		LSPServerVersion: "",
	}

	err = QuickSetup(context.Background(), ag, opts)
	require.NoError(t, err)
}
