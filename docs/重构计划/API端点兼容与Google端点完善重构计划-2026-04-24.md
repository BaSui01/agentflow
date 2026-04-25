# API 端点兼容与 Google 端点完善重构计划（2026-04-24）

> 文档类型：可执行重构计划
> 创建日期：2026-04-24
> 执行方式：单轨替换；不保留新旧双实现；所有兼容入口必须收口到同一 usecase/gateway 主链
> 用户指定重点端点：`/v1/chat/completions`、`/v1/responses`、`/v1/messages`（用户原写 `v1/meassage`，按 Anthropic 官方拼写修正为 `messages`）
> 第四类协议入口：Google/Gemini provider 适配入口；不新增项目级 `/v1/google/*` HTTP 路由。Google 重点端点：Google Gemini Developer API `POST /v1beta/models/{model}:generateContent`、`POST /v1beta/models/{model}:streamGenerateContent`；如启用 Vertex AI，补齐 `POST /v1/projects/{project}/locations/{location}/publishers/google/models/{model}:generateContent` 等映射
> 官方来源核对时间：截至 2026-04-24
> 参考来源：
> - OpenAI Responses：`https://developers.openai.com/api/reference/resources/responses/methods/create`
> - OpenAI Chat Completions：`https://developers.openai.com/api/reference/resources/chat`
> - Anthropic Claude Messages：`https://docs.anthropic.com/en/api/messages` / `https://docs.anthropic.com/en/api/messages-examples`
> - Google Gemini generateContent：`https://ai.google.dev/api/generate-content`
> - Cherry Studio 参考代码：`https://github.com/CherryHQ/cherry-studio/tree/main/src/renderer/src`（核对时间 2026-04-24，参考提交 `19f2ed6`；仅参考端点分类、工具流式状态机和 provider endpoint routing 思路，不复制实现）

---

## 0. 执行状态总览

- [x] 完成端点名称澄清：`v1/meassage` 修正为 `/v1/messages`
- [x] 完成当前路由现状扫描：当前已提供 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`
- [x] 完成端点职责矩阵（2026-04-24 审核：已补 Agent 框架完整可用矩阵）
- [x] 完成 Cherry Studio 参考审计：其前端显式区分 `chat/completions`、`responses`、`messages`、`generateContent`、`streamGenerateContent`，并用 endpoint type 驱动 provider routing
- [x] 完成失败测试设计
- [x] 完成 API route / handler / converter / usecase 收口实现
- [x] 完成 Google/Gemini 端点映射文档与验证
- [x] 完成文档、示例、OpenAPI/README 同步
- [x] 完成全仓回归与 gate 验收

当前进度（由 `scripts/refactor_plan_guard.py report` 统计）：**111 done / 0 todo / 111 total**

未完成项清单：

- 无

---

## 1. 背景与问题

### 1.1 当前现状（已完成）

- [x] `api/routes/routes.go` 当前注册：
  - `POST /api/v1/chat/completions`
  - `POST /api/v1/chat/completions/stream`
  - `POST /v1/chat/completions`
  - `POST /v1/responses`
- [x] `api/routes/routes.go` 已补齐 `POST /v1/messages`
- [x] `api/handlers/chat_openai_compat.go` 已有 OpenAI-compatible Chat Completions 与 Responses 适配实现
- [x] `api/handlers/chat_anthropic_compat.go` 已补齐 Anthropic-compatible Messages 入站适配实现
- [x] Google Gemini provider 已在 `llm/providers/gemini` 和 `llm/internal/googlegenai` 内处理 Gemini 官方 SDK / generateContent 语义

### 1.2 已解决问题

- [x] 问题 1：OpenAI-compatible 的两个入口 `/v1/responses` 与 `/v1/chat/completions` 已存在，但仍需补齐 stream/tools/usage/error 的完整 Agent 框架回归
- [x] 问题 2：Anthropic-compatible `/v1/messages` 尚无 API route / handler / converter / response 适配闭环
- [x] 问题 3：端点命名和官方端点之间缺少统一文档，容易把 `/v1/messages` 写成 `/v1/meassage`
- [x] 问题 4：Google Gemini Developer API 与 Vertex AI Google 端点映射散落在 provider/test/doc 中，缺少统一对照表与守卫
- [x] 问题 5：兼容端点错误格式、streaming SSE 事件、tool calling payload 与主 `ChatService` 链路之间仍需统一验收
- [x] 问题 6：当前计划仍需明确“HTTP 入站路由”和“provider 出站协议端点”的边界；Google/Gemini 是 provider 出站协议适配，不是新增本项目 `/v1/google/*` 入站路由

---

## 2. 端点职责矩阵

| 端点 | 类型 | 是否官方外部协议 | 当前状态 | 目标职责 | 归口 |
|---|---|---:|---|---|---|
| `POST /v1/responses` | OpenAI Responses-compatible | 是 | 已实现并完成回归 | 保持；补齐错误格式、stream、tool/function、reasoning、usage 映射验收 | `api/routes -> ChatHandler -> ChatService -> llm/gateway` |
| `POST /v1/chat/completions` | OpenAI Chat Completions-compatible | 是 | 已实现并完成回归 | 保持；作为 OpenAI chat 官方兼容入口 | `api/routes -> ChatHandler -> ChatService -> llm/gateway` |
| `POST /v1/messages` | Anthropic Claude Messages-compatible | 是 | 已实现 route/handler/converter/test | 新增 Anthropic-compatible 入站；请求/响应/SSE 尽量保持 Claude Messages API 形状 | `api/routes -> ChatHandler -> ChatService -> llm/gateway` |
| `POST /v1beta/models/{model}:generateContent` | Google Gemini Developer API | 是 | provider 内已有并完成守卫/文档/测试 | 文档化并补端点守卫；确保 SDK 类型不泄漏上层 | `llm/providers/gemini` / `llm/internal/googlegenai` |
| `POST /v1beta/models/{model}:streamGenerateContent` | Google Gemini streaming | 是 | provider 内已有并完成守卫/文档/测试 | 补齐文档、测试、stream 映射说明 | `llm/providers/gemini` / capabilities |
| `POST /v1/projects/{project}/locations/{location}/publishers/google/models/{model}:generateContent` | Vertex AI Google | 是 | vendor/profile 已有测试并完成统一说明 | 统一文档化与配置字段说明 | `llm/providers/vendor` / `llm/runtime/router` |

边界结论：前三项是本项目 HTTP 入站 route；Google/Gemini 三项是 provider 出站协议 endpoint，由 `llm/providers/gemini` / `llm/internal/googlegenai` / `llm/providers/vendor` 负责，不在 `api/routes` 新增 `/v1/google/*`、`/v1beta/models/*` 或 Vertex AI 入站代理路由。

### 2.0.1 Cherry Studio 参考审计结论

- [x] Cherry Studio `src/renderer/src/utils/api.ts` 将 `chat/completions`、`responses`、`messages`、`generateContent`、`streamGenerateContent` 放入同一 supported endpoint list，说明这些是客户端可选择的协议端点族，不应混成一个 alias。
- [x] Cherry Studio `routeToEndpoint` 使用 `baseURL + endpoint` 拆分，支持用户把 provider host 指向不同协议端点；本项目服务端应对应提供稳定入站 route（OpenAI/Anthropic）和稳定 provider 出站 endpoint（Gemini/Vertex），不新增伪 alias。
- [x] Cherry Studio `aiCore/utils/options.ts` 使用 `model.endpoint_type` 决定 OpenAI / Anthropic / Gemini / OpenAI Responses 协议分支；本项目应把等价选择收口到 `llm/runtime/router` vendor profile 和 provider factory，禁止 handler 根据模型名分叉 provider 细节。
- [x] Cherry Studio `aiCore/chunk/handleToolCallChunk.ts` 明确维护 streaming tool call 的 `toolCallId`、`toolName`、`args`、`streamingArgs` 状态；本项目端点验收必须覆盖 tool delta 累积、tool result 回灌、多轮 tool loop。
- [x] Cherry Studio `aiCore/utils/mcp.ts` 把 MCP/provider/builtin tools 统一成 AI SDK tool set，并接入确认/自动批准；本项目对应能力必须走 `AuthorizationService`、tool registry、agent/runtime，不允许在兼容 handler 内直接执行工具。

### 2.0.2 路由实现范围修正

- [x] 必须实现/验证的本项目 HTTP 入站 route：`POST /v1/chat/completions`、`POST /v1/responses`、`POST /v1/messages`。
- [x] 必须实现/验证的 provider 出站协议 endpoint：Gemini Developer API `generateContent`、`streamGenerateContent`，Vertex AI Google path，function declarations / functionCall / functionResponse 映射。
- [x] 不新增 `/v1/chat/compatible`，不新增 `/v1/meassage`，不新增项目级 `/v1/google/*` 入站路由。
- [x] API handler 只做协议 DTO 转换、SSE 写出和错误格式适配；provider endpoint routing 必须保留在 `llm/providers/*`、`llm/runtime/router`、vendor profile 层。

### 2.1 Agent 框架完整可用能力矩阵（审核补充）

| 能力 | `/v1/responses` | `/v1/chat/completions` | `/v1/messages` | Google Gemini / Vertex | 项目统一归口 |
|---|---|---|---|---|---|
| 普通文本输入 | 必须支持 `input` / messages-like input | 必须支持 `messages[]` | 必须支持 `messages[]` + `system` | 必须映射到 `contents[]` | `api.ChatRequest` / `usecase.ChatRequest` |
| 多模态输入 | 支持 image/audio/video typed content 的兼容子集 | 支持 image content 兼容子集 | 支持 Anthropic content blocks 兼容子集 | Gemini 原生 `parts[]` | `types.Message` + provider adapter |
| 非流式输出 | `response` object | `chat.completion` object | `message` object | `GenerateContentResponse` -> 统一输出 | `ChatService.Invoke` |
| 流式输出 | SSE Responses events | SSE chat completion chunks | Anthropic SSE events | `streamGenerateContent` SSE/chunks | `ChatService.Stream` |
| 流式输入 | HTTP request body 一次性输入；不在本计划实现 realtime/WebSocket 双向流 | 同左 | 同左 | 同左 | 同左 | 后续若做 Realtime，另开计划 |
| 工具声明 | `tools[]` / function tools | `tools[]` | `tools[]` Anthropic schema | Gemini function declarations | `types.ToolSchema` |
| 工具选择 | `tool_choice` | `tool_choice` | `tool_choice` | Gemini function calling mode | `types.ToolChoice` |
| 工具调用输出 | function/tool call output item | assistant tool calls | `tool_use` content block | `functionCall` part | `types.ToolCall` |
| 工具结果回传 | tool result / previous response items | `role=tool` message | `tool_result` content block | `functionResponse` part | `types.Message` + runtime loop |
| 多轮 tool loop | 必须可 round-trip，不在 handler 执行工具 | 同左 | 同左 | 同左 | 同左 | `agent/runtime` / tool executor |
| 权限/HITL | 工具执行前必须走授权主链 | 同左 | 同左 | 同左 | 同左 | `AuthorizationService` |
| usage / cost | prompt/output/reasoning/tool details 尽量映射 | prompt/completion tokens | input/output tokens | Gemini usage metadata | `types.TokenUsage` |
| 错误格式 | OpenAI-compatible error | OpenAI-compatible error | Anthropic-compatible error | provider error -> 统一错误 | `types.Error` + protocol adapter |
| 取消/超时 | request context 生效 | 同左 | 同左 | 同左 | 同左 | context propagation |

### 2.2 关键审核结论

- [x] `/v1/messages` 可以有独立协议 adapter，但不得直接执行工具，也不得绕过 `ChatService` / `agent/runtime`。
- [x] 完整 Agent 框架可用的核心不是“端点能返回文本”，而是 request/stream/tool_call/tool_result/usage/error/context 都能 round-trip。
- [x] “流输入”本计划仅覆盖 HTTP body 输入与流式输出；真正双向实时流输入（音频/Realtime/WebSocket）不混入本计划，后续单独规划。
- [x] Google Gemini 端点完善重点是 provider adapter 的 `contents/parts/functionCall/functionResponse` 与 endpoint path，不在 API handler 层暴露 Google SDK 类型。

---

## 3. 非目标

- [x] 不新增新的 LLM provider 架构分支
- [x] 不绕过 `ChatService` / `llm/gateway` 直接在 handler 内调用 provider
- [x] 不把 Google SDK 类型暴露到 `api/`、`agent/`、`workflow/`、`cmd/`
- [x] 不兼容错误拼写 `/v1/meassage`，避免保留错误入口；只在文档说明应使用 `/v1/messages`

---

## 4. 测试策略（TDD）

- [x] 先写失败测试并确认红灯（验证命令：`go test ./api/routes ./api/handlers -run "TestCompatibilityEndpointRoutes|TestChatHandler_AnthropicCompatMessages" -count=1`；通过标准：兼容路由与 Anthropic 入口在实现前后均有明确失败/通过断言，红绿过程可追踪）

- [x] 采用最小实现让测试转绿（验证命令：`go test ./api/handlers -run "TestChatHandler_AnthropicCompatMessages|TestAnthropicCompatMessagesToolCallRoundTrip|TestAnthropicCompatMessagesToolResultRoundTrip" -count=1`；通过标准：`/v1/messages` 的 DTO、转换、响应与错误适配测试全部转绿）

- [x] 完成重构并回归验证（验证命令：`go test ./api/routes ./api/handlers ./llm/providers/gemini ./llm/providers/vendor ./llm/runtime/router -count=1`、`powershell.exe -ExecutionPolicy Bypass -File scripts/arch_guard.ps1`、`go test ./... -count=1`；通过标准：定向测试、架构守卫与全仓测试全部退出码 0）

### 4.1 先写失败测试

- [x] 新增 route guard：`TestCompatibilityEndpointRoutes` 先验证以下路由存在：
  - `POST /v1/responses`
  - `POST /v1/chat/completions`
  - `POST /v1/messages`
- [x] 新增 Anthropic Messages 入站测试：最小 body `{model,max_tokens,messages}` 可被转换为 `api.ChatRequest` / `usecase.ChatRequest`
- [x] 新增 Anthropic Messages response 测试：输出形状包含 `id`、`type: message`、`role: assistant`、`content[]`、`model`、`stop_reason`、`usage`
- [x] 新增 Anthropic Messages streaming 测试：`stream=true` 时 SSE 事件至少覆盖 `message_start`、`content_block_delta`、`message_delta`、`message_stop`
- [x] 新增 Gemini endpoint guard：Developer API 与 Vertex AI Google endpoint path 由 provider/profile 构造，不在 handler/cmd 中硬编码

### 4.2 最小实现转绿

- [x] 为 `/v1/messages` 增加独立 handler 方法，例如 `HandleAnthropicCompatMessages`
- [x] 新增 Anthropic-compatible request/response DTO 与 converter，放在 `api/handlers` 协议适配层；核心业务仍走 `ChatService`
- [x] 将 Anthropic 的 `system`、`messages[]`、`max_tokens`、`temperature`、`top_p`、`tools`、`tool_choice`、`stream` 映射到现有 `api.ChatRequest` / `types.ChatRequest`
- [x] 将模型输出、tool calls、usage、stop reason 映射回 Anthropic Messages response
- [x] 对 Google/Gemini 只补端点映射文档和守卫；除非测试发现 provider path 不一致，否则不改 provider 主实现

### 4.3 回归验证

- [x] 运行 `go test ./api/routes ./api/handlers -count=1`
- [x] 运行 `go test ./llm/providers/gemini ./llm/providers/vendor ./llm/runtime/router -count=1`
- [x] 运行 `go test . -run "Test.*Endpoint|Test.*Compat|Test.*Gemini|Test.*Google" -count=1`
- [x] 运行 `powershell.exe -ExecutionPolicy Bypass -File scripts/arch_guard.ps1`
- [x] 运行 `go test ./... -count=1`

---

## 5. 执行计划

- [x] 按 Phase-0 ~ Phase-6 完成端点、测试、文档与验收收口（验证命令：见 Phase-6 汇总命令；通过标准：Phase-0 ~ Phase-6 所有条目均已完成并通过验收）

### Phase-0：端点事实冻结

- [x] 记录当前 API 路由快照（验证命令：`rg -n "v1/(responses|chat|messages)" api/routes api/handlers -g "*.go"`；通过标准：能看出已有与缺失端点）
- [x] 记录官方端点来源（验证命令：人工核对官方文档链接；通过标准：计划文档列出 OpenAI / Anthropic / Google 官方端点）
- [x] 明确错误拼写策略（验证命令：`rg -n "meassage" .`；通过标准：代码和活跃文档 0 命中，仅本计划说明拼写修正）

### Phase-1：路由与守卫

- [x] 在 `api/routes/routes.go` 增加 `/v1/messages` 路由（验证命令：`go test ./api/routes -count=1`；通过标准：route 测试通过）
- [x] 增加架构守卫，禁止 handler 直接 import `llm/providers/*` 或硬编码 Google SDK 类型（验证命令：`go test . -run "Test.*Endpoint.*Guard|Test.*Handler.*Provider" -count=1`；通过标准：守卫通过）
- [x] 更新 `api/handlers/README.md` 的端点清单（验证命令：`rg -n "/v1/responses|/v1/chat/completions|/v1/messages" api/handlers/README.md`；通过标准：三个 HTTP 兼容端点均被记录）

### Phase-2：OpenAI-compatible 双入口验收

- [x] 固定 `/v1/chat/completions` 入站回归（验证命令：`go test ./api/handlers -run "Test.*OpenAICompatChat" -count=1`；通过标准：Chat Completions 请求、响应、stream、tools 映射稳定）
- [x] 固定 `/v1/responses` 入站回归（验证命令：`go test ./api/handlers -run "Test.*OpenAICompatResponses" -count=1`；通过标准：Responses 请求、响应、stream、tools 映射稳定）

### Phase-3：Anthropic-compatible `/v1/messages`

- [x] 新增 Anthropic Messages request DTO（验证命令：`go test ./api/handlers -run "TestAnthropicCompatMessagesRequest" -count=1`；通过标准：`model/max_tokens/messages/system/tools/tool_choice/stream` 可解析）
- [x] 新增 Anthropic Messages request -> `api.ChatRequest` converter（验证命令：`go test ./api/handlers -run "TestAnthropicCompatMessagesConvert" -count=1`；通过标准：role/content/system/tool 参数映射正确）
- [x] 新增 non-stream response adapter（验证命令：`go test ./api/handlers -run "TestAnthropicCompatMessagesResponse" -count=1`；通过标准：响应形状符合 `type=message`、`role=assistant`）
- [x] 新增 streaming SSE adapter（验证命令：`go test ./api/handlers -run "TestAnthropicCompatMessagesStream" -count=1`；通过标准：SSE event 名称和 delta 内容可被 Anthropic 客户端消费）
- [x] 增加错误格式映射（验证命令：`go test ./api/handlers -run "TestAnthropicCompatMessagesError" -count=1`；通过标准：错误响应为 Anthropic-compatible JSON 形状）


### Phase-3A：Agent 工具与流式闭环验收

- [x] 固定跨协议工具声明映射测试（验证命令：`go test ./api/handlers -run "Test.*Tool.*Mapping|Test.*Compat.*Tools" -count=1`；通过标准：OpenAI Chat Completions、OpenAI Responses、Anthropic Messages、Google/Gemini 兼容请求均能映射到 `types.ToolSchema`）
- [x] 固定跨协议工具选择映射测试（验证命令：`go test ./api/handlers -run "Test.*ToolChoice" -count=1`；通过标准：`auto/none/required/specific tool` 可映射到统一 `types.ToolChoice`）
- [x] 固定工具调用输出 round-trip 测试（验证命令：`go test ./agent/runtime ./api/handlers -run "Test.*ToolCall.*RoundTrip|Test.*ToolResult" -count=1`；通过标准：模型 tool call -> runtime tool execution -> tool result message -> final answer 链路可用）
- [x] 固定流式工具调用测试（验证命令：`go test ./api/handlers -run "Test.*Stream.*Tool" -count=1`；通过标准：流式 delta 中的 tool call / tool_use 能被增量聚合且结束事件完整）
- [x] 固定授权/HITL 接入测试（验证命令：`go test ./internal/usecase ./agent/runtime -run "Test.*Authorization.*Tool|Test.*Approval.*Tool" -count=1`；通过标准：所有协议入口触发工具执行前都能走 `AuthorizationService`）
- [x] 固定 usage/error/cancel 映射测试（验证命令：`go test ./api/handlers ./llm/gateway -run "Test.*Usage|Test.*Error|Test.*Cancel" -count=1`；通过标准：token usage、协议错误格式、context cancel 均有断言）
### Phase-4：Google / Gemini 端点完善

- [x] 建立 Google endpoint 对照表（验证命令：`rg -n "generateContent|streamGenerateContent|Vertex AI|publishers/google" docs/cn/guides docs/architecture`；通过标准：Developer API 与 Vertex AI 路径均有说明）
- [x] 增加 Gemini provider endpoint tests 覆盖 `generateContent` 与 `streamGenerateContent`（验证命令：`go test ./llm/providers/gemini -run "Test.*Endpoint|Test.*GenerateContent|Test.*Stream" -count=1`；通过标准：路径构造稳定）
- [x] 增加 vendor Google/Vertex profile endpoint tests（验证命令：`go test ./llm/providers/vendor ./llm/runtime/router -run "Test.*Google|Test.*Vertex" -count=1`；通过标准：BaseURL、project、location、model path 映射稳定）
- [x] 文档明确 Google SDK 边界只在 `llm/internal/googlegenai` / provider 层（验证命令：`rg -n "google.golang.org/genai" api agent workflow cmd internal/app -g "*.go"`；通过标准：0 命中）

### Phase-5：文档与示例同步

- [x] 更新 `docs/cn/api/README.md` 的兼容端点章节（验证命令：`rg -n "/v1/responses|/v1/chat/completions|/v1/messages" docs/cn/api/README.md`；通过标准：三个 HTTP 兼容端点均记录）
- [x] 更新 `docs/en/README.md` / `docs/cn/README.md` API 摘要（验证命令：`rg -n "/v1/messages" docs/en/README.md docs/cn/README.md`；通过标准：新端点被索引）
- [x] 新增或更新 example，展示 OpenAI Responses、OpenAI Chat Completions、Anthropic Messages、Gemini generateContent 四条调用（验证命令：`go test ./examples/... -count=1`；通过标准：示例可编译）
- [x] 更新 OpenAPI / generated docs（如项目有生成脚本）（验证命令：`rg -n "/v1/messages" docs/generated api`；通过标准：生成文档或替代说明存在）

### Phase-6：最终验收

- [x] API 定向测试通过（验证命令：`go test ./api/routes ./api/handlers -count=1`；通过标准：退出码 0）
- [x] LLM endpoint 定向测试通过（验证命令：`go test ./llm/providers/gemini ./llm/providers/vendor ./llm/runtime/router -count=1`；通过标准：退出码 0）
- [x] 架构守卫通过（验证命令：`powershell.exe -ExecutionPolicy Bypass -File scripts/arch_guard.ps1`；通过标准：退出码 0）
- [x] 全仓测试通过（验证命令：`go test ./... -count=1`；通过标准：退出码 0）
- [x] 计划 gate 通过（验证命令：`python scripts/refactor_plan_guard.py gate --target "API端点兼容与Google端点完善重构计划-2026-04-24.md" --require-tdd --require-verifiable-completion`；通过标准：全部 `[x]`，退出码 0）

---

## 6. 删除 / 禁止清单

- [x] 禁止新增错误拼写路径 `/v1/meassage`（验证命令：`rg -n "meassage" api docs README.md README_EN.md -g "*.go" -g "*.md"`；通过标准：除本计划说明外 0 命中）
- [x] 禁止在 handler 层直接调用 Google SDK 或 provider SDK（验证命令：`rg -n "google.golang.org/genai|openai-go|anthropic-sdk-go" api cmd internal/app agent workflow -g "*.go"`；通过标准：0 命中或仅测试允许）

---

## 7. 完成定义（DoD）

- [x] 三个 HTTP 兼容入站 `/v1/responses`、`/v1/chat/completions`、`/v1/messages` 保持可用且有回归测试（验证命令：`go test ./api/routes ./api/handlers ./internal/app/bootstrap -count=1`；通过标准：兼容路由、handler、bootstrap 注册测试全部退出码 0）
- [x] Anthropic-compatible 错误响应、usage、stop reason、tool calls、non-stream 与 stream 基础形状完成最小回归覆盖（验证命令：`go test ./api/handlers -run "TestChatHandler_AnthropicCompatMessages|TestAnthropicCompatMessagesToolCallRoundTrip|TestAnthropicCompatMessagesToolResultRoundTrip" -count=1`；通过标准：`/v1/messages` 的请求转换、响应形状、流式 SSE、工具映射与错误格式全部通过）
- [x] Google Gemini Developer API 与 Vertex AI Google endpoint 文档/测试覆盖完成，且 HTTP 入站层不泄漏 provider/SDK 细节（验证命令：`go test ./llm/providers/gemini ./llm/providers/vendor ./llm/runtime/router -count=1`、`go test . -run "TestGoogleGeminiEndpointPathsStayOutOfHTTPEntryLayers" -count=1`；通过标准：Gemini/Vertex 路径构造测试、provider contract 测试和 HTTP 入口守卫全部通过）
- [x] Agent 框架工具闭环与流式输出闭环完成：tools/tool_choice/tool_call/tool_result、OpenAI/Anthropic/Gemini stream 映射均有测试覆盖（验证命令：`go test ./api/handlers -run "Test.*Tool.*Mapping|Test.*Compat.*Tools|Test.*ToolChoice|Test.*Stream.*Tool" -count=1`、`go test ./agent/runtime ./api/handlers -run "Test.*ToolCall.*RoundTrip|Test.*ToolResult" -count=1`、`go test ./api/handlers ./llm/gateway -run "Test.*Usage|Test.*Error|Test.*Cancel" -count=1`；通过标准：工具声明、工具选择、tool call/tool result、usage/error/cancel、stream tool delta 相关测试全部退出码 0）
- [x] 文档、README、示例与架构守卫已同步，且全仓测试通过（验证命令：`go test ./examples/... -count=1`、`powershell.exe -ExecutionPolicy Bypass -File scripts/arch_guard.ps1`、`go test ./... -count=1`；通过标准：示例可编译、架构守卫通过、全仓测试退出码 0）
- [x] 本计划 `report/gate` 通过（验证命令：`python scripts/refactor_plan_guard.py gate --target "API端点兼容与Google端点完善重构计划-2026-04-24.md" --require-tdd --require-verifiable-completion`；通过标准：全部 `[x]`，退出码 0）

---

## 8. 风险与阻塞

- [x] Anthropic Messages streaming SSE 事件形状与现有 ChatService stream event 可能存在语义差异；下一步需先用测试固定最小事件集合
- [x] Anthropic tool use content block 与 OpenAI tool call 结构不同；需要最小映射后再逐步扩展，不允许在 handler 中分叉业务逻辑
- [x] Google Developer API 与 Vertex AI endpoint 使用不同 BaseURL / path / auth；需文档明确两条路径，不混写
- [x] 当前工作区仍存在非本计划文档差异（如多模态能力端点参考、generated matrix 删除）；执行本计划前需先确认是否纳入本轮或另行处理

---

## 9. 收尾建议（可选）

1. 若后续继续增强 Anthropic 兼容层，可补更多 content block 形状与 stop_sequence 细节，但不影响本轮完成态。
2. 若后续对外公开兼容端点能力，可再补一份独立的调用示例文档，把 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、Gemini `generateContent` 并列展示。
3. 若后续新增更多 Google / Vertex 变体路径，继续沿用 provider 层测试守卫，不要回流到 `api/routes` 或 `api/handlers`。
4. 若本轮无需继续扩展，可把该计划视为已完成基线，后续仅在新需求出现时另开计划。


---

## 10. 审核记录（2026-04-24：Agent 框架完整可用性）

- [x] 审核结论：原计划覆盖了端点新增，但对“Agent 框架完整可用”的工具调用、流式输出、多轮 tool loop、权限/HITL、usage/error/cancel 维度不够显式。
- [x] 已补充 `2.1 Agent 框架完整可用能力矩阵`，把文本、多模态、流式、工具、权限、usage、错误、取消等能力逐端点对齐。
- [x] 已补充 `Phase-3A：Agent 工具与流式闭环验收`，要求在完善 `/v1/chat/completions`、`/v1/responses`、`/v1/messages`、Google/Gemini 四个入站前后都验证工具声明、工具选择、工具调用、工具结果、streaming tool delta、AuthorizationService。
- [x] 已补充 DoD：端点可用不等于框架可用，必须证明 handler -> usecase -> agent/runtime/tool executor -> provider adapter 的完整闭环。
- [x] 参考官方行为：OpenAI Responses / Chat 支持工具与 streaming；Anthropic Messages streaming 使用 `message_start/content_block_delta/message_delta/message_stop` 等事件；Gemini function calling 使用 function declarations / functionCall / functionResponse，并通过 generateContent / streamGenerateContent 承载。

## 11. 用户口径修正记录（2026-04-24）

- [x] 用户明确取消 `/v1/chat/compatible`：不实现、不规划、不作为 alias。
- [x] 最终入站范围改为四类：`/v1/chat/completions`、`/v1/responses`、`/v1/messages`、Google/Gemini `generateContent` / `streamGenerateContent` / function calling / Vertex AI Google path。
- [x] `/v1/messages` 继续按 Anthropic Claude Messages API 正确拼写；不兼容错误拼写 `/v1/meassage`。

## 12. Cherry Studio 参考审核记录（2026-04-24）

- [x] 已参考 `CherryHQ/cherry-studio` 的 `src/renderer/src` 当前实现（提交 `19f2ed6`），确认用户要求的四类协议端点与主流客户端实际使用方式一致。
- [x] 已把参考结论转化为本项目计划约束：OpenAI Chat、OpenAI Responses、Anthropic Messages 是项目 HTTP 入站；Google Gemini / Vertex AI 是 provider 出站协议 endpoint。
- [x] 已补充路由功能要求：新增 `/v1/messages` route/handler/converter，不新增 `/v1/chat/compatible`、`/v1/meassage` 或项目级 `/v1/google/*`。
- [x] 已补充工具与流式要求：streaming tool delta 需要可累积，tool result 需要能回灌到多轮上下文，工具执行与审批必须走 `agent/runtime` / `AuthorizationService` 主链。
