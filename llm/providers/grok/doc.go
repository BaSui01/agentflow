// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 grok 提供 xAI Grok 模型的 Provider 适配实现。该包基于
openaicompat 兼容层封装，对接 xAI API（api.x.ai），处理工具调用
与流式输出能力。

# 核心结构体

  - GrokProvider — 嵌入 openaicompat.Provider，配置 xAI 专属
    BaseURL（api.x.ai），使用 Bearer Token 认证

# 构造函数

  - NewGrokProvider(cfg, logger) — 创建实例，默认模型 grok-beta

# 支持能力

  - Chat Completions（/v1/chat/completions，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 图像生成（Aurora: grok-2-image，/v1/images/generations）
  - Embedding（grok-embedding-beta，/v1/embeddings）
  - HealthCheck / ListModels（委托 openaicompat）
  - CredentialOverride 运行时凭证覆盖

# 不支持能力

  - 视频生成、音频生成、音频转录、微调任务管理
*/
package grok
