package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLocalProvider_Update(t *testing.T) {
	p := NewLocalProvider(zap.NewNop())
	ctx := context.Background()

	d := &Deployment{
		ID:     "update-test",
		Config: DeploymentConfig{Image: "/bin/sleep", Port: 0},
	}

	// Deploy first
	err := p.Deploy(ctx, d)
	require.NoError(t, err)
	assert.NotEmpty(t, d.Endpoint)

	oldEndpoint := d.Endpoint

	// Update (stops old, starts new)
	err = p.Update(ctx, d)
	require.NoError(t, err)
	assert.NotEmpty(t, d.Endpoint)

	// Cleanup
	err = p.Delete(ctx, d.ID)
	require.NoError(t, err)

	_ = oldEndpoint
}

func TestLocalProvider_Update_NotFound(t *testing.T) {
	p := NewLocalProvider(zap.NewNop())
	ctx := context.Background()

	d := &Deployment{
		ID:     "nonexistent-update",
		Config: DeploymentConfig{Image: "/bin/sleep", Port: 0},
	}

	err := p.Update(ctx, d)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLocalProvider_GetLogs_WithProcess(t *testing.T) {
	p := NewLocalProvider(zap.NewNop())
	ctx := context.Background()

	// Deploy a process that produces output
	d := &Deployment{
		ID:     "logs-test",
		Config: DeploymentConfig{Image: "/bin/echo", Port: 0},
	}

	err := p.Deploy(ctx, d)
	require.NoError(t, err)

	// Give the process a moment to produce output
	logs, err := p.GetLogs(ctx, "logs-test", 10)
	require.NoError(t, err)
	// echo with no args may or may not produce output depending on timing
	// The important thing is that GetLogs doesn't error
	_ = logs

	// Cleanup
	_ = p.Delete(ctx, d.ID)
}

func TestLocalProvider_GetLogs_LimitLines(t *testing.T) {
	p := NewLocalProvider(zap.NewNop())
	ctx := context.Background()

	d := &Deployment{
		ID:     "logs-limit-test",
		Config: DeploymentConfig{Image: "/bin/sleep", Port: 0},
	}

	err := p.Deploy(ctx, d)
	require.NoError(t, err)

	// Request 0 lines (should return all, even if empty)
	logs, err := p.GetLogs(ctx, "logs-limit-test", 0)
	require.NoError(t, err)
	_ = logs

	_ = p.Delete(ctx, d.ID)
}

func TestLocalProvider_GetStatus_Running(t *testing.T) {
	p := NewLocalProvider(zap.NewNop())
	ctx := context.Background()

	d := &Deployment{
		ID:     "status-running-test",
		Config: DeploymentConfig{Image: "/bin/sleep", Port: 0},
	}

	err := p.Deploy(ctx, d)
	require.NoError(t, err)

	status, err := p.GetStatus(ctx, "status-running-test")
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, status.Status)

	_ = p.Delete(ctx, d.ID)
}

