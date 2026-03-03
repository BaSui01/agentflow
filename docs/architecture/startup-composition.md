# Startup Composition Chain

This document defines the runtime startup chain and composition boundaries.

## Runtime Chain

`cmd/agentflow/main.go:runServe -> internal/app/bootstrap.InitializeServeRuntime -> cmd/agentflow.NewServer(...).Start -> cmd/agentflow/server_handlers_runtime.go:initHandlers -> cmd/agentflow/server_http.go:startHTTPServer -> internal/app/bootstrap.RegisterHTTPRoutes -> api/routes -> api/handlers -> domain(agent/rag/workflow/llm)`

## Composition Boundaries

- `cmd/agentflow` is the composition root and lifecycle host.
- `internal/app/bootstrap` centralizes startup builders used by `cmd`.
- `api/handlers` stays focused on protocol conversion and delegates domain behavior.

## Current Builder Split

- `internal/app/bootstrap/bootstrap.go`
  - config loading/validation
  - logger and telemetry initialization
  - database connection setup
- `internal/app/bootstrap/handler_runtime_builder.go`
  - LLM runtime setup (`llm/runtime/router` multi-provider pool + routed provider as chat main entry)
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
- `internal/app/bootstrap/handler_adapters_builder.go`
  - agent registry/handler and api-key/tool-registry handler adapter builders
- `internal/app/bootstrap/agent_runtime_factory_builder.go`
  - default runtime-backed agent factory registration
- `internal/app/bootstrap/agent_tooling_runtime_builder.go`
  - hosted tool registry composition for agent runtime
  - ToolManager bridge wiring (`hosted.ToolRegistry -> agent.ToolManager`)
  - built-in retrieval tool and MCP tool bridge registration (`mcp_*`)
  - shared ToolManager injection target for both `AgentHandler` and `ChatHandler` local tool loop
  - DB-backed dynamic bindings reload support (`/api/v1/tools*` writes -> runtime reload)
  - runtime reload callback hook for resolver cache reset (new tool bindings effective immediately)
- `internal/app/bootstrap/mongo_wiring_builder.go`
  - Mongo prompt/conversation/run stores wiring
  - Mongo optional capabilities wiring (audit, memory, ab-testing, registry persistence)
- `internal/app/bootstrap/mongo_client_builder.go`
  - MongoDB client creation and startup logging
