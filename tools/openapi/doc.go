// Copyright 2025-2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

Package openapi 提供从 OpenAPI 规范自动生成 Agent 工具的能力。

该包解析 OpenAPI 3.x JSON 规范，将每个 API Operation 转换为
AgentFlow 可直接调用的 GeneratedTool，包含名称、描述、参数 Schema、
HTTP 方法和路径等完整信息。

# 核心接口/类型

  - Generator — OpenAPI 工具生成器，负责加载规范和生成工具
  - OpenAPISpec — 解析后的 OpenAPI 规范（Info / Servers / Paths）
  - GeneratedTool — 生成的工具定义，包含 llm.ToolSchema 和 HTTP 调用信息
  - GenerateOptions — 工具生成选项（BaseURL 覆盖、Tag 过滤、前缀等）
  - Operation / Parameter / RequestBody / JSONSchema — OpenAPI 结构体映射

# 主要能力

  - 规范加载：从 URL 加载 OpenAPI JSON 规范，内置缓存避免重复请求
  - 工具生成：遍历 Paths 中的所有 Operation，自动生成 GeneratedTool 列表
  - Tag 过滤：通过 IncludeTags / ExcludeTags 控制生成范围
  - 参数映射：自动将 path / query / header 参数和 RequestBody 转换为 JSON Schema
  - 安全传输：HTTP 请求使用 tlsutil.SecureHTTPClient
*/
package openapi
