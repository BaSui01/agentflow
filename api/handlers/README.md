# API Handlers

这个包提供了 AgentFlow HTTP API 的处理器实现。

## 📁 文件结构

```
api/handlers/
├── common.go   # 通用响应函数和错误处理
├── health.go   # 健康检查处理器
├── chat.go     # 聊天接口处理器
├── agent.go    # Agent 管理处理器
└── README.md   # 本文档
```

## 🎯 设计原则

### 1. 统一错误处理
所有 handler 使用 `types.Error` 进行错误处理，通过 `WriteError()` 函数统一返回错误响应。

```go
err := types.NewError(types.ErrInvalidRequest, "model is required")
WriteError(w, err, logger)
```

### 2. 统一响应格式
所有 API 响应使用统一的 `Response` 结构：

```json
{
  "success": true,
  "data": {...},
  "timestamp": "2026-02-20T10:00:00Z"
}
```

错误响应：

```json
{
  "success": false,
  "error": {
    "code": "INVALID_REQUEST",
    "message": "model is required",
    "retryable": false
  },
  "timestamp": "2026-02-20T10:00:00Z"
}
```

### 3. 类型安全
- 使用 `DecodeJSONBody()` 解码请求，自动验证 JSON 格式
- 使用 `ValidateContentType()` 验证 Content-Type
- 所有请求/响应都有明确的类型定义

## 📖 使用示例

### 健康检查

```go
healthHandler := handlers.NewHealthHandler(logger)

// 注册健康检查
healthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("postgres", db.Ping))
healthHandler.RegisterCheck(handlers.NewRedisHealthCheck("redis", redis.Ping))

// 注册路由
http.HandleFunc("/health", healthHandler.HandleHealth)
http.HandleFunc("/healthz", healthHandler.HandleHealthz)
http.HandleFunc("/ready", healthHandler.HandleReady)
http.HandleFunc("/version", healthHandler.HandleVersion(version, buildTime, gitCommit))
```

### 聊天接口

```go
chatHandler := handlers.NewChatHandler(provider, logger)

// 注册路由
http.HandleFunc("/v1/chat/completions", chatHandler.HandleCompletion)
http.HandleFunc("/v1/chat/completions/stream", chatHandler.HandleStream)
```

### Agent 管理

```go
agentHandler := handlers.NewAgentHandler(discoveryRegistry, agentRegistry, logger)

// 注册路由
http.HandleFunc("/v1/agents", agentHandler.HandleListAgents)
http.HandleFunc("/v1/agents/execute", agentHandler.HandleExecuteAgent)
http.HandleFunc("/v1/agents/plan", agentHandler.HandlePlanAgent)
http.HandleFunc("/v1/agents/health", agentHandler.HandleAgentHealth)
```

## 🔧 辅助函数

### WriteJSON
写入 JSON 响应（带正确的 Content-Type 和安全头）

```go
WriteJSON(w, http.StatusOK, data)
```

### WriteSuccess
写入成功响应（自动包装为 Response 结构）

```go
WriteSuccess(w, data)
```

### WriteError
写入错误响应（从 types.Error 转换）

```go
err := types.NewError(types.ErrInvalidRequest, "invalid input")
WriteError(w, err, logger)
```

### WriteErrorMessage
写入简单错误消息

```go
WriteErrorMessage(w, http.StatusBadRequest, types.ErrInvalidRequest, "invalid input", logger)
```

### DecodeJSONBody
解码 JSON 请求体（带验证）

```go
var req ChatRequest
if err := DecodeJSONBody(w, r, &req, logger); err != nil {
    return // 错误已自动写入响应
}
```

### ValidateContentType
验证 Content-Type 是否为 application/json

```go
if !ValidateContentType(w, r, logger) {
    return // 错误已自动写入响应
}
```

## 🎨 最佳实践

### 1. Handler 结构
每个 handler 应该包含：
- 依赖注入（logger, provider, registry 等）
- 请求验证
- 业务逻辑调用
- 响应转换
- 错误处理

```go
func (h *ChatHandler) HandleCompletion(w http.ResponseWriter, r *http.Request) {
    // 1. 验证 Content-Type
    if !ValidateContentType(w, r, h.logger) {
        return
    }

    // 2. 解码请求
    var req api.ChatRequest
    if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
        return
    }

    // 3. 验证请求
    if err := h.validateChatRequest(&req); err != nil {
        WriteError(w, err, h.logger)
        return
    }

    // 4. 调用业务逻辑
    resp, err := h.provider.Completion(ctx, llmReq)
    if err != nil {
        h.handleProviderError(w, err)
        return
    }

    // 5. 返回响应
    WriteSuccess(w, resp)
}
```

### 2. 错误处理
- 使用 `types.Error` 而不是 `fmt.Errorf`
- 设置正确的 HTTP 状态码
- 标记是否可重试
- 记录详细日志

```go
err := types.NewError(types.ErrInvalidRequest, "model is required").
    WithHTTPStatus(http.StatusBadRequest).
    WithRetryable(false)
WriteError(w, err, h.logger)
```

### 3. 日志记录
- 使用结构化日志（zap）
- 记录关键信息（请求 ID、耗时、Token 使用等）
- 错误日志包含完整上下文

```go
h.logger.Info("chat completion",
    zap.String("model", req.Model),
    zap.Int("tokens_used", resp.Usage.TotalTokens),
    zap.Duration("duration", duration),
)
```

### 4. 类型转换
- API 类型 ↔ 内部类型转换应该在 handler 层完成
- 使用专门的转换函数（如 `convertToLLMRequest`）
- 保持类型安全

```go
func (h *ChatHandler) convertToLLMRequest(req *api.ChatRequest) *llm.ChatRequest {
    messages := make([]types.Message, len(req.Messages))
    for i, msg := range req.Messages {
        messages[i] = types.Message(msg)
    }
    return &llm.ChatRequest{
        Model:    req.Model,
        Messages: messages,
        // ...
    }
}
```

## 🔒 安全考虑

1. **输入验证**：所有输入都应该验证
2. **Content-Type 检查**：防止 MIME 类型混淆攻击
3. **未知字段拒绝**：`DisallowUnknownFields()` 防止参数污染
4. **安全响应头**：`X-Content-Type-Options: nosniff`
5. **错误信息脱敏**：不暴露内部实现细节

## 📊 性能优化

1. **响应包装器**：使用 `ResponseWriter` 捕获状态码，避免重复写入
2. **流式响应**：大数据量使用 SSE 流式传输
3. **上下文超时**：所有请求都应该设置超时
4. **连接复用**：HTTP/2 支持

## 🧪 测试

每个 handler 都应该有对应的测试文件：

```
handlers/
├── common_test.go
├── health_test.go
├── chat_test.go
└── agent_test.go
```

测试应该覆盖：
- 正常流程
- 错误处理
- 边界条件
- 并发安全

## 📝 TODO

- [ ] 添加单元测试
- [ ] 添加集成测试
- [ ] 添加 OpenAPI 文档生成
- [ ] 添加请求限流
- [ ] 添加请求追踪（OpenTelemetry）
- [ ] 添加指标收集（Prometheus）

## 🔗 相关文档

- [API 类型定义](../types.go)
- [错误处理](../../types/error.go)
- [OpenAPI 规范](../openapi.yaml)
