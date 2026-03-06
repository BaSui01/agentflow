package llm

import (
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// LLMError LLM 层统一错误类型，包装 types.Error。
type LLMError struct {
	Base      *types.Error   `json:"base,inline"`
	Provider  string         `json:"provider,omitempty"`
	Model     string         `json:"model,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func (e *LLMError) Error() string {
	if e.Base != nil {
		return e.Base.Error()
	}
	return "[LLM] unknown error"
}

func (e *LLMError) Unwrap() error {
	if e.Base != nil {
		return e.Base.Unwrap()
	}
	return nil
}

// NewLLMError 创建 LLM 层错误。
func NewLLMError(code types.ErrorCode, message string) *LLMError {
	return &LLMError{
		Base:      types.NewError(code, message),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// NewLLMErrorWithCause 创建带原因的错误。
func NewLLMErrorWithCause(code types.ErrorCode, message string, cause error) *LLMError {
	return &LLMError{
		Base:      types.NewError(code, message).WithCause(cause),
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// WrapLLMError 将标准错误包装为 LLMError。
func WrapLLMError(err error, code types.ErrorCode, message string) *LLMError {
	if err == nil {
		return nil
	}
	return NewLLMErrorWithCause(code, message, err)
}

// WrapLLMErrorf 将标准错误包装为 LLMError（支持格式化）。
func WrapLLMErrorf(err error, code types.ErrorCode, format string, args ...any) *LLMError {
	if err == nil {
		return nil
	}
	return NewLLMErrorWithCause(code, fmt.Sprintf(format, args...), err)
}

// WithProvider 设置 Provider 信息。
func (e *LLMError) WithProvider(provider string) *LLMError {
	e.Provider = provider
	return e
}

// WithModel 设置 Model 信息。
func (e *LLMError) WithModel(model string) *LLMError {
	e.Model = model
	return e
}

// WithMetadata 添加元数据。
func (e *LLMError) WithMetadata(key string, value any) *LLMError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
	return e
}
