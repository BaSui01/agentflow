package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func (s *LSPServer) handleInitialize(ctx context.Context, params json.RawMessage) (any, error) {
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()

	return map[string]any{
		"capabilities": s.capabilities,
		"serverInfo": map[string]string{
			"name":    s.info.Name,
			"version": s.info.Version,
		},
	}, nil
}

// handleInitialized 处理 initialized 通知
func (s *LSPServer) handleInitialized(ctx context.Context, params json.RawMessage) (any, error) {
	s.logger.Info("LSP server initialized")
	return nil, nil
}

// handleShutdown 处理 shutdown 请求
func (s *LSPServer) handleShutdown(ctx context.Context, params json.RawMessage) (any, error) {
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()

	s.logger.Info("LSP server shutting down")
	return nil, nil
}

// handleExit 处理 exit 通知
func (s *LSPServer) handleExit(ctx context.Context, params json.RawMessage) (any, error) {
	s.logger.Info("LSP server exiting")
	return nil, nil
}

func (s *LSPServer) handleTextDocumentDidOpen(ctx context.Context, params json.RawMessage) (any, error) {
	var req DidOpenTextDocumentParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid didOpen params: %w", err)
	}

	uri := strings.TrimSpace(req.TextDocument.URI)
	if uri == "" {
		return nil, fmt.Errorf("textDocument.uri is required")
	}

	s.docMu.Lock()
	s.documents[uri] = &managedDocument{
		URI:        uri,
		LanguageID: req.TextDocument.LanguageID,
		Version:    req.TextDocument.Version,
		Text:       req.TextDocument.Text,
	}
	s.docMu.Unlock()

	return nil, nil
}

func (s *LSPServer) handleTextDocumentDidChange(ctx context.Context, params json.RawMessage) (any, error) {
	var req DidChangeTextDocumentParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid didChange params: %w", err)
	}

	uri := strings.TrimSpace(req.TextDocument.URI)
	if uri == "" {
		return nil, fmt.Errorf("textDocument.uri is required")
	}

	s.docMu.Lock()
	defer s.docMu.Unlock()

	doc, ok := s.documents[uri]
	if !ok {
		doc = &managedDocument{URI: uri}
		s.documents[uri] = doc
	}

	updated := doc.Text
	for _, change := range req.ContentChanges {
		next, err := applyTextChange(updated, change)
		if err != nil {
			return nil, err
		}
		updated = next
	}

	doc.Text = updated
	doc.Version = req.TextDocument.Version

	return nil, nil
}

func (s *LSPServer) handleTextDocumentDidClose(ctx context.Context, params json.RawMessage) (any, error) {
	var req DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid didClose params: %w", err)
	}

	uri := strings.TrimSpace(req.TextDocument.URI)
	if uri == "" {
		return nil, fmt.Errorf("textDocument.uri is required")
	}

	s.docMu.Lock()
	delete(s.documents, uri)
	s.docMu.Unlock()

	return nil, nil
}

func (s *LSPServer) handleTextDocumentCompletion(ctx context.Context, params json.RawMessage) (any, error) {
	var req CompletionParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid completion params: %w", err)
	}

	doc, ok := s.getDocumentSnapshot(req.TextDocument.URI)
	if !ok {
		return []CompletionItem{}, nil
	}

	prefix, err := identifierPrefixAtPosition(doc.Text, req.Position)
	if err != nil {
		return nil, err
	}
	prefix = strings.ToLower(prefix)

	candidates := append([]string{}, defaultCompletions()...)
	candidates = append(candidates, collectIdentifiers(doc.Text)...)

	seen := make(map[string]struct{}, len(candidates))
	items := make([]CompletionItem, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}

		if prefix != "" && !strings.HasPrefix(strings.ToLower(candidate), prefix) {
			continue
		}

		kind := CompletionVariable
		if isKeyword(candidate) {
			kind = CompletionKeyword
		}

		items = append(items, CompletionItem{Label: candidate, Kind: kind, InsertText: candidate})
	}

	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Label) < strings.ToLower(items[j].Label)
	})

	if len(items) > 50 {
		items = items[:50]
	}

	return items, nil
}

func (s *LSPServer) handleTextDocumentHover(ctx context.Context, params json.RawMessage) (any, error) {
	var req HoverParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid hover params: %w", err)
	}

	doc, ok := s.getDocumentSnapshot(req.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	word, wordRange, found, err := wordAtPosition(doc.Text, req.Position)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	hover := Hover{
		Contents: MarkupContent{
			Kind:  MarkupMarkdown,
			Value: fmt.Sprintf("`%s`\n\nIdentifier from `%s`", word, doc.URI),
		},
		Range: &wordRange,
	}

	return hover, nil
}

func (s *LSPServer) handleTextDocumentDefinition(ctx context.Context, params json.RawMessage) (any, error) {
	var req DefinitionParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid definition params: %w", err)
	}

	doc, ok := s.getDocumentSnapshot(req.TextDocument.URI)
	if !ok {
		return []Location{}, nil
	}

	word, _, found, err := wordAtPosition(doc.Text, req.Position)
	if err != nil {
		return nil, err
	}
	if !found {
		return []Location{}, nil
	}

	defRange, ok := findDefinitionRange(doc.Text, word)
	if !ok {
		return []Location{}, nil
	}

	return []Location{{URI: doc.URI, Range: defRange}}, nil
}

func (s *LSPServer) handleTextDocumentReferences(ctx context.Context, params json.RawMessage) (any, error) {
	var req ReferenceParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid references params: %w", err)
	}

	doc, ok := s.getDocumentSnapshot(req.TextDocument.URI)
	if !ok {
		return []Location{}, nil
	}

	word, _, found, err := wordAtPosition(doc.Text, req.Position)
	if err != nil {
		return nil, err
	}
	if !found {
		return []Location{}, nil
	}

	ranges := findWordRanges(doc.Text, word)
	if len(ranges) == 0 {
		return []Location{}, nil
	}

	definition, hasDefinition := findDefinitionRange(doc.Text, word)
	locations := make([]Location, 0, len(ranges))
	for _, current := range ranges {
		if !req.Context.IncludeDeclaration && hasDefinition && rangesEqual(current, definition) {
			continue
		}
		locations = append(locations, Location{URI: doc.URI, Range: current})
	}

	return locations, nil
}

func (s *LSPServer) handleTextDocumentDocumentSymbol(ctx context.Context, params json.RawMessage) (any, error) {
	var req DocumentSymbolParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid documentSymbol params: %w", err)
	}

	doc, ok := s.getDocumentSnapshot(req.TextDocument.URI)
	if !ok {
		return []DocumentSymbol{}, nil
	}

	return parseDocumentSymbols(doc.Text), nil
}

func (s *LSPServer) handleTextDocumentCodeAction(ctx context.Context, params json.RawMessage) (any, error) {
	var req CodeActionParams
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid codeAction params: %w", err)
	}

	actions := make([]CodeAction, 0, len(req.Context.Diagnostics)+1)
	for idx, diagnostic := range req.Context.Diagnostics {
		title := strings.TrimSpace(diagnostic.Message)
		if title == "" {
			title = "Resolve diagnostic"
		}

		actions = append(actions, CodeAction{
			Title:       "Quick fix: " + title,
			Kind:        CodeActionQuickFix,
			Diagnostics: []Diagnostic{diagnostic},
			IsPreferred: idx == 0,
		})
	}

	if len(actions) == 0 {
		actions = append(actions, CodeAction{
			Title: "Organize imports",
			Kind:  CodeActionSourceOrganizeImports,
		})
	}

	return actions, nil
}

