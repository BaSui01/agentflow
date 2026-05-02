# ADR-009: ChatRequest Provider-Specific 扩展字段迁移

## 状态
提议中

## 背景
`types.ChatRequest`（`types/llm_contract.go`）单结构体达 110 行，混合了通用字段和 provider-specific 扩展字段：

**Provider-specific 字段（应迁移至 `llm/core/`）**：
- OpenAI 扩展：`MaxCompletionTokens`, `ReasoningEffort`, `ReasoningSummary`, `ReasoningDisplay`, `InferenceSpeed`, `Store`, `Modalities`
- Gemini 扩展：`IncludeServerSideToolInvocations`
- Responses API：`Include`, `Truncation`
- 缓存控制：`PromptCacheKey`, `PromptCacheRetention`, `CacheControl`, `CachedContent`
- Web 搜索：`WebSearchOptions`
- 思维/推理扩展：`ReasoningMode`, `ThinkingType`, `ThinkingLevel`, `ThinkingBudget`, `IncludeThoughts`
- 安全/输出：`SafetySettings`, `OutputSpeech`, `OutputImage`, `MediaResolution`
- 其他：`PreviousResponseID`, `ConversationID`, `ThoughtSignatures`, `Verbosity`, `Phase`

## 决策
将 provider-specific 字段迁移至 `llm/core/extensions.go`（已存在），通过 `ChatRequest.Extensions` 或嵌入方式引用。

## 影响范围
- 所有构造和读取 ChatRequest 的代码需适配新结构
- `llm/providers/` 各 Provider 的 request adapter 需更新
- `api/handlers/` 的请求转换需更新
- `types/` 层保持通用字段，符合零依赖定位

## 执行计划
1. 创建 feature 分支 `refactor/chatrequest-extension-migration`
2. 在 `llm/core/extensions.go` 定义 `ProviderExtensions` 结构体
3. ChatRequest 增加 `Extensions ProviderExtensions` 字段
4. 逐个 Provider 迁移扩展字段读取
5. 移除 ChatRequest 上的 provider-specific 字段
6. 运行全量测试 + 架构守卫

## 风险
- 所有 Provider 实现需同步更新
- SDK 消费者可能依赖现有字段，需版本协调
- JSON 序列化兼容性需验证
