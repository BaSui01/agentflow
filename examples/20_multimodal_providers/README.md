# 多模态提供商 (Multimodal Providers)

展示向量嵌入、重排序、增强 RAG 检索、多模态路由器的使用。

## 功能

- **Embedding**：OpenAI、Voyage AI、Jina AI 三种嵌入提供商
- **Rerank**：Cohere、Jina 两种重排序提供商
- **增强 RAG**：Cohere 嵌入 + 重排序的混合检索
- **多模态路由器**：统一管理 Embedding/TTS/STT/Image/Rerank 提供商

## 前置条件

- Go 1.24+
- 环境变量（按需设置，未设置的提供商会跳过）：
  - `OPENAI_API_KEY` — OpenAI Embedding/TTS/STT/Image
  - `VOYAGE_API_KEY` — Voyage AI Embedding
  - `JINA_API_KEY` — Jina Embedding/Rerank
  - `COHERE_API_KEY` — Cohere Embedding/Rerank
  - `ELEVENLABS_API_KEY` — ElevenLabs TTS

## 运行

```bash
cd examples/20_multimodal_providers
go run main.go
```

## 代码说明

各提供商通过 `embedding.New*Provider`、`rerank.New*Provider` 创建。`multimodal.NewRouter` 统一注册和路由多模态能力，支持默认提供商和按名称选择。
