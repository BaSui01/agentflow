# Startup Composition Chain

This document defines the runtime startup chain and composition boundaries.

## Runtime Chain

`cmd/agentflow/main.go:runServe -> internal/app/bootstrap.InitializeServeRuntime -> cmd/agentflow.NewServer(...).Start -> cmd/agentflow/server_handlers_runtime.go:initHandlers -> cmd/agentflow/server_http.go:startHTTPServer -> internal/app/bootstrap.RegisterHTTPRoutes -> api/routes -> api/handlers -> domain(agent/rag/workflow/llm)`

## Composition Boundaries

- `cmd/agentflow` is the composition root and lifecycle host.
- `internal/app/bootstrap` centralizes startup builders used by `cmd`.
- `api/handlers` stays focused on protocol conversion and delegates domain behavior.
- `workflow` is the Layer 3 orchestrator; it is not an `agent` subtype and should only coordinate lower-level capabilities.
- `agent` and `rag` are peer Layer 2 domain capabilities; either may be called directly from handler/usecase entrypoints.

## Layer Map

```text
cmd/                         composition root
  -> api/                    protocol adapters
    -> workflow/             Layer 3 orchestration (optional entry)
    -> agent/                Layer 2 execution capability
    -> rag/                  Layer 2 retrieval capability
    -> llm/                  Layer 1 provider/gateway capability
      -> types/              Layer 0 zero-dependency contracts

pkg/                         horizontal infrastructure, reusable by multiple layers
internal/app/bootstrap/      startup-only builders/bridges, not domain decision logic
```

Rules:

- Not every request enters `workflow`; `agent` and `rag` remain direct domain entries.
- When a workflow uses an agent step, the dependency direction is `workflow -> agent`, never `agent -> workflow` as the default main chain.
- A single `agent` may use `rag` directly; `rag` is not reserved for workflow or multi-agent scenarios.

## Allowed / Forbidden Dependency Matrix

| Source | Allowed to depend on | Forbidden to depend on |
| --- | --- | --- |
| `types/` | none | `llm/`, `agent/`, `rag/`, `workflow/`, `api/`, `cmd/`, `internal/`, `config/`, `pkg/` |
| `llm/` | `types/`, `pkg/`, `config/` | `agent/`, `rag/`, `workflow/`, `api/`, `cmd/`, `internal/` |
| `agent/` | `types/`, `llm/`, `rag/`, `pkg/`, `config/` | `workflow/`, `api/`, `cmd/`, `internal/` |
| `rag/` | `types/`, `llm/`, `pkg/`, `config/` | `agent/`, `workflow/`, `api/`, `cmd/`, `internal/` |
| `workflow/` | `types/`, `llm/`, `agent/`, `rag/`, `pkg/`, `config/` | `api/`, `cmd/`, `internal/`, `agent/persistence` |
| `api/` | `types/`, `llm/`, `agent/`, `rag/`, `workflow/`, `config/` | provider implementation details, composition-root logic |
| `cmd/` | all runtime builders via `internal/app/bootstrap` and domain entrypoints | domain implementation hidden inside handler/business packages |
| `pkg/` | `types/` and other `pkg/` subpackages as needed | `api/`, `cmd/` |

Notes:

- `agent/` and `rag/` are peer Layer 2 capabilities, not parent/child modules.
- `workflow/` can orchestrate `agent/` and `rag/`, but that does not make `workflow/` the mandatory entry for all requests.
- `internal/app/bootstrap` is intentionally excluded from normal domain dependency graphs; it is startup-only composition support.

## Current Builder Split

- `internal/app/bootstrap/bootstrap.go`
  - config loading/validation
  - logger and telemetry initialization
  - database connection setup
- `internal/app/bootstrap/handler_runtime_builder.go`
  - LLM runtime setup (reusable main-provider assembly + default legacy multi-provider router path)
  - `BuildLLMHandlerRuntimeFromProvider(...)` now delegates to the public `llm/runtime/compose.Build(...)` seam so bootstrap and external projects reuse the same handler runtime wiring around any already-constructed main provider
  - `llm/runtime/compose.Runtime` now exposes a shared `Gateway`, so handler/runtime consumers reuse one unified chat entry instead of rebuilding provider-side adapters per domain
  - chat middleware chain setup
  - policy/cache/metrics/budget runtime wiring
- `internal/app/bootstrap/domain_runtime_builders.go`
  - protocol server runtime setup (MCP/A2A)
  - RAG runtime setup (embedding provider + vector store)
  - workflow runtime setup (DAG executor + DSL parser)
- `internal/app/bootstrap/multimodal_runtime_builder.go`
  - multimodal handler runtime setup (provider config + policy + store binding)
- `internal/app/bootstrap/multimodal_reference_store_builder.go`
  - redis reference store client construction and security validation
- `internal/app/bootstrap/http_auth_builder.go`
  - HTTP auth middleware selection and fail-closed guard construction
- `internal/app/bootstrap/http_middleware_builder.go`
  - HTTP middleware chain assembly and limiter lifecycle cancel wiring
- `internal/app/bootstrap/http_server_builder.go`
  - HTTP route registration and startup server config builders (app + metrics)
- `internal/app/bootstrap/hotreload_runtime_builder.go`
  - hot-reload manager/api handler construction and callback registration
  - config reload callbacks now rebuild the handler-facing text runtime in place (`main provider`, `chat`, `agent resolver`, `workflow`, `cost`) through the same public LLM runtime seam
  - workflow reload reuses one shared `hitl.InterruptManager`, so pending workflow interrupts survive text-runtime swaps instead of being orphaned by a new parser/runtime instance
  - previous agent resolver caches are reset only after a successful runtime swap, including rollback-driven restoration of the last good text runtime, so stale cached agents are torn down only after the replacement runtime is live
- `internal/app/bootstrap/handler_adapters_builder.go`
  - agent registry/handler and api-key/tool-registry/tool-approval handler adapter builders
- `internal/app/bootstrap/agent_runtime_factory_builder.go`
  - default runtime-backed agent factory registration
- `internal/app/bootstrap/agent_tool_approval_builder.go`
  - tool approval interrupt adapter for hosted-tool permission checks
  - bridges `llm/capabilities/tools.PermissionManager` approval callbacks onto a dedicated `hitl.InterruptManager`
  - maintains scoped temporary approval grants with configurable TTL
  - supports `memory / file / redis` grant backends, so approval windows can stay process-local, survive restarts, or be shared across instances
- `internal/app/bootstrap/agent_tool_policy_builder.go`
  - shared hosted-tool permission manager bootstrap
  - default risk-tier rules: safe read-only tools allow, mutating/exec/MCP tools require approval, unknown tools deny by default
  - agent-specific allow/deny filtering helpers for runtime tool exposure
- `internal/app/bootstrap/agent_tooling_runtime_builder.go`
  - hosted tool registry composition for agent runtime
  - shared permission-aware hosted tool execution chain (`ToolManager -> ToolRegistry.Execute -> permission check -> tool execute`)
  - ToolManager bridge wiring (`hosted.ToolRegistry -> agent.ToolManager`)
  - built-in retrieval tool and MCP tool bridge registration (`mcp_*`)
  - shared ToolManager injection target for both `AgentHandler` and `ChatHandler` local tool loop
  - DB-backed dynamic bindings reload support (`/api/v1/tools*` writes -> runtime reload)
  - runtime reload callback hook for resolver cache reset (new tool bindings effective immediately)
- `internal/app/bootstrap/capability_catalog_builder.go`
  - read-only runtime capability catalog for hosted tools, agent types, and multi-agent modes
  - used by server bootstrap for startup-time capability visibility and future统一能力面收敛
- `internal/app/bootstrap/mongo_wiring_builder.go`
  - Mongo prompt/conversation/run stores wiring
  - Mongo optional capabilities wiring (audit, memory, ab-testing, registry persistence)
- `internal/app/bootstrap/mongo_client_builder.go`
  - MongoDB client creation and startup logging

## Routed Provider Paths

- Default legacy startup path
  - `Handler/Service -> Gateway -> RoutedChatProvider -> MultiProviderRouter -> provider API`
  - This remains the built-in DB-backed provider/model/api_key routing path used by current bootstrap defaults.
- Recommended channel-based path
  - `Handler/Service -> Gateway -> ChannelRoutedProvider -> resolvers/selectors -> provider factory -> provider API`
  - This is the recommended single routed-provider chain when an external project already owns `channel / key / model mapping` semantics.
- Shared bootstrap seam
  - External projects use `llm/runtime/compose.Build(...)` to assemble middleware/cache/policy/tool-provider wiring around any main provider.
  - `internal/app/bootstrap.BuildLLMHandlerRuntimeFromProvider(...)` remains the composition-root adapter that reuses the same public assembly seam for the built-in server startup path.
  - The built-in startup chain now passes the shared runtime `Gateway` down into workflow runtime assembly and multimodal structured planning, so business-layer chat calls converge on `llmcore.Gateway`.
  - Built-in startup selection now uses `llm.main_provider_mode`, while public registration goes through `llm/runtime/compose.RegisterMainProviderBuilder(...)`.
  - Adapter-only integration, built-in config-switch usage, and `channelstore.NewMainProviderBuilder(...)` examples are documented in `docs/architecture/channel-routing-adapter-template.zh-CN.md` and `docs/architecture/channel-routing-adapter-template.md`.
  - Built-in config hot reload now reuses the same seam to rebuild the text runtime in place when `llm.main_provider_mode` or related LLM runtime config changes.
  - Hot reload only mutates handlers that were already bound at startup; if chat or cost routes were absent during initial route registration, a restart is still required to expose those endpoints later.
  - Workflow hot reload keeps using the same shared HITL manager across parser/runtime rebuilds, so approval/input interrupts remain resolvable after text-runtime reload.
  - Successful text-runtime reload also resets the previous resolver cache after the swap; if a rebuild fails, the rollback path re-applies the restored config through the same seam before any stale resolver cache is torn down.
- Boundary rule
  - `MultiProviderRouter` and `ChannelRoutedProvider` are alternative routed-provider entries. Do not wrap `MultiProviderRouter` inside `ChannelRoutedProvider`, and do not build a dual-routing request chain.

## Routing Decision Summary

- Use `Gateway -> RoutedChatProvider -> MultiProviderRouter` when you want the framework's built-in DB-backed `provider + api_key pool` path.
- Use `Gateway -> ChannelRoutedProvider` when your project already owns `channel / key / model mapping` semantics and only needs agentflow to host the reusable routed-provider chain.
- Treat them as two alternative entries behind `Gateway`, not a parent-child layering. One request should go through one routed-provider chain only.

## Phase 1 Scope

- Phase 1 only moves text `chat/completion/stream` onto the new channel-routed path.
- `image/video` stay outside `ChannelRoutedProvider` in Phase 1 because the cleaner multimodal extension surface is `llm/gateway + llm/capabilities/* + llm/providers/vendor.Profile`, not a larger `llm.Provider`.
- More concretely: image/video routing already depends on capability-specific provider surfaces and vendor profiles, so pushing them into `ChannelRoutedProvider` now would mix text routing concerns with multimodal capability dispatch too early.
- This keeps the startup chain single-entry while avoiding a premature merge of text routing and multimodal capability routing.
