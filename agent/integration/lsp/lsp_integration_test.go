package lsp

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLSPServerClient_DocumentFlow(t *testing.T) {
	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	logger := zap.NewNop()
	server := NewLSPServer(ServerInfo{Name: "test-lsp", Version: "1.0.0"}, clientToServerReader, serverToClientWriter, logger)
	client := NewLSPClient(serverToClientReader, clientToServerWriter, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = server.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		_ = client.Start(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		_ = clientToServerWriter.Close()
		_ = serverToClientWriter.Close()
		wg.Wait()
	})

	initCtx, initCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer initCancel()

	initResult, err := client.Initialize(initCtx, InitializeParams{
		ProcessID:    1,
		RootURI:      "file:///workspace",
		Capabilities: ClientCapabilities{},
	})
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	if initResult.ServerInfo.Name != "test-lsp" {
		t.Fatalf("unexpected server name: %s", initResult.ServerInfo.Name)
	}
	if initResult.Capabilities.CompletionProvider == nil {
		t.Fatalf("expected completion capability")
	}

	const uri = "file:///main.go"
	source := "package main\n\nvar value = 1\n\nfunc main() {\n\tfmt.Println(value)\n}\n"

	if err := client.TextDocumentDidOpen(DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "go",
			Version:    1,
			Text:       source,
		},
	}); err != nil {
		t.Fatalf("didOpen failed: %v", err)
	}

	requestCtx, requestCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer requestCancel()

	completions, err := client.TextDocumentCompletion(requestCtx, CompletionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 5, Character: 5},
	})
	if err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	if !hasCompletion(completions, "Println") {
		t.Fatalf("expected completion to include Println, got %#v", completions)
	}

	hover, err := client.TextDocumentHover(requestCtx, HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 5, Character: 7},
	})
	if err != nil {
		t.Fatalf("hover failed: %v", err)
	}
	if hover == nil || !strings.Contains(hover.Contents.Value, "Println") {
		t.Fatalf("unexpected hover result: %#v", hover)
	}

	defs, err := client.TextDocumentDefinition(requestCtx, DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 5, Character: 14},
	})
	if err != nil {
		t.Fatalf("definition failed: %v", err)
	}
	if len(defs) == 0 || defs[0].Range.Start.Line != 2 {
		t.Fatalf("unexpected definition result: %#v", defs)
	}

	refs, err := client.TextDocumentReferences(requestCtx, ReferenceParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 5, Character: 14},
		Context:      ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatalf("references failed: %v", err)
	}
	if len(refs) < 2 {
		t.Fatalf("expected at least 2 references, got %d", len(refs))
	}

	symbols, err := client.TextDocumentDocumentSymbol(requestCtx, DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("documentSymbol failed: %v", err)
	}
	if !hasDocumentSymbol(symbols, "main") {
		t.Fatalf("expected symbols to include main, got %#v", symbols)
	}

	actions, err := client.TextDocumentCodeAction(requestCtx, CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Range:        Range{Start: Position{Line: 5, Character: 1}, End: Position{Line: 5, Character: 10}},
		Context: CodeActionContext{Diagnostics: []Diagnostic{
			{Message: "unused variable", Severity: SeverityWarning},
		}},
	})
	if err != nil {
		t.Fatalf("codeAction failed: %v", err)
	}
	if len(actions) == 0 {
		t.Fatalf("expected at least one code action")
	}

	updated := strings.ReplaceAll(source, "value", "total")
	if err := client.TextDocumentDidChange(DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: updated},
		},
	}); err != nil {
		t.Fatalf("didChange failed: %v", err)
	}

	updatedDefs, err := client.TextDocumentDefinition(requestCtx, DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 5, Character: 14},
	})
	if err != nil {
		t.Fatalf("definition after change failed: %v", err)
	}
	if len(updatedDefs) == 0 || updatedDefs[0].Range.Start.Line != 2 {
		t.Fatalf("unexpected updated definition result: %#v", updatedDefs)
	}

	if err := client.TextDocumentDidClose(DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}); err != nil {
		t.Fatalf("didClose failed: %v", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := client.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func hasCompletion(items []CompletionItem, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func hasDocumentSymbol(items []DocumentSymbol, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
