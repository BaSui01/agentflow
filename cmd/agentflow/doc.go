// Copyright (c) AgentFlow Authors.
// Licensed under the MIT License.

/*
Package main 提供 AgentFlow 服务端程序入口。

# 概述

cmd/agentflow 是 AgentFlow 框架的可执行入口，提供 HTTP API 服务、
数据库迁移、健康检查和版本查询等子命令。程序支持 YAML 配置文件加载、
结构化日志（zap）、Prometheus 指标采集以及配置热重载。

# 核心类型

  - Server           — 主服务器，管理 HTTP、Metrics 双端口及优雅关闭
  - Middleware        — HTTP 中间件函数签名 func(http.Handler) http.Handler
  - responseWriter    — 包装 http.ResponseWriter 以捕获状态码

# 主要能力

  - 子命令：serve（启动服务）、migrate（数据库迁移）、version、health
  - 中间件链：Recovery、RequestID、SecurityHeaders、RequestLogger、
    CORS、RateLimiter（基于 IP）、APIKeyAuth（X-API-Key / query 参数）
  - 配置热重载：HotReloadManager 监听文件变更并回调
  - Metrics 服务器：独立端口暴露 /metrics（Prometheus）
  - 优雅关闭：信号监听 → 停止热更新 → 关闭 HTTP → 关闭 Metrics → Wait
  - 构建注入：Version、BuildTime、GitCommit 通过 ldflags 设置
*/
package main
