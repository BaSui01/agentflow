package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"go.uber.org/zap"
)

var (
	funcDeclPattern = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)
	typeDeclPattern = regexp.MustCompile(`^type\s+([A-Za-z_][A-Za-z0-9_]*)`)
	varDeclPattern  = regexp.MustCompile(`^(?:var|const)\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

// LSPServer LSP 服务器实现
type LSPServer struct {
	info         ServerInfo
	capabilities ServerCapabilities

	// 处理器
	handlers map[string]RequestHandler

	// 状态
	initialized bool
	shutdown    bool
	mu          sync.RWMutex

	// 通信
	reader    io.Reader
	bufReader *bufio.Reader
	writer    io.Writer
	writeMu   sync.Mutex

	// 文档缓存
	documents map[string]*managedDocument
	docMu     sync.RWMutex

	logger *zap.Logger
}

type managedDocument struct {
	URI        string
	LanguageID string
	Version    int
	Text       string
}

// ServerInfo 服务器信息
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities 服务器能力
type ServerCapabilities struct {
	TextDocumentSync           int                   `json:"textDocumentSync,omitempty"`
	CompletionProvider         *CompletionOptions    `json:"completionProvider,omitempty"`
	HoverProvider              bool                  `json:"hoverProvider,omitempty"`
	SignatureHelpProvider      *SignatureHelpOptions `json:"signatureHelpProvider,omitempty"`
	DefinitionProvider         bool                  `json:"definitionProvider,omitempty"`
	ReferencesProvider         bool                  `json:"referencesProvider,omitempty"`
	DocumentHighlightProvider  bool                  `json:"documentHighlightProvider,omitempty"`
	DocumentSymbolProvider     bool                  `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider    bool                  `json:"workspaceSymbolProvider,omitempty"`
	CodeActionProvider         *CodeActionOptions    `json:"codeActionProvider,omitempty"`
	DocumentFormattingProvider bool                  `json:"documentFormattingProvider,omitempty"`
	RenameProvider             bool                  `json:"renameProvider,omitempty"`
}

// CompletionOptions 补全选项
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	ResolveProvider   bool     `json:"resolveProvider,omitempty"`
}

// SignatureHelpOptions 签名帮助选项
type SignatureHelpOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// CodeActionOptions 代码操作选项
type CodeActionOptions struct {
	CodeActionKinds []CodeActionKind `json:"codeActionKinds,omitempty"`
}

// RequestHandler 请求处理器
type RequestHandler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// NewLSPServer 创建 LSP 服务器
func NewLSPServer(info ServerInfo, reader io.Reader, writer io.Writer, logger *zap.Logger) *LSPServer {
	if logger == nil {
		logger = zap.NewNop()
	}

	server := &LSPServer{
		info:         info,
		handlers:     make(map[string]RequestHandler),
		reader:       reader,
		bufReader:    bufio.NewReader(reader),
		writer:       writer,
		documents:    make(map[string]*managedDocument),
		capabilities: defaultServerCapabilities(),
		logger:       logger,
	}

	// 注册默认处理器
	server.registerDefaultHandlers()

	return server
}

func defaultServerCapabilities() ServerCapabilities {
	return ServerCapabilities{
		TextDocumentSync: TextDocumentSyncIncremental,
		CompletionProvider: &CompletionOptions{
			TriggerCharacters: []string{".", "_"},
		},
		HoverProvider:          true,
		DefinitionProvider:     true,
		ReferencesProvider:     true,
		DocumentSymbolProvider: true,
		CodeActionProvider: &CodeActionOptions{
			CodeActionKinds: []CodeActionKind{CodeActionQuickFix, CodeActionSourceOrganizeImports},
		},
	}
}

// registerDefaultHandlers 注册默认处理器
func (s *LSPServer) registerDefaultHandlers() {
	s.RegisterHandler("initialize", s.handleInitialize)
	s.RegisterHandler("initialized", s.handleInitialized)
	s.RegisterHandler("shutdown", s.handleShutdown)
	s.RegisterHandler("exit", s.handleExit)

	s.RegisterHandler("textDocument/didOpen", s.handleTextDocumentDidOpen)
	s.RegisterHandler("textDocument/didChange", s.handleTextDocumentDidChange)
	s.RegisterHandler("textDocument/didClose", s.handleTextDocumentDidClose)

	s.RegisterHandler("textDocument/completion", s.handleTextDocumentCompletion)
	s.RegisterHandler("textDocument/hover", s.handleTextDocumentHover)
	s.RegisterHandler("textDocument/definition", s.handleTextDocumentDefinition)
	s.RegisterHandler("textDocument/references", s.handleTextDocumentReferences)
	s.RegisterHandler("textDocument/documentSymbol", s.handleTextDocumentDocumentSymbol)
	s.RegisterHandler("textDocument/codeAction", s.handleTextDocumentCodeAction)
}

// RegisterHandler 注册请求处理器
func (s *LSPServer) RegisterHandler(method string, handler RequestHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = handler
}

// SetCapabilities 设置服务器能力
func (s *LSPServer) SetCapabilities(caps ServerCapabilities) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.capabilities = caps
}

// Start 启动服务器
func (s *LSPServer) Start(ctx context.Context) error {
	s.logger.Info("LSP server starting", zap.String("name", s.info.Name))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 读取消息
			msg, err := s.readMessage()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				s.logger.Error("failed to read message", zap.Error(err))
				continue
			}

			// 处理消息
			s.handleMessage(ctx, msg)
		}
	}
}

// readMessage 读取 LSP 消息
func (s *LSPServer) readMessage() (*LSPMessage, error) {
	contentLength := -1

	for {
		line, err := s.bufReader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if key == "content-length" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", value, err)
			}
			contentLength = parsed
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.bufReader, body); err != nil {
		return nil, err
	}

	var msg LSPMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// writeMessage 写入 LSP 消息
func (s *LSPServer) writeMessage(msg *LSPMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := s.writer.Write([]byte(header)); err != nil {
		return err
	}

	if _, err := s.writer.Write(body); err != nil {
		return err
	}

	return nil
}

// handleMessage 处理消息
func (s *LSPServer) handleMessage(ctx context.Context, msg *LSPMessage) {
	s.mu.RLock()
	handler, ok := s.handlers[msg.Method]
	s.mu.RUnlock()

	if !ok {
		if msg.ID != nil {
			s.sendError(msg.ID, -32601, "Method not found", nil)
		} else {
			s.logger.Debug("unknown notification ignored", zap.String("method", msg.Method))
		}
		return
	}

	result, err := handler(ctx, msg.Params)
	if err != nil {
		if msg.ID != nil {
			s.sendError(msg.ID, -32603, err.Error(), nil)
		}
		return
	}

	if msg.ID != nil {
		s.sendResponse(msg.ID, result)
	}
}

// sendResponse 发送响应
func (s *LSPServer) sendResponse(id interface{}, result interface{}) {
	msg := &LSPMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	if err := s.writeMessage(msg); err != nil {
		s.logger.Error("failed to send response", zap.Error(err))
	}
}

// sendError 发送错误
func (s *LSPServer) sendError(id interface{}, code int, message string, data interface{}) {
	msg := &LSPMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error: &LSPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	if err := s.writeMessage(msg); err != nil {
		s.logger.Error("failed to send error", zap.Error(err))
	}
}

// SendNotification 发送通知
func (s *LSPServer) SendNotification(method string, params interface{}) error {
	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		rawParams = data
	}

	msg := &LSPMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}

	return s.writeMessage(msg)
}

// ====== 默认处理器 ======

// handleInitialize 处理 initialize 请求
func (s *LSPServer) handleInitialize(ctx context.Context, params json.RawMessage) (interface{}, error) {
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()

	return map[string]interface{}{
		"capabilities": s.capabilities,
		"serverInfo": map[string]string{
			"name":    s.info.Name,
			"version": s.info.Version,
		},
	}, nil
}

// handleInitialized 处理 initialized 通知
func (s *LSPServer) handleInitialized(ctx context.Context, params json.RawMessage) (interface{}, error) {
	s.logger.Info("LSP server initialized")
	return nil, nil
}

// handleShutdown 处理 shutdown 请求
func (s *LSPServer) handleShutdown(ctx context.Context, params json.RawMessage) (interface{}, error) {
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()

	s.logger.Info("LSP server shutting down")
	return nil, nil
}

// handleExit 处理 exit 通知
func (s *LSPServer) handleExit(ctx context.Context, params json.RawMessage) (interface{}, error) {
	s.logger.Info("LSP server exiting")
	return nil, nil
}

func (s *LSPServer) handleTextDocumentDidOpen(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentDidChange(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentDidClose(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentCompletion(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentHover(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentDefinition(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentReferences(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentDocumentSymbol(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) handleTextDocumentCodeAction(ctx context.Context, params json.RawMessage) (interface{}, error) {
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

func (s *LSPServer) getDocumentSnapshot(uri string) (managedDocument, bool) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return managedDocument{}, false
	}

	s.docMu.RLock()
	doc, ok := s.documents[uri]
	s.docMu.RUnlock()
	if !ok || doc == nil {
		return managedDocument{}, false
	}

	return *doc, true
}

func applyTextChange(text string, change TextDocumentContentChangeEvent) (string, error) {
	if change.Range == nil {
		return change.Text, nil
	}

	start, err := positionToOffset(text, change.Range.Start)
	if err != nil {
		return "", err
	}
	end, err := positionToOffset(text, change.Range.End)
	if err != nil {
		return "", err
	}

	if start < 0 || end < start || end > len(text) {
		return "", fmt.Errorf("invalid content change range")
	}

	return text[:start] + change.Text + text[end:], nil
}

func positionToOffset(text string, position Position) (int, error) {
	if position.Line < 0 || position.Character < 0 {
		return 0, fmt.Errorf("invalid position %v", position)
	}

	lines := strings.Split(text, "\n")
	if position.Line >= len(lines) {
		return 0, fmt.Errorf("line out of range: %d", position.Line)
	}

	offset := 0
	for i := 0; i < position.Line; i++ {
		offset += len(lines[i]) + 1
	}

	lineRunes := []rune(lines[position.Line])
	if position.Character > len(lineRunes) {
		return 0, fmt.Errorf("character out of range: %d", position.Character)
	}

	offset += len(string(lineRunes[:position.Character]))
	return offset, nil
}

func identifierPrefixAtPosition(text string, position Position) (string, error) {
	line, ok := lineAt(text, position.Line)
	if !ok {
		return "", nil
	}

	runes := []rune(line)
	if position.Character < 0 {
		return "", fmt.Errorf("character out of range: %d", position.Character)
	}
	if position.Character > len(runes) {
		position.Character = len(runes)
	}

	start := position.Character
	for start > 0 && isIdentifierPart(runes[start-1]) {
		start--
	}

	return string(runes[start:position.Character]), nil
}

func wordAtPosition(text string, position Position) (string, Range, bool, error) {
	line, ok := lineAt(text, position.Line)
	if !ok {
		return "", Range{}, false, nil
	}

	runes := []rune(line)
	if position.Character < 0 || position.Character > len(runes) {
		return "", Range{}, false, fmt.Errorf("character out of range: %d", position.Character)
	}

	probe := position.Character
	if probe == len(runes) && probe > 0 {
		probe--
	}
	if probe < len(runes) && !isIdentifierPart(runes[probe]) && probe > 0 && isIdentifierPart(runes[probe-1]) {
		probe--
	}
 
	if probe < 0 || probe >= len(runes) || !isIdentifierPart(runes[probe]) {
		return "", Range{}, false, nil
	}

	start := probe
	for start > 0 && isIdentifierPart(runes[start-1]) {
		start--
	}

	end := probe + 1
	for end < len(runes) && isIdentifierPart(runes[end]) {
		end++
	}

	word := string(runes[start:end])
	wordRange := Range{
		Start: Position{Line: position.Line, Character: start},
		End:   Position{Line: position.Line, Character: end},
	}

	return word, wordRange, true, nil
}

func findDefinitionRange(text, word string) (Range, bool) {
	if word == "" {
		return Range{}, false
	}

	lines := strings.Split(text, "\n")
	for lineNumber, line := range lines {
		trimmed := strings.TrimSpace(line)

		if matchesDeclaration(trimmed, word) {
			start, end, ok := tokenRangeInLine(line, word)
			if !ok {
				continue
			}
			return Range{
				Start: Position{Line: lineNumber, Character: start},
				End:   Position{Line: lineNumber, Character: end},
			}, true
		}
	}

	ranges := findWordRanges(text, word)
	if len(ranges) == 0 {
		return Range{}, false
	}

	return ranges[0], true
}

func matchesDeclaration(trimmedLine string, word string) bool {
	if trimmedLine == "" {
		return false
	}

	if match := funcDeclPattern.FindStringSubmatch(trimmedLine); len(match) == 2 && match[1] == word {
		return true
	}
	if match := typeDeclPattern.FindStringSubmatch(trimmedLine); len(match) == 2 && match[1] == word {
		return true
	}
	if match := varDeclPattern.FindStringSubmatch(trimmedLine); len(match) == 2 && match[1] == word {
		return true
	}

	return false
}

func parseDocumentSymbols(text string) []DocumentSymbol {
	lines := strings.Split(text, "\n")
	symbols := make([]DocumentSymbol, 0)

	for lineNumber, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		name := ""
		kind := SymbolVariable

		switch {
		case funcDeclPattern.MatchString(trimmed):
			name = funcDeclPattern.FindStringSubmatch(trimmed)[1]
			kind = SymbolFunction
		case typeDeclPattern.MatchString(trimmed):
			name = typeDeclPattern.FindStringSubmatch(trimmed)[1]
			kind = SymbolClass
		case varDeclPattern.MatchString(trimmed):
			name = varDeclPattern.FindStringSubmatch(trimmed)[1]
			if strings.HasPrefix(trimmed, "const ") {
				kind = SymbolConstant
			} else {
				kind = SymbolVariable
			}
		default:
			continue
		}

		start, end, found := tokenRangeInLine(line, name)
		if !found {
			start, end = 0, utf8.RuneCountInString(line)
		}

		fullRange := Range{
			Start: Position{Line: lineNumber, Character: 0},
			End:   Position{Line: lineNumber, Character: utf8.RuneCountInString(line)},
		}

		symbols = append(symbols, DocumentSymbol{
			Name:           name,
			Kind:           kind,
			Range:          fullRange,
			SelectionRange: Range{Start: Position{Line: lineNumber, Character: start}, End: Position{Line: lineNumber, Character: end}},
		})
	}

	return symbols
}

func findWordRanges(text, word string) []Range {
	if word == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	ranges := make([]Range, 0)

	for lineNumber, line := range lines {
		runes := []rune(line)
		for idx := 0; idx < len(runes); {
			if !isIdentifierStart(runes[idx]) {
				idx++
				continue
			}

			end := idx + 1
			for end < len(runes) && isIdentifierPart(runes[end]) {
				end++
			}

			if string(runes[idx:end]) == word {
				ranges = append(ranges, Range{
					Start: Position{Line: lineNumber, Character: idx},
					End:   Position{Line: lineNumber, Character: end},
				})
			}

			idx = end
		}
	}

	return ranges
}

func tokenRangeInLine(line, word string) (int, int, bool) {
	runes := []rune(line)
	for idx := 0; idx < len(runes); {
		if !isIdentifierStart(runes[idx]) {
			idx++
			continue
		}

		end := idx + 1
		for end < len(runes) && isIdentifierPart(runes[end]) {
			end++
		}

		if string(runes[idx:end]) == word {
			return idx, end, true
		}

		idx = end
	}

	return 0, 0, false
}

func collectIdentifiers(text string) []string {
	unique := make(map[string]struct{})
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		runes := []rune(line)
		for idx := 0; idx < len(runes); {
			if !isIdentifierStart(runes[idx]) {
				idx++
				continue
			}

			end := idx + 1
			for end < len(runes) && isIdentifierPart(runes[end]) {
				end++
			}

			identifier := string(runes[idx:end])
			unique[identifier] = struct{}{}
			idx = end
		}
	}

	identifiers := make([]string, 0, len(unique))
	for identifier := range unique {
		identifiers = append(identifiers, identifier)
	}

	sort.Strings(identifiers)
	return identifiers
}

func lineAt(text string, lineNumber int) (string, bool) {
	if lineNumber < 0 {
		return "", false
	}

	lines := strings.Split(text, "\n")
	if lineNumber >= len(lines) {
		return "", false
	}

	return lines[lineNumber], true
}

func rangesEqual(a, b Range) bool {
	return a.Start.Line == b.Start.Line &&
		a.Start.Character == b.Start.Character &&
		a.End.Line == b.End.Line &&
		a.End.Character == b.End.Character
}

func isIdentifierStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentifierPart(r rune) bool {
	return isIdentifierStart(r) || unicode.IsDigit(r)
}

func defaultCompletions() []string {
	return []string{
		"append",
		"break",
		"const",
		"continue",
		"ctx",
		"else",
		"err",
		"for",
		"func",
		"if",
		"len",
		"make",
		"new",
		"nil",
		"Print",
		"Printf",
		"Println",
		"range",
		"return",
		"switch",
		"type",
		"var",
	}
}

func isKeyword(word string) bool {
	switch word {
	case "append", "break", "const", "continue", "else", "for", "func", "if", "len", "make", "new", "nil", "range", "return", "switch", "type", "var":
		return true
	default:
		return false
	}
}

// LSPMessage LSP 消息
type LSPMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *LSPError       `json:"error,omitempty"`
}

// LSPError LSP 错误
type LSPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
