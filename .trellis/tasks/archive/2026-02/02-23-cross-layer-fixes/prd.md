# 跨层一致性修复

## 目标

修复跨层检查发现的 4 个问题，提升框架的生产就绪度。

## 需求

### P1: Telemetry Shutdown 顺序修复
- **文件**: `cmd/agentflow/server.go` `Shutdown()` 方法
- **问题**: 当前 telemetry shutdown（step 1.5, line ~479）在 HTTP server shutdown（step 2, line ~487）之前执行，导致 shutdown 期间 in-flight 请求的 span/metric 丢失
- **修复**: 将 telemetry flush/shutdown 移到 HTTP server 和 Metrics server 关闭之后（step 3 和 step 4 之间，或 step 4 之后）
- **正确顺序**: 停止热更新 → 关闭 HTTP server → 关闭 Metrics server → flush telemetry → 等待 goroutine

### P2: Middleware 错误响应格式统一
- **文件**: `cmd/agentflow/middleware.go`
- **问题**: middleware 层的错误响应使用 3+ 种不同的 JSON 格式，与 handler 层的 `Response` 信封格式不一致
- **修复**: 所有 middleware 错误响应改用 `api/handlers.WriteErrorMessage` 或等效的统一格式 `{"success":false,"error":{"code":"...","message":"..."}}`
- **涉及位置**:
  - `Recovery` middleware — `http.Error()` 调用
  - `APIKeyAuth` middleware — `fmt.Fprint()` 调用
  - `RateLimiter` middleware — `fmt.Fprint()` 调用
  - `writeJSONError` helper — `fmt.Fprintf()` 调用
  - `TenantRateLimiter` middleware — `fmt.Fprint()` 调用
- **注意**: middleware 在 `cmd/agentflow` 包中，不能直接 import `api/handlers`（会造成循环依赖）。应在 middleware.go 中定义一个本地的 `writeMiddlewareError(w, statusCode, code, message)` helper，输出与 handler 层一致的 JSON 格式

### P3: AgentResolver 缓存安全
- **文件**: `cmd/agentflow/server.go` `buildAgentResolver()` 方法
- **问题 1**: 两个 goroutine 同时为同一 agentID 创建 agent 时，都会执行 Create+Init，但 LoadOrStore 只保留一个，另一个泄漏
- **问题 2**: `Server.Shutdown()` 不清理 `s.agents` 中缓存的 agent 实例
- **修复 1**: 使用 `sync.Map` 的 LoadOrStore 先占位（存一个 placeholder/channel），或使用 `singleflight.Group` 确保同一 agentID 只创建一次
- **修复 2**: 在 `Shutdown()` 中遍历 `s.agents`，对每个 agent 调用 teardown/stop

### P4: ValidateNonNegative 复用
- **文件**: `api/handlers/apikey.go`
- **问题**: `HandleCreateAPIKey` 和 `HandleUpdateAPIKey` 各有 4 处内联 `< 0` 检查，未使用 `common.go` 中已有的 `ValidateNonNegative` helper
- **修复**: 将 8 处内联检查替换为 `ValidateNonNegative()` 调用

## 验收标准

- [ ] Telemetry shutdown 在 HTTP/Metrics server 之后执行
- [ ] 所有 middleware 错误响应使用统一的 JSON 信封格式
- [ ] 同一 agentID 的并发创建不会泄漏 agent 实例
- [ ] Server.Shutdown() 清理所有缓存的 agent
- [ ] apikey.go 使用 ValidateNonNegative helper
- [ ] 所有现有测试通过
- [ ] `go vet ./...` 无错误
