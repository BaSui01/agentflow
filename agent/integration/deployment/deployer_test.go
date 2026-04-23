package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// mockProvider implements DeploymentProvider using function callbacks.
type mockProvider struct {
	deployFn    func(ctx context.Context, d *Deployment) error
	updateFn    func(ctx context.Context, d *Deployment) error
	deleteFn    func(ctx context.Context, id string) error
	getStatusFn func(ctx context.Context, id string) (*Deployment, error)
	scaleFn     func(ctx context.Context, id string, replicas int) error
	getLogsFn   func(ctx context.Context, id string, lines int) ([]string, error)
}

func (m *mockProvider) Deploy(ctx context.Context, d *Deployment) error {
	if m.deployFn != nil {
		return m.deployFn(ctx, d)
	}
	return nil
}

func (m *mockProvider) Update(ctx context.Context, d *Deployment) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, d)
	}
	return nil
}

func (m *mockProvider) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockProvider) GetStatus(ctx context.Context, id string) (*Deployment, error) {
	if m.getStatusFn != nil {
		return m.getStatusFn(ctx, id)
	}
	return &Deployment{ID: id, Status: StatusRunning}, nil
}

func (m *mockProvider) Scale(ctx context.Context, id string, replicas int) error {
	if m.scaleFn != nil {
		return m.scaleFn(ctx, id, replicas)
	}
	return nil
}

func (m *mockProvider) GetLogs(ctx context.Context, id string, lines int) ([]string, error) {
	if m.getLogsFn != nil {
		return m.getLogsFn(ctx, id, lines)
	}
	return []string{"log line 1"}, nil
}

func newTestDeployer(provider *mockProvider) *Deployer {
	d := NewDeployer(zap.NewNop())
	d.RegisterProvider(TargetLocal, provider)
	return d
}

func TestDeployLifecycle(t *testing.T) {
	var deployed, deleted atomic.Int32
	provider := &mockProvider{
		deployFn: func(_ context.Context, d *Deployment) error {
			deployed.Add(1)
			d.Endpoint = "http://127.0.0.1:9090"
			return nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			deleted.Add(1)
			return nil
		},
	}

	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "test-agent",
		AgentID: "agent-1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo", Port: 9090},
	})
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, dep.Status)
	assert.Equal(t, "http://127.0.0.1:9090", dep.Endpoint)
	assert.Equal(t, int32(1), deployed.Load())
	assert.True(t, strings.HasPrefix(dep.ID, "dep_"))
	assert.Equal(t, 1, dep.Replicas) // default

	// Get deployment
	got, err := deployer.GetDeployment(dep.ID)
	require.NoError(t, err)
	assert.Equal(t, dep.ID, got.ID)

	// Delete
	err = deployer.Delete(ctx, dep.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), deleted.Load())

	// Should be gone
	_, err = deployer.GetDeployment(dep.ID)
	assert.Error(t, err)
}

func TestDeployWithReplicas(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)

	dep, err := deployer.Deploy(context.Background(), DeployOptions{
		Name:     "replica-agent",
		AgentID:  "agent-r",
		Target:   TargetLocal,
		Replicas: 3,
		Config:   DeploymentConfig{Image: "/bin/echo", Port: 8080},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, dep.Replicas)
	assert.Equal(t, StatusRunning, dep.Status)
}

func TestDeployWithResources(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)

	dep, err := deployer.Deploy(context.Background(), DeployOptions{
		Name:    "resource-agent",
		AgentID: "agent-res",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
		Resources: ResourceConfig{
			CPURequest:    "100m",
			CPULimit:      "500m",
			MemoryRequest: "128Mi",
			MemoryLimit:   "512Mi",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "100m", dep.Resources.CPURequest)
	assert.Equal(t, "512Mi", dep.Resources.MemoryLimit)
}

func TestUpdateDeployment(t *testing.T) {
	var updateCalled atomic.Int32
	provider := &mockProvider{
		updateFn: func(_ context.Context, d *Deployment) error {
			updateCalled.Add(1)
			return nil
		},
	}

	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "update-agent",
		AgentID: "agent-u",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	err = deployer.Update(ctx, dep.ID, DeploymentConfig{Image: "/bin/echo", Port: 9090})
	require.NoError(t, err)
	assert.Equal(t, int32(1), updateCalled.Load())
}

func TestUpdateDeploymentNotFound(t *testing.T) {
	deployer := newTestDeployer(&mockProvider{})
	err := deployer.Update(context.Background(), "nonexistent", DeploymentConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deployment not found")
}

func TestDeleteNotFound(t *testing.T) {
	deployer := newTestDeployer(&mockProvider{})
	err := deployer.Delete(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deployment not found")
}

func TestDeployFailure(t *testing.T) {
	provider := &mockProvider{
		deployFn: func(_ context.Context, _ *Deployment) error {
			return fmt.Errorf("connection refused")
		},
	}

	deployer := newTestDeployer(provider)
	dep, err := deployer.Deploy(context.Background(), DeployOptions{
		Name:    "fail-agent",
		AgentID: "agent-2",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "bad-image"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deployment failed")
	assert.Equal(t, StatusFailed, dep.Status)
}

func TestScale(t *testing.T) {
	var scaledTo int
	provider := &mockProvider{
		scaleFn: func(_ context.Context, _ string, replicas int) error {
			scaledTo = replicas
			return nil
		},
	}

	deployer := newTestDeployer(provider)
	ctx := context.Background()

	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:     "scale-agent",
		AgentID:  "agent-3",
		Target:   TargetLocal,
		Replicas: 1,
		Config:   DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	err = deployer.Scale(ctx, dep.ID, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, scaledTo)

	got, err := deployer.GetDeployment(dep.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, got.Replicas)
}

func TestScaleNotFound(t *testing.T) {
	deployer := newTestDeployer(&mockProvider{})
	err := deployer.Scale(context.Background(), "nonexistent", 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deployment not found")
}

func TestNoProvider(t *testing.T) {
	deployer := NewDeployer(zap.NewNop())
	_, err := deployer.Deploy(context.Background(), DeployOptions{
		Name:   "test",
		Target: TargetKubernetes,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider for target")
}

func TestExportManifest(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)

	dep, err := deployer.Deploy(context.Background(), DeployOptions{
		Name:     "manifest-agent",
		AgentID:  "agent-4",
		Target:   TargetLocal,
		Replicas: 3,
		Config:   DeploymentConfig{Image: "myimage:latest", Port: 8080},
		Resources: ResourceConfig{
			CPURequest:    "100m",
			CPULimit:      "500m",
			MemoryRequest: "128Mi",
			MemoryLimit:   "512Mi",
		},
	})
	require.NoError(t, err)

	data, err := deployer.ExportManifest(dep.ID)
	require.NoError(t, err)

	var manifest map[string]interface{}
	err = json.Unmarshal(data, &manifest)
	require.NoError(t, err)
	assert.Equal(t, "apps/v1", manifest["apiVersion"])
	assert.Equal(t, "Deployment", manifest["kind"])
}

func TestGetDeploymentsByAgent(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)
	ctx := context.Background()

	// Deploy two for agent-A, one for agent-B.
	for i := 0; i < 2; i++ {
		_, err := deployer.Deploy(ctx, DeployOptions{
			Name:    fmt.Sprintf("dep-a-%d", i),
			AgentID: "agent-A",
			Target:  TargetLocal,
			Config:  DeploymentConfig{Image: "/bin/echo"},
		})
		require.NoError(t, err)
	}
	_, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "dep-b-0",
		AgentID: "agent-B",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	depsA := deployer.GetDeploymentsByAgent("agent-A")
	assert.Len(t, depsA, 2)

	depsB := deployer.GetDeploymentsByAgent("agent-B")
	assert.Len(t, depsB, 1)

	depsC := deployer.GetDeploymentsByAgent("agent-C")
	assert.Empty(t, depsC)
}

func TestEventCallbacks(t *testing.T) {
	var deployedID, deletedID, scaledID string
	var scaleFrom, scaleTo int

	provider := &mockProvider{}
	deployer := newTestDeployer(provider)
	deployer.SetCallbacks(DeploymentEventCallbacks{
		OnDeploy: func(d *Deployment) { deployedID = d.ID },
		OnDelete: func(id string) { deletedID = id },
		OnScale: func(id string, from, to int) {
			scaledID = id
			scaleFrom = from
			scaleTo = to
		},
	})

	ctx := context.Background()
	dep, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "cb-agent",
		AgentID: "agent-cb",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)
	assert.Equal(t, dep.ID, deployedID)

	err = deployer.Scale(ctx, dep.ID, 3)
	require.NoError(t, err)
	assert.Equal(t, dep.ID, scaledID)
	assert.Equal(t, 1, scaleFrom)
	assert.Equal(t, 3, scaleTo)

	err = deployer.Delete(ctx, dep.ID)
	require.NoError(t, err)
	assert.Equal(t, dep.ID, deletedID)
}

func TestListDeployments(t *testing.T) {
	provider := &mockProvider{}
	deployer := newTestDeployer(provider)
	ctx := context.Background()

	assert.Empty(t, deployer.ListDeployments())

	_, err := deployer.Deploy(ctx, DeployOptions{
		Name:    "list-1",
		AgentID: "a1",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	_, err = deployer.Deploy(ctx, DeployOptions{
		Name:    "list-2",
		AgentID: "a2",
		Target:  TargetLocal,
		Config:  DeploymentConfig{Image: "/bin/echo"},
	})
	require.NoError(t, err)

	assert.Len(t, deployer.ListDeployments(), 2)
}

func TestGenerateDeploymentIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateDeploymentID()
		assert.False(t, seen[id], "duplicate ID generated: %s", id)
		seen[id] = true
		assert.True(t, strings.HasPrefix(id, "dep_"))
	}
}
