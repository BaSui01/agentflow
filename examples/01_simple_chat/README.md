# 简单对话 (Simple Chat)

使用 OpenAI Provider 发起一次基本的 LLM 对话请求，展示最简单的 AgentFlow 使用方式。

## 功能

- 创建 OpenAI Provider 并配置 API Key
- 发送单轮对话请求（ChatCompletion）
- 打印响应内容和 Token 使用量

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`

## 运行

```bash
cd examples/01_simple_chat
go run main.go
```

## 代码说明

通过 `providers.OpenAIConfig` 配置 Provider，使用 `provider.Completion()` 发送请求。演示了 AgentFlow LLM 抽象层的基本用法：构建 `ChatRequest`、调用 Provider、读取 `ChatResponse`。
