package federation

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/execution/protocol/a2a"
)

// DiscoveryRegistryAdapter adapts discovery.DiscoveryService to the
// DiscoveryRegistry interface used by the federation bridge.
type DiscoveryRegistryAdapter struct {
	service *discovery.DiscoveryService
}

// NewDiscoveryRegistryAdapter creates a new adapter wrapping the given
// DiscoveryService.
func NewDiscoveryRegistryAdapter(service *discovery.DiscoveryService) *DiscoveryRegistryAdapter {
	return &DiscoveryRegistryAdapter{service: service}
}

// RegisterAgent converts an AgentRegistration to discovery.AgentInfo and
// registers it via the discovery service.
func (a *DiscoveryRegistryAdapter) RegisterAgent(ctx context.Context, info *AgentRegistration) error {
	if info == nil {
		return fmt.Errorf("agent registration is nil")
	}

	card := a2a.NewAgentCard(info.Name, info.Name, info.Endpoint, "1.0")
	for _, cap := range info.Capabilities {
		card.AddCapability(cap, cap, a2a.CapabilityTypeTask)
	}
	if info.Metadata != nil {
		for k, v := range info.Metadata {
			card.SetMetadata(k, v)
		}
	}
	if info.Organization != "" {
		card.SetMetadata("organization", info.Organization)
	}

	agentInfo := discovery.AgentInfoFromCard(card, false)
	agentInfo.Endpoint = info.Endpoint

	// Use Card.Name as the agent ID (matches discovery registry convention).
	return a.service.RegisterAgent(ctx, agentInfo)
}

// UnregisterAgent removes an agent from the discovery service.
func (a *DiscoveryRegistryAdapter) UnregisterAgent(ctx context.Context, agentID string) error {
	return a.service.UnregisterAgent(ctx, agentID)
}

// UpdateAgentStatus updates an agent's status in the discovery registry.
func (a *DiscoveryRegistryAdapter) UpdateAgentStatus(ctx context.Context, agentID string, status string) error {
	return a.service.Registry().UpdateAgentStatus(ctx, agentID, discovery.AgentStatus(status))
}

// Compile-time check that DiscoveryRegistryAdapter implements DiscoveryRegistry.
var _ DiscoveryRegistry = (*DiscoveryRegistryAdapter)(nil)
