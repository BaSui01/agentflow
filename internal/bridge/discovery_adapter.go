package bridge

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/skills"
)

// DiscoveryRegistrarAdapter adapts discovery.Registry to skills.CapabilityRegistrar.
// This adapter lives in internal/bridge to avoid circular dependencies between
// agent/skills and agent/discovery (§15 workflow-local interfaces).
type DiscoveryRegistrarAdapter struct {
	registry discovery.Registry
}

// NewDiscoveryRegistrarAdapter creates a new adapter wrapping a discovery.Registry.
func NewDiscoveryRegistrarAdapter(registry discovery.Registry) *DiscoveryRegistrarAdapter {
	return &DiscoveryRegistrarAdapter{registry: registry}
}

// RegisterCapability converts a skills.CapabilityDescriptor to discovery.CapabilityInfo
// and registers it with the discovery registry.
func (a *DiscoveryRegistrarAdapter) RegisterCapability(ctx context.Context, desc *skills.CapabilityDescriptor) error {
	now := time.Now()

	capInfo := &discovery.CapabilityInfo{
		Capability: a2a.Capability{
			Name:        desc.Name,
			Description: desc.Description,
			Type:        a2a.CapabilityType(desc.Category),
		},
		AgentID:       desc.AgentID,
		AgentName:     desc.AgentName,
		Status:        discovery.CapabilityStatusActive,
		Score:         50.0,
		Tags:          desc.Tags,
		Metadata:      desc.Metadata,
		RegisteredAt:  now,
		LastUpdatedAt: now,
	}

	return a.registry.RegisterCapability(ctx, desc.AgentID, capInfo)
}

// UnregisterCapability removes a capability from the discovery registry.
func (a *DiscoveryRegistrarAdapter) UnregisterCapability(ctx context.Context, agentID string, capabilityName string) error {
	return a.registry.UnregisterCapability(ctx, agentID, capabilityName)
}
