# 修复生产审计 P2 问题（可直接修复的 13 项）

## 目标
修复生产就绪度审计中发现的 P2 问题中可直接修复的 13 项，提升代码质量和一致性。

## 排除说明
以下 P2 问题暂不在本次修复范围内（需要大规模重构、新依赖或独立设计）：
- 架构分层重构（middleware/ORM 模型移动）
- singleflight / 高性能 JSON 库（新依赖）
- sync.Pool 池化（需仔细评估）
- healthz/startup probe 改造（需设计）
- HPA/PDB 部署配置
- 熔断器错误率模式（新功能）
- E2E 测试改进、覆盖率阈值（已在 P1 中处理）
- Duration 类型不一致、handoff/batch Message 子集（需评估影响面）
- Config API 双重认证逻辑（需设计）

## 工作分组

### Group 1: API Handler 响应一致性（2 项）
负责文件: `api/handlers/common.go`, `api/handlers/apikey.go`

1. **WriteError 不传递 request_id** — `api/handlers/common.go:79-84`
   - 在 `WriteError` 中从 request context 或 `X-Request-ID` header 读取 request_id
   - 填充到错误响应的 `request_id` 字段中

2. **HandleCreateAPIKey 返回 201 缺少 timestamp** — `api/handlers/apikey.go:228-231`
   - 在 201 响应中添加 `Timestamp` 字段（`time.Now()`）

### Group 2: Config API 修复（3 项）
负责文件: `config/api.go`

3. **handleReload Content-Type 验证不一致** — `config/api.go:268-278`
   - 统一要求 POST 请求必须携带 `Content-Type: application/json`
   - 使用与 `api/handlers/common.go` 相同的 `ValidateContentType` 逻辑

4. **limit 查询参数无上限** — `config/api.go:372`
   - 添加 `limit` 上限校验（max 1000），超过时 clamp 到 1000

5. **apiError 字段与 api.ErrorInfo 不完全一致** — `config/api.go:34-37`
   - 在 `apiError` 结构体中添加缺失的 `Details` 字段（可选）
   - 注意：不要引入循环依赖，config 包不能 import api 包

### Group 3: LLM + Agent 契约字段修复（2 项）
负责文件: `llm/providers/common.go`, `agent/protocol/a2a/generator.go`

6. **ToLLMChatResponse 未转换 CreatedAt** — `llm/providers/common.go:240-278`
   - 将 `OpenAICompatResponse.Created`（int64 unix timestamp）映射到 `ChatResponse.CreatedAt`（time.Time）
   - 使用 `time.Unix(resp.Created, 0)`

7. **A2A convertToolSchema 丢失 Version 字段** — `agent/protocol/a2a/generator.go:134-154`
   - 在 `convertToolSchema` 函数中添加 `Version` 字段复制

### Group 4: 中间件 + Telemetry 修复（2 项）
负责文件: `cmd/agentflow/middleware.go`, `internal/telemetry/telemetry.go`

8. **中间件错误响应使用手写 JSON** — `cmd/agentflow/middleware.go:518-523`
   - 将手写 JSON 模板替换为结构化 `json.Marshal` 序列化
   - 定义一个本地 errorResponse 结构体（避免 import api 包导致循环依赖）

9. **OTLP 传输默认 Insecure** — `internal/telemetry/telemetry.go:61,70`
   - 将 `WithInsecure()` 改为条件化：仅当配置了 `OTLPInsecure: true` 时使用
   - 默认使用 TLS 连接

### Group 5: 性能/指标/安全修复（4 项）
负责文件: `config/defaults.go`, `internal/database/pool.go`, `internal/metrics/collector.go`, `agent/skills/skill.go`

10. **MaxIdleConns 默认值不一致** — `config/defaults.go:82` vs `internal/database/pool.go:50`
    - 统一为 10（database/pool.go 的值）

11. **Cache metrics 缺少指标** — `internal/metrics/collector.go`
    - 添加 `cache_evictions_total` Counter 和 `cache_size` Gauge 指标注册

12. **Skills filepath.Join 路径遍历风险** — `agent/skills/skill.go:104`
    - 在 `filepath.Join` 之后校验结果路径是否仍在 `dir` 目录下
    - 使用 `filepath.Rel` 或 `strings.HasPrefix` 检查

13. **TLS 1.3 未显式配置** — `pkg/tlsutil/tlsutil.go`
    - 在 `DefaultTLSConfig()` 中添加 `MaxVersion: tls.VersionTLS13`

## 验收标准
- [ ] 所有修改通过 `go build ./...`
- [ ] 所有修改通过 `go vet ./...`
- [ ] 现有测试不被破坏
- [ ] 每个 Group 的修改可独立验证

## 技术约束
- 不引入新的外部依赖
- 不引入循环依赖（特别注意 config 包和 cmd 包）
- 遵循项目现有代码风格和规范
