package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"regexp"
	"sync"

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
type RequestHandler func(ctx context.Context, params json.RawMessage) (any, error)

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
