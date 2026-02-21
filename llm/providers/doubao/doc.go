// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 doubao 提供字节跳动豆包（Doubao）模型的 Provider 适配实现。该包基于
openaicompat 兼容层封装，对接豆包 Ark 平台 API，复用统一请求结构与
流式分片模型。

# 核心结构体

  - DoubaoProvider — 嵌入 openaicompat.Provider，配置豆包专属
    BaseURL（ark.cn-beijing.volces.com）与 EndpointPath（/api/v3/chat/completions）

# 构造函数

  - NewDoubaoProvider(cfg, logger) — 创建实例，默认模型 Doubao-1.5-pro-32k

# 支持能力

  - Chat Completions（/api/v3/chat/completions，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 音频生成（/api/v3/audio/speech）
  - Embedding（/api/v3/embeddings）
  - HealthCheck / ListModels（委托 openaicompat）
  - CredentialOverride 运行时凭证覆盖

# 不支持能力

  - 图像生成、视频生成、音频转录、微调任务管理
*/
package doubao
