// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 kimi 提供月之暗面 Kimi 模型的 Provider 适配实现。Kimi 使用
OpenAI 兼容的 API 格式，因此本包通过嵌入 openaicompat.Provider 复用
HTTP 处理、SSE 解析、消息转换等通用逻辑，仅定制差异部分。

# 核心结构体

  - KimiProvider — 嵌入 openaicompat.Provider，对接月之暗面
    Moonshot API，无额外 RequestHook 定制

# 定制行为

  - 默认 BaseURL: https://api.moonshot.cn
  - 默认兜底模型: moonshot-v1-8k
  - 认证方式: Bearer Token（标准 OpenAI 格式）

# 支持能力

  - Chat Completion（同步，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 健康检查、模型列表（委托 openaicompat）

# 不支持能力

  图像生成、视频生成、音频生成、音频转录、Embedding、微调
  均返回 NotSupportedError。
*/
package kimi
