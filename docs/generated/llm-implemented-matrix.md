# LLM Provider 能力实现矩阵

> 自动生成文档，基于 `llm/capabilities/` 目录实际实现。

## 主聊天供应商能力矩阵

| Provider | 图像生成 | 视频生成 | TTS | STT | Embedding | Rerank | 音乐 | 3D | 内容审核 |
|----------|:--------:|:--------:|:---:|:---:|:---------:|:------:|:----:|:--:|:--------:|
| OpenAI   | ✓        | ✓        | ✓   | ✓   | ✓         | ✗      | ✗    | ✗  | ✓        |
| Anthropic| ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |
| Gemini   | ✓        | ✓        | ✗   | ✗   | ✓         | ✗      | ✗    | ✗  | ✗        |
| DeepSeek | ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |
| Qwen     | ✗        | ✗        | ✗   | ✗   | ✗         | ✓      | ✗    | ✗  | ✗        |
| GLM      | ✗        | ✗        | ✗   | ✗   | ✗         | ✓      | ✗    | ✗  | ✗        |
| Grok     | ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |
| Doubao   | ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |
| Kimi     | ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |
| Mistral  | ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |
| Hunyuan  | ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |
| MiniMax  | ✗        | ✓        | ✗   | ✗   | ✗         | ✗      | ✓    | ✗  | ✗        |
| Llama    | ✗        | ✗        | ✗   | ✗   | ✗         | ✗      | ✗    | ✗  | ✗        |

## 独立能力供应商

以下供应商不属于上述 13 个主聊天供应商，但提供特定能力实现：

| 能力类型 | 供应商 |
|----------|--------|
| Embedding | Cohere, Jina, Voyage |
| 图像生成 | Flux |
| 视频生成 | Luma |
| TTS | ElevenLabs |
| STT | Deepgram |
| 音乐 | Suno |
| 3D | Meshy, Tripo |
| Tools/WebSearch | Tavily, Firecrawl, Jina Reader |
