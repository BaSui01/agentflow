package bootstrap

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/agent"
	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
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
