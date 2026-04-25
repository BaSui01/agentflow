package bootstrap

import (
	"context"
	"testing"

	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildAgentRegistries(t *testing.T) {
	discoveryRegistry, agentRegistry := BuildAgentRegistries(zap.NewNop())
	require.NotNil(t, discoveryRegistry)
	require.NotNil(t, agentRegistry)
}

func TestBuildAgentService_WithResolver(t *testing.T) {
	registry := discovery.NewCapabilityRegistry(nil, zap.NewNop())
	resolverCalled := false
	resolver := func(ctx context.Context, agentID string) (agent.Agent, error) {
		_ = ctx
		_ = agentID
		resolverCalled = true
		return nil, assert.AnError
	}

	svc := BuildAgentService(registry, resolver)
	require.NotNil(t, svc)
	_, err := svc.ResolveForOperation(context.Background(), "agent-1", usecase.AgentOperationExecute)
	require.NotNil(t, err)
	assert.True(t, resolverCalled)
}

func TestBuildAgentService_WithoutResolverFallsBackToRegistry(t *testing.T) {
	registry := discovery.NewCapabilityRegistry(nil, zap.NewNop())
	svc := BuildAgentService(registry, nil)
	require.NotNil(t, svc)
	_, err := svc.ResolveForOperation(context.Background(), "missing", usecase.AgentOperationExecute)
	require.NotNil(t, err)
}

func TestBuildToolingHandlerBundle_BuildsHandlersAndCatalog(t *testing.T) {
	db := setupToolRegistryTestDB(t)
	_, agentRegistry := BuildAgentRegistries(zap.NewNop())
	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "memory"

	bundle, err := BuildToolingHandlerBundle(ToolingHandlerBundleInput{
		Cfg:                 cfg,
		DB:                  db,
		Logger:              zap.NewNop(),
		ToolApprovalManager: hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), zap.NewNop()),
		AgentRegistry:       agentRegistry,
	})
	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.ToolingRuntime)
	require.NotNil(t, bundle.ToolRegistryHandler)
	require.NotNil(t, bundle.ToolProviderHandler)
	require.NotNil(t, bundle.ToolApprovalHandler)
	require.NotNil(t, bundle.AuthAuditHandler)
	require.Nil(t, bundle.ToolApprovalRedis)
	require.NotNil(t, bundle.CapabilityCatalog)
	assert.NotEmpty(t, bundle.CapabilityCatalog.AgentTypes)
	assert.NotEmpty(t, bundle.CapabilityCatalog.Modes)
}

func TestBuildToolingHandlerBundle_RegistersConfiguredShellAndFileOpsBehindToolManager(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "memory"
	cfg.HostedTools.FileOps.Enabled = true
	cfg.HostedTools.FileOps.AllowedPaths = []string{t.TempDir()}
	cfg.HostedTools.FileOps.MaxFileSize = 1024
	cfg.HostedTools.Shell.Enabled = true

	bundle, err := BuildToolingHandlerBundle(ToolingHandlerBundleInput{
		Cfg:    cfg,
		Logger: zap.NewNop(),
	})

	require.NoError(t, err)
	require.NotNil(t, bundle)
	require.NotNil(t, bundle.ToolingRuntime)
	require.NotNil(t, bundle.ToolingRuntime.ToolManager)
	assert.Contains(t, bundle.ToolingRuntime.ToolNames, "run_command")
	assert.Contains(t, bundle.ToolingRuntime.ToolNames, "write_file")
	assert.Contains(t, bundle.ToolingRuntime.ToolNames, "edit_file")
	assert.Contains(t, bundle.ToolingRuntime.ToolNames, "read_file")
	assert.Contains(t, bundle.ToolingRuntime.ToolNames, "list_directory")
}
