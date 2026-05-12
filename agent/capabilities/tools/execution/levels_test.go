package execution

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildExecutionLevelsGroupsBySatisfiedDependencies(t *testing.T) {
	levels := BuildExecutionLevels([]string{"fetch", "analyze", "summarize", "notify"}, map[string][]string{
		"analyze":   {"fetch"},
		"summarize": {"analyze"},
	})

	assert.Equal(t, [][]string{{"fetch", "notify"}, {"analyze"}, {"summarize"}}, levels)
}

func TestBuildExecutionLevelsEmptyOrder(t *testing.T) {
	assert.Nil(t, BuildExecutionLevels(nil, map[string][]string{"x": {"y"}}))
}
