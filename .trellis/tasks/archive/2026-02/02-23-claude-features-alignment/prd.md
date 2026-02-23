# Claude Code 特性对齐：Structured Outputs + Tool Streaming + Skills 桥接

## 目标
对齐 Claude Code/Agent SDK 的三大核心特性，提升 AgentFlow 框架的能力完整度。

## 工作流 A：Structured Outputs（结构化输出）

### A1 - P0: ChatRequest + ToolChoice 类型升级
- `llm/provider.go`: `ChatRequest` 添加 `ResponseFormat` 字段
- `llm/provider.go`: `ToolChoice` 从 `string` 改为 `any`（支持复杂对象）
- 定义 `ResponseFormat` 类型：支持 `json_object` / `json_schema` 两种模式

### A2 - P0: Anthropic Provider 补 tool_choice
- `llm/providers/anthropic/provider.go`: `claudeRequest` 添加 `ToolChoice` 字段
- `Completion()` 方法传递 `req.ToolChoice` 到 Claude API
- 支持 `{"type": "any"}` / `{"type": "auto"}` / `{"type": "tool", "name": "xxx"}`

### A3 - P1: OpenAI Provider 实现 response_format
- `llm/providers/openaicompat/`: `OpenAICompatRequest` 添加 `ResponseFormat` 字段
- OpenAI provider 传递 `response_format: {type: "json_schema", json_schema: {...}}`
- 所有 OpenAI 兼容 provider 自动继承

### A4 - P1: StructuredOutputProvider 真实实现
- OpenAI provider 实现 `StructuredOutputProvider` 接口
- `StructuredOutput[T].generateNative()` 真正使用 API 级别 response_format

## 工作流 B：Fine-grained Tool Streaming（工具细粒度流式）

### B1 - P0: StreamingToolFunc 签名
- `llm/tools/executor.go`: 新增 `StreamingToolFunc` 类型
- 支持 context-based emitter 模式（与 RuntimeStreamEmitter 一致）
- `DefaultExecutor` 支持注册 `StreamingToolFunc`

### B2 - P1: ReAct 循环集成 StreamableToolExecutor
- `llm/tools/react.go`: `ExecuteStream` 检测并使用 `StreamableToolExecutor`
- 工具中间事件转发为 `ReActStreamEvent`

### B3 - P2: tool_progress 事件端到端
- `agent/runtime_stream.go`: 新增 `RuntimeStreamToolProgress` 事件类型
- `api/handlers/agent.go`: SSE 转发 `tool_progress` 事件
- 客户端可看到工具执行中间状态

## 工作流 C：Skills ↔ Discovery 桥接

### C1 - P1: Skill 自动注册为 Capability
- `agent/skills/` 与 `agent/discovery/` 之间添加桥接层
- Skill 注册时自动创建对应的 CapabilityInfo

### C2 - P1: SkillsExtension 接口适配
- `types/extensions.go` 的 `SkillsExtension` 与 `agent/skills/DefaultSkillManager` 对齐
- 添加 adapter 实现

### C3 - P2: 差异化内置 Agent 类型
- `agent/registry.go`: 为 6 种内置类型预装不同的 Skills/Prompt 组合

## 验收标准
- [ ] `ChatRequest.ResponseFormat` 可用，至少 OpenAI + Anthropic 两个 provider 支持
- [ ] `ToolChoice` 支持复杂对象形式
- [ ] `StreamingToolFunc` 可注册并在 ReAct 循环中使用
- [ ] 工具执行中间状态可通过 SSE 推送到客户端
- [ ] Skill 注册后可通过 Discovery 系统发现
- [ ] 所有变更通过 `go build ./...` 和 `go vet ./...`
