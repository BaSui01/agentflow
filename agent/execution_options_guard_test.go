package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoopExecutor_DoesNotReadLegacyControlFallbacks(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(".", "loop_executor.go"))
	if err != nil {
		t.Fatalf("read loop_executor.go: %v", err)
	}
	text := string(content)
	for _, needle := range []string{
		"ResolveRunConfig(",
		"DisablePlannerEnabled(",
		"topLevelLoopBudget(",
	} {
		if strings.Contains(text, needle) {
			t.Fatalf("loop_executor.go must not depend on legacy control fallback %q", needle)
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

	adapterContent, err := os.ReadFile(filepath.Join(".", "chat_request_adapter.go"))
	if err != nil {
		t.Fatalf("read chat_request_adapter.go: %v", err)
	}
	if !strings.Contains(string(adapterContent), "ChatRequest{") {
		t.Fatal("chat_request_adapter.go must remain the primary ChatRequest construction surface")
	}
}

