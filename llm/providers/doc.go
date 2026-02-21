// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 providers 提供跨模型服务商的通用适配与辅助能力，是所有具体 Provider
实现的公共基础层。各服务商子包（openai、anthropic、deepseek 等）依赖本包
完成请求/响应转换、错误映射、多模态调用等共享逻辑。

# 核心类型

  - BaseProviderConfig — 所有 Provider 共享的基础配置（APIKey、BaseURL、Model、Timeout）
  - OpenAICompat* 系列 — OpenAI 兼容 API 的通用请求/响应/工具调用结构体
  - RetryableProvider — 带指数退避重试的 Provider 包装器
  - RetryConfig — 重试策略配置（最大次数、初始延迟、退避因子）

# 核心函数

  - MapHTTPError — 将 HTTP 状态码映射为语义化的 llm.Error（含 Retryable 标记）
  - ConvertMessagesToOpenAI / ConvertToolsToOpenAI — 统一消息与工具格式转换
  - ToLLMChatResponse — OpenAI 兼容响应到 llm.ChatResponse 的转换
  - ChooseModel — 按优先级选择模型（请求 > 默认 > 兜底）
  - ListModelsOpenAICompat — 通用模型列表获取

# 多模态辅助

  - GenerateImageOpenAICompat — 通用图像生成
  - GenerateVideoOpenAICompat — 通用视频生成
  - GenerateAudioOpenAICompat — 通用音频生成
  - CreateEmbeddingOpenAICompat — 通用 Embedding 生成
  - NotSupportedError — 不支持能力的标准错误构造

# 支持能力

  - 统一错误语义映射（401/403/429/5xx/529 等）
  - 指数退避重试（Completion 与 Stream 连接阶段）
  - OpenAI 兼容格式的请求/响应序列化
  - 多模态生成（图像、视频、音频、Embedding）的通用 HTTP 执行
  - Bearer Token 标准认证 header 构建
*/
package providers
