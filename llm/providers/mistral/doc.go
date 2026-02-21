// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 mistral 提供 Mistral AI 模型的 Provider 适配实现。Mistral 使用
OpenAI 兼容的 API 格式，本包通过嵌入 openaicompat.Provider 复用
HTTP 处理、SSE 解析等通用逻辑，并自行实现音频转录和 Embedding 端点。

# 核心结构体

  - MistralProvider — 嵌入 openaicompat.Provider，额外实现
    TranscribeAudio 和 CreateEmbedding 方法

# 定制行为

  - 默认 BaseURL: https://api.mistral.ai
  - 默认兜底模型: mistral-large-latest
  - 音频转录: 使用 Voxtral 模型（voxtral-mini-transcribe-2-2602），
    通过 multipart/form-data 上传至 /v1/audio/transcriptions
  - Embedding: 通过 /v1/embeddings 端点，标准 JSON 请求

# 支持能力

  - Chat Completion（同步，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 音频转录（Voxtral，/v1/audio/transcriptions）
  - Embedding（/v1/embeddings）
  - 健康检查、模型列表（委托 openaicompat）

# 不支持能力

  图像生成、视频生成、音频生成、微调
  均返回 NotSupportedError。
*/
package mistral
