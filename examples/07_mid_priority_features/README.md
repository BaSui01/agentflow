# 中优先级功能 (Medium Priority Features)

展示 Hosted Tools、Agent Handoff、Crews 协作、对话模式、双向流式、追踪系统。

## 功能

- **Hosted Tools**：OpenAI SDK 风格的托管工具注册
- **Agent Handoff**：Agent 间任务移交协议
- **Crews**：CrewAI 风格的角色化团队协作
- **Declarative Agent**：YAML/JSON 声明式 Agent 定义与工厂加载
- **Conversation**：AutoGen 风格的多 Agent 对话（轮询模式）
- **Bidirectional Streaming**：双向流式通信管理
- **Tracing**：LangSmith 风格的调用追踪和可观测性

## 前置条件

- Go 1.24+
- 无需 API Key（Hosted web_search 默认回退到 DuckDuckGo；配置 `TAVILY_API_KEY` / `FIRECRAWL_API_KEY` / `SEARXNG_BASE_URL` 时会按顺序尝试）

## 运行

```bash
cd examples/07_mid_priority_features
go run main.go
```

## 代码说明

大部分功能模块通过 Mock Agent 演示 API 用法和配置方式，不依赖真实 LLM 调用；其中 Hosted web_search 使用仓内 provider-backed 实现，按 `tavily -> firecrawl -> searxng -> duckduckgo` 顺序进行运行时回退。重点展示各子系统的初始化、注册和配置流程。
