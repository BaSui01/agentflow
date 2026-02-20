# brainstorm: 框架优化分析

## 目标

全面分析 AgentFlow 项目架构和代码质量，识别值得优化的方向，为后续改进提供决策依据。

## 已知信息

* AgentFlow 是纯 Go 后端框架，Go 1.24，依赖层次清晰：`types/ ← llm/ ← agent/ ← workflow/ ← api/ ← cmd/`
* 已完成 `openaicompat` 基类重构，9 个 OpenAI 兼容 provider 已瘦身至 ~30 行
* 项目有 18 个规范文档，每个 provider 子包都有 `doc.go`
* 跨 provider 的 property test 覆盖良好（15 个跨 provider property test 文件）

## 发现的问题

### 🔴 高严重度

#### H1. `openaicompat` 基类零测试
- **位置**: `llm/providers/openaicompat/`
- **影响**: 这是 9 个 provider 的核心实现（Completion, Stream, HealthCheck, ListModels），却没有任何直接测试
- **风险**: 基类改动可能悄悄破坏所有下游 provider

#### H2. `circuitbreaker` 和 `idempotency` 零测试
- **位置**: `llm/circuitbreaker/`, `llm/idempotency/`
- **影响**: 这两个是生产可靠性的关键组件

#### H3. Provider Config 结构体大量重复
- **位置**: `llm/providers/config.go`
- **详情**: 13 个 Config 结构体中 11 个字段完全相同（APIKey, BaseURL, Model, Timeout），只有 OpenAIConfig 和 LlamaConfig 有额外字段

### 🟡 中严重度

#### M1. Gemini/Claude 与通用函数重复
- **位置**: `gemini/provider.go:622-660`, `anthropic/provider.go:620-662`
- **详情**: `mapGeminiError`/`mapClaudeError` 与 `MapHTTPError` 逻辑几乎相同；`readGeminiErrMsg`/`readClaudeErrMsg` 与 `ReadErrorMessage` 功能相似；`chooseGeminiModel`/`chooseClaudeModel` 与 `ChooseModel` 完全相同

#### M2. Multimodal header 构建匿名函数重复 (~15 处)
- **位置**: 各 provider 的 `multimodal.go`
- **详情**: 每个方法都内联了相同的 Bearer token header 构建函数，而 `openaicompat.Provider` 已有 `buildHeaders` 方法

#### M3. `multimodal_helpers.go` 四个函数结构高度重复
- **位置**: `llm/providers/multimodal_helpers.go`
- **详情**: Image/Video/Audio/Embedding 四个函数共享几乎完全相同的 HTTP 请求/响应处理逻辑

#### M4. `context.Value` 使用字符串 key
- **位置**: `llm/providers/openai/provider.go:149`
- **详情**: 使用 `ctx.Value("previous_response_id")` 违反 Go 最佳实践，项目其他地方都用了自定义 key 类型

#### M5. CORS 硬编码通配符
- **位置**: `config/api.go:328`
- **详情**: `Access-Control-Allow-Origin: *` 不适合生产环境

#### M6. Agent API 层 registry 集成未完成
- **位置**: `api/handlers/agent.go`
- **详情**: 9 处 TODO 标记，Agent API 层尚未与 agent registry 集成

#### M7. Doubao provider 零测试
- **位置**: `llm/providers/doubao/`

#### M8. Config 子模块测试缺失
- **位置**: `config/api.go`, `config/watcher.go`, `config/defaults.go`

### 🟢 低严重度

#### L1. `hotreload.go` 自定义标准库函数
- **位置**: `config/hotreload.go:1053-1074`
- **详情**: 自定义 `toLower`/`contains` 可直接用 `strings.ToLower`/`strings.Contains`

#### L2. `internal/server/manager.go` 无测试

## 待解决问题

* 用户希望优先解决哪些方向？（代码重复 vs 测试覆盖 vs 安全加固 vs 功能完善）
* 是否需要对 Anthropic/Gemini 做类似 openaicompat 的基类抽取？

## 技术说明

* 项目已有 `openaicompat` 基类重构的成功经验，可复用此模式
* 跨 provider property test 已覆盖核心行为属性，新增测试应优先覆盖基类
* Makefile 覆盖率阈值仅 24%，有提升空间
