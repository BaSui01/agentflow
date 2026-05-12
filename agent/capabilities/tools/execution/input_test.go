package execution

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDependenciesSatisfiedRequiresEveryDependencyCompleted(t *testing.T) {
	deps := map[string][]string{"report": {"fetch", "analyze"}}

	assert.True(t, DependenciesSatisfied("fetch", deps, nil, nil))
	assert.False(t, DependenciesSatisfied("report", deps, map[string]bool{"fetch": true}, nil))
	assert.False(t, DependenciesSatisfied("report", deps, map[string]bool{"fetch": true}, map[string]error{"analyze": errors.New("boom")}))
	assert.True(t, DependenciesSatisfied("report", deps, map[string]bool{"fetch": true, "analyze": true}, nil))
}

func TestBuildCapabilityInputIncludesOnlyAvailableUpstreamDependencyResults(t *testing.T) {
	input := BuildCapabilityInput(
		"report",
		"original",
		map[string][]string{"report": {"fetch", "missing"}},
		func(name string) (any, bool) {
			results := map[string]any{"fetch": "rows", "unrelated": "ignored"}
			v, ok := results[name]
			return v, ok
		},
	)

	assert.Equal(t, "original", input["input"])
	assert.Equal(t, map[string]any{"fetch": "rows"}, input["upstream"])
	assert.NotContains(t, input, "unrelated")
}

func TestBuildCapabilityInputOmitsUpstreamWhenNoDependencyResultExists(t *testing.T) {
	input := BuildCapabilityInput("fetch", map[string]any{"q": "x"}, map[string][]string{"fetch": {"missing"}}, nil)

	assert.Equal(t, map[string]any{"q": "x"}, input["input"])
	assert.NotContains(t, input, "upstream")
}
