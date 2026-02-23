# 并行修复 P0 + 实现 P1/P2 优化

## 目标
修复生产就绪度审计 v3 发现的 P0 安全/稳定性问题，并完成之前规划的 P2 修复项。

## 工作分组（按并行 Agent 划分）

### Agent 1: P0 — SQL Schema + 迁移修复
负责文件: `migrations/postgres/000001_init_schema.up.sql`, `migrations/mysql/000001_init_schema.up.sql`, `migrations/sqlite/000001_init_schema.up.sql`

**问题**: `sc_llm_provider_api_keys` 表缺少 `base_url` 列，但 Go 结构体 `LLMProviderAPIKey` 有 `BaseURL` 字段。

**修复方案**: 创建新的迁移文件 `000002_add_base_url_to_api_keys` (up/down 配对)，为三种数据库方言各添加：
- UP: `ALTER TABLE sc_llm_provider_api_keys ADD COLUMN base_url VARCHAR(500) DEFAULT '';`
- DOWN: `ALTER TABLE sc_llm_provider_api_keys DROP COLUMN base_url;`

### Agent 2: P0 — Shell 注入 + Docker 执行器修复
负责文件: `agent/execution/executor.go`, `agent/execution/docker_exec.go`

1. **Shell 注入** — `executor.go:412-417`
   - Go/Rust 语言：改用 `os.WriteFile` 写临时文件，不用 `echo` + shell 拼接
   - 使用 `os.CreateTemp` 创建安全临时文件，写入代码后再执行
   - 确保临时文件在执行后清理（defer os.Remove）

2. **DockerBackend.Run() mock** — `executor.go` 附近
   - 如果 `DockerBackend.Run()` 是 mock，添加明确的 `// TODO: implement real Docker execution` 注释
   - 在调用时如果检测到 mock，返回 `ErrNotImplemented` 错误而非静默成功

### Agent 3: P0 — 指标无界增长 + EventBus 安全
负责文件: `agent/observability/metrics.go`, `agent/event.go`

1. **LatencyHistory/QualityHistory 无界增长** — `metrics.go:68-70`
   - 改为环形缓冲区，最大保留最近 1000 条记录
   - 添加 `maxHistorySize` 常量
   - 在追加时检查长度，超过则截断旧数据

2. **EventBus processEvents 缺少 recover** — `event.go:116+`
   - 在 `processEvents` 的事件处理循环中添加 `defer-recover` 保护
   - handler panic 时记录日志但不崩溃整个 EventBus

### Agent 4: P2 Group 1+2 — API Handler + Config API 修复
负责文件: `api/handlers/common.go`, `api/handlers/apikey.go`, `config/api.go`

1. **WriteError 不传递 request_id** — `common.go:79-84`
   - 从 request context 或 `X-Request-ID` header 读取 request_id
   - 填充到错误响应的 `request_id` 字段

2. **HandleCreateAPIKey 返回 201 缺少 timestamp** — `apikey.go:228-231`
   - 在 201 响应中添加 `Timestamp` 字段

3. **handleReload Content-Type 验证不一致** — `config/api.go:268-278`
   - POST 请求统一要求 `Content-Type: application/json`

4. **limit 查询参数无上限** — `config/api.go:372`
   - 添加 max 1000 上限，超过时 clamp

5. **apiError 字段不完整** — `config/api.go:34-37`
   - 添加 `Details` 字段（不引入循环依赖）

### Agent 5: P2 Group 3+4 — LLM 契约 + 中间件修复
负责文件: `llm/providers/common.go`, `agent/protocol/a2a/generator.go`, `cmd/agentflow/middleware.go`, `internal/telemetry/telemetry.go`

注意：部分项可能已修复，需先检查当前代码状态。

6. **ToLLMChatResponse 未转换 CreatedAt** — `common.go:240-278`
   - 检查是否已修复；如未修复，用 `time.Unix(resp.Created, 0)` 映射

7. **A2A convertToolSchema 丢失 Version** — `generator.go:134-154`
   - 检查是否已修复；如未修复，添加 Version 字段复制

8. **中间件手写 JSON** — `middleware.go:518-523`
   - 替换为 `json.Marshal` + 本地 errorResponse 结构体

9. **OTLP 默认 Insecure** — `telemetry.go:61,70`
   - 条件化 `WithInsecure()`，仅当配置 `OTLPInsecure: true` 时使用

### Agent 6: P2 Group 5 — 性能/指标/安全修复
负责文件: `config/defaults.go`, `internal/database/pool.go`, `internal/metrics/collector.go`, `agent/skills/skill.go`, `pkg/tlsutil/tlsutil.go`

10. **MaxIdleConns 不一致** — 检查是否已修复，如未修复统一为 10

11. **Cache metrics 缺少指标** — `collector.go`
    - 添加 `cache_evictions_total` Counter 和 `cache_size` Gauge

12. **Skills 路径遍历** — `skill.go:104`
    - `filepath.Join` 后校验路径仍在 `dir` 下

13. **TLS 1.3 未显式配置** — `tlsutil.go`
    - 添加 `MaxVersion: tls.VersionTLS13`

## 验收标准
- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] 现有测试不被破坏
- [ ] 不引入新外部依赖
- [ ] 不引入循环依赖

## 技术约束
- config 包不能 import api 包
- cmd 包不能 import api 包（避免循环依赖）
- 遵循项目现有代码风格
