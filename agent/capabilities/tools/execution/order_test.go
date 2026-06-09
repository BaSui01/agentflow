package execution

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateExecutionOrderDependenciesFirst(t *testing.T) {
	order := CalculateExecutionOrder([]string{"summarize", "notify", "fetch"}, map[string][]string{
		"summarize": {"analyze"},
		"analyze":   {"fetch"},
	})

	assertBefore(t, order, "fetch", "analyze")
	assertBefore(t, order, "analyze", "summarize")
	assert.Contains(t, order, "notify")
}

func TestCalculateExecutionOrderIncludesDependencyOnlyNodes(t *testing.T) {
	order := CalculateExecutionOrder([]string{"report"}, map[string][]string{
		"report": {"query", "summarize"},
	})

	assert.ElementsMatch(t, []string{"query", "summarize", "report"}, order)
	assertBefore(t, order, "query", "report")
	assertBefore(t, order, "summarize", "report")
}

func TestHasCircularDependencyDetectsCyclesWithoutLeakingVisitedBranches(t *testing.T) {
	graph := map[string][]string{
		"a": {"b", "c"},
		"b": {"d"},
		"c": {"d"},
	}
	assert.False(t, HasCircularDependency(graph, "a"))

	graph["d"] = []string{"a"}
	assert.True(t, HasCircularDependency(graph, "a"))
}

func assertBefore(t *testing.T, order []string, before, after string) {
	t.Helper()
	beforeIndex := indexOf(order, before)
	afterIndex := indexOf(order, after)
	if beforeIndex < 0 || afterIndex < 0 {
		t.Fatalf("expected %q and %q in order %v", before, after, order)
	}
	if beforeIndex >= afterIndex {
		t.Fatalf("expected %q before %q in order %v", before, after, order)
	}
}

func indexOf(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}
