# 修复生产审计 P1 问题

## 目标
修复生产就绪度审计中发现的 11 个 P1 问题，提升项目生产就绪度评分。

## 工作分组

### Group A: 安全修复（3 项）
负责文件: `cmd/agentflow/middleware.go`

1. **HSTS 响应头缺失** — `middleware.go:381-393`
   - 在 `SecurityHeaders()` 中添加 `Strict-Transport-Security: max-age=31536000; includeSubDomains`

2. **JWT HMAC Secret 无最小长度校验** — `middleware.go:435`
   - 在 `JWTAuth()` 初始化时校验 `len(cfg.Secret) >= 32`，不满足时返回错误或记录 WARN 日志
   - 注意：不要 panic，使用 logger.Warn 警告

3. **Auth 中间件在无认证配置时完全禁用** — `cmd/agentflow/server.go:407-410`
   - 在 `buildAuthMiddleware` 返回 nil 时，如果不是显式 dev mode，记录 WARN 级别日志
   - 添加配置项 `AllowNoAuth bool` (默认 false)，生产模式下无认证配置时拒绝启动

### Group B: CI 质量门禁（3 项）
负责文件: `.github/workflows/ci.yml`, `Makefile`

4. **golangci-lint 未在 CI 中运行**
   - 在 CI workflow 中添加 golangci-lint 步骤（使用 golangci/golangci-lint-action）
   - 放在 test 步骤之前

5. **govulncheck 非阻塞**
   - 移除 `.github/workflows/ci.yml` 中 govulncheck 步骤的 `continue-on-error: true`

6. **覆盖率阈值未在 CI 中强制执行**
   - 在 CI 中添加 `make coverage-check` 步骤
   - 将 Makefile 中 `COVERAGE_THRESHOLD` 默认值从 40 提升至 55

### Group C: 接口契约修复（3 项）
负责文件: `api/handlers/chat.go`, `llm/providers/common.go`, `llm/providers/anthropic/provider.go`

7. **`convertToLLMRequest` 丢失 Metadata 和 Timestamp** — `api/handlers/chat.go:240-247`
   - 在消息转换循环中添加 `Metadata` 和 `Timestamp` 字段复制

8. **`ConvertMessagesToOpenAI` 丢失 Images 字段** — `llm/providers/common.go:194-218`
   - 在 OpenAI 消息转换中处理 `Images` 字段
   - 如果 Images 非空，构建 multimodal content array（text + image_url parts）

9. **`convertToClaudeMessages` 丢失 Images 字段** — `llm/providers/anthropic/provider.go:270-326`
   - 在 Claude 消息转换中处理 `Images` 字段
   - 如果 Images 非空，构建 Claude 的 content blocks（text + image source）

### Group D: 基础设施 + API（2 项）
负责文件: `internal/telemetry/telemetry.go`, `api/openapi.yaml`

10. **OTel Logs 支柱缺失** — `internal/telemetry/telemetry.go`
    - 不需要完整的 OTel Logs SDK
    - 在现有 zap logger 中添加 trace context 字段注入：创建一个 zap Core wrapper，从 context 中提取 trace_id 和 span_id 并自动添加到日志字段
    - 或者：在 `telemetry.go` 中导出 `LoggerWithTrace(ctx, logger) *zap.Logger` 辅助函数

11. **条件路由注册未在 OpenAPI spec 中说明** — `api/openapi.yaml`
    - 在 Chat、Provider/APIKey 相关端点的 description 中添加条件说明
    - 使用 `x-conditional` 扩展字段标注启用条件

## 验收标准
- [ ] 所有修改通过 `go build ./...`
- [ ] 所有修改通过 `go vet ./...`
- [ ] 现有测试不被破坏
- [ ] 每个 Group 的修改可独立验证

## 技术约束
- 不引入新的外部依赖（OTel Logs SDK 除外，如果选择完整方案）
- 遵循项目现有代码风格和规范
- JWT refresh token 机制（P1-3 中提到的）暂不实现，仅做密钥长度校验
