package core

import (
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

// RAGError 统一 RAG 错误类型，映射到 types.ErrorCode。
type RAGError struct {
	Code    types.ErrorCode
	Message string
	Cause   error
}

func (e *RAGError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *RAGError) Unwrap() error {
	return e.Cause
}

// NewRAGError 创建 RAG 错误。
func NewRAGError(code types.ErrorCode, message string, cause error) *RAGError {
	return &RAGError{Code: code, Message: message, Cause: cause}
}

// ErrUpstream 上游服务错误（embedding/rerank provider 不可用）。
func ErrUpstream(message string, cause error) *RAGError {
	return NewRAGError(types.ErrUpstreamError, message, cause)
}

// ErrTimeout 超时错误。
func ErrTimeout(message string, cause error) *RAGError {
	return NewRAGError(types.ErrTimeout, message, cause)
}

// ErrInternal 内部错误。
func ErrInternal(message string, cause error) *RAGError {
	return NewRAGError(types.ErrInternalError, message, cause)
}

// ErrConfig 配置错误。
func ErrConfig(message string, cause error) *RAGError {
	return NewRAGError(types.ErrInvalidRequest, message, cause)
}
