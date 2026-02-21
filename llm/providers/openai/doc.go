// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 openai 提供 OpenAI 模型的 Provider 适配实现。该包在 openaicompat
基础上扩展，同时支持传统 Chat Completions API 和 2025 年推出的
Responses API（/v1/responses），是多数兼容 Provider 的参考实现。

# 核心结构体

  - OpenAIProvider — 嵌入 openaicompat.Provider，覆写 Completion 以支持
    Responses API 路由；通过 OpenAIConfig.UseResponsesAPI 开关切换

# 支持能力

  - Chat Completions（传统 /v1/chat/completions，委托 openaicompat）
  - Responses API（/v1/responses，支持 previous_response_id 多轮关联）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 图像生成（DALL·E / gpt-image-1）
  - 音频生成（TTS: tts-1, tts-1-hd, gpt-4o-mini-tts）
  - 音频转录（Whisper / gpt-4o-transcribe）
  - Embedding（text-embedding-3-small/large）
  - 微调任务管理（创建、列表、查询、取消）
  - Organization header 支持
  - RewriterChain 请求改写链
  - CredentialOverride 运行时凭证覆盖

# 上下文传递

  - WithPreviousResponseID / PreviousResponseIDFromContext
    用于在 context 中传递 Responses API 的 previous_response_id
*/
package openai
