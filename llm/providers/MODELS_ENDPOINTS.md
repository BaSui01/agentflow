# Provider Models Endpoints 参考文档

> 更新时间：2026年3月22日

本文档记录了各个 LLM Provider 的 `/models` 端点信息和最新模型列表。

---

## ✅ 已验证的 Provider 端点

### 1. OpenAI
- **端点**: `GET /v1/models`
- **Base URL**: `https://api.openai.com`
- **认证方式**: Bearer Token
- **最新模型**:
  - `gpt-4o` - 最新的 GPT-4 Omni 模型（支持文本和视觉）
  - `gpt-4o-mini` - 轻量级 GPT-4o 模型
  - `gpt-4-turbo` - GPT-4 Turbo（支持视觉和函数调用）
  - `gpt-3.5-turbo` - 经典 GPT-3.5 模型
  - `o1` - 推理模型系列
  - `o1-mini` - 轻量级推理模型
- **多模态支持**: ✅ 视觉（gpt-4o, gpt-4-turbo）

---

### 2. Anthropic Claude
- **端点**: `GET /v1/models`
- **Base URL**: `https://api.anthropic.com`
- **认证方式**: x-api-key Header
- **响应格式**:
  ```json
  {
    "data": [
      {
        "id": "claude-opus-4-20250514",
        "created_at": "2025-05-14T00:00:00Z",
        "display_name": "Claude Opus 4",
        "type": "model"
      }
    ],
    "has_more": false,
    "first_id": "...",
    "last_id": "..."
  }
  ```
- **最新模型**:
  - `claude-opus-4-20250514` - 最强大的 Claude 模型
  - `claude-sonnet-4-20250514` - 平衡性能和成本
  - `claude-haiku-4-20250514` - 最快速且经济
  - `claude-3.5-sonnet` - Claude 3.5 系列
- **多模态支持**: ✅ 视觉（所有 Claude 4 模型）
- **特殊功能**:
  - ✅ Computer Use (UI 自动化)
  - ✅ Extended Thinking (深度推理)
  - ✅ Code Execution (代码执行)

---

### 3. Google Gemini
- **端点**: `GET /v1beta/models`
- **Base URL**: `https://generativelanguage.googleapis.com`
- **认证方式**: x-goog-api-key Header
- **最新模型**:
  - **Gemini 3 系列** (最新):
    - `gemini-3-pro` - 最先进的推理模型
    - `gemini-3-flash` - 高性价比模型
    - `gemini-3.1-pro` - 高级智能和复杂问题解决
  - **Gemini 2.5 系列** (生产就绪):
    - `gemini-2.5-pro` - 复杂任务的最先进模型
    - `gemini-2.5-flash` - 低延迟、高吞吐量任务
    - `gemini-2.5-flash-lite` - 最快且最经济的多模态模型
  - **Gemini 2.0 系列**:
    - `gemini-2.0-flash-exp` - 实验版本
    - `gemini-2.0-flash-live` - 低延迟双向语音和视频
- **多模态支持**:
  - ✅ 视觉（所有 Gemini 模型）
  - ✅ 音频（Gemini 2.5 Flash Live）
  - ✅ 视频（Gemini 2.5 Flash Live）
- **生成媒体模型**:
  - `veo-3.1` - 视频生成
  - `imagen-4` - 图像生成（最高 2K 分辨率）
  - `nano-banana-pro` - 4K 图像生成
  - `lyria` - 音乐生成

---

### 4. DeepSeek
- **端点**: `GET /models` 或 `GET /v1/models`
- **Base URL**: `https://api.deepseek.com`
- **认证方式**: Bearer Token
- **最新模型**:
  - `deepseek-chat` - DeepSeek-V3.2 非推理模式
  - `deepseek-reasoner` - DeepSeek-V3.2 推理模式
- **上下文长度**: 128K tokens
- **多模态支持**: ❌ 仅文本

---

### 5. Qwen (通义千问)
- **端点**: `GET /compatible-mode/v1/models`
- **Base URL**: `https://dashscope.aliyuncs.com`
- **认证方式**: Bearer Token
- **最新模型**:
  - `qwen-max` - 通义千问最强模型
  - `qwen-plus` - 通义千问增强模型
  - `qwen-turbo` - 通义千问快速模型
  - `qwen-vl-max` - 视觉语言模型
  - `qwen-vl-plus` - 视觉语言增强模型
  - `qwen-audio` - 音频理解模型
- **多模态支持**:
  - ✅ 视觉（qwen-vl-* 系列）
  - ✅ 音频（qwen-audio）

---

### 6. GLM (智谱清言)
- **端点**: `GET /api/paas/v4/models`
- **Base URL**: `https://open.bigmodel.cn`
- **认证方式**: Bearer Token
- **最新模型**:
  - `glm-4-plus` - GLM-4 Plus 模型
  - `glm-4-flash` - GLM-4 Flash 快速模型
  - `glm-4v` - GLM-4 视觉模型
  - `glm-4-air` - GLM-4 Air 轻量级模型
  - `cogview-3` - 图像生成模型
  - `cogvideo` - 视频生成模型
- **多模态支持**:
  - ✅ 视觉（glm-4v）
  - ✅ 图像生成（cogview-3）
  - ✅ 视频生成（cogvideo）

---

### 7. Grok (xAI)
- **端点**: `GET /v1/models`
- **Base URL**: `https://api.x.ai`
- **认证方式**: Bearer Token
- **最新模型**:
  - `grok-beta` - Grok 测试版模型
  - `grok-vision-beta` - Grok 视觉模型
- **上下文长度**: 131,072 tokens
- **多模态支持**: ✅ 视觉（grok-vision-beta）

---

### 8. Doubao (豆包)
- **端点**: `GET /api/v3/models`
- **Base URL**: `https://ark.cn-beijing.volces.com`
- **认证方式**: Bearer Token
- **最新模型**:
  - `doubao-pro-32k` - 豆包 Pro 32K 模型
  - `doubao-pro-128k` - 豆包 Pro 128K 模型
  - `doubao-lite-32k` - 豆包 Lite 32K 模型
  - `doubao-vision` - 豆包视觉模型
- **多模态支持**: ✅ 视觉（doubao-vision）

---

### 9. Kimi (月之暗面)
- **端点**: `GET /v1/models`
- **Base URL**: `https://api.moonshot.cn`
- **认证方式**: Bearer Token
- **最新模型**:
  - `moonshot-v1-8k` - Kimi 8K 上下文模型
  - `moonshot-v1-32k` - Kimi 32K 上下文模型
  - `moonshot-v1-128k` - Kimi 128K 超长上下文模型
- **多模态支持**: ❌ 仅文本

---

### 10. Mistral AI
- **端点**: `GET /v1/models`
- **Base URL**: `https://api.mistral.ai`
- **认证方式**: Bearer Token
- **最新模型**:
  - `mistral-large-latest` - Mistral 最强大的模型
  - `mistral-medium-latest` - Mistral 中等模型
  - `mistral-small-latest` - Mistral 轻量级模型
  - `pixtral-12b` - Mistral 视觉模型
- **多模态支持**: ✅ 视觉（pixtral-12b）

---

### 11. Hunyuan (腾讯混元)
- **端点**: `GET /v1/models`
- **Base URL**: `https://api.hunyuan.cloud.tencent.com/v1`
- **认证方式**: Bearer Token
- **最新模型**:
  - `hunyuan-pro` - 混元 Pro 模型
  - `hunyuan-standard` - 混元标准模型
  - `hunyuan-lite` - 混元轻量级模型
  - `hunyuan-vision` - 混元视觉模型
- **多模态支持**: ✅ 视觉（hunyuan-vision）

---

### 12. MiniMax
- **端点**: `GET /v1/models`
- **Base URL**: `https://api.minimax.io`
- **认证方式**: Bearer Token
- **最新模型**:
  - `abab6.5s-chat` - ABAB 6.5s 对话模型
  - `abab6.5-chat` - ABAB 6.5 对话模型
  - `abab5.5-chat` - ABAB 5.5 对话模型
  - `speech-01` - 语音合成模型
  - `music-01` - 音乐生成模型
- **多模态支持**:
  - ✅ 语音合成（speech-01）
  - ✅ 音乐生成（music-01）

---

### 13. Llama (Meta)
- **端点**: `GET /v1/models`
- **Base URL**:
  - Together AI: `https://api.together.xyz`
  - Replicate: `https://api.replicate.com`
  - OpenRouter: `https://openrouter.ai/api`
- **认证方式**: Bearer Token
- **最新模型**:
  - `meta-llama/Llama-3.3-70B-Instruct-Turbo` - Llama 3.3 70B
  - `meta-llama/Llama-3.2-90B-Vision-Instruct-Turbo` - Llama 3.2 90B 视觉模型
  - `meta-llama/Llama-3.2-11B-Vision-Instruct-Turbo` - Llama 3.2 11B 视觉模型
- **多模态支持**: ✅ 视觉（Llama 3.2 Vision 系列）

---

## 📊 多模态能力总结

| Provider | 视觉 | 音频 | 视频 | 图像生成 | 视频生成 | 音乐生成 |
|----------|------|------|------|----------|----------|----------|
| OpenAI | ✅ | ❌ | ❌ | ✅ (DALL-E) | ❌ | ❌ |
| Claude | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Gemini | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| DeepSeek | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Qwen | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ |
| GLM | ✅ | ❌ | ❌ | ✅ | ✅ | ❌ |
| Grok | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Doubao | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Kimi | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Mistral | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Hunyuan | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| MiniMax | ❌ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Llama | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |

---

## 🔧 实现状态

| Provider | 端点实现 | 端点路径 | 状态 |
|----------|---------|---------|------|
| OpenAI | ✅ | `/v1/models` | 已验证 |
| Claude | ✅ | `/v1/models` | 已验证 |
| Gemini | ✅ | `/v1beta/models` | 已验证 |
| DeepSeek | ✅ | `/models` | 需验证 |
| Qwen | ✅ | `/compatible-mode/v1/models` | 已验证 |
| GLM | ✅ | `/api/paas/v4/models` | 需验证 |
| Grok | ✅ | `/v1/models` | 需验证 |
| Doubao | ✅ | `/api/v3/models` | 需验证 |
| Kimi | ✅ | `/v1/models` | 需验证 |
| Mistral | ✅ | `/v1/models` | 需验证 |
| Hunyuan | ✅ | `/v1/models` | 需验证 |
| MiniMax | ✅ | `/v1/models` | 需验证 |
| Llama | ✅ | `/v1/models` | 需验证 |

---

## 📝 注意事项

1. **端点差异**:
   - 大部分 Provider 使用 `/v1/models`
   - Gemini 使用 `/v1beta/models`（beta 版本）
   - Qwen 使用 `/compatible-mode/v1/models`（兼容模式）
   - GLM 使用 `/api/paas/v4/models`（PaaS 版本）
   - Doubao 使用 `/api/v3/models`（v3 版本）

2. **认证方式**:
   - 大部分使用 `Authorization: Bearer <token>`
   - Claude 使用 `x-api-key: <key>`
   - Gemini 使用 `x-goog-api-key: <key>`

3. **响应格式**:
   - OpenAI 兼容格式: `{ "object": "list", "data": [...] }`
   - Claude 格式: `{ "data": [...], "has_more": false, "first_id": "...", "last_id": "..." }`
   - Gemini 格式: `{ "models": [...] }`

4. **多模态模型**:
   - 视觉模型通常需要特殊的输入格式（base64 编码的图片）
   - 音频/视频模型可能需要额外的 API 端点
   - 生成类模型（图像、视频、音乐）通常有独立的 API

---

## 🚀 下一步

1. ✅ 为所有 Provider 实现 `ListModels` 方法
2. ⏳ 验证各个 Provider 的端点是否正确
3. ⏳ 添加多模态模型的支持
4. ⏳ 添加模型元数据（上下文长度、价格等）
5. ⏳ 添加模型能力检测（是否支持函数调用、视觉等）

---

**参考资料**:
- [OpenAI API Documentation](https://platform.openai.com/docs/api-reference/models)
- [Anthropic Claude API Documentation](https://platform.claude.com/docs/en/api/models)
- [Google Gemini API Documentation](https://ai.google.dev/gemini-api/docs/models)
- [DeepSeek API Documentation](https://api-docs.deepseek.com/)
