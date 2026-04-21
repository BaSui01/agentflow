package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- Deployer edge cases: provider removed after deploy ---

func TestDeployer_Update_NoProviderForTarget(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "orphan-agent",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	// Remove the provider to simulate missing provider
	deployer.mu.Lock()
	delete(deployer.providers, TargetLocal)
	deployer.mu.Unlock()

	err = deployer.Update(ctx, dep.ID, DeploymentConfig{Image: "new"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider for target")
}

func TestDeployer_Delete_NoProviderForTarget(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "orphan-del",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	deployer.mu.Lock()
	delete(deployer.providers, TargetLocal)
	deployer.mu.Unlock()

	err = deployer.Delete(ctx, dep.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider for target")
}

func TestDeployer_Scale_NoProviderForTarget(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "orphan-scale",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	deployer.mu.Lock()
	delete(deployer.providers, TargetLocal)
	deployer.mu.Unlock()

	err = deployer.Scale(ctx, dep.ID, 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider for target")
}

// --- DockerProvider: Update, Delete, GetStatus, GetLogs (no Docker) ---

func TestDockerProvider_Update_NoDocker(t *testing.T) {
	p := NewDockerProvider(zap.NewNop())
	d := &Deployment{
		ID:     "docker-update-test",
		Config: DeploymentConfig{Image: "myimage:latest"},
	}
	// Update calls Delete then Deploy; Delete will fail without Docker
	err := p.Update(context.Background(), d)
	assert.Error(t, err)
}

func TestDockerProvider_Delete_NoDocker(t *testing.T) {
	p := NewDockerProvider(zap.NewNop())
	err := p.Delete(context.Background(), "nonexistent-container")
	assert.Error(t, err)
}

func TestDockerProvider_GetStatus_NoDocker(t *testing.T) {
	p := NewDockerProvider(zap.NewNop())
	dep, err := p.GetStatus(context.Background(), "nonexistent-container")
	require.NoError(t, err)
	// When docker inspect fails, returns StatusStopped
	assert.Equal(t, StatusStopped, dep.Status)
}

func TestDockerProvider_GetLogs_NoDocker(t *testing.T) {
	p := NewDockerProvider(zap.NewNop())
	_, err := p.GetLogs(context.Background(), "nonexistent-container", 10)
	assert.Error(t, err)
}

func TestDockerProvider_GetLogs_DefaultLines(t *testing.T) {
	p := NewDockerProvider(zap.NewNop())
	// lines <= 0 should default to 100
	_, err := p.GetLogs(context.Background(), "nonexistent-container", 0)
	assert.Error(t, err)
}

func TestDockerProvider_Deploy_WithEnvAndDefaultPort(t *testing.T) {
	p := NewDockerProvider(zap.NewNop())
	d := &Deployment{
		ID: "env-test",
		Config: DeploymentConfig{
			Image: "myimage:latest",
			Port:  0, // should default to 8080
			Environment: map[string]string{
				"KEY": "VALUE",
			},
		},
	}
	// Will fail because docker is not available, but exercises the code path
	err := p.Deploy(context.Background(), d)
	assert.Error(t, err)
}
