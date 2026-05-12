package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBestCandidateByScoreThenLoad(t *testing.T) {
	candidates := []ScoredCandidate{
		{ID: "a1", Score: 80, Load: 0.1},
		{ID: "a2", Score: 90, Load: 0.8},
		{ID: "a3", Score: 90, Load: 0.2},
	}

	best, ok := BestCandidate(candidates)

	assert.True(t, ok)
	assert.Equal(t, ScoredCandidate{ID: "a3", Score: 90, Load: 0.2}, best)
	assert.Equal(t, []ScoredCandidate{{ID: "a1", Score: 80, Load: 0.1}, {ID: "a2", Score: 90, Load: 0.8}, {ID: "a3", Score: 90, Load: 0.2}}, candidates)
}

func TestBestCandidateEmpty(t *testing.T) {
	_, ok := BestCandidate(nil)

	assert.False(t, ok)
}

func TestCountAssignmentsForOwner(t *testing.T) {
	assignments := map[string]string{"search": "agent-1", "report": "agent-1", "notify": "agent-2"}

	assert.Equal(t, 2, CountAssignmentsForOwner(assignments, "agent-1"))
	assert.Equal(t, 0, CountAssignmentsForOwner(assignments, "agent-3"))
}
