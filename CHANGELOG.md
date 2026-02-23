# Changelog

本文件记录 AgentFlow 的版本变更历史。格式遵循 [Keep a Changelog](https://keepachangelog.com/)。

## [Unreleased]

### Fixed
- 修复 6 处返回 `nil, nil` 的 stub 实现，改为返回空切片（memory_coordinator, base, discovery/registry, context/window, knowledge_graph）
- 修复 workflow/steps.go 中 3 个 placeholder 实现，未注入依赖时返回明确的 `ErrNotConfigured` 错误
- 实现 tools/openapi/generator.go 的本地文件加载功能
- 删除 llm/db_init.go 中整个 `SeedExampleData` 函数（含 13 个 provider、52 个 model 的硬编码种子数据），API key 统一走号池系统
- 修复 api/handlers/agent.go SSE 流中 json.Marshal 错误被静默忽略的问题
- 修复 llm/image/openai.go 和 llm/speech/openai_stt.go 中 multipart WriteField 错误被忽略的问题
- 统一 6 处 logger fallback 为 `zap.NewNop()`，消除 `logger, _ = zap.NewProduction()` 模式

### Removed
- 删除 `llm/db_init.go`，表结构统一由 SQL migration 管理，不再使用 GORM AutoMigrate
- 删除 `Router.SelectProvider()` DEPRECATED 兼容方法
- 删除 `llm.IsRetryable` 兼容别名，直接使用 `types.IsRetryable`
- 删除 `llm/retry.IsRetryable` 兼容别名，使用 `IsRetryableError`
- 删除 `llm/tools/executor.go` 中 `rateLimiter` 兼容包装器，直接使用 `tokenBucketLimiter`
- 修复 `MultiProviderRouter.SelectProviderWithModel` 的 default 分支：不再静默回退到 health 策略，改为返回明确错误
- 修复 `APIKeyPool.SelectKey` 的 default 分支：不再静默回退到 weighted_random，改为返回明确错误
- 修复 `NewAPIKeyPool` 空 strategy 校验：不再静默默认为 weighted_random，改为 panic

### Added
- CONTRIBUTING.md 贡献指南
- CHANGELOG.md 版本变更记录
- `workflow.ErrNotConfigured` 哨兵错误，用于标识未配置的 step 依赖

## [v0.2.0]

### Fixed
- TLS 加固：全量替换 31 个 HTTP Client，集中化 TLS 工具包
- API 输入校验：agentID 正则校验 + 错误码去重
- 并发安全：channel double-close + goroutine leak + panic recovery
- CI 测试修复：LLM Judge 浮点精度 + MiniMax XML tool call 解析
- 迁移 testify/mock 到 function callback 模式

### Added
- `internal/tlsutil` 集中化 TLS 工具包
- 数据库迁移 000002: API key 表增加 base_url 字段
- `pkg/` 新增 cache, database, metrics, middleware 模块

### Changed
- README 全量更新：补齐核心特性/项目结构/示例表/技术栈
- 重构 cmd/agentflow 入口（main, middleware, migrate, server）

## [v0.1.0]

### Added
- 核心 Agent 框架：BaseAgent, AgentBuilder, ReAct 循环, Reflection
- 13+ LLM 提供商支持（OpenAI, Anthropic, Gemini, DeepSeek 等）
- 多提供商路由（成本/健康/QPS 负载均衡）
- RAG 系统：混合检索、多跳推理、语义缓存
- DAG 工作流引擎 + YAML DSL
- 记忆系统：工作记忆、长期记忆、情景记忆、语义记忆
- Guardrails：PII 检测、注入防护、输出验证
- MCP/A2A 协议支持
- 浏览器自动化（chromedp）
- 多模态支持（图像、视频、语音、音乐、3D）
- HTTP API + OpenAPI 3.0 规范
- Docker + docker-compose 部署
- GitHub Actions CI/CD
- 21 个示例场景
- 中英文双语文档
