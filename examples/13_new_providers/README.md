# 新增提供商 (New Providers)

展示 Mistral、腾讯混元、Moonshot Kimi、Meta Llama 四个 LLM 提供商的接入。

## 功能

- **Mistral AI**：欧洲 LLM 提供商，使用 `mistral-large-latest` 模型
- **腾讯混元**：国内 LLM 提供商，使用 `hunyuan-lite` 模型
- **Moonshot Kimi**：月之暗面 LLM，使用 `moonshot-v1-8k` 模型
- **Meta Llama**：通过 Together AI 托管，使用 `Llama-3.3-70B` 模型
- 每个提供商演示健康检查 + 对话补全

## 前置条件

- Go 1.24+
- 环境变量（按需设置，未设置的提供商会跳过）：
  - `MISTRAL_API_KEY`
  - `HUNYUAN_API_KEY`
  - `KIMI_API_KEY`
  - `TOGETHER_API_KEY`（Llama）

## 运行

```bash
cd examples/13_new_providers
go run main.go
```

## 代码说明

每个提供商使用对应的 Config 和 Provider 构造函数，统一通过 `llm.Provider` 接口调用。未设置 API Key 的提供商会打印跳过信息。
