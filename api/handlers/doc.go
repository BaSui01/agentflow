// Copyright (c) AgentFlow Authors.
// Licensed under the MIT License.

/*
Package handlers 提供 AgentFlow HTTP API 的请求处理器实现。

# 概述

handlers 包实现了 AgentFlow 所有 HTTP 端点的请求处理逻辑，
包括聊天补全、Agent 管理、健康检查以及统一的响应/错误处理。
所有 Handler 均遵循标准 net/http 接口，通过 Swagger 注解生成 API 文档。

# 核心类型

  - ChatHandler      — 聊天补全处理器，支持同步与 SSE 流式响应
  - AgentHandler     — Agent CRUD、执行、规划与健康检查
  - HealthHandler    — 服务健康检查（/health, /healthz, /ready）
  - Response         — 统一 JSON 响应结构（success + data + error + timestamp）
  - ErrorInfo        — 结构化错误信息，含 code、message、retryable 标记
  - ResponseWriter   — 包装 http.ResponseWriter 以捕获状态码
  - HealthCheck      — 可插拔健康检查接口（Database、Redis 等）

# 主要能力

  - 统一响应格式：WriteSuccess / WriteError / WriteJSON 辅助函数
  - 请求验证：DecodeJSONBody（1 MB 限制 + 严格模式）、ValidateContentType
  - ErrorCode → HTTP 状态码自动映射（4xx/5xx）
  - SSE 流式输出：ChatHandler.HandleStream 支持 text/event-stream
  - Agent 生命周期：列表、查询、执行、规划、健康检查
  - 可扩展健康检查：RegisterCheck 注册自定义 HealthCheck 实现
*/
package handlers
