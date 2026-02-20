package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// LSPClient LSP 客户端实现
type LSPClient struct {
	serverInfo   *ServerInfo
	capabilities *ServerCapabilities

	// 通信
	reader    io.Reader
	bufReader *bufio.Reader
	writer    io.Writer
	writeMu   sync.Mutex

	// 请求管理
	nextID    int64
	pending   map[int64]chan *LSPMessage
	pendingMu sync.RWMutex

	// 通知处理器
	notificationHandlers map[string]NotificationHandler
	handlersMu           sync.RWMutex

	// 状态
	initialized bool
	mu          sync.RWMutex

	logger *zap.Logger
}

// NotificationHandler 通知处理器
type NotificationHandler func(method string, params json.RawMessage)

// NewLSPClient 创建 LSP 客户端
func NewLSPClient(reader io.Reader, writer io.Writer, logger *zap.Logger) *LSPClient {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &LSPClient{
		reader:               reader,
		bufReader:            bufio.NewReader(reader),
		writer:               writer,
		pending:              make(map[int64]chan *LSPMessage),
		notificationHandlers: make(map[string]NotificationHandler),
		logger:               logger,
	}
}

// Initialize 初始化客户端
func (c *LSPClient) Initialize(ctx context.Context, params InitializeParams) (*InitializeResult, error) {
	result, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		return nil, fmt.Errorf("failed to parse initialize result: %w", err)
	}

	c.mu.Lock()
	c.initialized = true
	c.serverInfo = &initResult.ServerInfo
	c.capabilities = &initResult.Capabilities
	c.mu.Unlock()

	// 发送 initialized 通知
	if err := c.sendNotification("initialized", map[string]any{}); err != nil {
		return nil, err
	}

	c.logger.Info("LSP client initialized",
		zap.String("server", initResult.ServerInfo.Name))

	return &initResult, nil
}

// Shutdown 关闭客户端
func (c *LSPClient) Shutdown(ctx context.Context) error {
	_, err := c.sendRequest(ctx, "shutdown", nil)
	if err != nil {
		return err
	}

	// 发送 exit 通知
	return c.sendNotification("exit", nil)
}

// TextDocumentCompletion 请求代码补全
func (c *LSPClient) TextDocumentCompletion(ctx context.Context, params CompletionParams) ([]CompletionItem, error) {
	result, err := c.sendRequest(ctx, "textDocument/completion", params)
	if err != nil {
		return nil, err
	}

	var items []CompletionItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, fmt.Errorf("failed to parse completion result: %w", err)
	}

	return items, nil
}

// TextDocumentHover 请求悬停信息
func (c *LSPClient) TextDocumentHover(ctx context.Context, params HoverParams) (*Hover, error) {
	result, err := c.sendRequest(ctx, "textDocument/hover", params)
	if err != nil {
		return nil, err
	}

	var hover Hover
	if err := json.Unmarshal(result, &hover); err != nil {
		return nil, fmt.Errorf("failed to parse hover result: %w", err)
	}

	return &hover, nil
}

// TextDocumentDefinition 请求定义位置
func (c *LSPClient) TextDocumentDefinition(ctx context.Context, params DefinitionParams) ([]Location, error) {
	result, err := c.sendRequest(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	var locations []Location
	if err := json.Unmarshal(result, &locations); err != nil {
		return nil, fmt.Errorf("failed to parse definition result: %w", err)
	}

	return locations, nil
}

// TextDocumentReferences 请求引用位置
func (c *LSPClient) TextDocumentReferences(ctx context.Context, params ReferenceParams) ([]Location, error) {
	result, err := c.sendRequest(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}

	var locations []Location
	if err := json.Unmarshal(result, &locations); err != nil {
		return nil, fmt.Errorf("failed to parse references result: %w", err)
	}

	return locations, nil
}

// TextDocumentDocumentSymbol 请求文档符号
func (c *LSPClient) TextDocumentDocumentSymbol(ctx context.Context, params DocumentSymbolParams) ([]DocumentSymbol, error) {
	result, err := c.sendRequest(ctx, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}

	var symbols []DocumentSymbol
	if err := json.Unmarshal(result, &symbols); err != nil {
		return nil, fmt.Errorf("failed to parse document symbol result: %w", err)
	}

	return symbols, nil
}

// TextDocumentCodeAction 请求代码操作
func (c *LSPClient) TextDocumentCodeAction(ctx context.Context, params CodeActionParams) ([]CodeAction, error) {
	result, err := c.sendRequest(ctx, "textDocument/codeAction", params)
	if err != nil {
		return nil, err
	}

	var actions []CodeAction
	if err := json.Unmarshal(result, &actions); err != nil {
		return nil, fmt.Errorf("failed to parse code action result: %w", err)
	}

	return actions, nil
}

// TextDocumentDidOpen 发送文档打开通知。
func (c *LSPClient) TextDocumentDidOpen(params DidOpenTextDocumentParams) error {
	return c.sendNotification("textDocument/didOpen", params)
}

// TextDocumentDidChange 发送文档变更通知。
func (c *LSPClient) TextDocumentDidChange(params DidChangeTextDocumentParams) error {
	return c.sendNotification("textDocument/didChange", params)
}

// TextDocumentDidClose 发送文档关闭通知。
func (c *LSPClient) TextDocumentDidClose(params DidCloseTextDocumentParams) error {
	return c.sendNotification("textDocument/didClose", params)
}

// RegisterNotificationHandler 注册通知处理器
func (c *LSPClient) RegisterNotificationHandler(method string, handler NotificationHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.notificationHandlers[method] = handler
}

// Start 启动客户端消息循环
func (c *LSPClient) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := c.readMessage()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				c.logger.Error("failed to read message", zap.Error(err))
				continue
			}

			c.handleMessage(msg)
		}
	}
}

// sendRequest 发送请求
func (c *LSPClient) sendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)

	// 创建响应通道
	respChan := make(chan *LSPMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// 发送请求
	msg := &LSPMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
	}

	if params != nil {
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		msg.Params = paramsJSON
	}

	if err := c.writeMessage(msg); err != nil {
		return nil, err
	}

	// 等待响应
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		resultJSON, err := json.Marshal(resp.Result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return resultJSON, nil
	}
}

// sendNotification 发送通知
func (c *LSPClient) sendNotification(method string, params any) error {
	msg := &LSPMessage{
		JSONRPC: "2.0",
		Method:  method,
	}

	if params != nil {
		paramsJSON, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		msg.Params = paramsJSON
	}

	return c.writeMessage(msg)
}

// readMessage 读取消息
func (c *LSPClient) readMessage() (*LSPMessage, error) {
	// 读取 Header
	contentLength := -1
	for {
		line, err := c.bufReader.ReadString('\n')
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

	// 读取消息体
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.bufReader, body); err != nil {
		return nil, err
	}

	// 解析 JSON
	var msg LSPMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// writeMessage 写入消息
func (c *LSPClient) writeMessage(msg *LSPMessage) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := c.writer.Write([]byte(header)); err != nil {
		return err
	}

	if _, err := c.writer.Write(body); err != nil {
		return err
	}

	return nil
}

// handleMessage 处理消息
func (c *LSPClient) handleMessage(msg *LSPMessage) {
	// 响应消息
	if msg.ID != nil {
		if id, ok := parseMessageID(msg.ID); ok {
			c.pendingMu.RLock()
			respChan, exists := c.pending[id]
			c.pendingMu.RUnlock()

			if exists {
				respChan <- msg
			}
		}
		return
	}

	// 通知消息
	if msg.Method != "" {
		c.handlersMu.RLock()
		handler, exists := c.notificationHandlers[msg.Method]
		c.handlersMu.RUnlock()

		if exists {
			go handler(msg.Method, msg.Params)
		}
	}
}

func parseMessageID(id any) (int64, bool) {
	switch v := id.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// ====== 参数类型 ======

// InitializeParams 初始化参数
type InitializeParams struct {
	ProcessID             int                `json:"processId"`
	RootURI               string             `json:"rootUri"`
	Capabilities          ClientCapabilities `json:"capabilities"`
	InitializationOptions any        `json:"initializationOptions,omitempty"`
}

// ClientCapabilities 客户端能力
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument,omitempty"`
}

// TextDocumentClientCapabilities 文本文档客户端能力
type TextDocumentClientCapabilities struct {
	Completion    *CompletionClientCapabilities    `json:"completion,omitempty"`
	Hover         *HoverClientCapabilities         `json:"hover,omitempty"`
	SignatureHelp *SignatureHelpClientCapabilities `json:"signatureHelp,omitempty"`
}

// CompletionClientCapabilities 补全客户端能力
type CompletionClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// HoverClientCapabilities 悬停客户端能力
type HoverClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// SignatureHelpClientCapabilities 签名帮助客户端能力
type SignatureHelpClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// InitializeResult 初始化结果
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   ServerInfo         `json:"serverInfo"`
}

// CompletionParams 补全参数
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// HoverParams 悬停参数
type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// DefinitionParams 定义参数
type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ReferenceParams 引用参数
type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      ReferenceContext       `json:"context"`
}

// DocumentSymbolParams 文档符号参数
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// CodeActionParams 代码操作参数
type CodeActionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      CodeActionContext      `json:"context"`
}

// CodeActionContext 代码操作上下文
type CodeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// DidOpenTextDocumentParams 文档打开参数。
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// TextDocumentItem 文档内容。
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId,omitempty"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// DidChangeTextDocumentParams 文档变更参数。
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent 文档变更条目。
type TextDocumentContentChangeEvent struct {
	Range       *Range `json:"range,omitempty"`
	RangeLength int    `json:"rangeLength,omitempty"`
	Text        string `json:"text"`
}

// DidCloseTextDocumentParams 文档关闭参数。
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// TextDocumentIdentifier 文本文档标识
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}
