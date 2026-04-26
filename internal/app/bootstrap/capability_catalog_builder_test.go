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
	webSearchTool, err := hosted.NewProviderBackedWebSearchHostedTool(hosted.ToolProviderConfig{
		Provider:       string(hosted.ToolProviderDuckDuckGo),
		TimeoutSeconds: 15,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create web search tool: %v", err)
	}
	toolRegistry.Register(webSearchTool)

	agentRegistry := agent.NewAgentRegistry(zap.NewNop())

	catalog := BuildCapabilityCatalog(toolRegistry, agentRegistry, []string{"parallel", "review"})
	require.NotNil(t, catalog)

	assert.NotEmpty(t, catalog.GeneratedAt)
	assert.Contains(t, catalog.Tools, CapabilityTool{
		Name:        "web_search",
		Type:        string(hosted.ToolTypeWebSearch),
		Description: "Search the web for information. Returns a list of relevant results with titles, URLs, and snippets.",
	})
	assert.Contains(t, catalog.AgentTypes, CapabilityAgentType{Name: string(agent.TypeAssistant)})
	assert.Contains(t, catalog.Modes, CapabilityMode{Name: "parallel"})
	assert.Contains(t, catalog.Modes, CapabilityMode{Name: "review"})
}
