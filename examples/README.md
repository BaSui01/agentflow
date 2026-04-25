# AgentFlow 示例代码

本目录包含 AgentFlow 的示例程序，用于演示各项功能用法。

## 官方入口示例

| 示例 | 说明 |
|------|------|
| [`22_sdk_official_surface`](./22_sdk_official_surface) | 使用 `sdk.New(opts).Build(ctx)` 装配 Agent、ToolManager、RetrievalProvider、Team 和 Workflow；无需外部 API Key。 |

## 日志说明

示例代码为保持简洁，统一使用标准库 `log` 进行输出。**生产环境代码应使用 `go.uber.org/zap` 等结构化日志库**，以获得更好的可观测性与日志级别控制。
