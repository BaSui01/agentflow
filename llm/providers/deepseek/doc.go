// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 deepseek 提供 DeepSeek 模型的 Provider 适配实现。DeepSeek 使用
OpenAI 兼容的 API 格式，因此本包通过嵌入 openaicompat.Provider 复用
HTTP 处理、SSE 解析、消息转换等通用逻辑，仅定制差异部分。

# 核心结构体

  - DeepSeekProvider — 嵌入 openaicompat.Provider，通过 RequestHook
    实现 DeepSeek 特有的请求修改逻辑

# 定制行为

  - 默认 BaseURL: https://api.deepseek.com
  - 默认兜底模型: deepseek-chat
  - Endpoint: /chat/completions
  - RequestHook: 当 ReasoningMode 为 "thinking" 或 "extended" 时，
    自动切换模型为 deepseek-reasoner

# 支持能力

  - Chat Completion（同步，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 深度推理模式（deepseek-reasoner 自动路由）
  - 健康检查、模型列表（委托 openaicompat）

# 不支持能力

  图像生成、视频生成、音频生成、音频转录、Embedding、微调
  均返回 NotSupportedError。
*/
package deepseek
