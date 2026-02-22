# 流式对话 (Streaming)

使用 OpenAI Provider 的流式接口，逐块接收 LLM 响应。

## 功能

- 创建 OpenAI Provider
- 使用 `provider.Stream()` 发起流式请求
- 逐块（chunk）打印响应内容

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`

## 运行

```bash
cd examples/02_streaming
go run main.go
```

## 代码说明

调用 `provider.Stream()` 返回 `<-chan StreamChunk`，通过 `for range` 遍历 channel 逐块输出内容。适用于需要实时展示 LLM 输出的场景。
