# 视频与图像厂商及端点说明

本文档汇总 AgentFlow 已接入的**视频生成**与**图像生成**厂商、默认 Base URL、主要端点路径，以及**流式/轮询**支持情况。所有 Base URL 均可通过配置覆盖。

### 接入完善清单（配置即用）

| 类别 | 已完整接入（配置对应 API Key 即可使用） | 未接入（可扩展） |
|------|----------------------------------------|------------------|
| **图像** | openai、gemini、flux、stability、ideogram、tongyi（通义万相）、**zhipu（智谱）**、**baidu（文心一格）**、**doubao（豆包/火山）**、**tencent（腾讯混元生图）**、**kling（可灵）** | — |
| **视频** | sora、runway、veo、kling、luma、minimax-video、seedance（即梦）、Grok（经 Chat） | — |

- **已接入**：在 `config` 中配置对应 `*_api_key`（及可选 `*_base_url`）后，`GET /api/v1/multimodal/capabilities` 会返回该 provider，`POST /api/v1/multimodal/image` 或 `.../video` 请求体传 `provider: "xxx"` 即可调用。
- **一个 Key 双用（图像+视频）**：**OpenAI**（`openai_api_key` → openai 图像 + sora 视频，未配 `sora_api_key` 时 Sora 复用该 key）、**Google**（`google_api_key` → gemini 图像 + veo 视频）、**可灵**（`kling_api_key` → kling 图像 + kling 视频）。
- **字节**：视频使用 **seedance**（即梦，配置 `seedance_api_key`）；图像使用 **doubao**（豆包/火山 SeeDream，配置 `doubao_api_key`）。两者为不同产品与 endpoint，需分别配置。
- **未接入**：需在 `llm/capabilities/image`（或 video）中新增 provider 实现，并在多模态 builder、config、bootstrap 中注册。

---

## 〇、厂商分类概览

| 类型 | 说明 | 本项目中 |
|------|------|----------|
| **仅图像** | 只提供文生图/图生图 | OpenAI DALL·E、BFL Flux、Stability AI、**Ideogram**、**通义万相**、**智谱**、**文心一格（百度）**、**豆包/火山** |
| **仅视频** | 只提供文生视频/图生视频 | Runway、Luma、MiniMax、即梦 Seedance、Grok（Sora 可由 OpenAI 同一 key 启用，见下） |
| **图像 + 视频** | 同一厂商/同一 API 体系下同时提供图像与视频 | **OpenAI**（DALL·E 图像 + Sora 视频）、**Google**（Gemini 图像 + Veo/Gemini Video）、**可灵 Kling**（文生图 + 文生视频/图生视频）；三者均支持同一 API Key 配置一次即同时启用图像与视频 |

- 配置 **OpenAI**（`multimodal.image.openai_api_key`）后，会同时注册 **图像** `openai` 与 **视频** `sora`（Sora 未单独配置 `sora_api_key` 时复用该 key）。
- 配置 **Google / Gemini** 后，多模态会同时注册 **图像** `gemini` 与 **视频** `veo`，可共用一个 API Key。
- **可灵**：**视频**与**图像**共用同一 API Key（`Multimodal.Video.KlingAPIKey`），配置一次即可同时使用 `provider: "kling"` 做文生图与文生视频/图生视频。
- **字节**：**视频** 使用 即梦 **seedance**（`Multimodal.Video.SeedanceAPIKey`），**图像** 使用 豆包/火山 **doubao**（`Multimodal.Image.DoubaoAPIKey`），为两套配置与 endpoint。

---

## 一、视频生成厂商（Video Providers）

| 厂商 | Provider 名称 | 默认 Base URL | 主要端点与流程 | 流式/轮询 |
|------|----------------|---------------|----------------|-----------|
| **OpenAI Sora** | `sora` | `https://api.openai.com` | POST `/v1/videos` 创建 → GET `/v1/videos/{id}` 轮询；可选 GET `/v1/videos/{id}/content` 下载 | 轮询 |
| **Runway** | `runway` | `https://api.runwayml.com` | POST `/v1/image_to_video` 提交 → GET `/v1/tasks/{id}` 轮询 | 轮询 |
| **Google Veo** | `veo` | `https://generativelanguage.googleapis.com` | POST `.../models/{model}:generateVideos` 长轮询 → GET `.../operations/{name}` 查询 | 轮询 |
| **Google Gemini Video** | `gemini` / `gemini-video` | 同 Google AI / Vertex 配置 | 使用 Gemini 多模态 API 生成视频 | 轮询 |
| **可灵 Kling** | `kling` | `https://api.klingai.com` | POST `/v1/videos/text2video` 或 `/v1/videos/image2video` → GET `/v1/videos/{task_id}` 轮询；支持 `callback_url` 回调 | 轮询 / 回调 |
| **Luma** | `luma` | `https://api.lumalabs.ai` | POST `/dream-machine/v1/generations` 提交与轮询 | 轮询 |
| **MiniMax** | `minimax-video` | `https://api.minimax.chat` | POST `/v1/video_generation` → GET `/v1/query/video_generation`、`/v1/files/retrieve` | 轮询 |
| **即梦 Seedance** | `seedance` / `即梦` | `https://api.seedance.ai` | POST `/v2/generate/text` 或 `/v2/generate/image`（图生视频）→ GET `/v2/tasks/{task_id}`、`/v2/tasks/{task_id}/result` | 轮询 |
| **Grok (x.ai)** | 通过 Chat 能力 | `https://api.x.ai` | POST `/v1/videos/generations` 提交 → GET `/v1/videos/generations/{id}` 轮询 | 轮询 |

- **可灵**：视频与图像共用 `KlingAPIKey`；图像为 POST `/kling/v1/images/generations` 提交 → GET `/kling/v1/images/generations/{task_id}` 轮询。
- **字节**：视频为 **即梦 seedance**（上表）；图像为 **豆包 doubao**（见下方图像厂商表），需分别配置 API Key。
- **配置键**（示例）：`Multimodal.Video.{Sora|Runway|Kling|Luma|MiniMax|Seedance}APIKey` / `BaseURL`，以及 `DefaultVideoProvider`。
- 视频均为**异步**：提交任务后通过轮询（或可灵 `callback_url`）获取结果，无原生“流式生成视频帧”的 SSE。

---

## 二、图像生成厂商（Image Providers）

| 厂商 | Provider 名称 | 默认 Base URL | 主要端点 | 流式支持 |
|------|----------------|---------------|----------|----------|
| **OpenAI (DALL·E)** | `openai` | `https://api.openai.com` | POST `/v1/images/generations`（及 edits、variations） | 支持：请求体 `stream: true` 时走 **SSE** |
| **Google Gemini** | `gemini` | `https://generativelanguage.googleapis.com`（可覆盖） | `POST .../v1beta/models/{model}:generateContent`（同步）或 `streamGenerateContent?alt=sse`（流式）；默认 `gemini-3-pro-image-preview`；支持 `imageConfig`（imageSize 1K/2K/4K、aspectRatio）、`system_instruction`、`google_search` grounding、`thinkingConfig`、`safetySettings`；通过请求 `metadata` 字段透传（见下方 Gemini 专属参数表） | **原生 SSE 真流式**：文字思考 token 实时推送（`image_generation.thinking`），图像数据随后到达 |
| **BFL Flux** | `flux` | `https://api.bfl.ml` / `https://api.bfl.ai` | POST `/v1/{model}`，GET `/v1/get_result?id=...` 轮询 | 非流式（轮询） |
| **Stability AI** | `stability` | `https://api.stability.ai` | POST `/v1/generation/{engine_id}/text-to-image`（Stable Diffusion） | 非流式 |
| **Ideogram** | `ideogram` | `https://api.ideogram.ai` | POST `/v1/ideogram-v3/generate`（multipart/form-data），Api-Key 头 | 非流式 |
| **通义万相（阿里）** | `tongyi` | `https://dashscope.aliyuncs.com` | POST `/api/v1/services/aigc/text2image/image-synthesis`，Bearer 鉴权 | 非流式 |
| **智谱 GLM-Image** | `zhipu` | `https://open.bigmodel.cn` | POST `/api/paas/v4/images/generations`，Bearer 鉴权 | 非流式 |
| **文心一格（百度）** | `baidu` | `https://aip.baidubce.com` | OAuth 取 token → POST `ernievilg/v1/txt2img` → 轮询 getImg | 非流式 |
| **豆包/火山 SeeDream** | `doubao` | `https://ark.cn-beijing.volces.com` | POST `/v1/images/generations`，Bearer 鉴权 | 非流式 |
| **腾讯混元生图** | `tencent` | `https://aiart.tencentcloudapi.com` | TC3-HMAC-SHA256 签名；SubmitTextToImageJob 提交 → QueryTextToImageJob 轮询，ResultImage 返回 URL | 非流式 |
| **可灵 Kling** | `kling` | `https://api.klingai.com` | POST `/kling/v1/images/generations` 提交 → GET `/kling/v1/images/generations/{task_id}` 轮询；与视频共用 API Key | 非流式 |

### 2.1 Gemini 图像模型版本

| 模型 ID | 代号 | 发布时间 | 最高分辨率 | 生成速度 | 参考图 | Search grounding | 价格（标准/4K） |
|---------|------|---------|----------|---------|--------|-----------------|----------------|
| `gemini-2.5-flash-image` | Nano Banana | 2025.08 | 1K | 2-3 秒 | 有限 | — | $0.039/— |
| `gemini-3-pro-image-preview` | Nano Banana Pro | 2025.11 | 4K | 8-12 秒 | 最多 14 张 | ✅ | $0.134/$0.24 |
| `gemini-3.1-flash-image-preview` | Nano Banana 2 | 2026.02 | 4K | 4-6 秒 | 待确认 | 预期支持 | ~$0.05/~$0.15 |

默认模型为 `gemini-3-pro-image-preview`，可通过请求体 `model` 字段覆盖。

### 2.2 Gemini 图像专属参数（通过 `metadata` 传入）

调用 `POST /api/v1/multimodal/image` 时，请求体中的 `metadata` 字段可透传以下 Gemini 专属参数：

| `metadata` 键 | 类型 | 说明 | 示例值 |
|---------------|------|------|--------|
| `image_size` | string | 图像分辨率，优先级高于通用 `size` 字段 | `"1K"` / `"2K"` / `"4K"` |
| `aspect_ratio` | string | 宽高比 | `"1:1"` / `"16:9"` / `"9:16"` / `"2:3"` / `"3:2"` / `"4:3"` / `"3:4"` / `"4:5"` / `"5:4"` / `"21:9"` |
| `response_modalities` | string | 响应模态，逗号分隔；流式时默认 `"TEXT,IMAGE"` 以获得思考反馈 | `"IMAGE"` / `"TEXT,IMAGE"` |
| `enable_search` | string | 启用 Google Search grounding（仅 Pro 模型支持） | `"true"` |
| `system_prompt` | string | 系统提示词，引导模型风格/行为 | `"以水墨画风格绘制，笔触写意"` |
| `thinking_budget` | string | thinking token 预算（整数），较大值提升复杂构图质量 | `"1024"` / `"2048"` |
| `person_generation` | string | 人物生成策略 | `"ALLOW_ALL"` / `"ALLOW_ADULT"` / `"ALLOW_NONE"` |
| `safety_threshold` | string | 统一安全过滤阈值，应用于全部 harm category | `"BLOCK_NONE"` / `"BLOCK_FEW"` / `"BLOCK_SOME"` / `"BLOCK_MOST"` |
| `candidate_count` | string | 候选数量（整数，默认 1） | `"1"` / `"4"` |

> **注意**：`image_size` 中 1K≈1024px、2K≈2048px、4K≈4096px；通用 `size` 字段（如 `"1024x1024"`）会自动映射到最近的 Gemini 分辨率档位。`gemini-2.5-flash-image` 仅支持 1K；`gemini-3-pro-image-preview` 和 `gemini-3.1-flash-image-preview` 支持 1K/2K/4K。

**SSE 流式说明**：当请求携带 `"stream": true` 时，Gemini provider 会走 `streamGenerateContent?alt=sse` 原生流式端点。此时 `response_modalities` 默认为 `"TEXT,IMAGE"`，客户端会实时收到：
- `image_generation.started`：生成开始
- `image_generation.thinking`：模型输出的描述/思考文字（连续多条，可直接展示进度）
- `image_generation.completed`：图像数据（base64）到达
- `image_generation.done` → `data: [DONE]`：完成

**请求示例（流式 + 联网搜索 + 4K）**：
```json
{
  "provider": "gemini",
  "model": "gemini-3-pro-image-preview",
  "prompt": "今日上海外滩的真实天气场景，写实摄影风格",
  "stream": true,
  "metadata": {
    "image_size": "4K",
    "aspect_ratio": "16:9",
    "enable_search": "true",
    "system_prompt": "生成高饱和度写实摄影风格图像，避免卡通化"
  }
}
```

**请求示例（纯图像模式 + 安全限制放宽）**：
```json
{
  "provider": "gemini",
  "model": "gemini-3.1-flash-image-preview",
  "prompt": "赛博朋克风格未来都市夜景",
  "metadata": {
    "image_size": "4K",
    "aspect_ratio": "21:9",
    "response_modalities": "IMAGE",
    "safety_threshold": "BLOCK_FEW",
    "thinking_budget": "1024"
  }
}
```

---

- **图像流式**：多模态 API 在 `stream: true` 时返回 SSE，事件序列：`image_generation.started` → `[image_generation.thinking]*`（仅 Gemini 等原生流式 provider）→ `image_generation.completed`（每张图，含 index、url/b64_json、output_format、quality、size、usage 等）→ `image_generation.done` → `data: [DONE]`；错误为 `event: error`。
- **原生流式 vs 包装流式**：Gemini provider 实现了 `StreamingProvider` 接口，走 `streamGenerateContent?alt=sse`，图像生成过程中实时推送文字思考 token；其他 provider 为阻塞生成后包装 SSE，无 `thinking` 事件。
- **能力标识**：`GET /api/v1/multimodal/capabilities` 的 `features.image_stream` 表示是否支持图像流式。
- **说明**：多模态 API 默认装配的图像厂商为 **openai**、**gemini**、**flux**（BFL）、**stability**、**ideogram**、**tongyi**（通义万相）、**zhipu**（智谱）、**baidu**（文心一格）、**doubao**（豆包/火山）、**tencent**（腾讯混元生图）、**kling**（可灵）。**可灵**图像与视频共用 `KlingAPIKey`（配置在 Video 下）。**字节**图像使用 **doubao**，与视频 **seedance**（即梦）为不同配置。OpenAI/Gemini 支持 `stream: true` 的 SSE 流式，其余为轮询或同步返回。

---

## 三、业界常见厂商一览（扩展参考）

以下为业界常见图像/视频 API 厂商，**未全部接入**；已接入的见上表，未接入的可在 `llm/capabilities/image`、`llm/capabilities/video` 及多模态 builder 中按需扩展。

| 厂商 | 图像 | 视频 | 备注 |
|------|------|------|------|
| **OpenAI** | ✅ DALL·E | ✅ Sora | 已接入 |
| **Google** | ✅ Gemini / Imagen | ✅ Veo / Gemini Video | 已接入 Gemini 图像 + Veo |
| **BFL Flux** | ✅ | — | 已接入 |
| **Stability AI** | ✅ | — | 已接入（Stable Diffusion） |
| **Ideogram** | ✅ | — | 已接入（Ideogram 3.0） |
| **Midjourney** | 有 API 生态 | — | 多通过第三方聚合 |
| **Runway** | — | ✅ | 已接入 |
| **可灵 Kling** | ✅ 图像 | ✅ 视频 | 已接入；图像与视频共用 `kling_api_key`，支持 callback_url |
| **Luma** | — | ✅ Dream Machine | 已接入 |
| **MiniMax** | 有图像能力 | ✅ 视频 | 当前仅接入视频 |
| **字节** | ✅ doubao（豆包/火山 SeeDream） | ✅ seedance（即梦） | 图像：`doubao_api_key`；视频：`seedance_api_key`，两套配置 |
| **Grok (x.ai)** | — | ✅ | 通过 Chat 能力接入 |
| **通义万相（阿里）** | ✅ | — | 已接入（DashScope 万相） |
| **腾讯混元生图** | ✅ | — | 已接入（aiart，TC3 签名，SubmitTextToImageJob + QueryTextToImageJob 轮询） |
| **文心一格（百度）** | ✅ | — | 已接入（ERNIE-ViLG，OAuth + 轮询） |
| **智谱** | ✅ | — | 已接入（GLM-Image） |
| **豆包/火山** | ✅ | 部分有视频 | 已接入图像（SeeDream）；视频可按需扩展 |

---

## 四、配置与能力查询

- **多模态能力**：`GET /api/v1/multimodal/capabilities` 返回 `image_providers`、`video_providers`、`default_image_provider`、`default_video_provider` 以及 `features`（如 `image_stream`、`text_to_video`、`image_to_video`）。
- **Base URL 覆盖**：各厂商的默认 Base URL 见上表；在配置中设置对应 `*BaseURL` 即可覆盖（需重启或按热加载规则生效）。
- **视频 Provider 选择**：请求体或查询参数中可指定 `provider`（如 `sora`、`kling`、`seedance`）；不指定时使用 `default_video_provider`。

---

## 五、参考文档

- OpenAI Videos API: https://platform.openai.com/docs/api-reference/videos  
- 可灵开发文档: https://app.klingai.com/cn/dev/document-api  
- 即梦/火山引擎 API 以官方最新文档为准；Seedance Base URL 可能随发布调整，可通过配置覆盖。

---

## 六、在项目中启用

在项目中启用多模态能力需满足：`multimodal.enabled: true`，并至少配置一种图像或视频厂商的 API Key。配置方式为 YAML 或环境变量（环境变量名见下）。

### 6.1 配置文件示例（YAML）

```yaml
multimodal:
  enabled: true
  default_image_provider: openai
  default_video_provider: veo
  reference_store_backend: redis
  image:
    openai_api_key: "sk-..."
    openai_base_url: "https://api.openai.com"
    gemini_api_key: ""
    flux_api_key: ""
    flux_base_url: "https://api.bfl.ai"
    stability_api_key: ""
    stability_base_url: "https://api.stability.ai"
    ideogram_api_key: ""
    ideogram_base_url: "https://api.ideogram.ai"
    tongyi_api_key: ""
    tongyi_base_url: "https://dashscope.aliyuncs.com"
    zhipu_api_key: ""
    zhipu_base_url: "https://open.bigmodel.cn"
    baidu_api_key: ""
    baidu_secret_key: ""
    baidu_base_url: "https://aip.baidubce.com"
    doubao_api_key: ""
    doubao_base_url: "https://ark.cn-beijing.volces.com"
    tencent_secret_id: ""
    tencent_secret_key: ""
    tencent_base_url: "https://aiart.tencentcloudapi.com"
  video:
    google_api_key: ""
    runway_api_key: ""
    sora_api_key: ""
    kling_api_key: ""
    luma_api_key: ""
    minimax_api_key: ""
    seedance_api_key: ""
    seedance_base_url: "https://api.seedance.ai"
```

### 6.2 环境变量（部分）

- 图像：`OPENAI_API_KEY`、`OPENAI_BASE_URL`（配置后同时启用 openai 图像与 sora 视频，除非单独配置 `SORA_API_KEY`）；`GEMINI_API_KEY`（与 video 共用 Google 时可选）；`FLUX_API_KEY`、`FLUX_BASE_URL`（BFL Flux）；`STABILITY_API_KEY`、`STABILITY_BASE_URL`（Stability AI）；`IDEOGRAM_API_KEY`、`IDEOGRAM_BASE_URL`（Ideogram）；`TONGYI_API_KEY`、`TONGYI_BASE_URL`（阿里通义万相）；`ZHIPU_API_KEY`、`ZHIPU_BASE_URL`（智谱）；`BAIDU_API_KEY`、`BAIDU_SECRET_KEY`、`BAIDU_BASE_URL`（文心一格）；`DOUBAO_API_KEY`、`DOUBAO_BASE_URL`（豆包/火山）；`TENCENT_SECRET_ID`、`TENCENT_SECRET_KEY`、`TENCENT_BASE_URL`（腾讯混元生图，TC3 签名）
- 视频：`RUNWAY_API_KEY`、`SORA_API_KEY`、`KLING_API_KEY`（可灵，同 key 支持图像+视频）、`LUMA_API_KEY`、`MINIMAX_API_KEY`、`SEEDANCE_API_KEY`（即梦/字节视频）及对应 `*_BASE_URL`
- **可灵**：`KLING_API_KEY` / `KLING_BASE_URL` 同时启用图像与视频
- **字节**：视频 即梦 `SEEDANCE_API_KEY` / `SEEDANCE_BASE_URL`；图像 豆包 `DOUBAO_API_KEY` / `DOUBAO_BASE_URL`
- 默认厂商：`DEFAULT_IMAGE_PROVIDER`、`DEFAULT_VIDEO_PROVIDER`

完整字段见 `config/loader.go` 中 `MultimodalConfig`、`MultimodalImageConfig`、`MultimodalVideoConfig` 的 `env` 标签。

### 6.3 API 使用方式

- **查询能力**：`GET /api/v1/multimodal/capabilities`
- **图像生成（含流式）**：`POST /api/v1/multimodal/image`，请求体可带 `provider`、`stream: true` 则响应为 SSE
- **视频生成**：`POST /api/v1/multimodal/video`，请求体可带 `provider`、可选 `callback_url`（可灵）
- **引用上传**：`POST /api/v1/multimodal/references` 上传图片，获得 `reference_id` 供 image-to-image / image-to-video 使用

### 6.4 引用图存储（Reference Store）

- 生产环境必须配置 `reference_store_backend: redis` 并配置 `redis.addr`，由组合根注入 Redis 实现的 `ReferenceStore`。
- **仅测试或开发**：若构造 Handler 时传入 `referenceStore == nil`，将使用内存实现；数据不持久、进程重启即丢失，**不得用于生产**。
