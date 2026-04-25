# 模型字段与 Agent 框架接入指南

> 更新时间：2026-04-24  
> 目标：把“上游模型最新字段 / 能力”与 AgentFlow 当前 `Model / Control / Tools` 主面对应起来，说明哪些已经可用、哪些仍需补齐。  
> 适用范围：`types/`、`agent/runtime`、`llm/gateway`、`llm/providers/*`、`api/types.go`、`docs/cn/guides/`。  
> 官方核验：截至 2026-04-24。

---

## 1. 先说结论

1. **“最新模型数据”** 和 **“每次请求要传的字段”** 不是一回事。
   - 最新模型数据是 catalog / registry 事实，例如：模型 ID、别名、Stage、上下文窗口、最大输出、是否支持 thinking / tools / structured output、是否即将退役。
   - 请求字段是每次调用的运行时参数，例如：`model`、`reasoning.effort`、`response_format`、`tool_choice`、`previous_response_id`。
2. AgentFlow 当前已经有**正确的主链方向**：
   - 仓库级正式入口：`sdk.New(opts).Build(ctx)`
   - 单 Agent 正式入口：`agent/runtime`
   - 多 Agent 正式入口：`agent/team`
   - 显式编排正式入口：`workflow/runtime`
   - 统一授权入口：`internal/usecase/authorization_service.go`
3. 当前运行时已经有**正式主面**：`types.AgentConfig{Model, Control, Tools}`。
4. **低层 DTO 与正式主面已经完成第一轮对齐**：`types.ChatRequest` / `api.ChatRequest` 中已有的通用模型请求字段，已通过 `types.ModelOptions` 与 `agent/adapters.ChatRequestAdapter` 接入正式运行时主链；少数 provider-native 深水区字段仍保留在 provider 边界继续收口。
5. 因此现在最正确的做法不是把 provider SDK struct 往上抬，而是：
   - 保持上层只依赖 `Model / Control / Tools`
   - 把缺失字段按语义收口到正式主面
   - 继续让 `ChatRequestAdapter` 在边界处降级到 provider DTO

一句话版：**上层保留统一语义，边界层负责厂商协议细节，模型目录事实单独建 registry，不要把所有厂商原生字段直接倒进 AgentConfig。**

---

## 2. 当前代码里的真实落点

| 位置 | 文件 | 当前职责 |
|---|---|---|
| 仓库级正式入口 | `sdk.New(opts).Build(ctx)` | 统一装配 Agent / Workflow / RAG / Provider |
| 单 Agent runtime 入口 | `agent/runtime` | 单 Agent 构建、执行、闭环 |
| 正式运行时配置入口 | `types/config.go` | `types.AgentConfig`，正式主面是 `Model / Control / Tools` |
| 运行时归一化 | `types/execution_options.go` | `AgentConfig -> ExecutionOptions` |
| 边界适配器 | `agent/adapters/chat.go` | `ExecutionOptions -> types.ChatRequest` |
| 低层 LLM DTO | `types/llm_contract.go` | provider / gateway 侧真实请求响应结构 |
| API 入站 DTO | `api/types.go` | HTTP/API 协议层字段展开 |
| 供应商 profile | `llm/providers/vendor/profile.go` | 语言到默认模型的 fallback 映射，不是完整模型目录 |
| compat 厂商 request hook / 校验 | `llm/providers/vendor/chat_profiles.go` | DeepSeek / Qwen / Grok / Kimi 等 compat provider 的字段修正与本地拒绝 |
| 工具授权统一入口 | `internal/usecase/authorization_service.go` | 工具执行 / 审批 / HITL 主链 |

---

## 3. 当前正式主面已经覆盖了什么

### 3.1 `Model` 面

`types.ModelOptions` 当前已经覆盖：

- `Provider`
- `Model`
- `RoutePolicy`
- `MaxTokens`
- `MaxCompletionTokens`
- `Temperature`
- `TopP`
- `Stop`
- `FrequencyPenalty`
- `PresencePenalty`
- `RepetitionPenalty`
- `N`
- `LogProbs`
- `TopLogProbs`
- `User`
- `ResponseFormat`
- `StreamOptions`
- `ServiceTier`
- `ReasoningEffort`
- `ReasoningSummary`
- `ReasoningDisplay`
- `ReasoningMode`
- `ThinkingType`
- `ThinkingLevel`
- `ThinkingBudget`
- `IncludeThoughts`
- `MediaResolution`
- `InferenceSpeed`
- `Store`
- `Modalities`
- `PromptCacheKey`
- `PromptCacheRetention`
- `CacheControl`
- `CachedContent`
- `Include`
- `Truncation`
- `PreviousResponseID`
- `ConversationID`
- `ThoughtSignatures`
- `Verbosity`
- `Phase`
- `WebSearchOptions`

这意味着下面这些已经能在**正式主面**表达：

- OpenAI / GPT-5 系列的 `reasoning.effort`、`verbosity`、`phase`
- Anthropic thinking 的 `display`、`thinking_type`（adaptive / enabled / disabled）
- Gemini thinking 的 `thinking_level`、`thinking_budget`、`include_thoughts`
- Gemini media 的 `media_resolution`
- 基础 structured output
- 基础 web search 配置
- 基础采样 / 长度 / 路由参数，以及日志概率 / 多候选输出 / provider user 标识
- OpenAI Responses 的 `previous_response_id` / `conversation_id` / `include` / `truncation`
- `service_tier` / `store` / `modalities` / `stream_options`
- prompt cache / cached content / thought signature 连续性字段

### 3.2 `Control` 面

`types.AgentControlOptions` 当前已经覆盖：

- `SystemPrompt`
- `Timeout`
- `MaxReActIterations`
- `MaxLoopIterations`
- `MaxConcurrency`
- `DisablePlanner`
- `Context`
- `Reflection`
- `Guardrails`
- `Memory`
- `ToolSelection`
- `PromptEnhancer`

### 3.3 `Tools` 面

`types.ToolProtocolOptions` 当前已经覆盖：

- `AllowedTools`
- `ToolWhitelist`
- `DisableTools`
- `Handoffs`
- `ToolModel`
- `ToolChoice`
- `ParallelToolCalls`
- `ToolCallMode`

`types.ToolChoice` 还支持：

- `Mode`
- `ToolName`
- `AllowedTools`
- `DisableParallelToolUse`
- `IncludeServerSideToolInvocations`

这已经足够表达：

- OpenAI `allowed_tools`
- Anthropic / OpenAI / Gemini 的通用 `tool_choice`
- 并行工具调用开关
- 统一 tool protocol 语义

---

## 4. 当前正式主面还缺什么

下面这些字段**已经存在于低层 DTO**，并已在 2026-04-24 的第一轮代码落地中提升到 `ModelOptions` 主面，同时由 `agent/adapters.DefaultChatRequestAdapter` 映射到 `types.ChatRequest`：

| 字段 / 语义 | 现在在哪里 | 当前状态 |
|---|---|---|
| `previous_response_id` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `conversation_id` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `include` / `truncation` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `store` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `service_tier` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `modalities` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `stream_options` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `prompt_cache_key` / `prompt_cache_retention` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `cache_control` / `cached_content` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `ReasoningMode` | `types.ModelOptions` -> `types.ChatRequest` | 已接入正式主面，provider 侧限制仍由 compat profile 校验 |
| `ThoughtSignatures` | `types.ModelOptions` -> `types.ChatRequest` / `ChatResponse` | 已接入正式主面 |
| `frequency_penalty` / `presence_penalty` / `repetition_penalty` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| `n` / `logprobs` / `top_logprobs` / `user` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面 |
| Gemini `thinkingBudget` / `thinkingLevel` / `includeThoughts` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面（切片 B） |
| Gemini `mediaResolution` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面（切片 B） |
| Gemini `speechConfig` / `imageConfig` / `safetySettings` | 尚未进入正式主面 | 多模态控制字段没有统一入口 |
| Anthropic `thinking.type` / adaptive thinking | `types.ModelOptions.ThinkingType` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面（切片 C） |
| OpenAI `verbosity` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面（切片 A） |
| OpenAI `phase` | `types.ModelOptions` -> `types.ChatRequest` / `api.ChatRequest` | 已接入正式主面（切片 A） |

这就是当前最核心的事实：

> **正式主面已经覆盖低层 DTO 中主要的通用模型请求字段以及三大 native provider（OpenAI / Gemini / Anthropic）的 thinking 控制字段；剩余工作集中在 Gemini speech / image / safety 深水区字段。**

---

## 5. 按厂商看：字段应该怎么落

## 5.1 OpenAI GPT-5 / Responses API

官方页面当前重点字段：

- `model`
- `reasoning.effort`
- `text.verbosity`
- `previous_response_id`
- `conversation`
- `tool_choice.allowed_tools`
- `include`
- `truncation`
- `service_tier`
- `store`
- `phase`

### 当前最合理的落法

| 上游字段 | 当前应该放哪里 | 现状 |
|---|---|---|
| `model` | `Model.Model` | 已有 |
| `reasoning.effort` | `Model.ReasoningEffort` | 已有 |
| `tool_choice.allowed_tools` | `Tools.ToolChoice.AllowedTools` | 已有 |
| `text.verbosity` | **缺少正式字段** | 待补 |
| `previous_response_id` | `Model.PreviousResponseID` | 已接入正式主面 |
| `conversation` / `conversation_id` | `Model.ConversationID` | 已接入正式主面 |
| `include` / `truncation` | `Model.Include` / `Model.Truncation` | 已接入正式主面 |
| `service_tier` / `store` | `Model.ServiceTier` / `Model.Store` | 已接入正式主面 |
| `phase` | **缺少正式字段** | 待补 |

### 建议

不要把 OpenAI Responses 的原始 request struct 暴露到 `agent/runtime`。应在正式主面内新增统一语义，例如：

- `Model.Verbosity`
- `Model.Conversation.PreviousResponseID`
- `Model.Conversation.ConversationID`
- `Model.Output.Include`
- `Model.Output.Truncation`
- `Model.Output.Store`
- `Model.Output.ServiceTier`
- `Model.Output.Phase`

---

## 5.2 Anthropic Claude / Messages API

官方页面当前重点字段：

- `thinking.type`
- `thinking.display`
- `effort` / `adaptive thinking`
- `tool_choice`
- `thinking` / `redacted_thinking` blocks
- `signature_delta`
- Models API 的 `capabilities`

### 当前最合理的落法

| 上游字段 | 当前应该放哪里 | 现状 |
|---|---|---|
| `thinking.display` | `Model.ReasoningDisplay` | 已有 |
| thinking 可展示摘要 | `Message.ReasoningSummaries` | 已有 |
| thinking / redacted thinking opaque state | `Message.OpaqueReasoning` / `Message.ThinkingBlocks` | 已有 |
| `thinking.type` / adaptive vs enabled | **缺少正式字段** | 待补 |
| `budget_tokens` / `effort` | `Model.ReasoningEffort` 只能覆盖一部分 | 语义不完整 |
| 模型 capability facts | `types.ModelDescriptor` / `types.ModelCatalog` | 已有代码侧模型目录结构，具体官方快照后续注入 |

### 建议

Anthropic 方向不要把 `thinking` block 处理逻辑塞到 handler 或业务层；应继续保持：

- 响应侧：`Message.ReasoningSummaries` / `OpaqueReasoning` / `ThinkingBlocks`
- 请求侧：新增统一 `ThinkingOptions`
- 授权 / 工具：继续统一走 `AuthorizationService`

特别注意：

- Claude 4.7 以后不再接受手动 `budget_tokens` thinking
- 工具循环里必须原样回传 thinking / redacted_thinking blocks
- thinking + tool use 的回合连续性不应在 `agent/runtime` 之外散落实现

---

## 5.3 Google Gemini / GenerateContent

官方页面当前重点字段：

- `generationConfig.responseMimeType`
- `generationConfig.responseJsonSchema`
- `thinkingConfig.includeThoughts`
- `thinkingConfig.thinkingBudget`
- `thinkingConfig.thinkingLevel`
- `responseModalities`
- `speechConfig`
- `imageConfig`
- `mediaResolution`
- `safetySettings`
- `cachedContent`
- `usageMetadata.thoughtsTokenCount`
- `modelStatus.modelStage`

### 当前最合理的落法

| 上游字段 | 当前应该放哪里 | 现状 |
|---|---|---|
| `responseMimeType` / `responseJsonSchema` | `Model.ResponseFormat` | 部分已覆盖 |
| `cachedContent` | `Model.CachedContent` | 已接入正式主面 |
| `thinkingBudget` / `thinkingLevel` / `includeThoughts` | **缺少正式字段** | 待补 |
| `responseModalities` | `Model.Modalities` | 已接入通用输出模态字段；Gemini 专有映射仍在 provider 边界补齐 |
| `speechConfig` / `imageConfig` / `mediaResolution` / `safetySettings` | **缺少正式字段** | 待补 |
| `thoughtsTokenCount` | `usage` 统计归一化 | 需要统一暴露 |
| `modelStage` / `retirementTime` | `types.ModelDescriptor` / `types.ModelCatalog` | 已有代码侧模型目录结构，具体数据源仍需由运行时组合或文档快照生成 |

### 建议

Gemini 不要继续靠 provider 内部“偷渡字段”长期扩展。建议尽快在正式主面补：

- `Model.Thinking.Level`
- `Model.Thinking.Budget`
- `Model.Thinking.IncludeSummary`
- `Model.Output.ResponseModalities`
- `Model.Output.MediaResolution`
- `Model.Output.StructuredOutput`
- `Model.Safety`
- `Model.Cache`

这样 `llm/providers/gemini` 只负责把统一语义翻译成 `GenerationConfig`，而不是自己维护另一套 runtime API。

---

## 5.4 xAI Grok

官方页面当前重点信息：

- 官方默认建议：`grok-4.20`
- `reasoning.effort` 只对 `grok-4.20-multi-agent` 有意义，而且控制的是 **agent 数量**，不是思考深度
- reasoning content 可以通过加密内容 round-trip
- server-side tools / MCP / search 工具有独立计费与字段语义

### 当前项目现状

`llm/providers/vendor/chat_profiles.go` 已经做了本地 compat 校验：

- reasoning mode 会把空模型映射到 `grok-4.20-reasoning`
- 对 reasoning 请求直接拒绝：
  - `stop`
  - `frequency_penalty`
  - `presence_penalty`
  - `reasoning_effort`

### 建议

这说明当前项目文档必须同时区分：

1. **官方当前推荐别名**：`grok-4.20`
2. **项目当前 compat 实现别名**：`grok-4.20-reasoning`

下一步不要把 xAI 的特例逻辑扩散到业务层；继续保留在 compat provider profile 校验层，同时在正式主面中增加能表达“多 agent 研究深度”的统一语义，而不是复用 OpenAI 式 `reasoning_effort` 文案硬套。

---

## 5.5 DeepSeek / Qwen / Kimi 等 compat 厂商

这些厂商当前更适合按 **compat profile + request hook + validation** 模式继续收口：

- `DeepSeek`：alias / V4 升级 / thinking vs non-thinking 继续在 compat layer 处理
- `Qwen`：thinking + JSON structured output 冲突，继续在 compat layer 拒绝
- `Kimi`：thinking 模式对 `tool_choice` / 采样字段限制更严，继续在 compat layer 拒绝

换句话说：

> **compat 厂商先统一“约束校验与模型切换规则”，再考虑是否值得进入正式主面。**

---

## 6. “最新模型数据”应该怎么记

当前仓库文档主要还是**手工快照**。这能工作，但不够长期稳定。

### 推荐拆成两层

#### 层 1：文档快照

保留当前 `docs/cn/guides/` 里的：

- `模型厂商与模型中文命名规范.md`
- `近12个月主流多模态模型总表.md`
- `模型与媒体端点参考.md`

它们负责：

- 中文命名口径
- 时间点快照
- 公开可读的比较说明
- 可公开复核的 benchmark / eval 事实快照

#### 层 2：代码侧模型目录（已新增结构）

`types/model_catalog.go` 已新增统一模型目录结构：

- `ModelDescriptor`
- `ModelCapability`
- `ModelEndpointFamily`
- `ModelCatalog`

建议最少字段：

| 字段 | 含义 |
|---|---|
| `Provider` | `openai` / `anthropic` / `gemini` / ... |
| `ID` | 官方模型 ID |
| `DisplayName` | 展示名 |
| `Aliases` | alias 列表 |
| `Stage` | `preview` / `stable` / `deprecated` / `retired` / `coming_soon` |
| `ContextWindowTokens` | 最大上下文 token 数 |
| `MaxOutputTokens` | 最大输出 |
| `InputModalities` | text / image / audio / video / document |
| `OutputModalities` | text / image / audio |
| `Capabilities` | `reasoning` / `thinking` / `tool_calling` / `structured_output` / `streaming` / `web_search` 等能力集合 |
| `EndpointFamilies` | `openai_responses` / `anthropic_messages` / `gemini_generate_content` 等端点族 |
| `RetiresAt` | 退役时间 |
| `SourceURLs` | 官方来源 |
| `VerifiedAt` | 最近核验时间 |

这会比今天的 `vendor.Profile.LanguageModels map[string]string` 更适合作为“最新模型数据”的代码事实来源。当前实现只提供零依赖目录结构和查找能力，具体官方模型快照仍应从 `docs/cn/guides/` 或后续生成脚本单向注入，避免在 provider 层散落硬编码。

### 6.3 benchmark / eval 事实不进入 `ModelOptions`

近期如果要补 `SWE-Bench`、`BrowseComp`、`GPQA`、`Humanity's Last Exam`、$WER$ 等评测信息，必须继续区分两类事实：

- **benchmark / eval 事实**：模型版本、评测名、分数、是否含 tools / high compute、来源 URL、核验日期。
- **运行时请求字段**：每次调用要传给 provider 的参数，如 `model`、`reasoning.effort`、`tool_choice`、`response_format`。

因此：

1. **不要**因为要在文档中展示 benchmark，就把 benchmark 字段加进 `types.ModelOptions`、`types.ChatRequest` 或 `api.ChatRequest`。
2. benchmark 更适合放在：
    - `docs/cn/guides/` 文档快照；或
    - `types.ModelDescriptor` / `ModelCatalog` 的旁路元数据（如果未来要结构化管理）。
3. `agent/runtime`、`workflow/runtime`、`api/handlers` 仍只处理**请求语义**，不处理“模型战绩表”。

---

## 7. 当前实现“怎么办”——最小可用策略

如果你今天就要把 AgentFlow 跑起来，推荐这样做：

1. **上层只写 `types.AgentConfig{Model, Control, Tools}`**。
2. **不要让业务层直接构造 provider SDK request**。
3. **确实还没有正式主面字段时，只允许在 adapter / gateway / API 边界临时使用低层 DTO 字段**。
4. **compat 厂商的特殊限制统一放在 `llm/providers/vendor/chat_profiles.go`**。
5. **工具审批 / HITL / 授权继续只走 `AuthorizationService`**。

最小示例：

```go
maxCompletionTokens := 2048
serviceTier := "priority"

cfg := types.AgentConfig{
    Core: types.CoreConfig{
        ID:   "coding-agent",
        Name: "Coding Agent",
        Type: "assistant",
    },
    Model: types.ModelOptions{
        Provider:            "openai",
        Model:               "gpt-5.4",
        MaxTokens:           4096,
        MaxCompletionTokens: &maxCompletionTokens,
        ReasoningEffort:     "medium",
        ReasoningSummary:    "auto",
        ServiceTier:         &serviceTier,
        PreviousResponseID: "resp_prev_123",
        ConversationID:     "conv_123",
        Include:            []string{"reasoning.encrypted_content"},
        Truncation:         "auto",
        ResponseFormat: &types.ResponseFormat{
            Type: types.ResponseFormatJSONObject,
        },
    },
    Control: types.AgentControlOptions{
        SystemPrompt:      "你是一个严格遵守仓库约束的代码代理。",
        MaxLoopIterations: 8,
        ToolSelection: &types.ToolSelectionConfig{
            Enabled:  true,
            MaxTools: 8,
        },
    },
    Tools: types.ToolProtocolOptions{
        AllowedTools: []string{"web_search", "code_executor"},
        ToolChoice: &types.ToolChoice{
            Mode:         types.ToolChoiceModeAllowed,
            AllowedTools: []string{"web_search", "code_executor"},
        },
    },
}
```

如果你现在还需要 `previous_response_id`、`conversation_id`、`cachedContent` 之类字段：

- **现在**：优先通过 `types.AgentConfig.Model` / `types.ExecutionOptions.Model` 写入正式主面
- **边界**：由 `agent/adapters.DefaultChatRequestAdapter` 统一降级到 `types.ChatRequest`
- **后续**：只把 provider-native 深水区字段继续留在 provider 边界翻译，不让业务层直接构造 provider SDK request

---

## 8. 推荐的完善顺序

### Phase 1：补模型目录，不改运行时入口（已完成结构）

已新增统一 `ModelDescriptor / ModelCatalog`，把“最新模型数据”的代码承载结构从 `vendor.Profile.LanguageModels` 一类 fallback map 中解耦出来。下一步可由文档快照或生成脚本注入具体模型数据。

### Phase 2：补正式主面的缺失字段（第一轮已完成）

第一轮已补：

- 对话连续性：`previous_response_id`、`conversation_id`
- thinking 通用模式：`reasoning_mode` / `thought_signatures`
- 输出控制：`include`、`truncation`、`service_tier`、`store`
- 采样与日志概率：`frequency_penalty`、`presence_penalty`、`repetition_penalty`、`n`、`logprobs`、`top_logprobs`、`user`
- 缓存：`prompt_cache_key`、`prompt_cache_retention`、`cache_control`、`cached_content`
- 多模态输出通用字段：`modalities`

第二轮已补（切片 A-D）：

- OpenAI Responses：`verbosity`、`phase`（切片 A）
- Gemini thinking / media：`thinking_level`、`thinking_budget`、`include_thoughts`、`media_resolution`（切片 B）
- Anthropic thinking mode：`thinking_type`（切片 C）
- compat 厂商统一收口：所有 compat hook / validation 统一使用 `resolveCompatThinkingMode()`，`ThinkingType` 优先于 `ReasoningMode`（切片 D）

后续仍需补的 provider-native 深水区字段：

- Gemini safety/media：`speechConfig`、`imageConfig`、`safetySettings`
- OpenAI Responses 更多字段：如后续 API 新增的 `phase` 子类型等

### Phase 3：把 provider-native 校验继续锁在边界

- OpenAI native：`llm/providers/openai`
- Anthropic native：`llm/providers/anthropic`
- Gemini native：`llm/providers/gemini`
- compat 厂商：`llm/providers/vendor/chat_profiles.go`

### Phase 4：补回归测试

至少覆盖：

- OpenAI Responses：`previous_response_id`、`phase`、`allowed_tools`
- Anthropic：`thinking` / `redacted_thinking` / `signature_delta` round-trip
- Gemini：`thinkingLevel` / `thinkingBudget` / `responseJsonSchema` / `thoughtsTokenCount`
- xAI Grok：official alias 与 compat alias 的校验边界
- 工具授权：所有协议入口都必须先经过 `AuthorizationService`

### Phase 5：让文档和代码一起更新

之后凡是新增模型字段，不要只改 provider 代码；至少同步：

- 本文档
- `模型与媒体端点参考.md`
- `近12个月主流多模态模型总表.md`
- `docs/cn/tutorials/02.Provider配置指南.md`

### 8.1 “全部接入”不是“全部 SDK 字段上抬”

这里所说的“全部接入”，应该理解为：

1. **跨厂商可复用的请求语义** 全部进入正式主面 `types.AgentConfig{Model, Control, Tools}`；
2. **provider-native 协议差异** 继续留在 `llm/providers/*` 与 compat profile 边界；
3. **模型目录事实**（型号、Stage、能力、退役时间、来源）进入 `ModelCatalog`；
4. **benchmark / eval 事实** 继续停留在文档快照或目录旁路元数据，不进入 runtime request surface。

一句话版：**接入的是统一语义，不是每家 SDK 的全部原生 struct。**

### 8.2 推荐执行切片（按顺序推进）

建议按下面 5 个切片推进，而不是一次性同时扩多个 provider。

| 切片 | 目标 | 主要改动点 | 完成标志 | 状态 |
|---|---|---|---|---|
| A：补 OpenAI Responses 完整度 | 把 `verbosity`、`phase` 进入正式主面 | `types/execution_options.go`、`agent/adapters/chat.go`、`types/llm_contract.go`、`api/types.go`、`llm/providers/openai/*` | 业务层仅通过 `AgentConfig.Model` 就能表达这两个字段，并正确降级到 Responses 请求 | ✅ 已完成 |
| B：补 Gemini thinking / media 主面 | 把 `thinkingLevel`、`thinkingBudget`、`includeThoughts`、`mediaResolution` 收口到统一语义，并归一化 `thoughtsTokenCount` | `types/execution_options.go`、`agent/adapters/chat.go`、`types/llm_contract.go`、`api/types.go`、`llm/providers/gemini/*` | Gemini thinking / media 不再依赖 provider 内部偷渡字段；usage 中能看到 thoughts token 用量 | ✅ 已完成 |
| C：补 Anthropic thinking mode 语义 | 把 `thinking.type` / adaptive thinking 之类正式收口，同时保持 thinking blocks round-trip | `types/*`、`agent/adapters/chat.go`、`llm/providers/anthropic/*` | Claude 请求侧不再只靠 `ReasoningEffort` 硬映射；响应侧 thinking / redacted_thinking / signature_delta 连续性正确 | ✅ 已完成 |
| D：收口 compat 厂商约束语义 | 把 DeepSeek / Qwen / Kimi / Grok 的模型切换、字段拒绝和 request hook 继续统一在 compat 边界 | `llm/providers/vendor/chat_profiles.go`、`llm/providers/vendor/profile.go` | compat 厂商限制不扩散到 handler / runtime；特殊规则集中、可检索、可测 | ✅ 已完成 |
| E：拆开模型事实与 benchmark 事实 | 用 `ModelCatalog` 承载模型目录事实；benchmark 只保留在 docs 或旁路元数据 | `types/model_catalog.go`、相关 docs、后续可能的 catalog 注入点 | 请求面不出现 benchmark 字段；模型 facts 与运行时参数职责彻底分离 | ✅ 已完成 |

### 8.3 每个切片的最小落地要求

每新增一个 **provider-neutral** 字段，都应至少同步更新以下链路：

- `types.ModelOptions`
- `ModelOptions.clone()`
- `AgentConfig.hasFormalMainFace()`
- `mergeModelOptions(...)`
- `agent/adapters.DefaultChatRequestAdapter.Build(...)`
- `types.ChatRequest`
- `api.ChatRequest`（如果 HTTP 面也要暴露）
- 对应 provider translator / mapper
- `types` 与 `agent/adapters` 的测试

这条规则的目的，是确保**正式主面 > 归一化 > 边界适配 > provider 请求**这条链是完整的，而不是“只在低层 DTO 加字段”。

### 8.4 各切片的推荐顺序与原因

#### 先做 OpenAI，再做 Gemini

推荐先完成切片 A，再进入切片 B：

- OpenAI Responses 语义最稳定，`verbosity` / `phase` 的收口最清晰；
- Gemini 缺口最大，但也最适合在 OpenAI 正式主面补齐后复用统一模式；
- 这样做可以先把主面骨架立住，再承接更复杂的 provider-native 深水区字段。

#### 再做 Anthropic，然后收 compat

- Anthropic 重点不是“加更多字段”，而是把 thinking mode 语义与响应连续性表达完整；
- compat 厂商的重点不是“字段数量”，而是**模型切换规则、约束校验和 request hook 的集中收口**。

换句话说：

> **native provider 先立统一语义，compat provider 再立统一约束。**

### 8.5 不同类型事实应该放哪里

| 事实类型 | 推荐承载位置 | 不应该放哪里 |
|---|---|---|
| 每次请求都要传的统一语义 | `types.AgentConfig{Model, Control, Tools}` | 仅 provider DTO / handler 内部临时结构 |
| provider-native 协议细节 | `llm/providers/*`、`llm/providers/vendor/chat_profiles.go` | `agent/runtime`、`workflow/runtime`、`api/handlers` |
| 模型目录事实（Stage / alias / retire date / capabilities） | `types.ModelCatalog` / `ModelDescriptor` | `vendor.Profile.LanguageModels` 这种 fallback-only map |
| benchmark / eval 事实 | `docs/cn/guides/*` 或目录旁路元数据 | `types.ModelOptions`、`types.ChatRequest`、`api.ChatRequest` |

### 8.6 建议补齐的正式主面字段优先级

按当前官方 API 语义与项目缺口，推荐优先级如下：

1. **OpenAI**：`Verbosity`、`Phase`
2. **Gemini**：`ThinkingLevel`、`ThinkingBudget`、`IncludeThoughts`、`MediaResolution`
3. **Anthropic**：`ThinkingType` / adaptive thinking 统一语义
4. **xAI Grok**：单独的 multi-agent 深度语义（不要继续硬复用 OpenAI 式 `ReasoningEffort`）
5. **目录层**：`ModelCatalog` 的 stage / source / verifiedAt / retire date 结构化注入

其中有一条必须单独强调：

> `grok-4.20-multi-agent` 的 `reasoning.effort` 表示的是 **agent 数量 / 研究深度**，不是 OpenAI 风格的思考深度；因此不要继续把它当成通用 `ReasoningEffort` 文案来硬套。

### 8.7 全部接入完成判定（DoD）

当下面几条同时成立时，才算“全部接入”真正完成：

1. **业务层只写 `AgentConfig`**，不需要直接构造 provider SDK request；
2. **主要 native provider 的统一语义已进入正式主面**，至少覆盖 OpenAI Responses、Gemini thinking/media、Anthropic thinking mode；
3. **compat 厂商的特殊限制统一收口在 provider/profile 边界**，不散落到 runtime / handler；
4. **模型目录事实与 benchmark 事实已经从 request surface 拆开**；
5. **response / usage 已完成必要归一化**，尤其是 thinking signatures、reasoning summaries、thoughts token usage；
6. **测试与文档同步完成**，包括 adapter、provider、architecture guard 与对应中文 / 英文文档。

如果只完成了“provider 能发请求”，但没有完成上面这些链路与边界要求，那最多只能算“局部接通”，不能算真正意义上的“全部接入”。

### 8.8 可直接转开发任务单的执行清单

> 本节把上面的 5 个切片进一步展开成可执行任务单。默认规则：**每个切片单独完成、单独验证、单独同步文档**，不要并发改多个 provider 再统一收尾。

#### 切片 A：补 OpenAI Responses 正式主面

| 项目 | 内容 |
|---|---|
| 目标 | 让 `verbosity`、`phase` 能通过正式主面进入 OpenAI Responses 请求，不再只能靠低层 DTO 或 provider 内部特殊处理。 |
| 最低必改文件 | `types/execution_options.go`、`types/execution_options_test.go`、`types/config.go`、`types/llm_contract.go`、`agent/adapters/chat.go`、`agent/adapters/chat_test.go`、`api/types.go`、`llm/providers/openai/provider.go`、`llm/providers/openai/provider_test.go`、`architecture_guard_test.go` |
| 预期新增字段名 | 建议新增：`types.ModelOptions.Verbosity`、`types.ModelOptions.Phase`；对应降级字段：`types.ChatRequest.Verbosity`、`types.ChatRequest.Phase`；如果 HTTP 面需要透出，再补 `api.ChatRequest.Verbosity`、`api.ChatRequest.Phase` |
| 实施要点 | 同步更新 `ModelOptions.clone()`、`AgentConfig.hasFormalMainFace()`、`mergeModelOptions(...)`、`DefaultChatRequestAdapter.Build(...)`；不要只在 `types.ChatRequest` 加字段 |
| 测试命令 | `go test ./types -run "TestAgentConfigExecutionOptions" -count=1` ；`go test ./agent/adapters -run TestDefaultChatRequestAdapter_BuildMapsFormalModelFields -count=1` ；`go test ./llm/providers/openai -count=1` ；`go test . -run "TestDependencyDirectionGuards|TestAgentExecutionOptionsArchitectureGuards" -count=1` |
| 验收样例 | 给定 `AgentConfig.Model{Provider:"openai", Model:"gpt-5.4", Verbosity:"low", Phase:"commentary"}`，adapter 产出的 `types.ChatRequest` 必须带上对应字段；OpenAI provider 发出的 Responses 请求必须映射为 `text.verbosity="low"` 且 round-trip 保留 assistant `phase` 值 |

#### 切片 B：补 Gemini thinking / media 主面

| 项目 | 内容 |
|---|---|
| 目标 | 把 Gemini 的 thinking / media 控制从 provider 内部偷渡字段收口到正式主面，并把 `thoughtsTokenCount` 归一化到 usage。 |
| 最低必改文件 | `types/execution_options.go`、`types/execution_options_test.go`、`types/config.go`、`types/llm_contract.go`、`agent/adapters/chat.go`、`agent/adapters/chat_test.go`、`api/types.go`、`llm/providers/gemini/provider.go`、`llm/providers/gemini/provider_test.go`、`llm/providers/gemini/provider_extra_test.go`、`architecture_guard_test.go` |
| 联动候选文件 | 若 `mediaResolution` 还走 legacy helper 路径，连带检查 `llm/providers/gemini/legacy_helpers_test.go`；若 HTTP 面想显式暴露字段，联动 `api/openapi.yaml` |
| 预期新增字段名 | 建议新增：`types.ModelOptions.ThinkingLevel`、`types.ModelOptions.ThinkingBudget`、`types.ModelOptions.IncludeThoughts`、`types.ModelOptions.MediaResolution`；usage 侧不新增 request 字段，而是把 Gemini 返回的 `thoughtsTokenCount` 归一化到 `CompletionTokensDetails.ReasoningTokens` |
| 实施要点 | `ThinkingLevel` 与 `ThinkingBudget` 必须保持互斥语义；不要把 Google SDK 的 `GenerationConfig` 原样上抬；`MediaResolution` 先做统一语义，不先引入 `SpeechConfig` / `ImageConfig` 全量结构 |
| 测试命令 | `go test ./types -run "TestAgentConfigExecutionOptions" -count=1` ；`go test ./agent/adapters -run TestDefaultChatRequestAdapter_BuildMapsFormalModelFields -count=1` ；`go test ./llm/providers/gemini -count=1` ；`go test . -run "TestDependencyDirectionGuards|TestAgentExecutionOptionsArchitectureGuards" -count=1` |
| 验收样例 | 给定 `AgentConfig.Model{Provider:"gemini", Model:"gemini-3.1-pro-preview", ThinkingLevel:"high", IncludeThoughts:true, MediaResolution:"media_resolution_high"}`，Gemini provider 构造的请求必须出现对应 thinking / media 配置；若上游返回 `thoughtsTokenCount=5`，归一化后的 usage 必须反映到 `CompletionTokensDetails.ReasoningTokens=5` |

#### 切片 C：补 Anthropic thinking mode 语义

| 项目 | 内容 |
|---|---|
| 目标 | 把 Claude thinking mode 请求语义补齐，同时保持 `thinking` / `redacted_thinking` / `signature_delta` 的 round-trip 不被 runtime 或 handler 打散。 |
| 最低必改文件 | `types/execution_options.go`、`types/execution_options_test.go`、`types/config.go`、`types/llm_contract.go`、`types/message.go`、`agent/adapters/chat.go`、`agent/adapters/chat_test.go`、`api/types.go`、`llm/providers/anthropic/provider.go`、`llm/providers/anthropic/provider_test.go`、`architecture_guard_test.go` |
| 联动候选文件 | 若 API compat 输出面需要同步，联动 `api/handlers/chat_anthropic_compat.go`、`api/handlers/chat_converter.go`、相关 handler tests |
| 预期新增字段名 | 建议新增单一语义字段：`types.ModelOptions.ThinkingType`（例如 `enabled` / `adaptive`）；避免同时新增多个互斥布尔位。若需要输出摘要控制，再评估是否补 `IncludeThinkingSummary` 一类字段 |
| 实施要点 | `ThinkingType` 不等于 `ReasoningEffort`；不能把 Claude adaptive thinking 强塞进 OpenAI 式 effort 语义；响应侧继续复用 `types.Message.ReasoningSummaries`、`OpaqueReasoning`、`ThinkingBlocks` |
| 测试命令 | `go test ./types -run "TestAgentConfigExecutionOptions" -count=1` ；`go test ./agent/adapters -run TestDefaultChatRequestAdapter_BuildMapsFormalModelFields -count=1` ；`go test ./llm/providers/anthropic -count=1` ；`go test . -run "TestDependencyDirectionGuards|TestAgentExecutionOptionsArchitectureGuards" -count=1` |
| 验收样例 | 给定带 `ThinkingBlocks` 和 `OpaqueReasoning` 的 assistant 历史消息，再发起下一轮 Anthropic 请求时，provider 必须原样回传 `thinking` / `redacted_thinking` blocks；流式场景下 `signature_delta` 不能丢失，最终组装出的 `types.Message.ThinkingBlocks[*].Signature` 必须完整 |

#### 切片 D：收口 compat 厂商约束语义

| 项目 | 内容 |
|---|---|
| 目标 | 把 DeepSeek / Qwen / Kimi / Grok 的模型切换、字段拒绝和 request hook 继续统一收口到 compat 边界，不向 runtime / handler 扩散。 |
| 最低必改文件 | `llm/providers/vendor/chat_profiles.go`、`llm/providers/vendor/chat_profiles_hooks_test.go`、`llm/providers/vendor/chat_contract_matrix_test.go`、`llm/providers/vendor/profile.go`、`llm/providers/vendor/profile_test.go` |
| 条件性共享链路文件 | 若本切片同时引入统一 request 语义（例如 Grok multi-agent 深度），再联动 `types/execution_options.go`、`types/execution_options_test.go`、`types/llm_contract.go`、`agent/adapters/chat.go`、`agent/adapters/chat_test.go`、`architecture_guard_test.go` |
| 预期新增字段名 | 默认**不新增**正式主面字段；若确认需要暴露 Grok multi-agent 深度，建议只新增一个统一字段，如 `types.ModelOptions.MultiAgentDepth` 或 `ResearchDepth`，不要继续复用 `ReasoningEffort` |
| 实施要点 | compat 切片的优先目标是**规则集中**，不是字段增多；Qwen thinking + JSON structured output、Kimi thinking + sampling、DeepSeek alias 路由、Grok alias 与字段禁用，都应集中在 compat profile 层 |
| 测试命令 | `go test ./llm/providers/vendor -count=1` ；若新增正式主面字段，再加 `go test ./types -run "TestAgentConfigExecutionOptions" -count=1`、`go test ./agent/adapters -run TestDefaultChatRequestAdapter_BuildMapsFormalModelFields -count=1` ；最后跑 `go test . -run "TestDependencyDirectionGuards|TestAgentExecutionOptionsArchitectureGuards" -count=1` |
| 验收样例 | 1）Qwen thinking + JSON structured output 在本地直接拒绝，不发送上游请求；2）Kimi thinking 模式下不允许的采样字段在 compat 层直接报错；3）空模型的 Grok reasoning 请求仍按 compat 规则路由到 `grok-4.20-reasoning`；4）DeepSeek `deepseek-chat` / `deepseek-reasoner` 的 alias 规则与退役窗口保持集中且可测 |

#### 切片 E：拆开模型事实与 benchmark 事实

| 项目 | 内容 |
|---|---|
| 目标 | 让 `ModelCatalog` 继续承载模型目录事实，同时明确 benchmark / eval 只停留在文档快照或目录旁路元数据，不进入 request surface。 |
| 最低必改文件 | `types/model_catalog.go`、`types/model_catalog_test.go`、`docs/cn/guides/模型字段与Agent框架接入指南.md`、`docs/cn/guides/模型与媒体端点参考.md`、`docs/cn/guides/近12个月主流多模态模型总表.md`、`docs/cn/guides/多模态能力端点参考.md`、`docs/cn/guides/视频与图像厂商及端点说明.md` |
| 联动候选文件 | 如果后续决定做脚本化注入，再评估 `scripts/` 下生成脚本或注入工具；如果中文命名口径有变化，再同步 `docs/cn/guides/模型厂商与模型中文命名规范.md` |
| 预期新增字段名 | **request 面：无新增字段。** 目录层优先复用已有 `ModelDescriptor` 字段（如 `Stage`、`ReleaseDate`、`RetiresAt`、`VerifiedAt`、`SourceURLs`、`Metadata`）；如果未来确需结构化 benchmark，新增也只能发生在 catalog 层（例如 `BenchmarkFacts`），不能进入 `ModelOptions` / `ChatRequest` |
| 实施要点 | `vendor.Profile.LanguageModels` 仍只做 fallback；不要把 benchmark、榜单、退役时间等目录事实重新塞回 provider profile 或 request DTO |
| 测试命令 | `go test ./types -run "TestModelCatalog" -count=1` ；如 catalog 注入影响到 execution options 合并逻辑，再加 `go test ./types -run "TestAgentConfigExecutionOptions" -count=1` ；必要时跑 `go test . -run "TestDependencyDirectionGuards|TestAgentExecutionOptionsArchitectureGuards" -count=1` |
| 验收样例 | `catalog.Lookup("openai", "gpt-5.4")` 返回的 descriptor 必须带上 stage / source / verifiedAt 等目录事实，且返回的是 clone；同时 `types.ModelOptions`、`types.ChatRequest`、`api.ChatRequest` 中不应出现 benchmark 相关字段 |

### 8.9 建议的任务拆分粒度

如果要把上面内容直接转成开发任务单，推荐 1 个切片 = 1 个任务目录 / 1 个 PR / 1 组测试证据。不要把 A+B+C+D+E 一次性打成“大一统改造”，否则：

- 回归失败时很难定位是哪一层断了；
- 文档、provider、adapter、catalog 的职责容易一起漂移；
- 架构守卫一旦报错，修复成本会明显上升。

最稳妥的推进顺序仍然是：

`A(OpenAI) -> B(Gemini) -> C(Anthropic) -> D(compat) -> E(catalog/docs)`

每完成一个切片，就同步更新本节对应任务单的状态与验证证据。

---

## 9. 一条必须守住的边界

**不要把 provider SDK 的原生 request / response struct 直接上抬到 `agent/runtime`、`workflow/runtime`、`api/handlers`。**

因为一旦这么做，最后一定会出现：

- OpenAI 一套字段
- Anthropic 一套字段
- Gemini 一套字段
- compat 厂商再来一套字段

然后你的 Agent 框架会从“统一主面”退化成“多厂商 DTO 大杂烩”。

正确做法永远是：

> `Model / Control / Tools` 负责统一语义，`ChatRequestAdapter` 和 provider adapter 负责协议翻译。

---

## 10. 官方来源与代码事实

### 官方来源

- OpenAI: <https://developers.openai.com/api/docs/guides/latest-model>, <https://developers.openai.com/api/reference/resources/responses/methods/create>
- Anthropic: <https://platform.claude.com/docs/en/about-claude/models/overview>, <https://platform.claude.com/docs/en/api/messages>, <https://platform.claude.com/docs/en/api/models/list>, <https://platform.claude.com/docs/en/docs/build-with-claude/extended-thinking>
- Gemini: <https://ai.google.dev/gemini-api/docs/models>, <https://ai.google.dev/api/generate-content>, <https://ai.google.dev/api/rest/v1beta/GenerationConfig>, <https://ai.google.dev/gemini-api/docs/thinking>
- xAI: <https://docs.x.ai/docs/models/>, <https://docs.x.ai/developers/model-capabilities/text/reasoning>
- DeepSeek: <https://api-docs.deepseek.com/updates/>

### 本仓库代码事实

- 正式运行时主面：`types/config.go`
- 运行时归一化：`types/execution_options.go`
- 边界适配器：`agent/adapters/chat.go`
- 低层 LLM 契约：`types/llm_contract.go`
- API 入站 DTO：`api/types.go`
- compat 厂商 hook / validation：`llm/providers/vendor/chat_profiles.go`
- provider 语言 fallback：`llm/providers/vendor/profile.go`
- 工具授权主链：`internal/usecase/authorization_service.go`
