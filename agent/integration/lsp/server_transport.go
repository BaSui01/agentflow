package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

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
func (s *LSPServer) sendResponse(id any, result any) {
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
func (s *LSPServer) sendError(id any, code int, message string, data any) {
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
func (s *LSPServer) SendNotification(method string, params any) error {
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
