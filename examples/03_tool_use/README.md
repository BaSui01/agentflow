# 工具调用 (Tool Use / Function Calling)

展示 AgentFlow 的核心功能：LLM 工具调用（Function Calling）完整流程。

## 功能

- 定义工具函数（天气查询、计算器）及 JSON Schema
- 注册工具到 `ToolRegistry`
- 发送带 Tools 的 ChatRequest，LLM 自动决定是否调用工具
- `ToolExecutor` 执行工具调用，结果回传 LLM 生成最终回答
- 多轮工具调用循环（最多 5 轮）

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`

## 运行

```bash
cd examples/03_tool_use
go run main.go
```

## 代码说明

核心流程：定义工具 -> 注册到 Registry -> 构建带 `Tools` 字段的请求 -> LLM 返回 `ToolCalls` -> Executor 执行 -> 结果作为 `tool` 消息回传 -> LLM 生成最终回答。这是 ReAct 模式的基础。
