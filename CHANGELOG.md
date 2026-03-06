# Changelog

All notable changes to AgentFlow will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
