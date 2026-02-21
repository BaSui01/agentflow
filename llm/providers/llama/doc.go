// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 llama 提供 Meta Llama 系列模型的 Provider 适配实现。Llama 为开源
模型，需通过第三方推理平台调用，本包支持 Together AI、Replicate 和
OpenRouter 三种后端，均使用 OpenAI 兼容 API 格式，通过嵌入
openaicompat.Provider 复用通用逻辑。

# 核心结构体

  - LlamaProvider — 嵌入 openaicompat.Provider，根据 cfg.Provider
    字段自动选择后端平台及对应 BaseURL

# 定制行为

  - 支持后端: together（默认）、replicate、openrouter
  - 默认 BaseURL 按后端自动设置:
    together → https://api.together.xyz
    replicate → https://api.replicate.com
    openrouter → https://openrouter.ai/api
  - 默认兜底模型: meta-llama/Llama-3-70b-chat-hf
  - ProviderName 格式: llama-{backend}（如 llama-together）

# 支持能力

  - Chat Completion（同步，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 健康检查、模型列表（委托 openaicompat）

# 不支持能力

  图像生成、视频生成、音频生成、音频转录、Embedding、微调
  均返回 NotSupportedError。
*/
package llama
