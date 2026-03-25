# API Handlers

`api/handlers` 负责 HTTP 协议适配：请求解析、参数校验、错误映射、响应序列化。  
业务执行统一下沉到 service/usecase（如 `agent_service.go`、`rag_service.go`、`workflow_service.go`），由 `cmd/agentflow` + `internal/app/bootstrap` 完成运行时装配。

## 当前职责边界

- Handler：HTTP 入参校验、JSON 解码、SSE 写出、统一响应格式。
- Service：执行领域用例（Agent/RAG/Workflow/APIKey/Multimodal）。
- Bootstrap：构建 Provider/Store/Facade/Registry，并注入 Handler。

## 路由前缀（真实链路）

路由注册在 `api/routes/routes.go`，统一挂载到 `/api/v1/*`：

- Chat: `/api/v1/chat/capabilities`、`/api/v1/chat/completions`、`/api/v1/chat/completions/stream`
- OpenAI 兼容入站：`/v1/chat/completions`、`/v1/responses`（协议适配到同一 ChatService/gateway 链路）
- Agent: `/api/v1/agents`、`/api/v1/agents/capabilities`、`/api/v1/agents/execute`、`/api/v1/agents/execute/stream`、`/api/v1/agents/plan`、`/api/v1/agents/health`
- RAG: `/api/v1/rag/query`、`/api/v1/rag/index`
- Workflow: `/api/v1/workflows/execute`、`/api/v1/workflows/parse`、`/api/v1/workflows`
- Multimodal: `/api/v1/multimodal/*`
- Protocol: `/api/v1/mcp/*`、`/api/v1/a2a/*`
- Provider API Key: `/api/v1/providers/*`
- Tool Registry: `/api/v1/tools*`
  - Tool Provider Config: `/api/v1/tools/providers`、`/api/v1/tools/providers/{provider}`、`/api/v1/tools/providers/reload`
- Config API: `/api/v1/config*`

## 工具共用与自动生效

- 对外工具注册入口：`/api/v1/tools*`（列表/创建/更新/删除/targets/reload）。
- 对外工具 Provider 配置入口：`/api/v1/tools/providers/*`（按 provider 持久化 `web_search` 配置并触发 runtime reload）。
- `chat` 与 `agent` 共享同一套 runtime `ToolManager`（同一注册中心，不是两套独立工具池）。
- 当 DB 中工具注册发生变更时，服务层会触发 runtime reload；成功后会重置 agent resolver 缓存，确保新工具白名单立即生效（无需重启进程）。
- 关键链路：`cmd/agentflow/server_handlers_runtime.go`（`toolRegistryRuntimeAdapter.ReloadBindings -> onReload -> resolver.ResetCache`）。

## 关键构造示例

### Chat Handler

```go
chatHandler := handlers.NewChatHandler(provider, policyManager, logger)
http.HandleFunc("/api/v1/chat/completions", chatHandler.HandleCompletion)
http.HandleFunc("/api/v1/chat/completions/stream", chatHandler.HandleStream)
```

说明：`ChatHandler` 通过 `ChatService` 统一路由参数并调用 `llm/gateway` `Invoke/Stream`，不在 handler 层拼装 provider 细节。

### Agent Handler

```go
agentHandler := handlers.NewAgentHandler(discoveryRegistry, agentRegistry, logger, resolver)
http.HandleFunc("GET /api/v1/agents", agentHandler.HandleListAgents)
http.HandleFunc("POST /api/v1/agents/execute", agentHandler.HandleExecuteAgent)
```

说明：`AgentHandler` 的执行、规划、流式调用统一走 `AgentService`。

多 Agent 执行也走同一入口，不新增旁路 handler：

```json
{
  "agent_ids": ["planner", "coder", "reviewer", "tester", "synthesizer"],
  "mode": "parallel",
  "content": "并行分析并汇总方案",
  "context": {
    "aggregation_strategy": "merge_all"
  }
}
```

- `agent_id` 与 `agent_ids` 二选一。
- `agent_ids` 最多 5 个。
- 未显式传 `mode` 时，多 Agent 请求默认走 `parallel`。
- `plan/stream` 当前仅支持单 agent，不支持 `agent_ids`。

## 统一响应与错误

- 成功响应：`WriteSuccess(w, data)`
- 错误响应：`WriteError(w, err, logger)`（`types.Error`）
- 入参解析：`DecodeJSONBody(...)`
- Content-Type 校验：`ValidateContentType(...)`

## 当前测试覆盖

已包含 `chat/agent/apikey/multimodal/rag/workflow/common/health` 等 handler 与 service 测试文件（`*_test.go`）。
