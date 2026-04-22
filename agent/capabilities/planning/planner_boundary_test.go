package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlannerPackage_RemainsInfrastructureOnly(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read planner package: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		source := string(content)
		if strings.Contains(source, "\"github.com/BaSui01/agentflow/agent\"") {
			t.Fatalf("%s should not import root agent package", name)
		}
		if strings.Contains(source, "LoopExecutor") {
			t.Fatalf("%s should not depend on LoopExecutor", name)
		}
		if strings.Contains(source, "BaseAgent.Execute") {
			t.Fatalf("%s should not reference BaseAgent.Execute", name)
		}
		if strings.Contains(source, "ReasoningRegistry") {
			t.Fatalf("%s should not manage runtime reasoning registry wiring", name)
		}
	}
}

func TestPlannerTools_ExposePlanInfrastructureOnly(t *testing.T) {
	schemas := GetPlannerToolSchemas()
	if len(schemas) != 3 {
		t.Fatalf("expected exactly 3 planner tool schemas, got %d", len(schemas))
	}

	got := []string{schemas[0].Name, schemas[1].Name, schemas[2].Name}
	want := []string{ToolCreatePlan, ToolUpdatePlan, ToolGetPlanStatus}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("planner tools changed from plan infrastructure set: got %v want %v", got, want)
		}
	}
}
