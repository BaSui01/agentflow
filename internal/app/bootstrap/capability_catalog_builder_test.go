package bootstrap

import (
	"testing"

	"github.com/BaSui01/agentflow/agent/integration/hosted"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildCapabilityCatalog_CollectsRuntimeCapabilities(t *testing.T) {
	toolRegistry := hosted.NewToolRegistry(zap.NewNop())
	toolRegistry.Register(hosted.NewWebSearchTool(hosted.WebSearchConfig{Endpoint: "http://example.com"}))

	agentRegistry := agent.NewAgentRegistry(zap.NewNop())

	catalog := BuildCapabilityCatalog(toolRegistry, agentRegistry, []string{"parallel", "review"})
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
