// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 gemini 提供 Google Gemini 模型的 Provider 适配实现。该包直接对接
Gemini REST API（generativelanguage.googleapis.com），自行处理请求构建、
响应解析、流式输出与多模态能力，不依赖 openaicompat 兼容层。

# 核心结构体

  - GeminiProvider — 独立实现，持有 http.Client、GeminiConfig 与
    RewriterChain；使用 x-goog-api-key 请求头认证
  - geminiRequest / geminiResponse — Gemini 原生请求/响应结构
  - geminiContent / geminiPart — 多模态内容与分片（文本、图片、函数调用）

# 构造函数

  - NewGeminiProvider(cfg, logger) — 创建实例，默认模型 gemini-3-pro

# 支持能力

  - Chat Completions（/v1beta/models/{model}:generateContent）
  - 流式输出（/v1beta/models/{model}:streamGenerateContent）
  - 原生 Function Calling / Tool Use
  - 图像生成（Imagen 4: imagen-4.0-generate-001 等）
  - 视频生成（Veo 3.1: veo-3.1-generate-preview 等）
  - 音频生成（TTS: gemini-2.5-flash-preview-tts 等，30+ 语音）
  - Embedding（gemini-embedding-001，支持 MRL 128-3072 维度）
  - ListModels / HealthCheck
  - CredentialOverride 运行时凭证覆盖

# 不支持能力

  - 音频转录、微调任务管理
*/
package gemini
