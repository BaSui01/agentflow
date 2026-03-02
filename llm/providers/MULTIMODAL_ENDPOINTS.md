# Provider 多模态能力端点参考

> 更新时间：2026年2月20日

本文档记录了各个 LLM Provider 的多模态能力（图像生成、音频、视频、Embedding、微调等）及其对应的 API 端点。

---

## 能力矩阵（代码已实现，Implemented Matrix）

> 本表为"代码已实现能力矩阵"，不等同官方能力矩阵。由 `llm/providers/capability_matrix.go` 声明，通过 `scripts/gen_llm_matrix.ps1` 生成。禁止手工直接改表格内容。

| Provider | 图像生成 | 视频生成 | 音频生成 | 音频转录 | Embedding | 微调 |
|----------|---------|---------|---------|---------|-----------|------|
| OpenAI | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| Claude | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Gemini | ✅ | ✅ | ✅ | ❌ | ✅ | ❌ |
| DeepSeek | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Qwen | ✅ | ❌ | ✅ | ❌ | ✅ | ❌ |
| GLM | ✅ | ✅ | ❌ | ❌ | ✅ | ❌ |
| Grok | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ |
| Doubao | ✅ | ❌ | ✅ | ❌ | ✅ | ❌ |
| Kimi | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Mistral | ❌ | ❌ | ❌ | ✅ | ✅ | ❌ |
| Hunyuan | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| MiniMax | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| Llama | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

---

## 🖼️ 图像生成 (Image Generation)

### OpenAI DALL-E
- **端点**: `POST /v1/images/generations`
- **模型**:
  - `dall-e-3` - 最新的 DALL-E 3 模型
  - `dall-e-2` - DALL-E 2 模型
- **支持的尺寸**:
  - DALL-E 3: `1024x1024`, `1792x1024`, `1024x1792`
  - DALL-E 2: `256x256`, `512x512`, `1024x1024`
- **质量选项**: `standard`, `hd`
- **风格选项**: `vivid`, `natural`

**示例请求**:
```json
{
  "model": "dall-e-3",
  "prompt": "A cute baby sea otter",
  "n": 1,
  "size": "1024x1024",
  "quality": "hd",
  "style": "vivid"
}
```

---

### Google Imagen
- **端点**: `POST /v1beta/models/imagen-4:predict`
- **模型**:
  - `imagen-4` - 最新的 Imagen 4 模型（最高 2K 分辨率）
  - `nano-banana-pro` - 4K 图像生成
- **支持的尺寸**: 最高 `2048x2048`（Imagen 4）、`4096x4096`（Nano Banana Pro）

---

### GLM CogView
- **端点**: `POST /api/paas/v4/images/generations`
- **模型**:
  - `cogview-3` - CogView 3 图像生成模型
- **支持的尺寸**: `1024x1024`, `1024x1792`, `1792x1024`

---

## 🎬 视频生成 (Video Generation)

### Google Veo
- **端点**: `POST /v1beta/models/veo-3.1:predict`
- **模型**:
  - `veo-3.1` - 最新的 Veo 3.1 视频生成模型
- **支持的分辨率**: 最高 `1920x1080`
- **支持的时长**: 最长 60 秒

---

### GLM CogVideo
- **端点**: `POST /api/paas/v4/videos/generations`
- **模型**:
  - `cogvideo` - CogVideo 视频生成模型
- **支持的分辨率**: `1280x720`, `1920x1080`
- **支持的时长**: 最长 6 秒

---

## 🎵 音频生成 (Audio/Speech Generation)

### OpenAI TTS
- **端点**: `POST /v1/audio/speech`
- **模型**:
  - `tts-1` - 标准 TTS 模型
  - `tts-1-hd` - 高清 TTS 模型
- **支持的语音**: `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`
- **支持的格式**: `mp3`, `opus`, `aac`, `flac`, `wav`, `pcm`
- **语速范围**: `0.25` - `4.0`

**示例请求**:
```json
{
  "model": "tts-1",
  "input": "Hello, world!",
  "voice": "alloy",
  "speed": 1.0,
  "response_format": "mp3"
}
```

---

### Google Gemini Audio
- **端点**: `POST /v1beta/models/gemini-2.5-flash-live:generateContent`
- **模型**:
  - `gemini-2.5-flash-live` - 支持低延迟双向语音
- **支持的格式**: `mp3`, `wav`

---

### Qwen Audio
- **端点**: `POST /compatible-mode/v1/audio/speech`
- **模型**:
  - `qwen-audio` - 通义千问音频模型
- **支持的格式**: `mp3`, `wav`

---

### Doubao TTS
- **端点**: `POST /api/v3/audio/speech`
- **模型**:
  - `doubao-tts` - 豆包语音合成模型
- **支持的格式**: `mp3`, `wav`

---

### MiniMax Speech
- **端点**: `POST /v1/audio/speech`
- **模型**:
  - `speech-01` - MiniMax 语音合成模型
  - `music-01` - MiniMax 音乐生成模型
- **支持的格式**: `mp3`, `wav`

---

## 🎤 音频转录 (Audio Transcription)

### OpenAI Whisper
- **端点**: `POST /v1/audio/transcriptions`
- **模型**:
  - `whisper-1` - Whisper 语音识别模型
- **支持的语言**: 99+ 种语言
- **支持的格式**: `mp3`, `mp4`, `mpeg`, `mpga`, `m4a`, `wav`, `webm`
- **响应格式**: `json`, `text`, `srt`, `verbose_json`, `vtt`

**示例请求**:
```json
{
  "model": "whisper-1",
  "file": "<audio_file_binary>",
  "language": "en",
  "response_format": "json"
}
```

---

## 📝 Embedding

### OpenAI Embeddings
- **端点**: `POST /v1/embeddings`
- **模型**:
  - `text-embedding-3-large` - 最大的 embedding 模型（3072 维）
  - `text-embedding-3-small` - 小型 embedding 模型（1536 维）
  - `text-embedding-ada-002` - 经典 Ada 模型（1536 维）
- **支持的维度**: 可自定义（`dimensions` 参数）

**示例请求**:
```json
{
  "model": "text-embedding-3-small",
  "input": ["Hello, world!", "How are you?"],
  "encoding_format": "float"
}
```

---

### Gemini Embeddings
- **端点**: `POST /v1beta/models/text-embedding-004:embedContent`
- **模型**:
  - `text-embedding-004` - Gemini embedding 模型（768 维）
- **支持的维度**: 768

---

### Qwen Embeddings
- **端点**: `POST /compatible-mode/v1/embeddings`
- **模型**:
  - `text-embedding-v1` - 通义千问 embedding 模型
  - `text-embedding-v2` - 通义千问 embedding v2 模型
- **支持的维度**: 1536

---

### GLM Embeddings
- **端点**: `POST /api/paas/v4/embeddings`
- **模型**:
  - `embedding-2` - GLM embedding 模型
- **支持的维度**: 1024

---

### Doubao Embeddings
- **端点**: `POST /api/v3/embeddings`
- **模型**:
  - `doubao-embedding` - 豆包 embedding 模型
- **支持的维度**: 1024

---

### Mistral Embeddings
- **端点**: `POST /v1/embeddings`
- **模型**:
  - `mistral-embed` - Mistral embedding 模型
- **支持的维度**: 1024

---

## 🔄 微调 (Fine-Tuning)

### OpenAI Fine-Tuning
- **创建任务**: `POST /v1/fine_tuning/jobs`
- **列出任务**: `GET /v1/fine_tuning/jobs`
- **获取任务**: `GET /v1/fine_tuning/jobs/{job_id}`
- **取消任务**: `POST /v1/fine_tuning/jobs/{job_id}/cancel`
- **支持的模型**:
  - `gpt-4o-mini-2024-07-18`
  - `gpt-3.5-turbo-0125`
  - `gpt-3.5-turbo-1106`
  - `davinci-002`
  - `babbage-002`

**示例请求**:
```json
{
  "model": "gpt-3.5-turbo",
  "training_file": "file-abc123",
  "validation_file": "file-def456",
  "hyperparameters": {
    "n_epochs": 3,
    "batch_size": 4,
    "learning_rate_multiplier": 0.1
  },
  "suffix": "my-custom-model"
}
```

---

## 🎯 实现建议

### 1. 为 OpenAI Provider 添加多模态支持

```go
// llm/providers/openai/multimodal.go

// GenerateImage 生成图像
func (p *OpenAIProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
    endpoint := fmt.Sprintf("%s/v1/images/generations", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// GenerateAudio 生成音频
func (p *OpenAIProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
    endpoint := fmt.Sprintf("%s/v1/audio/speech", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// TranscribeAudio 转录音频
func (p *OpenAIProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
    endpoint := fmt.Sprintf("%s/v1/audio/transcriptions", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// CreateEmbedding 创建 Embedding
func (p *OpenAIProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
    endpoint := fmt.Sprintf("%s/v1/embeddings", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// CreateFineTuningJob 创建微调任务
func (p *OpenAIProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
    endpoint := fmt.Sprintf("%s/v1/fine_tuning/jobs", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}
```

### 2. 为 Gemini Provider 添加多模态支持

```go
// llm/providers/gemini/multimodal.go

// GenerateImage 生成图像
func (p *GeminiProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
    endpoint := fmt.Sprintf("%s/v1beta/models/imagen-4:predict", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// GenerateVideo 生成视频
func (p *GeminiProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
    endpoint := fmt.Sprintf("%s/v1beta/models/veo-3.1:predict", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// CreateEmbedding 创建 Embedding
func (p *GeminiProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
    endpoint := fmt.Sprintf("%s/v1beta/models/text-embedding-004:embedContent", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}
```

### 3. 为 GLM Provider 添加多模态支持

```go
// llm/providers/glm/multimodal.go

// GenerateImage 生成图像
func (p *GLMProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
    endpoint := fmt.Sprintf("%s/api/paas/v4/images/generations", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// GenerateVideo 生成视频
func (p *GLMProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
    endpoint := fmt.Sprintf("%s/api/paas/v4/videos/generations", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}

// CreateEmbedding 创建 Embedding
func (p *GLMProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
    endpoint := fmt.Sprintf("%s/api/paas/v4/embeddings", strings.TrimRight(p.cfg.BaseURL, "/"))
    // ... 实现逻辑
}
```

---

## 📚 参考资料

- [OpenAI API Documentation - Images](https://platform.openai.com/docs/api-reference/images)
- [OpenAI API Documentation - Audio](https://platform.openai.com/docs/api-reference/audio)
- [OpenAI API Documentation - Embeddings](https://platform.openai.com/docs/api-reference/embeddings)
- [OpenAI API Documentation - Fine-tuning](https://platform.openai.com/docs/api-reference/fine-tuning)
- [Google Gemini API Documentation](https://ai.google.dev/gemini-api/docs)
- [Qwen API Documentation](https://help.aliyun.com/zh/model-studio/)
- [GLM API Documentation](https://open.bigmodel.cn/dev/api)

---

## 🚀 下一步

1. ⏳ 为 OpenAI Provider 实现多模态方法
2. ⏳ 为 Gemini Provider 实现多模态方法
3. ⏳ 为 GLM Provider 实现多模态方法
4. ⏳ 为其他 Provider 实现 Embedding 方法
5. ⏳ 添加多模态能力的单元测试
6. ⏳ 添加多模态能力的集成测试
