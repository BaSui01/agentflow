# Changelog

All notable changes to AgentFlow will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.9.0] - 2026-04-01

### Added
- 新增 hosted tool 审批主链：高风险工具默认进入 `require_approval`，并提供 `/api/v1/tools/approvals*` 管理接口
- 新增授权窗口管理能力：支持 `request / agent_tool / tool` 三种 scope，以及 `grant_ttl` 临时授权窗口
- 新增工具审批授权窗口持久化后端：支持 `memory / file / redis`，其中 Redis 模式支持多实例共享授权窗口
- 新增审批可观测能力：支持审批统计、过期清理、active grants 查看与手动撤销、审批历史查询
- 新增只读 runtime capability catalog，用于汇总 hosted tools、agent types 与 multi-agent modes
- 新增 `AdaptiveWeightedSelector`：基于运行时成功率/429 率/失败率动态调整渠道权重，配套 `MetricsSource` 接口与 `InMemoryMetricsSource` 默认实现
- 新增 `InMemoryQuotaPolicy`：支持每日限额、每分钟速率限制、并发限制三重配额管控，配套 `QuotaStore` 接口
- 新增 `CascadeCooldownController`：Key 级冷却（可配失败阈值）+ 渠道级级联冷却（所有 key 失败时自动触发），支持运行时状态查询
- 新增 `AsyncUsageRecorder`：Worker pool 模式异步写回调用记录，队列满时降级同步写，支持 graceful shutdown drain

### Changed
- hosted tool 执行统一收敛到 shared permission-aware runtime，权限检查不再由单个工具各自分散实现
- tool approval grant 改为基于 fingerprint 的 scoped temporary grant，并支持跨进程恢复
- tool approval/history 相关配置统一收敛到 `hosted_tools.approval.*`

## [1.8.12] - 2026-03-31

### Added
- 统一消息/API 契约新增 `reasoning_summaries` 与 `opaque_reasoning`，在保留 `ReasoningContent` 兼容字段的同时区分“可展示 summary”与“不可展示 opaque state”

### Changed
- OpenAI Responses API 请求侧在 reasoning 开启时显式设置 `reasoning.summary`，并请求 `reasoning.encrypted_content`
- OpenAI/Anthropic/Gemini provider 与 agent/react 流式组装统一保留 provider-native reasoning/thinking 元数据，避免在 sync/stream/assembled 路径被压平或丢失

### Fixed
- 修复 OpenAI Responses 同步/流式路径未解析 `reasoning` output item，导致 reasoning summary 与 opaque reasoning state 丢失的问题
- 修复 Anthropic extended thinking 流式 `signature` / `redacted_thinking` 丢失的问题
- 修复 Gemini `thoughtSignature` 未 round-trip、OpenAI-compatible `reasoning_content` 回归风险，以及 API/handler 层对 reasoning 新字段的透传缺口

## [1.8.11] - 2026-03-26
### Added
- 新增 `ChannelRoutedProvider` 文本主链与 `channel_types` 抽象接口，支持 `ChannelSelector`、`ModelMappingResolver`、`SecretResolver`、`UsageRecorder`、`CooldownController`、`QuotaPolicy`、`ProviderConfigSource`
- 新增 `llm.main_provider_mode=channel_routed` 与公共 `llm/runtime/compose` main-provider registry，允许组合根按配置切换 legacy / channel-routed 主链
- 新增 `llm/runtime/router/extensions/channelstore` 与 `extensions/runtimepolicy`，提供静态 store、priority+weight selector、runtime policy 参考实现，以及外部 adapter/template 文档
- 新增 channel-routed 文本链回归测试，覆盖 startup mode switch、chat/stream、OpenAI-compatible、失败链 usage/baseURL/remoteModel 贯通

### Changed
- 文本 runtime hot reload 改为复用共享 workflow HITL manager；成功 swap 后才清理旧 resolver cache，缺失启动路由时明确要求重启
- `channelstore.NewMainProviderBuilder(...)` 支持透传 `ChatProviderFactory` / `LegacyProviderFactory`
- `ChannelRoutedProvider` 在上游未返回 `model` 时统一回填 resolved remote model，保持 direct response、gateway decision、usage record 一致

### Fixed
- `wireMongoStores` 在 `mongoClient/resolver/discoveryRegistry` 缺失时安全跳过，避免非完整启动/热更新测试场景下的空指针依赖
- workflow auto-approve handler 改为幂等注册，避免共享 HITL manager 下重复注册副作用

## [1.8.10] - 2026-03-23

### Changed
- `completion.go`: 合并 `steering==nil` 和 `IsZero()` 两个重复分支为单一条件
- `react.go`: 合并流式/非流式工具执行后的重复结果处理逻辑（-11行）
- `planner.go`: 提取 `advancePlanStatus()` 消除 `UpdatePlan`/`SetTaskResult` 重复的状态推进代码
- `executor.go`: 删除单行委托 `setTaskResult`，调用处直接使用 planner 公开 API
- `dispatcher.go`: `selectRoundRobin` 对 map keys 排序保证轮询顺序确定性

### Fixed
- `planner.go`: `generatePlanID` 使用完整 UUID 替代截断12位，消除高并发碰撞风险

## [1.8.6] - 2026-03-23

### Fixed
- OpenAI Responses API `function_call` 的 `arguments` 字段类型从 `json.RawMessage` 改为 `string`，符合官方 API 规范

### Added
- Agent Execute 新增 `CostCalculator` 成本估算，`Output.Cost` 字段不再为 0
- ReAct token 用量改为增量计算，避免多轮迭代中 PromptTokens 重复累加
- TTS 新增 `language` 字段支持
- STT 新增 `temperature` 字段支持
- Embedding 新增 `input_type` 字段支持

## [1.7.0] - 2026-03-22

### Added
- Canary (金丝雀) 发布改进：`CanaryConfig` 支持从数据库加载活跃部署、`CanaryDeployment` 新增 `AutoRollback`/`RollbackReason` 字段、`ProviderStats` 统计结构
- HierarchicalAgent 优化：`EnableLoadBalance` 负载均衡配置项、`WorkerStatus` 状态追踪增强
- 示例代码全面适配 v1.7.0：04/07/09/17/18/19 示例更新
- Agent 功能矩阵报告更新（`examples/98_agent_feature_matrix`）

### Changed
- 升级版本至 1.7.0
- `HierarchicalAgent` 重构：`TaskCoordinator` 工作者状态管理从简单 map 改为 `WorkerStatus` 结构体，支持负载均衡策略
- `CanaryConfig` 重构：新增 `Stop()` 生命周期方法、`loadFromDB()` 安全处理 nil db、部署映射改为 `provider_id` 索引
- 示例 07 (Mid Priority Features) README 更新 Hosted Tools 回退链说明
- 示例 17/18/19 README 更新适配最新 API

### Fixed
- `CanaryConfig` nil db panic 防护
- `HierarchicalAgent` 并发安全：`workerStatus` 读写锁保护

## [1.6.2] - 2026-03-09

### Added
- Gemini 图像 provider：新增 3 个模型常量（`gemini-2.5-flash-image` / `gemini-3-pro-image-preview` / `gemini-3.1-flash-image-preview`）
- Gemini 图像 provider：实现 `StreamingProvider` 接口，通过 `streamGenerateContent?alt=sse` 支持原生 SSE 流式；文字思考 token 实时推送
- Gemini 图像 provider：补全所有 API 参数支持——`imageConfig`（imageSize 1K/2K/4K、aspectRatio）、`system_instruction`、`tools/google_search`（联网 grounding）、`thinkingConfig`（thinking_budget）、`safetySettings`（统一阈值）、`candidateCount`；均通过 `req.Metadata` 透传
- `image.StreamingProvider` 可选扩展接口与 `StreamChunk` 类型，供原生流式 provider 实现（类型断言感知，零破坏现有 `Provider` 接口）
- 多模态 handler：`handleImageStream` 感知 `StreamingProvider`，Gemini 走原生流式路径，新增 `image_generation.thinking` SSE 事件类型

### Changed
- Gemini 图像 `SupportedSizes()` 改为返回 Gemini 原生格式 `["1K","2K","4K"]`，并自动映射通用像素格式（"1024x1024"→"1K" 等）
- 文档 `docs/VIDEO_IMAGE_PROVIDERS.md` 补充 Gemini 模型版本对比表、全量参数说明、流式事件序列描述及请求示例

## [1.6.1] - 2026-03-09

### Added
- 多模态图像厂商：智谱(zhipu)、文心一格(baidu)、豆包/火山(doubao)、腾讯混元生图(tencent)、可灵(kling)；腾讯混元 TC3-HMAC-SHA256 签名实现
- 可灵 Kling 图像 provider：与视频共用 `KlingAPIKey`，一 Key 双用
- 文档 `docs/VIDEO_IMAGE_PROVIDERS.md`：接入清单、字节/可灵说明、一 Key 双用（OpenAI/Gemini/Kling）

### Changed
- 升级版本至 1.6.1
- OpenAI 一 Key 双用：配置 `openai_api_key` 且未配 `sora_api_key` 时同时注册 openai 图像与 sora 视频
- OpenAPI：image_providers / video_providers 描述更新

## [1.6.0] - 2026-03-06

### Added
- Deliberation multi-agent mode: multi-round self-reflection with convergence detection, replacing placeholder implementation
- SharedState/Blackboard interface for inter-agent shared state with InMemorySharedState implementation
- OrchestrationStep for Workflow DAG: bridge multi-agent collaboration (collaboration/hierarchical/crew/deliberation) into workflow nodes via DSL `type: orchestration`
- AgentTeam unified abstraction: `Team` interface with adapters for Collaboration, Hierarchical, and Crew modes (`agent/teamadapter`)
- File operations tools: `read_file`, `write_file`, `edit_file`, `list_directory` with path allowlist security
- Shell command tool: `run_command` with command blacklist/whitelist and timeout support
- MCP Client: `DefaultMCPClient` with `StdioTransport` (subprocess) and `SSETransport` (HTTP SSE) for connecting to external MCP servers
- Declarative tool chain DSL: `ToolChain` with sequential execution, argument mapping, and error strategies (fail/skip/retry)
- Workflow Checkpoint PostgreSQL persistence: `PostgreSQLCheckpointStore` with JSONB storage and version management
- PDF document loader: `PDFLoader` using `pdftotext` with fallback to raw text extraction
- HTML document loader: `HTMLLoader` using `golang.org/x/net/html` with script/style filtering
- Unified cost tracking service: `CostTracker` with per-provider, per-model, per-agent cost aggregation

### Changed
- Upgraded version to 1.6.0
- Exported `SortedAgentIDs`, `AggregateUsage`, `MergeMetadata` from `agent/collaboration` for cross-package reuse
- Unified error system: `types.Error` → `agent.Error` → `llm.Error` → `core.StepError` → `api.ErrorInfo` layered wrapping
- Eliminated `config` → `api` transitive dependency; config types inlined locally
- Migrated handler orchestration logic to `internal/usecase/` (agent_service, chat_service)
- Migrated `ReferenceStore` from `api/handlers/` to `pkg/storage/`
- Migrated `GormToolRegistryStore` from `api/handlers/` to `agent/hosted/store.go`
- Agent root package slimmed: extracted `errors.go`, `request.go`, `run_config.go`, `memory_facade.go`
- All workflow step errors unified to `core.NewStepError` format
- DSL parser/validator: magic strings replaced with `core.StepType` constants
- Config defaults: hardcoded addresses/values extracted to named constants
- Architecture guard expanded: 8 tests covering file budget, dependency direction, handler infra imports, DSL magic strings

### Fixed
- **Security**: shell_tool command injection hardened (dangerous patterns + path traversal); docker_exec path traversal; WebSocket Origin validation; CORS `"*"` rejection; audio/multimodal file path traversal
- **Goroutine safety**: added `defer recover()` to federation/hierarchical/TreeOfThought/reasoning goroutines; recover errors propagated via channels instead of silently logged
- **Resource leaks**: longrunning executor map cleanup; InMemoryVectorStore max entries + eviction; MCP CloseAll stops healthLoop; idempotency Manager exposes Close(); discovery timer reuse
- **Concurrency**: skills registry TOCTOU eliminated (Lock-recheck pattern); RefreshIndex single-Lock; configMu separated from execMu in agent base; shared_state watcher skip logged
- **Input validation**: nil input guards on react.go/integration.go/Pipeline.Execute; skills manager/skill empty-value checks; prepareChatRequest messages validation
- **Observability**: TraceID injected into ctx at ExecuteEnhanced entry; all integration middleware logs carry trace_id; dag_executor logs enriched with trace_id/workflow_id; RequestLogger records request_id; core modules panic on nil logger
- **API contract**: health endpoint uses unified `api.Response`; SSE errors aligned to `api.ErrorInfo`; agent list pagination metadata; `types.Error.HTTPStatus` hidden from JSON (`json:"-"`)
- **State machine**: A2A errors mapped to `types.ErrorCode`; `StepTypeHumanInput` constant added; `StorageType` constants for config loader
- Architecture guard: `pkg/middleware` allowlist updated; `tool_registry_service.go` gorm dependency removed
- Test fixes: streaming/gateway logger nil panic; workflow step error message assertions

## [1.5.0] - 2026-03-06

### Added
- Web Search provider with database persistence and auto-registration at startup
- Tool registration API with DB-driven auto-reload and shared registry
- OpenAI-compatible endpoints (`POST /v1/chat/completions`, `POST /v1/responses`)
- Gemini and Anthropic compatible endpoint tolerant parsing
- Livecheck regression tests for compatible endpoints
- LLM provider capability matrix documentation (9 capability columns + standalone providers)
- HTTP API overview in Chinese and English README

### Changed
- Unified chat and agent route parameters with multi-provider routing entry
- Replaced simplified implementations with full business implementations in agent, RAG, and audit modules
- Updated Chinese/English documentation to match actual codebase features
- Fixed directory navigation doc (removed references to non-existent directories)
- Upgraded architecture guard with stricter enforcement policies

### Fixed
- Architecture guard false positives
- CI pipeline: removed reference to non-existent `tests/contracts` package
- Makefile: fixed `test-e2e` and `test-integration` targets to use build tags instead of non-existent directories
- Livecheck script: replaced `panic()` with proper error handling

## [1.4.5] - 2026-03-02

### Added
- Unified multi-module protocol and capability implementation
- Tool registration routes and runtime reload regression tests
- Shared chat-agent tool registry

### Changed
- Converged processing pipeline docs and examples
- Cleaned up runtime assembly details

## [1.4.0] - 2026-02-28

### Added
- Multi-provider routing main entry
- Unified chat and agent route parameters
- Processing pipeline convergence

### Changed
- Architecture guard tooling improvements
- Cross-layer observability contract updates

---

For full commit history, see [GitHub Commits](https://github.com/BaSui01/agentflow/commits/master).
