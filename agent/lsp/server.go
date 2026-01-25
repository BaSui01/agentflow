package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"go.uber.org/zap"
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
	reader io.Reader
	writer io.Writer

	logger *zap.Logger
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
	server := &LSPServer{
		info:     info,
		handlers: make(map[string]RequestHandler),
		reader:   reader,
		writer:   writer,
		logger:   logger,
	}

	// 注册默认处理器
	server.registerDefaultHandlers()

	return server
}

// registerDefaultHandlers 注册默认处理器
func (s *LSPServer) registerDefaultHandlers() {
	s.RegisterHandler("initialize", s.handleInitialize)
	s.RegisterHandler("initialized", s.handleInitialized)
	s.RegisterHandler("shutdown", s.handleShutdown)
	s.RegisterHandler("exit", s.handleExit)
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
			go s.handleMessage(ctx, msg)
		}
	}
}

// readMessage 读取 LSP 消息
func (s *LSPServer) readMessage() (*LSPMessage, error) {
	// 读取 Content-Length 头
	var contentLength int
	for {
		var line string
		_, err := fmt.Fscanln(s.reader, &line)
		if err != nil {
			return nil, err
		}

		if line == "\r\n" || line == "" {
			break
		}

		if _, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength); err == nil {
			continue
		}
	}

	// 读取消息体
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, body); err != nil {
		return nil, err
	}

	// 解析 JSON
	var msg LSPMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// writeMessage 写入 LSP 消息
func (s *LSPServer) writeMessage(msg *LSPMessage) error {
	// 序列化消息
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// 写入头和消息体
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
		s.sendError(msg.ID, -32601, "Method not found", nil)
		return
	}

	// 调用处理器
	result, err := handler(ctx, msg.Params)
	if err != nil {
		s.sendError(msg.ID, -32603, err.Error(), nil)
		return
	}

	// 发送响应
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
