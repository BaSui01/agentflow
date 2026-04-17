package bootstrap

import (
	"os"
	"strings"
	"testing"
)

func loadDomainRuntimeBuildersSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("domain_runtime_builders.go")
	if err != nil {
		t.Fatalf("failed to read domain_runtime_builders.go: %v", err)
	}
	return string(data)
}

func TestBuildWorkflowRuntimeUsesWorkflowRuntimeBuilder(t *testing.T) {
	src := loadDomainRuntimeBuildersSource(t)

	requiredSnippets := []string{
		"workflowruntime.NewBuilder(",
		"builder.WithStepDependencies(",
		"rt.Parser.RegisterCondition(",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(src, snippet) {
			t.Fatalf("workflow bootstrap wiring must contain %q", snippet)
		}
	}

	forbiddenSnippets := []string{
		"workflow.NewDAGExecutor(",
		"dsl.NewParser(",
		"workflow.NewFacade(",
	}
	for _, snippet := range forbiddenSnippets {
		if strings.Contains(src, snippet) {
			t.Fatalf("workflow bootstrap wiring must not directly call %q", snippet)
		}
	}
}
