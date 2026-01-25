# Provider 层 2026 年更新指南

## 重大变化总览

### 1. OpenAI - Responses API 迁移（重要）

**Assistant API 将于 2026年8月26日下线**

- ✅ **新 API**: Responses API (`/v1/responses`)
- ❌ **废弃**: Assistants API (2026-08-26 下线)
- 🆕 **GPT-5**: 272K context, $1.25/M tokens
- 🆕 **内置工具**: Web Search, File Search, Computer Use

**迁移要点**:
```go
// 启用 Responses API
cfg := providers.OpenAIConfig{
    APIKey: "sk-xxx",
    UseResponsesAPI: true, // 新增配置
}
```

**Thought Signatures**: 新 API 返回加密的推理签名，需在后续调用中传递以保持推理链。

### 2. Claude (Anthropic) - 重大升级

**模型更新**:
- ✅ **Claude Opus 4.5**: $5/$25 (降价66%), 1M context
- ✅ **Claude Sonnet 4.5**: 混合推理模式
- ✅ **Claude Haiku 4.5**: $1/$5 (降价67%)
- ❌ **Claude Opus 3**: 2026-01-05 已下线

**新特性**:
- 1M tokens 上下文窗口 (5x 提升)
- 混合推理架构 (快速/深度思考模式)
- 改进的工具编排
- Memory Tool 支持

**API 变化**:
```go
// 需要更新模型名称
req := &llm.ChatRequest{
    Model: "claude-opus-4.5-20260105", // 新版本
}
```

### 3. Gemini (Google) - Gemini 3 发布

**重大更新**:
- 🆕 **Gemini 3 Pro**: 最智能模型
- 🆕 **Gemini 3 Flash**: 闪电速度
- 🆕 **Thought Signatures**: 必须传递以保持推理链
- 🆕 **media_resolution**: 精细控制多模态 token 使用

**API 变化**:
```go
// Thought Signatures (必需)
type GeminiRequest struct {
    // ... 其他字段
    ThoughtSignatures []string `json:"thought_signatures,omitempty"`
}

// Media Resolution 控制
type MediaConfig struct {
    Resolution string `json:"resolution"` // "low", "medium", "high"
}
```

**上下文窗口**:
- Gemini 3 Pro: 1M tokens
- Gemini 1.5 Pro: 2M tokens (部分工作流)

### 4. DeepSeek - V3.1 混合推理

**模型更新**:
- ✅ **DeepSeek-V3.1-Terminus**: 最新版本
- ✅ **混合推理**: `deepseek-chat` (快速) / `deepseek-reasoner` (思考)
- 🆕 **Agent 能力**: Code Agent, Search Agent

**性能提升**:
- AIME 2025: 70.0 → 87.5 (+17.5)
- GPQA: 71.5 → 81.0 (+9.5)
- SWE-bench: 66.0

**API 使用**:
```go
// 快速模式
req := &llm.ChatRequest{
    Model: "deepseek-chat",
}

// 推理模式
req := &llm.ChatRequest{
    Model: "deepseek-reasoner",
}
```

### 5. Qwen (通义千问) - Qwen 3 发布

**重大更新**:
- 🆕 **Qwen 3**: 2026-04-29 发布
- 🆕 **Qwen3-235B-A22B**: 旗舰模型
- 🆕 **Qwen3-Coder-480B**: 代码专用
- 📈 **上下文**: 256K native, 1M with extrapolation

**训练数据**: 36 trillion tokens (2x Qwen2.5)

**API 兼容**: 完全兼容 OpenAI 格式

### 6. Mistral AI - Mistral 3 系列

**新模型**:
- 🆕 **Mistral Large 3**: 675B total, 41B active (MoE)
- 🆕 **Mistral 3 (14B/8B/3B)**: 密集模型
- 🆕 **视觉支持**: 多模态能力
- 🆕 **推理模式**: 2025-09 更新

**特性**:
- OpenAI 兼容 API
- 原生 Function Calling
- OCR API (table_format, hyperlinks)

## 上下文窗口对比 (2026)

| Provider | Model | Context | 实际可用 |
|----------|-------|---------|---------|
| OpenAI | GPT-5.2 | 272K | ~180K |
| Claude | Opus 4.5 | 1M | ~850K |
| Gemini | 3 Pro | 1M | ~850K |
| Gemini | 1.5 Pro | 2M | ~1.7M |
| DeepSeek | V3.1 | 128K | ~100K |
| Qwen | Qwen3 | 256K-1M | ~200K-850K |
| Llama | 4 Scout | 10M | ~8.5M |

**注意**: 实际可用约为宣传值的 85%

## 定价变化 (2026)

### 降价趋势

| Provider | Model | 旧价格 | 新价格 | 降幅 |
|----------|-------|--------|--------|------|
| Claude | Opus 4.5 | $15/$75 | $5/$25 | -66% |
| Claude | Haiku 4.5 | $3/$15 | $1/$5 | -67% |
| OpenAI | GPT-5 | - | $1.25/M | 新模型 |

## 必须更新的代码

### 1. OpenAI Provider

```go
// providers/openai/provider.go

// 添加 Thought Signatures 支持
type openAIRequest struct {
    // ... 现有字段
    PreviousResponseID string `json:"previous_response_id,omitempty"` // 新增
    Store              bool   `json:"store,omitempty"`                // 新增
}

// 添加 Responses API 端点
func (p *OpenAIProvider) completionWithResponsesAPI(ctx context.Context, req *llm.ChatRequest, apiKey string) (*llm.ChatResponse, error) {
    // 已实现
}
```

### 2. Claude Provider

```go
// providers/anthropic/provider.go

// 更新默认模型
const (
    DefaultModel = "claude-opus-4.5-20260105" // 更新
)

// 添加混合推理模式支持
type ClaudeRequest struct {
    // ... 现有字段
    ReasoningMode string `json:"reasoning_mode,omitempty"` // "fast" | "extended"
}
```

### 3. Gemini Provider

```go
// providers/gemini/provider.go

// 添加 Thought Signatures
type GeminiRequest struct {
    // ... 现有字段
    ThoughtSignatures []string      `json:"thought_signatures,omitempty"`
    MediaResolution   *MediaConfig  `json:"media_resolution,omitempty"`
}

type MediaConfig struct {
    Resolution string `json:"resolution"` // "low", "medium", "high"
}
```

### 4. DeepSeek Provider

```go
// providers/deepseek/provider.go

// 更新默认模型
const (
    DefaultChatModel     = "deepseek-chat"     // V3.1-Terminus
    DefaultReasonerModel = "deepseek-reasoner" // V3.1-Terminus Think
)
```

### 5. Qwen Provider

```go
// providers/qwen/provider.go

// 更新默认模型
const (
    DefaultModel = "qwen3-235b-a22b" // 更新到 Qwen3
)

// 支持超长上下文
func (p *QwenProvider) supportsExtendedContext() bool {
    return true // 256K-1M
}
```

## 新增功能支持

### 1. 混合推理模式

```go
// llm/types.go

type ChatRequest struct {
    // ... 现有字段
    ReasoningMode string `json:"reasoning_mode,omitempty"` // "fast" | "extended" | "thinking"
}
```

### 2. Thought Signatures

```go
// llm/types.go

type ChatRequest struct {
    // ... 现有字段
    ThoughtSignatures []string `json:"thought_signatures,omitempty"`
}

type ChatResponse struct {
    // ... 现有字段
    ThoughtSignatures []string `json:"thought_signatures,omitempty"`
}
```

### 3. 多模态分辨率控制

```go
// llm/types.go

type MediaResolution struct {
    Resolution string `json:"resolution"` // "low", "medium", "high"
    MaxTokens  int    `json:"max_tokens,omitempty"`
}

type ChatRequest struct {
    // ... 现有字段
    MediaResolution *MediaResolution `json:"media_resolution,omitempty"`
}
```

## 迁移检查清单

### 高优先级 (必须)

- [ ] OpenAI: 迁移到 Responses API (2026-08-26 前)
- [ ] Claude: 更新模型名称到 4.5 系列
- [ ] Gemini: 添加 Thought Signatures 支持
- [ ] 所有: 更新上下文窗口限制

### 中优先级 (推荐)

- [ ] 添加混合推理模式支持
- [ ] 实现 Thought Signatures 传递
- [ ] 更新默认模型到最新版本
- [ ] 添加多模态分辨率控制

### 低优先级 (可选)

- [ ] 优化超长上下文处理
- [ ] 添加新模型的特定优化
- [ ] 实现成本优化策略

## 测试建议

### 1. 兼容性测试

```bash
# 测试所有 Provider
go test ./providers/... -v

# 测试集成
go test ./tests/integration/... -v
```

### 2. 上下文窗口测试

```go
func TestLongContext(t *testing.T) {
    providers := []struct {
        name    string
        maxCtx  int
        safeCtx int
    }{
        {"gpt-5", 272000, 230000},
        {"claude-opus-4.5", 1000000, 850000},
        {"gemini-3-pro", 1000000, 850000},
    }
    
    for _, p := range providers {
        t.Run(p.name, func(t *testing.T) {
            // 测试接近上限的上下文
        })
    }
}
```

### 3. 新特性测试

```go
func TestThoughtSignatures(t *testing.T) {
    // 测试 Thought Signatures 传递
}

func TestHybridReasoning(t *testing.T) {
    // 测试混合推理模式
}
```

## 性能优化建议

### 1. 上下文管理

```go
// 实现智能上下文压缩
func (m *ContextManager) CompressForProvider(ctx string, provider string) string {
    limits := map[string]int{
        "gpt-5":          230000,
        "claude-opus-4.5": 850000,
        "gemini-3-pro":    850000,
    }
    
    maxTokens := limits[provider]
    if len(ctx) > maxTokens {
        return m.compress(ctx, maxTokens)
    }
    return ctx
}
```

### 2. 成本优化

```go
// 根据任务选择最优模型
func SelectOptimalModel(task Task) string {
    if task.RequiresReasoning {
        return "deepseek-reasoner" // 便宜且强大
    }
    if task.RequiresSpeed {
        return "gemini-3-flash" // 最快
    }
    if task.RequiresQuality {
        return "claude-opus-4.5" // 最好
    }
    return "gpt-5" // 平衡
}
```

## 参考资源

- [OpenAI Responses API](https://developers.openai.com/blog/responses-api)
- [Claude 4.5 Release](https://www.anthropic.com/news/claude-opus-4-5)
- [Gemini 3 Guide](https://ai.google.dev/gemini-api/docs/gemini-3)
- [DeepSeek V3.1 Updates](https://api-docs.deepseek.com/updates/)
- [Qwen 3 Release](https://github.com/QwenLM/Qwen3)
- [Mistral 3 Announcement](https://mistral.ai/news/mistral-3)

## 更新时间线

| 日期 | 事件 |
|------|------|
| 2025-12 | GPT-5.2, Gemini 3 发布 |
| 2026-01-05 | Claude Opus 3 下线 |
| 2026-04-29 | Qwen 3 发布 |
| 2026-05-28 | DeepSeek V3.1 发布 |
| 2026-08-26 | OpenAI Assistants API 下线 ⚠️ |

---

**最后更新**: 2026-01-26
