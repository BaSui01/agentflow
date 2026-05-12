package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapabilityIndexAddRemoveAndAgents(t *testing.T) {
	idx := NewCapabilityIndex[string]()
	idx.Add("search", "agent-1", "cap-1")
	idx.Add("search", "agent-2", "cap-2")

	agents := idx.AgentIDs("search")
	assert.ElementsMatch(t, []string{"agent-1", "agent-2"}, agents)

	caps := idx.Capabilities("search")
	assert.Len(t, caps, 2)
	assert.Equal(t, "cap-1", caps["agent-1"])

	idx.Remove("search", "agent-1")
	assert.ElementsMatch(t, []string{"agent-2"}, idx.AgentIDs("search"))

	idx.Remove("search", "agent-2")
	assert.Empty(t, idx.AgentIDs("search"))
}
