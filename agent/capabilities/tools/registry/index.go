package registry

// CapabilityIndex maps capability names to agent-specific capability records.
type CapabilityIndex[T any] struct {
	items map[string]map[string]T
}

// NewCapabilityIndex creates an empty capability index.
func NewCapabilityIndex[T any]() *CapabilityIndex[T] {
	return &CapabilityIndex[T]{items: make(map[string]map[string]T)}
}

// Add indexes a capability for an agent.
func (i *CapabilityIndex[T]) Add(capabilityName, agentID string, capability T) {
	if i.items[capabilityName] == nil {
		i.items[capabilityName] = make(map[string]T)
	}
	i.items[capabilityName][agentID] = capability
}

// Remove removes an agent capability from the index.
func (i *CapabilityIndex[T]) Remove(capabilityName, agentID string) {
	if agentCaps, exists := i.items[capabilityName]; exists {
		delete(agentCaps, agentID)
		if len(agentCaps) == 0 {
			delete(i.items, capabilityName)
		}
	}
}

// Capabilities returns a shallow copy of capabilities by agent ID.
func (i *CapabilityIndex[T]) Capabilities(capabilityName string) map[string]T {
	agentCaps, exists := i.items[capabilityName]
	if !exists {
		return nil
	}
	out := make(map[string]T, len(agentCaps))
	for agentID, capability := range agentCaps {
		out[agentID] = capability
	}
	return out
}

// AgentIDs returns all agent IDs indexed for a capability.
func (i *CapabilityIndex[T]) AgentIDs(capabilityName string) []string {
	agentCaps := i.Capabilities(capabilityName)
	if len(agentCaps) == 0 {
		return nil
	}
	out := make([]string, 0, len(agentCaps))
	for agentID := range agentCaps {
		out = append(out, agentID)
	}
	return out
}
