package tools

import tooldiscovery "github.com/BaSui01/agentflow/agent/capabilities/tools/discovery"

func discoveryFilterAgent(agent *AgentInfo) tooldiscovery.FilterAgent {
	if agent == nil {
		return tooldiscovery.FilterAgent{}
	}
	capabilities := make([]tooldiscovery.FilterCapability, 0, len(agent.Capabilities))
	for i := range agent.Capabilities {
		capability := &agent.Capabilities[i]
		capabilities = append(capabilities, tooldiscovery.FilterCapability{
			Name: capability.Capability.Name,
			Tags: append([]string(nil), capability.Tags...),
		})
	}
	return tooldiscovery.FilterAgent{
		IsLocal:      agent.IsLocal,
		Status:       string(agent.Status),
		Capabilities: capabilities,
	}
}

func discoveryAgentFilter(filter *DiscoveryFilter) tooldiscovery.AgentFilter {
	if filter == nil {
		return tooldiscovery.AgentFilter{}
	}
	statuses := make([]string, 0, len(filter.Status))
	for _, status := range filter.Status {
		statuses = append(statuses, string(status))
	}
	return tooldiscovery.AgentFilter{
		Capabilities: append([]string(nil), filter.Capabilities...),
		Tags:         append([]string(nil), filter.Tags...),
		Status:       statuses,
		Local:        filter.Local,
		Remote:       filter.Remote,
	}
}
