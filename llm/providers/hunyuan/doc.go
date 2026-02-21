// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 hunyuan 提供腾讯混元（Hunyuan）模型的 Provider 适配实现。该包基于
openaicompat 兼容层封装，对接混元 OpenAI 兼容接口
（api.hunyuan.cloud.tencent.com），补齐模型标识与响应字段语义。

# 核心结构体

  - HunyuanProvider — 嵌入 openaicompat.Provider，配置混元专属
    BaseURL（api.hunyuan.cloud.tencent.com/v1）

# 构造函数

  - NewHunyuanProvider(cfg, logger) — 创建实例，默认模型 hunyuan-pro

# 支持能力

  - Chat Completions（/v1/chat/completions，委托 openaicompat）
  - 流式输出（SSE，委托 openaicompat）
  - 原生 Function Calling / Tool Use
  - HealthCheck / ListModels（委托 openaicompat）
  - CredentialOverride 运行时凭证覆盖

# 不支持能力

  - 图像生成、视频生成、音频生成、音频转录、Embedding、微调任务管理
*/
package hunyuan
