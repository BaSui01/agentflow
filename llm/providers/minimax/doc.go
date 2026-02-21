// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 minimax 提供 MiniMax 模型的 Provider 适配实现。MiniMax 使用
OpenAI 兼容的 API 格式，本包通过嵌入 openaicompat.Provider 复用
HTTP 处理、SSE 解析等通用逻辑，并覆写 Stream 方法以处理 MiniMax
特有的 XML 格式 tool call。

# 核心结构体

  - MiniMaxProvider — 嵌入 openaicompat.Provider，覆写 Stream
    方法以解析 <tool_calls> XML 标签中的工具调用

# 定制行为

  - 默认 BaseURL: https://api.minimax.io
  - 默认兜底模型: abab6.5s-chat
  - Stream 后处理: 检测 content 中的 <tool_calls>...</tool_calls>
    XML 标签，自动提取并转换为标准 llm.ToolCall 结构
  - 音频生成: 支持，通过 /v1/audio/speech 端点

# 支持能力

  - Chat Completion（同步，委托 openaicompat）
  - 流式输出（SSE，含 XML tool call 后处理）
  - 原生 Function Calling / Tool Use
  - 音频生成（TTS，/v1/audio/speech）
  - 健康检查、模型列表（委托 openaicompat）

# 不支持能力

  图像生成、视频生成、音频转录、Embedding、微调
  均返回 NotSupportedError。
*/
package minimax
