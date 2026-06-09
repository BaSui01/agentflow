package discovery

// AgentFilter describes agent discovery constraints without depending on the root tools package.
type AgentFilter struct {
	Capabilities []string
	Tags         []string
	Status       []string
	Local        *bool
	Remote       *bool
}

// FilterAgent is the minimal agent shape needed for discovery filtering.
type FilterAgent struct {
	IsLocal      bool
	Status       string
	Capabilities []FilterCapability
}

// FilterCapability is the minimal capability shape needed for discovery filtering.
type FilterCapability struct {
	Name string
	Tags []string
}

// MatchesAgentFilter reports whether an agent satisfies a discovery filter.
func MatchesAgentFilter(agent FilterAgent, filter AgentFilter) bool {
	if filter.Local != nil && *filter.Local && !agent.IsLocal {
		return false
	}
	if filter.Remote != nil && *filter.Remote && agent.IsLocal {
		return false
	}
	if len(filter.Status) > 0 && !containsString(filter.Status, agent.Status) {
		return false
	}
	for _, requiredCapability := range filter.Capabilities {
		if !hasCapability(agent.Capabilities, requiredCapability) {
			return false
		}
	}
	for _, requiredTag := range filter.Tags {
		if !hasTag(agent.Capabilities, requiredTag) {
			return false
		}
	}
	return true
}

func hasCapability(capabilities []FilterCapability, name string) bool {
	for _, capability := range capabilities {
		if capability.Name == name {
			return true
		}
	}
	return false
}

func hasTag(capabilities []FilterCapability, tag string) bool {
	for _, capability := range capabilities {
		if containsString(capability.Tags, tag) {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
