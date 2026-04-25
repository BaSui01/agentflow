# SDK 官方入口与工具示例

本示例演示当前推荐的 SDK 产品化接入面：

- `sdk.New(opts).Build(ctx)` 统一装配 runtime。
- `sdk.AgentOptions.ToolManager` 注入工具注册与执行表面。
- `sdk.AgentOptions.RetrievalProvider` 注入检索上下文。
- `sdk.LLMOptions.ToolProvider` 演示工具调用链路可使用独立 provider。
- `agent/team` 构建 supervisor / selector Team。
- `workflow/runtime` 构建 DAG，并在 action node 中调用 Agent。

示例使用本地 scripted provider，不需要外部 API Key。

## 运行

```bash
go run ./examples/22_sdk_official_surface
```

## 入口边界

推荐路径是 `sdk -> agent/runtime -> agent/team -> workflow/runtime`。示例不导入 `internal/app/bootstrap`，也不导入 Team 内部 engine 包。
