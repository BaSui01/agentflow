package bootstrap

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/agent/multiagent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildCapabilityCatalog_CollectsRuntimeCapabilities(t *testing.T) {
	toolRegistry := hosted.NewToolRegistry(zap.NewNop())
	toolRegistry.Register(hosted.NewWebSearchTool(hosted.WebSearchConfig{Endpoint: "http://example.com"}))

	agentRegistry := agent.NewAgentRegistry(zap.NewNop())
	modeRegistry := multiagent.NewModeRegistry()
	modeRegistry.Register(testModeStrategy{name: "parallel"})
	modeRegistry.Register(testModeStrategy{name: "review"})

	catalog := BuildCapabilityCatalog(toolRegistry, agentRegistry, modeRegistry)
	require.NotNil(t, catalog)

	assert.NotEmpty(t, catalog.GeneratedAt)
	assert.Contains(t, catalog.Tools, CapabilityTool{
		Name:        "web_search",
		Type:        string(hosted.ToolTypeWebSearch),
		Description: "Search the web for current information",
	})
	assert.Contains(t, catalog.AgentTypes, CapabilityAgentType{Name: string(agent.TypeAssistant)})
	assert.Contains(t, catalog.Modes, CapabilityMode{Name: "parallel"})
	assert.Contains(t, catalog.Modes, CapabilityMode{Name: "review"})
}

type testModeStrategy struct {
	name string
}

func (t testModeStrategy) Name() string { return t.name }

func (t testModeStrategy) Execute(_ context.Context, _ []agent.Agent, _ *agent.Input) (*agent.Output, error) {
	return &agent.Output{}, nil
}
