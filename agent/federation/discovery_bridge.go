package federation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DiscoveryRegistry is a local interface matching the subset of
// discovery.CapabilityRegistry needed for federation sync.
// Using local interface pattern (§15).
type DiscoveryRegistry interface {
	RegisterAgent(ctx context.Context, info *AgentRegistration) error
	UnregisterAgent(ctx context.Context, agentID string) error
	UpdateAgentStatus(ctx context.Context, agentID string, status string) error
}

// AgentRegistration is the local representation for registering an agent.
type AgentRegistration struct {
	ID           string
	Name         string
	Endpoint     string
	Capabilities []string
	Organization string
	Metadata     map[string]string
}

// DiscoveryBridge syncs federated nodes with the discovery registry.
type DiscoveryBridge struct {
	orchestrator *Orchestrator
	registry     DiscoveryRegistry
	config       BridgeConfig
	logger       *zap.Logger
	stopCh       chan struct{}
	closeOnce    sync.Once
}

// BridgeConfig holds configuration for the DiscoveryBridge.
type BridgeConfig struct {
	SyncInterval time.Duration // how often to sync all nodes
	AutoSync     bool          // auto-sync on node registration
}

// DefaultBridgeConfig returns a BridgeConfig with sensible defaults.
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		SyncInterval: 60 * time.Second,
		AutoSync:     true,
	}
}

// NewDiscoveryBridge creates a new DiscoveryBridge.
func NewDiscoveryBridge(
	orchestrator *Orchestrator,
	registry DiscoveryRegistry,
	config BridgeConfig,
	logger *zap.Logger,
) *DiscoveryBridge {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.SyncInterval == 0 {
		config.SyncInterval = 60 * time.Second
	}
	return &DiscoveryBridge{
		orchestrator: orchestrator,
		registry:     registry,
		config:       config,
		logger:       logger.With(zap.String("component", "discovery_bridge")),
		stopCh:       make(chan struct{}),
	}
}

// SyncNode registers a single federated node's capabilities in discovery.
func (b *DiscoveryBridge) SyncNode(ctx context.Context, node *FederatedNode) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}

	reg := &AgentRegistration{
		ID:           node.ID,
		Name:         node.Name,
		Endpoint:     node.Endpoint,
		Capabilities: node.Capabilities,
		Metadata:     node.Metadata,
	}

	if err := b.registry.RegisterAgent(ctx, reg); err != nil {
		return fmt.Errorf("failed to sync node %s to discovery: %w", node.ID, err)
	}

	b.logger.Info("synced node to discovery",
		zap.String("node_id", node.ID),
		zap.String("node_name", node.Name),
	)
	return nil
}

// SyncAllNodes syncs all known federated nodes to discovery.
func (b *DiscoveryBridge) SyncAllNodes(ctx context.Context) error {
	nodes := b.orchestrator.ListNodes()
	var firstErr error
	for _, node := range nodes {
		if err := b.SyncNode(ctx, node); err != nil {
			b.logger.Warn("failed to sync node",
				zap.String("node_id", node.ID),
				zap.Error(err),
			)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Start begins periodic sync and auto-sync on node changes.
func (b *DiscoveryBridge) Start(ctx context.Context) error {
	if b.config.AutoSync {
		b.orchestrator.SetOnNodeRegister(func(node *FederatedNode) {
			syncCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := b.SyncNode(syncCtx, node); err != nil {
				b.logger.Warn("auto-sync on register failed",
					zap.String("node_id", node.ID),
					zap.Error(err),
				)
			}
		})

		b.orchestrator.SetOnNodeUnregister(func(nodeID string) {
			syncCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := b.registry.UnregisterAgent(syncCtx, nodeID); err != nil {
				b.logger.Warn("auto-sync on unregister failed",
					zap.String("node_id", nodeID),
					zap.Error(err),
				)
			}
		})

		b.orchestrator.SetOnNodeStatusChange(func(nodeID string, status NodeStatus) {
			syncCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := b.registry.UpdateAgentStatus(syncCtx, nodeID, string(status)); err != nil {
				b.logger.Warn("auto-sync on status change failed",
					zap.String("node_id", nodeID),
					zap.Error(err),
				)
			}
		})
	}

	// Initial full sync.
	if err := b.SyncAllNodes(ctx); err != nil {
		b.logger.Warn("initial sync had errors", zap.Error(err))
	}

	go b.periodicSync(ctx)

	b.logger.Info("discovery bridge started",
		zap.Duration("sync_interval", b.config.SyncInterval),
		zap.Bool("auto_sync", b.config.AutoSync),
	)
	return nil
}

// Stop stops the bridge. It is safe to call multiple times.
func (b *DiscoveryBridge) Stop() {
	b.closeOnce.Do(func() { close(b.stopCh) })
	b.logger.Info("discovery bridge stopped")
}

func (b *DiscoveryBridge) periodicSync(ctx context.Context) {
	ticker := time.NewTicker(b.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		case <-ticker.C:
			syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := b.SyncAllNodes(syncCtx); err != nil {
				b.logger.Warn("periodic sync had errors", zap.Error(err))
			}
			cancel()
		}
	}
}
