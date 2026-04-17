# AgentFlow 快速开始

本指南帮助你在 5 分钟内运行第一个 Agent。

## 前提条件

- Go 1.22+
- （可选）Docker - 如果你需要使用代码沙箱功能

## 安装

```bash
go get github.com/BaSui01/agentflow
```

## 最小可用示例

创建一个 `main.go`：

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/BaSui01/agentflow/agent"
    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/types"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()

    // 1. 创建 LLM Provider（示例使用内置兼容层）
    provider, err := llm.NewOpenAICompatProvider(llm.OpenAICompatConfig{
        BaseURL: "https://api.openai.com/v1",
        APIKey:  os.Getenv("OPENAI_API_KEY"),
    })
    if err != nil {
        log.Fatal(err)
    }

    // 2. 构建 Agent
    ag, err := agent.NewAgentBuilder(types.AgentConfig{
        Core: types.CoreConfig{
            ID:   "my-first-agent",
            Name: "My First Agent",
        },
        LLM: types.LLMConfig{
            Model: "gpt-4o-mini",
        },
    }).
        WithProvider(provider).
        WithLogger(logger).
        Build()
    if err != nil {
        log.Fatal(err)
    }

    // 3. 初始化并执行
    if err := ag.Init(context.Background()); err != nil {
        log.Fatal(err)
    }

    output, err := ag.Execute(context.Background(), &agent.Input{
        Content: "你好，请用一句话介绍自己",
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Agent:", output.Content)
}
```

运行：

```bash
export OPENAI_API_KEY=sk-xxx
go run main.go
```

## 配置并发

默认情况下，单个 Agent 实例的并发执行数为 1（互斥）。你可以通过 `WithMaxConcurrency` 提高并发上限：

```go
ag, err := agent.NewAgentBuilder(cfg).
    WithProvider(provider).
    WithMaxConcurrency(10). // 允许最多 10 个并发请求
    Build()
```

> 注意：`maxConcurrency` 受限于底层 Provider 的速率限制和机器资源。

## 下一步

- [沙箱环境配置](./sandbox_setup.md) - 启用代码执行能力
- `examples/` 目录 - 查看完整示例（流式输出、工具调用、RAG、多智能体编排）
- `api/openapi.yaml` - HTTP API 参考
