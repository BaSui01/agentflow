// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 glm 提供智谱 AI GLM 系列模型的 Provider 适配实现。该包基于
openaicompat 兼容层封装，对接智谱开放平台 API（open.bigmodel.cn），
支持统一补全、流式响应与多模态能力映射。

# 核心结构体

  - GLMProvider — 嵌入 openaicompat.Provider，配置智谱专属
    BaseURL（open.bigmodel.cn）与 EndpointPath（/api/paas/v4/chat/completions）

# 构造函数

  - NewGLMProvider(cfg, logger) — 创建实例，默认模型 glm-4-plus

# 支持能力

  - Chat Completions（/api/paas/v4/chat/completions，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - 图像生成（CogView，/api/paas/v4/images/generations）
  - 视频生成（CogVideo，/api/paas/v4/videos/generations）
  - Embedding（/api/paas/v4/embeddings）
  - HealthCheck / ListModels（委托 openaicompat）
  - CredentialOverride 运行时凭证覆盖

# 不支持能力

  - 音频生成、音频转录、微调任务管理
*/
package glm
