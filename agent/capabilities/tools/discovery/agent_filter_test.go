package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchesAgentFilterLocalRemoteStatusCapabilitiesAndTags(t *testing.T) {
	local := true
	remote := true
	agent := FilterAgent{
		IsLocal: true,
		Status:  "online",
		Capabilities: []FilterCapability{
			{Name: "search", Tags: []string{"fast", "reliable"}},
			{Name: "summarize", Tags: []string{"text"}},
		},
	}

	assert.True(t, MatchesAgentFilter(agent, AgentFilter{Local: &local, Status: []string{"online"}, Capabilities: []string{"search"}, Tags: []string{"fast"}}))
	assert.False(t, MatchesAgentFilter(agent, AgentFilter{Remote: &remote}))
	assert.False(t, MatchesAgentFilter(agent, AgentFilter{Status: []string{"offline"}}))
	assert.False(t, MatchesAgentFilter(agent, AgentFilter{Capabilities: []string{"code"}}))
	assert.False(t, MatchesAgentFilter(agent, AgentFilter{Tags: []string{"slow"}}))
}

func TestMatchesAgentFilterFalseLocalRemoteFlagsDoNotRequireOpposite(t *testing.T) {
	falseValue := false
	agent := FilterAgent{IsLocal: true, Status: "online"}

	assert.True(t, MatchesAgentFilter(agent, AgentFilter{Local: &falseValue}))
	assert.True(t, MatchesAgentFilter(agent, AgentFilter{Remote: &falseValue}))
}
