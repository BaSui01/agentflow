package federation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testFederationConfig() FederationConfig {
	return FederationConfig{
		NodeID:            "test-node",
		NodeName:          "Test Node",
		HeartbeatInterval: 50 * time.Millisecond,
		TaskTimeout:       5 * time.Second,
	}
}

// --- RegisterHandler ---

func TestOrchestrator_RegisterHandler(t *testing.T) {
	orch := NewOrchestrator(testFederationConfig(), zap.NewNop())

	called := false
	orch.RegisterHandler("test_type", func(ctx context.Context, task *FederatedTask) (any, error) {
		called = true
		return "result", nil
	})
	_ = called
}

// --- Start/Stop ---

func TestOrchestrator_StartStop(t *testing.T) {
	config := testFederationConfig()
	config.HeartbeatInterval = 50 * time.Millisecond
	orch := NewOrchestrator(config, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, orch.Start(ctx))
	time.Sleep(100 * time.Millisecond)
	cancel()
	orch.Stop()
}

// --- checkNodeHealth ---

func TestOrchestrator_CheckNodeHealth(t *testing.T) {
	config := testFederationConfig()
	orch := NewOrchestrator(config, zap.NewNop())

	// Register a node normally
	orch.RegisterNode(&FederatedNode{
		ID:     "stale-node",
		Status: NodeStatusOnline,
	})

	// Manually set LastSeen to old time (after registration)
	orch.mu.Lock()
	if node, ok := orch.nodes["stale-node"]; ok {
		node.LastSeen = time.Now().Add(-10 * time.Minute)
	}
	orch.mu.Unlock()

	ch := make(chan bool, 1)
	orch.SetOnNodeStatusChange(func(nodeID string, status NodeStatus) {
		if nodeID == "stale-node" && status == NodeStatusOffline {
			ch <- true
		}
	})

	orch.checkNodeHealth()

	select {
	case <-ch:
		// success
	case <-time.After(time.Second):
		t.Fatal("status change callback not called")
	}
}

// --- DiscoveryBridge ---

func TestDiscoveryBridge_NewWithNilRegistry(t *testing.T) {
	orch := NewOrchestrator(testFederationConfig(), zap.NewNop())

	bridge := NewDiscoveryBridge(orch, nil, DefaultBridgeConfig(), zap.NewNop())
	assert.NotNil(t, bridge)
}

// --- DiscoveryRegistryAdapter ---

func TestDiscoveryRegistryAdapter_New(t *testing.T) {
	adapter := NewDiscoveryRegistryAdapter(nil)
	assert.NotNil(t, adapter)
}

func TestDiscoveryRegistryAdapter_RegisterAgent_Nil(t *testing.T) {
	adapter := NewDiscoveryRegistryAdapter(nil)
	err := adapter.RegisterAgent(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}
