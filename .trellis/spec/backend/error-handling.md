# Error Handling

> AgentFlow 项目的错误处理约定和标准。

---

## Overview

AgentFlow 使用**结构化错误**（`types.Error`）作为统一的错误类型，支持错误码、HTTP 状态码、可重试标记、错误链和上下文追踪。

**核心原则**:
- 使用 `types.Error` 而非标准 `error` 传递领域错误
- 错误码统一使用 `ErrorCode` 类型定义
- 支持错误包装（Unwrap）用于错误链追踪
- API 层通过 `ErrorInfoFromTypesError` 转换为 DTO

---

## Error Types

### types.Error 结构

```go
// types/error.go
type Error struct {
    Code       ErrorCode    `json:"code"`           // 错误码
    Message    string       `json:"message"`        // 错误消息
    HTTPStatus int          `json:"-"`              // HTTP 状态码（内部使用）
    Retryable  bool         `json:"retryable"`      // 是否可重试
    Provider   string       `json:"provider,omitempty"` // 提供商名称
    Cause      error        `json:"-"`              // 底层错误（链式）
    Context    ErrorContext `json:"context,omitempty"`  // 追踪上下文
}

type ErrorContext struct {
    TraceID   string `json:"trace_id,omitempty"`
    AgentID   string `json:"agent_id,omitempty"`
    SessionID string `json:"session_id,omitempty"`
    RunID     string `json:"run_id,omitempty"`
}
```

### 错误码分类

**LLM 错误**:
```go
const (
    ErrInvalidRequest      ErrorCode = "INVALID_REQUEST"
    ErrAuthentication      ErrorCode = "AUTHENTICATION"
    ErrRateLimit           ErrorCode = "RATE_LIMIT"
    ErrModelNotFound       ErrorCode = "MODEL_NOT_FOUND"
    ErrUpstreamTimeout     ErrorCode = "UPSTREAM_TIMEOUT"
    ErrInternalError       ErrorCode = "INTERNAL_ERROR"
    // ...
)
```

**Agent 错误**:
```go
const (
    ErrAgentNotReady      ErrorCode = "AGENT_NOT_READY"
    ErrAgentNotFound      ErrorCode = "AGENT_NOT_FOUND"
    ErrAgentExecution     ErrorCode = "AGENT_EXECUTION"
    ErrGuardrailsViolated ErrorCode = "GUARDRAILS_VIOLATED"
    // ...
)
```

**授权错误**:
```go
const (
    ErrAuthzDenied             ErrorCode = "AUTHZ_DENIED"
    ErrAuthzServiceUnavailable ErrorCode = "AUTHZ_SERVICE_UNAVAILABLE"
    ErrApprovalExpired         ErrorCode = "APPROVAL_EXPIRED"
    // ...
)
```

---

## Error Handling Patterns

### 1. 创建错误

```go
// ✅ 使用构造函数创建带完整信息的错误
err := types.NewRateLimitError("rate limit exceeded").
    WithProvider("openai").
    WithContext(types.ErrorContext{
        TraceID: "trace-123",
    })

// ✅ 使用基础构造函数 + 链式方法
err := types.NewError(types.ErrAgentExecution, "execution failed").
    WithCause(originalErr).
    WithHTTPStatus(http.StatusInternalServerError).
    WithRetryable(false)
```

### 2. 包装标准错误

```go
// ✅ 使用 WrapError 包装标准错误
if err != nil {
    return types.WrapError(err, types.ErrUpstreamError, "provider call failed")
}

// ✅ 使用 WrapErrorf 添加格式化消息
if err != nil {
    return types.WrapErrorf(err, types.ErrToolValidation, "invalid argument: %s", argName)
}
```

### 3. 错误链与 Unwrap

```go
// Error 实现了 Unwrap 接口
func (e *Error) Unwrap() error {
    return e.Cause
}

// ✅ 使用 errors.As 提取特定错误类型
var agentErr *types.Error
if errors.As(err, &agentErr) {
    // 处理 types.Error
    code := agentErr.Code
    retryable := agentErr.Retryable
}
```

### 4. 检查错误码

```go
// ✅ 使用 IsErrorCode 检查错误码
if types.IsErrorCode(err, types.ErrRateLimit) {
    // 执行限流退避逻辑
}

// ✅ 使用 IsRetryable 检查是否可重试
if types.IsRetryable(err) {
    // 执行重试逻辑
}
```

### 5. 添加上下文

```go
// ✅ 在错误传播过程中添加上下文
func (s *Service) Process(ctx context.Context, req *Request) error {
    err := s.doWork(ctx, req)
    if err != nil {
        if typedErr, ok := types.AsError(err); ok {
            typedErr.WithContext(types.ErrorContext{
                TraceID:   telemetry.TraceIDFromContext(ctx),
                AgentID:   req.AgentID,
                SessionID: req.SessionID,
            })
        }
        return err
    }
    return nil
}
```

---

## API Error Responses

### 错误映射

API 层通过 `api/error_mapping.go` 将 `types.Error` 转换为 API DTO:

```go
// api/error_mapping.go
func ErrorInfoFromTypesError(err *types.Error, status int) *ErrorInfo {
    if err == nil {
        return nil
    }
    return &ErrorInfo{
        Code:       string(err.Code),
        Message:    err.Message,
        Retryable:  err.Retryable,
        HTTPStatus: status,
        Provider:   err.Provider,
    }
}

// HTTP 状态码映射
func HTTPStatusFromErrorCode(code types.ErrorCode) int {
    switch code {
    case types.ErrInvalidRequest:
        return http.StatusBadRequest
    case types.ErrAuthentication:
        return http.StatusUnauthorized
    case types.ErrRateLimit:
        return http.StatusTooManyRequests
    case types.ErrInternalError:
        return http.StatusInternalServerError
    // ...
    }
}
```

### HTTP 响应格式

```json
{
    "code": "RATE_LIMIT",
    "message": "rate limit exceeded",
    "retryable": true,
    "provider": "openai"
}
```

---

## Common Mistakes

### ❌ 错误: 直接使用标准错误

```go
// 不要这样做
return errors.New("something went wrong")

// ✅ 正确做法
return types.NewInternalError("something went wrong")
```

### ❌ 错误: 丢失原始错误

```go
// 不要这样做 - 丢失了原始错误信息
if err != nil {
    return types.NewError(types.ErrUpstreamError, "call failed")
}

// ✅ 正确做法 - 保留错误链
if err != nil {
    return types.WrapError(err, types.ErrUpstreamError, "call failed")
}
```

### ❌ 错误: 在错误中包含敏感信息

```go
// 不要这样做 - API Key 可能泄露
return types.NewError(types.ErrAuthentication, fmt.Sprintf("invalid key: %s", apiKey))

// ✅ 正确做法 - 只返回通用消息
return types.NewAuthenticationError("invalid API key")
```

### ❌ 错误: 使用字符串比较错误码

```go
// 不要这样做 - 无法处理包装错误
if err.Error() == "RATE_LIMIT" {
    // ...
}

// ✅ 正确做法 - 使用 IsErrorCode
if types.IsErrorCode(err, types.ErrRateLimit) {
    // ...
}
```

### ❌ 错误: 忽略可重试标记

```go
// 不要这样做 - 对所有错误都重试
if err != nil {
    time.Sleep(time.Second)
    retry()
}

// ✅ 正确做法 - 只重试标记为可重试的错误
if types.IsRetryable(err) {
    time.Sleep(time.Second)
    retry()
}
```

---

## Required Patterns

### 1. 错误构造函数

每个主要错误码都应该有对应的构造函数:

```go
// ✅ 在 types/error.go 中添加
func NewRateLimitError(message string) *Error {
    return NewError(ErrRateLimit, message).
        WithHTTPStatus(http.StatusTooManyRequests).
        WithRetryable(true)
}
```

### 2. 错误日志记录

```go
// ✅ 记录错误时包含结构化字段
if err != nil {
    if typedErr, ok := types.AsError(err); ok {
        logger.Error("operation failed",
            zap.String("error_code", string(typedErr.Code)),
            zap.String("error_message", typedErr.Message),
            zap.Error(typedErr.Cause), // 原始错误
        )
    } else {
        logger.Error("operation failed", zap.Error(err))
    }
}
```

### 3. 跨层错误传播

```go
// ✅ Handler 层: 转换错误为 HTTP 响应
func (h *Handler) Handle(c *gin.Context) {
    result, err := h.service.Process(ctx, req)
    if err != nil {
        if typedErr, ok := types.AsError(err); ok {
            c.JSON(typedErr.HTTPStatus, ErrorInfoFromTypesError(typedErr, typedErr.HTTPStatus))
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        }
        return
    }
    c.JSON(http.StatusOK, result)
}
```

---

## Testing Error Handling

```go
func TestErrorWrapping(t *testing.T) {
    // 创建基础错误
    baseErr := errors.New("connection refused")

    // 包装为 types.Error
    err := types.WrapError(baseErr, types.ErrUpstreamError, "provider call failed")

    // 验证 Unwrap
    assert.Equal(t, baseErr, errors.Unwrap(err))

    // 验证错误码提取
    assert.Equal(t, types.ErrUpstreamError, types.GetErrorCode(err))

    // 验证 IsRetryable
    assert.True(t, types.IsRetryable(err))
}
```
