package deployment

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- DockerProvider unit tests (no Docker required) ---

func TestNewDockerProvider(t *testing.T) {
	p := NewDockerProvider(nil)
	require.NotNil(t, p)
	assert.NotNil(t, p.logger)
}

func TestDockerProvider_Deploy_NoImage(t *testing.T) {
	p := NewDockerProvider(nil)
	err := p.Deploy(context.Background(), &Deployment{
		ID:     "test-1",
		Config: DeploymentConfig{Image: ""},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires Config.Image")
}

func TestDockerProvider_Scale_NotSupported(t *testing.T) {
	p := NewDockerProvider(nil)
	err := p.Scale(context.Background(), "dep-1", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support scaling")
}

func TestContainerName(t *testing.T) {
	d := &Deployment{ID: "dep_abc123"}
	assert.Equal(t, "agentflow-dep_abc123", containerName(d))
}

func TestSplitDockerLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single line", "hello\n", []string{"hello"}},
		{"multiple lines", "line1\nline2\nline3\n", []string{"line1", "line2", "line3"}},
		{"with carriage return", "line1\r\nline2\r\n", []string{"line1", "line2"}},
		{"empty lines filtered", "line1\n\nline2\n", []string{"line1", "line2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitDockerLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- LocalProvider unit tests ---

func TestNewLocalProvider(t *testing.T) {
	p := NewLocalProvider(nil)
	require.NotNil(t, p)
	assert.NotNil(t, p.processes)
	assert.NotNil(t, p.logger)
}

func TestLocalProvider_Deploy_NoImage(t *testing.T) {
	p := NewLocalProvider(nil)
	err := p.Deploy(context.Background(), &Deployment{
		ID:     "test-1",
		Config: DeploymentConfig{Image: ""},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires Config.Image")
}

func TestLocalProvider_Scale_NotSupported(t *testing.T) {
	p := NewLocalProvider(nil)
	err := p.Scale(context.Background(), "dep-1", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support scaling")
}

func TestLocalProvider_GetStatus_NotFound(t *testing.T) {
	p := NewLocalProvider(nil)
	dep, err := p.GetStatus(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, dep.Status)
}

func TestLocalProvider_Delete_NotFound(t *testing.T) {
	p := NewLocalProvider(nil)
	err := p.Delete(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestLocalProvider_GetLogs_NotFound(t *testing.T) {
	p := NewLocalProvider(nil)
	_, err := p.GetLogs(context.Background(), "nonexistent", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFindAvailablePort(t *testing.T) {
	port, err := findAvailablePort()
	require.NoError(t, err)
	assert.Greater(t, port, 0)
	assert.Less(t, port, 65536)
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single line no newline", "hello", []string{"hello"}},
		{"single line with newline", "hello\n", []string{"hello"}},
		{"multiple lines", "line1\nline2\nline3\n", []string{"line1", "line2", "line3"}},
		{"with carriage return", "line1\r\nline2\r\n", []string{"line1", "line2"}},
		{"empty lines filtered", "line1\n\nline2\n", []string{"line1", "line2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- LocalProvider Deploy + Delete lifecycle with real process ---

func TestLocalProvider_DeployAndDelete(t *testing.T) {
	p := NewLocalProvider(zap.NewNop())
	ctx := context.Background()

	d := &Deployment{
		ID:     "lifecycle-test",
		Config: DeploymentConfig{Image: "/bin/sleep", Port: 0},
	}

	err := p.Deploy(ctx, d)
	require.NoError(t, err)
	assert.NotEmpty(t, d.Endpoint)

	// GetStatus should show running
	status, err := p.GetStatus(ctx, "lifecycle-test")
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, status.Status)

	// Delete
	err = p.Delete(ctx, "lifecycle-test")
	require.NoError(t, err)

	// After delete, status should be stopped
	status, err = p.GetStatus(ctx, "lifecycle-test")
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, status.Status)
}

// --- Deployer with multiple providers ---

func TestDeployer_MultipleProviders(t *testing.T) {
	deployer := NewDeployer(zap.NewNop())

	localProvider := &mockProvider{}
	k8sProvider := &mockProvider{
		deployFn: func(_ context.Context, d *Deployment) error {
			d.Endpoint = "http://k8s-cluster:8080"
			return nil
		},
	}

	deployer.RegisterProvider(TargetLocal, localProvider)
	deployer.RegisterProvider(TargetKubernetes, k8sProvider)

	ctx := context.Background()

	// Deploy to local
	dep1, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "local-agent",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, dep1.Status)

	// Deploy to k8s
	dep2, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "k8s-agent",
		AgentID: "a2",
		Target:  TargetKubernetes,
		Config:  DeploymentConfig{Image: "myimage:latest"},
	})
	require.NoError(t, err)
	assert.Equal(t, "http://k8s-cluster:8080", dep2.Endpoint)

	// List should show both
	assert.Len(t, deployer.ListDeployments(), 2)
}

// --- Deployer Scale failure ---

func TestDeployer_ScaleFailure(t *testing.T) {
	provider := &mockProvider{
		scaleFn: func(_ context.Context, _ string, _ int) error {
			return fmt.Errorf("scaling failed")
		},
	}

	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "scale-fail",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	err = deployer.Scale(ctx, dep.ID, 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scaling failed")
}

// --- Deployer Delete failure ---

func TestDeployer_DeleteFailure(t *testing.T) {
	provider := &mockProvider{
		deleteFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("delete failed")
		},
	}

	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "del-fail",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	err = deployer.Delete(ctx, dep.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete deployment")

	// Deployment should still exist
	_, err = deployer.GetDeployment(dep.ID)
	require.NoError(t, err)
}

// --- Deployer ExportManifest not found ---

func TestDeployer_ExportManifest_NotFound(t *testing.T) {
	deployer := NewDeployer(nil)
	_, err := deployer.ExportManifest("nonexistent")
	assert.Error(t, err)
}

// --- Deployer with metadata ---

func TestDeployer_DeployWithMetadata(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)

	dep, err := deployer.Deploy(context.Background(), DeployOptions{
		Name:    "meta-agent",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
		Metadata: map[string]string{
			"team":    "platform",
			"version": "v2",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "platform", dep.Metadata["team"])
	assert.Equal(t, "v2", dep.Metadata["version"])
}

// --- Deployer Update failure ---

func TestDeployer_UpdateFailure(t *testing.T) {
	provider := &mockProvider{
		updateFn: func(_ context.Context, _ *Deployment) error {
			return fmt.Errorf("update failed")
		},
	}

	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "upd-fail",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	err = deployer.Update(ctx, dep.ID, DeploymentConfig{Image: "new-image"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}
