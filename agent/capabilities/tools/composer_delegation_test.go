package tools

import (
	"os"
	"strings"
	"testing"
)

func TestCapabilityComposerExecutionOrderDelegatesToExecutionHelpers(t *testing.T) {
	source, err := os.ReadFile("composer.go")
	if err != nil {
		t.Fatalf("read composer.go: %v", err)
	}
	body := string(source)

	for _, want := range []string{
		"toolexecution.CalculateExecutionOrder",
		"toolexecution.HasCircularDependency",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected composer.go to contain %q", want)
		}
	}
	for _, oldRootLogic := range []string{
		"inDegree :=",
		"delete(visited, capabilityName)",
	} {
		if strings.Contains(body, oldRootLogic) {
			t.Fatalf("expected execution ordering/cycle logic to live in execution subpackage, found %q", oldRootLogic)
		}
	}
}
