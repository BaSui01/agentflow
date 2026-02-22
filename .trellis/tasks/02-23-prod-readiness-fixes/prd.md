# 生产就绪度修复 — OTel 接线 + API 验证 + 测试覆盖

## 目标

修复生产就绪度审计中发现的 3 个关键问题，使框架达到可上线状态。

---

## P0: OTel 追踪接线

### 背景
`llm/observability/` 中已有 OTel Tracer/Meter 消费代码，但 `cmd/agentflow/main.go` 中没有 TracerProvider/MeterProvider 初始化。当前所有 span 和 OTel 指标都被 noop 实现丢弃。

### 需求
1. 新建 `internal/telemetry/telemetry.go`，封装 OTel SDK 初始化逻辑
2. 创建 `Init(cfg config.TelemetryConfig, logger *zap.Logger) (*Providers, error)` 函数：
   - 当 `cfg.Enabled == false` 时返回 noop（不注册任何 provider）
   - 当 `cfg.Enabled == true` 时：
     - 创建 OTLP gRPC trace exporter（endpoint 来自 `cfg.OTLPEndpoint`）
     - 创建 OTLP gRPC metric exporter（同 endpoint）
     - 配置 Resource（`service.name` = `cfg.ServiceName`，`service.version` 从 build info 获取）
     - 配置 TraceIDRatioSampler（`cfg.SampleRate`）
     - 注册为全局 TracerProvider 和 MeterProvider
3. `Providers` 结构体提供 `Shutdown(ctx context.Context) error` 方法，flush 并关闭 exporters
4. 在 `cmd/agentflow/main.go` 的 `runServe()` 中，日志初始化之后、`NewServer()` 之前调用 `telemetry.Init()`
5. 在 `Server.Shutdown()` 中调用 `providers.Shutdown()`
6. 需要新增 go 依赖：`go.opentelemetry.io/otel/sdk`、`go.opentelemetry.io/otel/sdk/metric`、`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`、`go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`、`go.opentelemetry.io/otel/semconv`

### 验收标准
- [ ] `cfg.Telemetry.Enabled = false` 时不连接任何外部服务
- [ ] `cfg.Telemetry.Enabled = true` 时注册全局 TracerProvider + MeterProvider
- [ ] Server shutdown 时 flush 所有 pending spans/metrics
- [ ] `go build ./cmd/agentflow/` 编译通过
- [ ] `go vet ./internal/telemetry/...` 通过

---

## P1: 高风险零测试包补充测试

### 背景
34 个零测试包中有高风险模块需要优先覆盖。

### 需求
为以下 3 个高优先级包编写单元测试：

#### 1. `agent/voice/` — `realtime_test.go`
- 测试 `NewVoiceAgent` 构造 + `GetState` / `GetMetrics`
- 测试 `VoiceSession.SendAudio`（closed session、buffer full）
- 测试 `VoiceSession.Close`（double close）
- 测试 `NativeAudioReasoner.Process`（mock provider、timeout）
- 使用 mock STTProvider/TTSProvider/NativeAudioProvider（inline function callback 模式）

#### 2. `llm/batch/` — `processor_test.go`
- 测试 `NewBatchProcessor` + `Close`（worker 启动/停止）
- 测试 `Submit`（正常、closed、context cancel）
- 测试 `SubmitSync`（正常、timeout）
- 测试 `BatchStats.BatchEfficiency`
- 并发安全性测试

#### 3. `agent/federation/` — 扩展 `orchestrator_test.go`
- 测试 `RegisterNode` / `UnregisterNode`
- 测试 `SubmitTask`（含能力匹配）
- 测试 `GetTask` / `ListNodes`
- 并发安全性测试

### 测试规范
- 白盒测试（same package）
- 表驱动测试，变量名 `tests` + `tt`
- 手写 mock + function callback 模式，禁止 `testify/mock`
- 使用 `testutil.TestContext(t)` 获取 context
- 使用 `t.Cleanup()` 而非 `defer`
- 使用 `t.Helper()` 标记 helper 函数
- 使用 `testutil.WaitFor` 而非 `time.Sleep`

### 验收标准
- [ ] 每个测试文件至少 5 个测试用例
- [ ] `go test ./agent/voice/... -v -race -count=1` 通过
- [ ] `go test ./llm/batch/... -v -race -count=1` 通过
- [ ] `go test ./agent/federation/... -v -race -count=1` 通过

---

## P2: API 请求体验证统一

### 背景
`apikey.go` handler 绕过了 `DecodeJSONBody` 的安全措施（1MB 限制、DisallowUnknownFields），各 handler 的业务验证不统一。

### 需求
1. 修复 `api/handlers/apikey.go`：
   - `HandleCreateAPIKey` 和 `HandleUpdateAPIKey` 改用 `ValidateContentType` + `DecodeJSONBody`
   - 添加字段验证：`base_url` 非空时验证 URL 格式、`priority`/`weight` >= 0、`rate_limit_rpm`/`rate_limit_rpd` >= 0
2. 增强 `api/handlers/chat.go` 的 `validateChatRequest`：
   - `max_tokens` > 0 验证（当设置时）
   - `messages[].role` 枚举验证（system/user/assistant/tool）
3. 在 `api/handlers/common.go` 中添加通用验证辅助函数：
   - `ValidateURL(s string) bool` — 验证 URL 格式
   - `ValidateEnum(value string, allowed []string) bool` — 枚举验证
   - `ValidateRange(value, min, max float64) bool` — 数值范围验证

### 验收标准
- [ ] `apikey.go` 所有 POST/PUT handler 使用 `ValidateContentType` + `DecodeJSONBody`
- [ ] 非法输入返回 400 + 结构化错误信息
- [ ] `go build ./api/...` 编译通过
- [ ] `go vet ./api/...` 通过

---

## 技术说明
- 不引入第三方验证库（保持项目现有风格：手写验证 + 共享辅助函数）
- OTel 初始化代码放在 `internal/telemetry/` 新包中，遵循 `internal/` 隔离约定
- 测试遵循 `.trellis/spec/unit-test/index.md` 中的所有规范
