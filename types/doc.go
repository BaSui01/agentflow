// Copyright (c) AgentFlow Authors.
// Licensed under the MIT License.

/*
Package types 提供 AgentFlow 框架的全局共享类型定义。

# 概述

types 是框架最底层的公共包，不依赖任何内部包，为 agent、workflow、llm、
api 等上层模块提供统一的类型契约。所有跨包共享的接口、结构体、枚举和
错误码均定义于此，以避免循环依赖。

# 核心接口与类型

  - Executor          — 最小 Agent 执行接口（ID + Execute）
  - Named             — 可选的 Agent 显示名称接口
  - Tokenizer         — 框架级 Token 计数接口（Message / ToolSchema 感知）
  - TokenCounter      — 最小 Token 计数接口（CountTokens(string) int）
  - Message           — 对话消息（Role、Content、ToolCalls、Images）
  - ToolSchema        — 工具定义（name + description + JSON Schema parameters）
  - ToolResult        — 工具执行结果
  - Error / ErrorCode — 结构化错误体系，含 HTTP 状态码、Retryable、Provider 标记
  - JSONSchema        — JSON Schema 定义与构建器（NewObjectSchema 等）
  - AgentConfig       — 模块化 Agent 配置（Core / LLM / Features / Extensions）
  - MemoryRecord      — 统一记忆条目（working / episodic / semantic / procedural）
  - ExtensionRegistry — 可选扩展注册表（Reflection、MCP、Guardrails 等）

# 主要能力

  - Context 传播：WithTraceID / WithTenantID / WithUserID / WithRunID 等
  - 错误工具链：WrapError / AsError / IsErrorCode / IsRetryable
  - 常用错误构造：NewInvalidRequestError / NewRateLimitError / NewTimeoutError
  - Token 估算：EstimateTokenizer（中英文字符分别计算）
  - 扩展接口：ReflectionExtension、MCPExtension、GuardrailsExtension 等 8 种
  - 配置校验：AgentConfig.Validate + Is*Enabled 便捷方法
*/
package types
