// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

包 claude 提供 Anthropic Claude 系列模型的 Provider 适配实现。
Claude API 与 OpenAI 格式有显著差异，本包负责将 AgentFlow 统一请求
映射到 Anthropic Messages API（/v1/messages），并处理认证、消息格式、
流式响应及工具调用等方面的协议转换。

# 核心结构体

  - ClaudeProvider — 独立实现 llm.Provider 接口（未嵌入 openaicompat），
    内置 RewriterChain 与安全 HTTP Client

# 协议差异

  - 认证使用 x-api-key 请求头（非 Bearer Token）
  - system 消息从 messages 数组中提取，单独传递到 system 字段
  - 消息 content 为数组形式，支持 text / tool_use / tool_result 混合
  - Tool 结果需包装为 user 角色的 tool_result 类型
  - 流式 SSE 事件结构独立（message_start / content_block_delta 等）

# 支持能力

  - Chat Completion（/v1/messages，同步）
  - 流式输出（SSE，含工具调用增量累积）
  - 原生 Function Calling（tool_use / tool_result）
  - 混合推理模式（ReasoningMode: fast / extended）
  - Thought Signatures（2026 新特性）
  - 模型列表查询（/v1/models）
  - 健康检查（HealthCheck）
  - CredentialOverride 运行时凭证覆盖
  - EmptyToolsCleaner 中间件自动清理空工具列表
*/
package claude
