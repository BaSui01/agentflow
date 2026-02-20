# brainstorm: LLM Provider 层重构 - 消除重复代码

## 目标

重构 `llm/providers/` 下的 13 个 LLM Provider 实现，消除约 1,292 行重复代码，统一错误处理、类型定义、消息转换、流式处理等通用逻辑，同时保持各 Provider 的差异化能力。

## 已知信息

### Provider 分类

* **直接实现型（9 个）**：OpenAI, Anthropic, Gemini, DeepSeek, Qwen, GLM, Grok, Doubao, MiniMax — 各自独立实现所有方法
* **继承型（4 个）**：Hunyuan, Kimi, Llama, Mistral — 嵌入 `*openai.OpenAIProvider` 并覆盖部分方法

### 更精确的重复代码统计（研究后修正）

| 类别 | 重复次数 | 浪费行数 |
|------|---------|---------|
| 类型定义（openAIMessage 等 9 个类型） | 6 个 Provider | ~360 行 |
| 消息转换函数 convertMessages() | 7 个 Provider | ~175 行 |
| 工具转换函数 convertTools() | 6 个 Provider | ~105 行 |
| 响应转换函数 toChatResponse() | 6 个 Provider | ~120 行 |
| 错误映射函数 mapError() | 6 个 Provider | ~160 行 |
| 错误读取函数 readErrMsg() | 6 个 Provider | ~64 行 |
| SSE 流式处理逻辑 | 6 个 Provider | ~350 行 |
| Credential Override 处理 | 10+ 个 Provider | ~78 行 |
| RewriterChain 初始化样板 | 10+ 个 Provider | ~80 行 |
| **总计** | | **~1,492 行** |

### 已有但未充分利用的通用代码

`llm/providers/common.go` 已经包含了完整的解决方案，但没人用：

* `common.go:16-84` — `MapHTTPError()` 通用 HTTP 错误映射
* `common.go:86-112` — `ReadErrorMessage()` 通用错误消息读取
* `common.go:114-191` — `OpenAICompat*` 类型定义
* `common.go:193-250` — `ConvertMessagesToOpenAI()` 消息转换
* `common.go:252-270` — `ConvertToolsToOpenAI()` 工具转换
* `common.go:272-278` — `ToLLMChatResponse()` 响应转换
* `common.go:298-339` — `ListModelsOpenAICompat()` 模型列表

注释明确写道：*"各提供者包目前定义了自己的副本；未来的重构可以统一这些定义."*

### Config 结构体重复

`llm/providers/config.go` 中 10/12 个 Config 结构体字段完全相同（APIKey, BaseURL, Model, Timeout），仅 OpenAIConfig 多了 Organization + UseResponsesAPI，LlamaConfig 多了 Provider。

### 已知代码异味

1. 所有 Provider 忽略 `json.Marshal()` 错误（10+ 处 `payload, _ := json.Marshal(body)`）
2. 错误处理不一致：OpenAI/Anthropic/Gemini 用 `providers.MapHTTPError()`，其他 6 个用自己的 `mapError()`
3. 流式处理中资源清理不一致（有的不关闭 resp.Body）
4. 超时设置不一致（Claude/Gemini 60s，其他 30s）— 这是有意为之的

### 业界参考

* **langchaingo**：OpenAI 兼容 Provider 直接导入 openai 子包并传不同 BaseURL，不复制代码
* **go-openai**：`ClientConfig.BaseURL` 字段让任何 OpenAI 兼容 API 复用同一客户端
* **kimi/provider.go**（本项目）：已经用嵌入模式将 Provider 缩减到 97 行，证明方案可行

## 假设（已验证）

* ✅ OpenAI 兼容的 Provider（DeepSeek, Qwen, GLM, Grok, Doubao, MiniMax）可以共享同一套类型定义和转换函数 — `common.go` 已有完整实现
* ✅ Anthropic 和 Gemini 因 API 格式差异较大，需要保留独立实现
* ✅ 继承型 Provider（Hunyuan, Kimi, Llama, Mistral）的嵌入模式可以保留并作为参考
* ✅ kimi/provider.go（97 行）已证明嵌入模式可行

## 待解决问题

（已全部解决）

## 决策（ADR-lite）

**背景**：13 个 LLM Provider 中有 ~1,492 行重复代码，`common.go` 已有共享函数但未被使用。

**决策**：选择方案 A — OpenAI 兼容基础 Provider（组合模式）。提取 `openaicompat.Provider` 到新包，OpenAI 兼容 Provider 嵌入它。

**后果**：
* 消除 ~1,400+ 行重复代码
* 新增 OpenAI 兼容 Provider 只需 ~30-50 行
* Anthropic/Gemini 保持独立但修复代码异味
* 需要为 DeepSeek ReasoningMode 等特殊功能设计 hook 机制
* 继承型 Provider（Kimi 等）可能需要调整嵌入目标（从 openai.OpenAIProvider → openaicompat.Provider）

## 需求

### Phase 1：提取 openaicompat 基础 Provider
* 创建 `llm/providers/openaicompat/` 包
* 将 `common.go` 中的 `OpenAICompat*` 类型、`ConvertMessagesToOpenAI`、`ConvertToolsToOpenAI`、`ToLLMChatResponse` 移入
* 实现完整的 `Provider` 接口（Completion, Stream, HealthCheck, Name, ListModels, SupportsNativeFunctionCalling）
* 提取通用 SSE 流式处理为 `StreamSSE()` 函数
* 提取通用 Credential Override 处理
* 提取通用 RewriterChain 初始化
* 支持 hook 机制：`RequestHook func(*ChatRequest) *ChatRequest` 用于 Provider 特殊字段（如 DeepSeek ReasoningMode）

### Phase 2：迁移 OpenAI 兼容 Provider
* 迁移 OpenAI Provider 使用 openaicompat（保留 Responses API 作为覆盖）
* 迁移 DeepSeek Provider（使用 RequestHook 处理 ReasoningMode）
* 迁移 Qwen, GLM, Grok, Doubao Provider
* 迁移 MiniMax Provider（可能需要响应格式 hook）
* 更新继承型 Provider（Kimi, Hunyuan, Llama, Mistral）嵌入目标

### Phase 3：修复代码异味
* 修复所有 `json.Marshal()` 错误忽略（10+ 处）
* 统一错误处理：所有 Provider 使用 `MapHTTPError()` + `ReadErrorMessage()`
* 修复流式处理中的资源清理不一致
* Anthropic/Gemini 独立实现中也修复相同的代码异味

### Phase 4：清理和验证
* 删除各 Provider 中不再需要的本地类型定义、转换函数、错误处理函数
* 更新 `common.go` — 移除已迁移到 openaicompat 的代码，保留非 OpenAI 兼容的通用函数
* 运行完整测试套件
* 更新 spec 文档

## 验收标准

* [ ] `llm/providers/openaicompat/` 包存在并实现完整 `Provider` 接口
* [ ] OpenAI 兼容 Provider（6 个）嵌入 `openaicompat.Provider`，每个 ≤100 行
* [ ] 消息转换、工具转换、响应转换使用 openaicompat 共享函数
* [ ] 错误处理统一使用 `MapHTTPError()` + `ReadErrorMessage()`
* [ ] SSE 流式处理使用 openaicompat 的 `StreamSSE()` 函数
* [ ] `json.Marshal()` 错误被正确处理（零 `_, _ = json.Marshal` 模式）
* [ ] 流式处理中 `resp.Body` 始终被正确关闭
* [ ] DeepSeek ReasoningMode 通过 hook 机制正常工作
* [ ] OpenAI Responses API 通过方法覆盖正常工作
* [ ] 继承型 Provider（Kimi, Hunyuan, Llama, Mistral）正常工作
* [ ] Anthropic 和 Gemini 独立实现中代码异味已修复
* [ ] `make lint` 通过
* [ ] `make test` 通过（含 race detector）
* [ ] 净减少代码行数 ≥ 1,000 行

## 完成定义（团队质量标准）

* 添加/更新了测试（单元/集成，视情况而定）
* `make lint` 通过
* `make test` 通过（含 race detector）
* 如果行为变更则更新文档/笔记
* 更新 `.trellis/spec/backend/quality-guidelines.md` 中的 Provider 实现模式

## 范围外（明确）

* 不改变 Provider 接口定义（`llm/provider.go`）
* 不改变 Router 层逻辑
* 不添加新的 Provider
* 不改变 API 行为或响应格式

## 技术说明

* Provider 接口：`llm/provider.go:54-73`（6 个方法）
* Provider 工厂：`llm/provider_wrapper.go:67-96`
* 通用代码：`llm/providers/common.go`（已有完整的共享函数，未被使用）
* Config 定义：`llm/providers/config.go`（12 个 Config 结构体）
* 13 个 Provider 实现：`llm/providers/*/provider.go`
* 中间件链：`llm/middleware.go` + `llm/middleware/rewriter.go`
* 重试装饰器：`llm/providers/retry_wrapper.go`
* 继承型 Provider 嵌入 `*openai.OpenAIProvider`

## 研究笔记

### 类似工具的做法

* **langchaingo**：OpenAI 兼容 Provider 直接导入 openai 子包传不同 BaseURL，不复制代码
* **go-openai**：`ClientConfig.BaseURL` 让任何 OpenAI 兼容 API 复用同一客户端；内部用 `requestBuilder` 统一 HTTP 层
* **ollama**：OpenAI 兼容层是纯适配器/翻译器，不是 Provider 实现

### 我们仓库/项目的约束

* `common.go` 已有完整的共享函数（`ConvertMessagesToOpenAI` 等），只需让 Provider 使用它们
* `kimi/provider.go`（97 行）已证明嵌入模式可行
* Anthropic 和 Gemini 的 API 格式完全不同，必须保留独立实现
* DeepSeek 有 `ReasoningMode` 等特殊字段，需要 hook 点
* OpenAI 有 Responses API（2025 年 3 月），需要保留扩展能力

### 这里可行的方案

**方案 A：OpenAI 兼容基础 Provider（组合模式）**（推荐）

* 工作方式：提取 `openaicompat.Provider` 到 `llm/providers/openaicompat/`，实现完整 `Provider` 接口。OpenAI 兼容 Provider（DeepSeek, Qwen, GLM, Grok, Doubao, MiniMax）嵌入它，只覆盖差异化部分（Name, BaseURL, 默认模型, 自定义 Header）。Anthropic/Gemini 保持独立。
* 优点：最大程度消除重复（~1,400+ 行）；新增 Provider 只需 ~30-50 行；已有 kimi 验证可行性；`common.go` 的共享函数直接被 openaicompat 使用
* 缺点：嵌入模式下 Stream() 可能多一层 channel hop；Provider 特殊字段（如 DeepSeek ReasoningMode）需要 hook 机制

**方案 B：共享 HTTP 层 + 工具函数（函数组合模式）**

* 工作方式：扩展 `common.go`，添加 `DoCompletion()` 和 `DoStream()` 通用 HTTP 函数。每个 Provider 保持独立结构体，但调用共享函数处理 HTTP、SSE 解析、错误映射。
* 优点：低耦合，Provider 保持独立；渐进式迁移友好；不改变现有包结构
* 缺点：消除重复不如方案 A 彻底（~800 行）；每个 Provider 仍需 ~150 行样板代码；构造函数和 RewriterChain 初始化仍然重复

**方案 C：策略模式（Template Method）**

* 工作方式：定义 `ProviderStrategy` 接口（Name, BaseURL, BuildHeaders, TransformRequest, ParseResponse, ParseStreamChunk），一个 `GenericProvider` 使用策略实现 `Provider` 接口。
* 优点：最高抽象度；新增 Provider 只需实现策略接口
* 缺点：抽象成本最高；策略接口设计需要非常小心以适应所有 Provider 的差异；过度设计风险（YAGNI）
