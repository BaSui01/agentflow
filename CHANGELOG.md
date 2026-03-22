# Changelog

All notable changes to AgentFlow will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
