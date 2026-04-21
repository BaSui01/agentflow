package main

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestToolRegistryRuntimeAdapter_ReloadBindings(t *testing.T) {
	runtime, err := bootstrap.BuildAgentToolingRuntime(bootstrap.AgentToolingOptions{}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, runtime)

	called := 0
	adapter := bootstrap.NewToolRegistryRuntimeAdapter(runtime, func(ctx context.Context) {
		_ = ctx
		called++
	})

	err = adapter.ReloadBindings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestToolRegistryRuntimeAdapter_NilRuntime(t *testing.T) {
	adapter := bootstrap.NewToolRegistryRuntimeAdapter(nil, nil)

	err := adapter.ReloadBindings(context.Background())
	require.NoError(t, err)
	assert.Nil(t, adapter.BaseToolNames())
}
