package main

import (
	"os"
	"strings"
	"testing"
)

func TestInitRAGHandlerUsesFactoryEntry(t *testing.T) {
	data, err := os.ReadFile("server_handlers_runtime.go")
	if err != nil {
		t.Fatalf("failed to read server_handlers_runtime.go: %v", err)
	}

	src := string(data)
	if !strings.Contains(src, "rag.NewEmbeddingProviderFromConfig(") {
		t.Fatal("initRAGHandler must use rag.NewEmbeddingProviderFromConfig")
	}
	if strings.Contains(src, "embedding.NewOpenAIProvider(") {
		t.Fatal("initRAGHandler must not directly call embedding.NewOpenAIProvider")
	}
}
