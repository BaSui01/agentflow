// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license.

/*
Package config 提供 AgentFlow 的配置管理功能。

# 概述

config 包负责应用配置的完整生命周期管理，包括多源加载、
运行时热重载、变更审计与 HTTP 管理 API。配置按
"默认值 -> YAML 文件 -> 环境变量" 的优先级合并。

# 核心结构

  - Config: 顶层配置聚合，涵盖 Server、Agent、Redis、
    Database、Qdrant、Weaviate、Milvus、LLM、Log、Telemetry
  - Loader: 配置加载器，支持 Builder 模式链式设置
    文件路径、环境变量前缀与自定义验证器
  - HotReloadManager: 热重载管理器，支持文件监听、
    局部字段更新、变更回调、自动回滚与版本化历史
  - FileWatcher: 文件变更监听器，基于轮询 + 去抖机制
    触发配置重载
  - ConfigAPIHandler: HTTP API 处理器，提供配置查询、
    更新、热重载触发与变更历史查询端点

# 主要能力

  - 多源加载: YAML 文件、环境变量（AGENTFLOW_ 前缀）、默认值
  - 热重载: 文件监听自动重载 + API 手动触发，支持字段级更新
  - 安全治理: 敏感字段脱敏（MaskSensitive / MaskAPIKey）、
    API Key 仅 Header 传递、CORS 控制
  - 变更审计: 环形缓冲历史记录、版本号追踪、回滚到任意版本
  - 配置验证: 内置基础校验 + 自定义 ValidateFunc 钩子

# 使用示例

	cfg, err := config.NewLoader().
		WithConfigPath("config.yaml").
		WithEnvPrefix("AGENTFLOW").
		Load()
*/
package config
