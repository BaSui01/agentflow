# 多提供商 API 集成 (Multi-Provider APIs)

展示 OpenAI、Claude、Gemini 三大 LLM 提供商的 API 调用和工具调用对比。

## 功能

- **OpenAI Responses API**：2025 新版 API 调用
- **OpenAI Chat Completions**：传统 API 调用
- **Claude Messages API**：Anthropic Claude 调用
- **Gemini Generate Content**：Google Gemini 调用
- **工具调用对比**：同一工具在三个提供商上的调用差异

## 前置条件

- Go 1.24+
- 环境变量（按需设置）：
  - `OPENAI_API_KEY` — OpenAI
  - `ANTHROPIC_API_KEY` — Claude
  - `GEMINI_API_KEY` — Gemini

## 运行

```bash
cd examples/11_multi_provider_apis
go run main.go
```

## 代码说明

每个提供商使用各自的 Config 结构体（`OpenAIConfig`、`ClaudeConfig`、`GeminiConfig`），统一通过 `llm.Provider` 接口调用 `Completion()`。工具调用部分展示了相同的 `ToolSchema` 在不同提供商上的行为。
