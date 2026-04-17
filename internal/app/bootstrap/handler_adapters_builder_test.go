package bootstrap

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/agent/hosted"
	mcpproto "github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestBuildChatHandler_ReturnsNilWithoutProvider(t *testing.T) {
	handler := BuildChatHandler(nil, nil, zap.NewNop())
	require.Nil(t, handler)
}

func TestBuildChatHandler_BindsRuntimeAndToolManager(t *testing.T) {
	runtime := &LLMHandlerRuntime{
		Provider:      &handlerRuntimeProvider{name: "main"},
		PolicyManager: nil,
		Ledger:        nil,
	}
	tooling := &AgentToolingRuntime{ToolManager: &testToolManager{}}

	handler := BuildChatHandler(runtime, tooling, zap.NewNop())
	require.NotNil(t, handler)
}

func TestBuildToolingHandlerBundle_WithDBAndApprovalManager(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&hosted.ToolRegistration{}, &hosted.ToolProviderConfig{}))

	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "memory"
	cfg.HostedTools.Approval.Scope = "request"

	discoveryRegistry, agentRegistry := BuildAgentRegistries(zap.NewNop())
	require.NotNil(t, discoveryRegistry)
	require.NotNil(t, agentRegistry)

	bundle, err := BuildToolingHandlerBundle(ToolingHandlerOptions{
		Config:             cfg,
		DB:                 db,
		MCPServer:          &testBootstrapMCPServer{},
		EnableMCPTools:     true,
		ApprovalManager:    hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		AgentRegistry:      agentRegistry,
		ResolverResetCache: func(context.Context) {},
		Logger:             zap.NewNop(),
	})
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.ToolingRuntime)
	require.NotNil(t, bundle.RegistryRuntime)
	require.NotNil(t, bundle.ToolRegistryHandler)
	require.NotNil(t, bundle.ToolProviderHandler)
	require.NotNil(t, bundle.ToolApprovalHandler)
	require.NotNil(t, bundle.CapabilityCatalog)
	assert.NotEmpty(t, bundle.CapabilityCatalog.Modes)
}

func TestBuildToolingHandlerBundle_WithoutDBDisablesDBHandlers(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "memory"

	_, agentRegistry := BuildAgentRegistries(zap.NewNop())
	bundle, err := BuildToolingHandlerBundle(ToolingHandlerOptions{
		Config:          cfg,
		DB:              nil,
		MCPServer:       &testBootstrapMCPServer{},
		EnableMCPTools:  true,
		ApprovalManager: hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		AgentRegistry:   agentRegistry,
		Logger:          zap.NewNop(),
	})
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.ToolingRuntime)
	require.Nil(t, bundle.ToolRegistryHandler)
	require.Nil(t, bundle.ToolProviderHandler)
	require.NotNil(t, bundle.ToolApprovalHandler)
}

func TestApplyReloadedTextRuntimeBindings_WarnsWhenRoutesWereNotBoundAtStartup(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	runtime := &LLMHandlerRuntime{
		Provider:      &handlerRuntimeProvider{name: "main"},
		CostTracker:   observability.NewCostTracker(observability.NewCostCalculator()),
		PolicyManager: nil,
	}

	result := ApplyReloadedTextRuntimeBindings(ReloadedTextRuntimeOptions{
		Runtime:         runtime,
		HTTPServerBound: true,
		Logger:          logger,
	})

	require.Nil(t, result.ChatHandler)
	require.Nil(t, result.CostHandler)
	messages := observed.FilterLevelExact(zap.WarnLevel).All()
	require.Len(t, messages, 2)
	assert.Contains(t, messages[0].Message, "chat runtime")
	assert.Contains(t, messages[1].Message, "cost runtime")
}

func TestApplyReloadedTextRuntimeBindings_CreatesHandlersBeforeHTTPBinding(t *testing.T) {
	logger := zap.NewNop()
	runtime := &LLMHandlerRuntime{
		Provider:      &handlerRuntimeProvider{name: "main"},
		CostTracker:   observability.NewCostTracker(observability.NewCostCalculator()),
		PolicyManager: nil,
	}
	tooling := &AgentToolingRuntime{ToolManager: &testToolManager{}}

	result := ApplyReloadedTextRuntimeBindings(ReloadedTextRuntimeOptions{
		Runtime:         runtime,
		ToolingRuntime:  tooling,
		HTTPServerBound: false,
		Logger:          logger,
	})

	require.NotNil(t, result.ChatHandler)
	require.NotNil(t, result.CostHandler)
}

type handlerRuntimeProvider struct {
	name string
}

func (p *handlerRuntimeProvider) Completion(context.Context, *types.ChatRequest) (*types.ChatResponse, error) {
	return &types.ChatResponse{}, nil
}

func (p *handlerRuntimeProvider) Stream(context.Context, *types.ChatRequest) (<-chan types.StreamChunk, error) {
	ch := make(chan types.StreamChunk)
	close(ch)
	return ch, nil
}

func (p *handlerRuntimeProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *handlerRuntimeProvider) SupportsNativeFunctionCalling() bool { return false }
func (p *handlerRuntimeProvider) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}
func (p *handlerRuntimeProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
func (p *handlerRuntimeProvider) Name() string                     { return p.name }

type testToolManager struct{}

func (m *testToolManager) GetAllowedTools(string) []types.ToolSchema { return nil }
func (m *testToolManager) ExecuteForAgent(context.Context, string, []types.ToolCall) []llmtools.ToolResult {
	return nil
}

type testBootstrapMCPServer struct{}

func (s *testBootstrapMCPServer) GetServerInfo() mcpproto.ServerInfo { return mcpproto.ServerInfo{} }
func (s *testBootstrapMCPServer) ListResources(context.Context) ([]mcpproto.Resource, error) {
	return nil, nil
}
func (s *testBootstrapMCPServer) GetResource(context.Context, string) (*mcpproto.Resource, error) {
	return nil, nil
}
func (s *testBootstrapMCPServer) SubscribeResource(context.Context, string) (<-chan mcpproto.Resource, error) {
	ch := make(chan mcpproto.Resource)
	close(ch)
	return ch, nil
}
func (s *testBootstrapMCPServer) ListTools(context.Context) ([]mcpproto.ToolDefinition, error) {
	return []mcpproto.ToolDefinition{{Name: "echo"}}, nil
}
func (s *testBootstrapMCPServer) CallTool(context.Context, string, map[string]any) (any, error) {
	return map[string]any{"ok": true}, nil
}
func (s *testBootstrapMCPServer) ListPrompts(context.Context) ([]mcpproto.PromptTemplate, error) {
	return nil, nil
}
func (s *testBootstrapMCPServer) GetPrompt(context.Context, string, map[string]string) (string, error) {
	return "", nil
}
func (s *testBootstrapMCPServer) SetLogLevel(string) error { return nil }

var _ agent.ToolManager = (*testToolManager)(nil)
var _ mcpproto.MCPServer = (*testBootstrapMCPServer)(nil)
