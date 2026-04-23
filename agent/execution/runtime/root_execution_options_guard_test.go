package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoopExecutor_DoesNotReadLegacyControlFallbacks(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(".", "builder.go"))
	if err != nil {
		t.Fatalf("read builder.go: %v", err)
	}
	text := string(content)
	start := strings.Index(text, "// Merged from loop_executor.go.")
	end := strings.Index(text, "// Merged from loop_executor_runtime.go.")
	if start == -1 || end == -1 || end <= start {
		t.Fatal("builder.go must keep explicit merged loop executor section markers")
	}
	text = text[start:end]
	for _, needle := range []string{
		"ResolveRunConfig(",
		"DisablePlannerEnabled(",
		"topLevelLoopBudget(",
	} {
		if strings.Contains(text, needle) {
			t.Fatalf("builder.go must not depend on legacy control fallback %q", needle)
		}
	}
}

func TestChatRequestConstruction_IsCentralizedInAdapter(t *testing.T) {
	requestContent, err := os.ReadFile(filepath.Join(".", "request.go"))
	if err != nil {
		t.Fatalf("read request.go: %v", err)
	}
	if strings.Contains(string(requestContent), "ChatRequest{") {
		t.Fatal("request.go must not construct ChatRequest directly; use ChatRequestAdapter")
	}

	adapterContent, err := os.ReadFile(filepath.Join(".", "adapters", "chat.go"))
	if err != nil {
		t.Fatalf("read adapters/chat.go: %v", err)
	}
	if !strings.Contains(string(adapterContent), "ChatRequest{") {
		t.Fatal("agent/adapters/chat.go must remain the primary ChatRequest construction surface")
	}
}


