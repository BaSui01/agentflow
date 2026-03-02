package main

import (
	"os"
	"strings"
	"testing"
)

func loadServerHandlersRuntimeSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("server_handlers_runtime.go")
	if err != nil {
		t.Fatalf("failed to read server_handlers_runtime.go: %v", err)
	}
	return string(data)
}

func TestRAGHandlerUsesBootstrapFactoryEntry(t *testing.T) {
	src := loadServerHandlersRuntimeSource(t)
	if !strings.Contains(src, "bootstrap.BuildRAGHandlerRuntime(") {
		t.Fatal("RAG handler wiring must use bootstrap.BuildRAGHandlerRuntime")
	}
	if strings.Contains(src, "rag.NewEmbeddingProviderFromConfig(") {
		t.Fatal("cmd wiring must not directly call rag.NewEmbeddingProviderFromConfig")
	}
	if strings.Contains(src, "embedding.NewOpenAIProvider(") {
		t.Fatal("cmd wiring must not directly call embedding.NewOpenAIProvider")
	}
}

func TestHandlerRuntimeWiringUsesBootstrapEntries(t *testing.T) {
	src := loadServerHandlersRuntimeSource(t)

	requiredEntries := []string{
		"bootstrap.BuildLLMHandlerRuntime(",
		"bootstrap.BuildMultimodalRuntime(",
		"bootstrap.BuildProtocolRuntime(",
		"bootstrap.BuildWorkflowRuntime(",
		"bootstrap.BuildAgentHandler(",
	}
	for _, entry := range requiredEntries {
		if !strings.Contains(src, entry) {
			t.Fatalf("handler wiring must use bootstrap entry %q", entry)
		}
	}

	forbiddenDirectConstructors := []string{
		"mcp.NewMCPServer(",
		"a2a.NewHTTPServer(",
		"workflow.NewDAGExecutor(",
		"dsl.NewParser(",
		"handlers.NewMultimodalHandler(",
	}
	for _, constructor := range forbiddenDirectConstructors {
		if strings.Contains(src, constructor) {
			t.Fatalf("cmd wiring must not directly call low-level constructor %q", constructor)
		}
	}
}
