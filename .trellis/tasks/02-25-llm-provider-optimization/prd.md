# LLM Provider 全面优化：模型更新、Reasoning Mode、错误处理

## 目标

根据各 LLM 供应商最新官方文档，对项目中 14 个 provider 实现进行针对性优化。涵盖默认模型更新、reasoning mode 支持、provider-specific 参数处理和错误码映射。

---

## 批次 1：默认模型更新 + DeepSeek Reasoner 修复（低风险）

### 1.1 默认模型更新

更新以下 provider 的 fallback 默认模型：

| Provider | 当前默认模型 | 更新为 | 原因 |
|----------|-------------|--------|------|
| Grok | `grok-beta` | `grok-3` | grok-beta 已过时，grok-3 是稳定的最新通用模型 |
| MiniMax | `abab6.5s-chat` | `MiniMax-Text-01` | abab6.5s 已过时，Text-01 是稳定的新一代模型 |
| Kimi | `moonshot-v1-8k` | `moonshot-v1-32k` | 8k context 太小，32k 是更合理的默认值 |
| Llama | `meta-llama/Llama-3-70b-chat-hf` | `meta-llama/Llama-3.3-70B-Instruct-Turbo` | Llama 3.3 是最新稳定版 |
| Hunyuan | `hunyuan-pro` | `hunyuan-turbos-latest` | hunyuan-pro 已下线，turbos 是推荐的通用模型 |

保持不变的 provider（已是合理默认值）：
- DeepSeek: `deepseek-chat` ✅
- Qwen: `qwen3-235b-a22b` ✅（或考虑更新为 `qwen3-max`）
- GLM: `glm-4-plus` ✅
- Mistral: `mistral-large-latest` ✅（latest alias 自动指向最新版）
- Doubao: `Doubao-1.5-pro-32k` ✅

### 1.2 DeepSeek Reasoner 参数修复

**问题**：DeepSeek reasoner 模型不支持 `temperature`、`top_p`、`presence_penalty`、`frequency_penalty` 参数。当前 RequestHook 只切换模型名，未剥离这些参数。

**修复**：在 `deepseekRequestHook` 中，当切换到 reasoner 时：
- 将 `Temperature` 设为 0（零值，JSON omitempty 会忽略）
- 将 `TopP` 设为 0
- 清空不支持的参数

**问题**：多轮对话中，reasoner 返回的 `reasoning_content` 不应被拼接到后续请求的 context 中，否则 API 返回 400。

**修复**：在 `ConvertMessagesToOpenAI` 或 DeepSeek 的 RequestHook 中，检测并剥离 assistant 消息中的 reasoning_content 字段。

### 验收标准
- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 无新增警告
- [ ] 所有现有测试通过
- [ ] 每个 provider 的默认模型已更新

---

## 批次 2：Reasoning Mode 支持（中等风险）

### 2.1 统一 Reasoning Mode RequestHook 模式

参照 DeepSeek 的 `deepseekRequestHook` 模式，为以下 provider 添加 reasoning mode 支持：

#### Grok (xAI)
- 当 `ReasoningMode == "thinking"` 或 `"extended"` 时，切换模型为对应的 reasoning 变体
- 映射规则：默认 → `grok-3-mini`（reasoning 模型）
- 如果用户已指定 grok-4 系列模型，切换到对应的 reasoning 变体

#### GLM (智谱)
- 当 `ReasoningMode == "thinking"` 或 `"extended"` 时，切换模型为 `glm-z1-flash`
- GLM-Z1 系列是智谱的 reasoning 模型

#### Kimi (月之暗面)
- 当 `ReasoningMode == "thinking"` 或 `"extended"` 时，切换模型为 `k1`
- k1 是 Kimi 的 reasoning 模型

#### Qwen (通义千问)
- Qwen3 系列原生支持 thinking mode，通过 `enable_thinking: true` 参数控制
- 当 `ReasoningMode == "thinking"` 或 `"extended"` 时，在请求体中添加 `enable_thinking: true`
- 这需要在 RequestHook 中修改请求体

#### Mistral
- 当 `ReasoningMode == "thinking"` 或 `"extended"` 时，切换模型为 `magistral-medium-latest`
- Magistral 系列是 Mistral 的 reasoning 模型

#### Hunyuan (腾讯混元)
- 当 `ReasoningMode == "thinking"` 或 `"extended"` 时，切换模型为 `hunyuan-t1`
- Hunyuan-T1 和 hunyuan-a13b 支持 thinking chain

### 验收标准
- [ ] 每个 provider 的 reasoning mode 切换正确
- [ ] `go build ./...` 通过
- [ ] 所有现有测试通过

---

## 批次 3：Provider 特定优化（中等风险）

### 3.1 MiniMax 新模型兼容

- 检查 MiniMax 新模型（M2.5, M2, M1, Text-01）是否仍使用 XML tool call 格式
- 如果新模型已支持标准 JSON tool calling，使 XML 解析仅对旧模型（abab 系列）生效
- 在 StreamOverride 中添加模型名判断逻辑

### 3.2 Provider-Specific 错误码映射

在 `common.go` 的 `MapHTTPError` 中增加 provider-specific 错误处理：

#### DeepSeek 特定错误
- 400 + `reasoning_content` 相关 → 提示需要剥离 reasoning_content
- 402 → `ErrQuotaExceeded`（余额不足）

#### 通用增强
- 408 → `ErrTimeout`（请求超时，retryable）
- 413 → `ErrInvalidRequest`（请求体过大）
- 422 → `ErrInvalidRequest`（参数验证失败）

### 3.3 Hunyuan Function Calling 路由

- 当请求包含 tools 且模型不是 `hunyuan-functioncall` 时，考虑自动路由到 function calling 专用模型
- 通过 RequestHook 实现

### 验收标准
- [ ] MiniMax 新旧模型都能正确处理 tool calling
- [ ] 错误码映射更精确
- [ ] `go build ./...` 通过
- [ ] 所有现有测试通过

---

## 技术说明

- 所有 RequestHook 修改遵循现有 DeepSeek 的模式
- 不修改 `openaicompat.Provider` 基类接口
- 每个批次独立可交付
- 优先保证向后兼容：用户已配置的模型名不受影响（只改 fallback 默认值）
