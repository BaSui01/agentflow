# LLM 层重构增加功能（能力矩阵修正与补齐计划）

> 日期：2026-03-02  
> 目标：修正错误能力矩阵，并补齐已被官方支持但当前 `llm` 层缺失的能力实现。  
> 约束：遵循项目“单轨替换、禁止兼容分支”规则，不引入新旧双轨。

## 0. 最终目标（升级）

你的目标已明确为：

1. **代码需支持各供应商官方已发布的全部能力**（以官方 API 文档为准，不以 OpenAI-Compatible 名义推断）。
2. 在 `llm` 层形成统一能力抽象，不让上层关心各家异构端点。
3. 能力状态分为 `官方支持 / 代码已实现 / 已联调验证` 三个层级，避免“看起来支持但不可用”。

## 1. 结论摘要

当前“多模态能力矩阵”存在两类问题：

1. 文档与代码实现不一致（可立即修复）。
2. 代码实现落后于官方能力（需新增实现，不是仅改文档）。

且还存在第三类问题：

3. 现有矩阵粒度过粗，只覆盖“图像/视频/音频”大类，缺少你关心的关键子能力（如文生图、文生视频、图生视频、口播、数字人、工具调用、重排等）。

## 1.1 全能力域定义（后续统一按此口径）

基于当前 `llm/core/contracts.go` + `llm/gateway/gateway.go` + `llm/capabilities/*`，能力域统一拆分为：

1. **Chat**：对话补全、流式、结构化输出、原生工具调用。
2. **Tools**：工具执行层（Web Search/Web Scrape/ReAct/并行/限流/降级）。
3. **Image**：
   - 文生图（Text-to-Image）
   - 图像编辑（Image Edit）
4. **Video**：
   - 文生视频（Text-to-Video）
   - 图生视频（Image-to-Video）
   - 视频编辑/延展（若官方支持）
5. **Audio**：
   - TTS（文本转语音）
   - STT/ASR（语音转文本）
   - 口播（可视作 TTS 特化：播报风格/情感/角色）
6. **Avatar（数字人）**：
   - 数字人生成
   - 数字人口播/驱动（音频/视频资产）
7. **Embedding**：向量嵌入。
8. **Rerank**：重排序。
9. **Moderation**：内容审核。
10. **Music**：音乐生成。
11. **ThreeD**：3D 生成。

## 2. 证据基线

### 2.1 代码事实矩阵（以 `llm/providers/*/multimodal.go` 为准）

| Provider | 图像 | 视频 | 音频生成 | 音频转录 | Embedding | 微调 |
|---|---|---|---|---|---|---|
| OpenAI | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ |
| Claude(Anthropic) | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
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

代码证据（示例）：
- `Qwen` 图像能力：`GenerateImage` 已实现 [`llm/providers/qwen/multimodal.go`](e:/code/agentflow/llm/providers/qwen/multimodal.go:14)
- `Grok` 图像/Embedding：已实现 [`llm/providers/grok/multimodal.go`](e:/code/agentflow/llm/providers/grok/multimodal.go:14), [`llm/providers/grok/multimodal.go`](e:/code/agentflow/llm/providers/grok/multimodal.go:36)
- `Doubao` 图像：已实现 [`llm/providers/doubao/multimodal.go`](e:/code/agentflow/llm/providers/doubao/multimodal.go:12)
- `Mistral` 音频转录：已实现 [`llm/providers/mistral/multimodal.go`](e:/code/agentflow/llm/providers/mistral/multimodal.go:35)

### 2.2 当前文档矩阵与代码冲突（确定错误）

冲突来源：
- [`./llm层重构.md`](./llm层重构.md)
- [`../../llm/providers/MULTIMODAL_ENDPOINTS.md`](../../llm/providers/MULTIMODAL_ENDPOINTS.md)

确定冲突项（高置信度）：

1. Qwen 图像：文档 `❌`，代码 `✅`
2. Grok 图像：文档 `❌`，代码 `✅`
3. Grok Embedding：文档 `❌`，代码 `✅`
4. Doubao 图像：文档 `❌`，代码 `✅`
5. Mistral 音频转录：文档 `❌`，代码 `✅`

### 2.3 官方能力与当前代码差异（需要新增实现）

已通过官方文档核验的差异：

1. OpenAI 视频生成：官方已支持，代码当前 `❌`  
   证据：[OpenAI Video Generation Guide](https://platform.openai.com/docs/guides/video-generation)（含 `/v1/videos`）。
2. xAI Grok 视频生成：官方已支持，代码当前 `❌`  
   证据：[xAI Video Generation Guide](https://docs.x.ai/docs/guides/video-generation), [xAI Models Overview](https://docs.x.ai/docs/models#inference-models)。
3. Qwen 视频生成（万相）：官方文档存在视频生成 API，代码当前 `❌`  
   证据：[阿里云百炼文生视频 API 参考](https://help.aliyun.com/zh/model-studio/developer-reference/text-to-video-api-reference)。

补充证据：
- xAI 图像生成官方文档（与代码 `✅` 一致）：[xAI Image Generation Guide](https://docs.x.ai/docs/guides/image-generation)。
- Mistral 音频转录官方文档（与代码 `✅` 一致）：[Mistral Audio & Transcription](https://docs.mistral.ai/capabilities/audio_transcription)。
- DeepSeek 官方快速开始当前聚焦 Chat（与代码多数 `❌` 基本一致）：[DeepSeek API Docs](https://api-docs.deepseek.com/)。

说明：本次已调用 Grok Web Search MCP 做并行检索（session：`3eb22369364b`、`23aed590ec88`、`12ef8812f205`），但该 MCP 返回 `sources_count=0`，因此最终结论以可直接复核的官方 URL 为准。

## 2.4 当前“全能力域”覆盖现状（代码层）

这是“框架能力”而不是“13 家 chat provider 单独能力”：

| 能力域 | 子能力 | 当前代码状态 | 主要实现位置 |
|---|---|---|---|
| Chat | Completion/Stream | ✅ | `llm/provider.go`, `llm/gateway/gateway.go` |
| Chat | Native Tool Calling | ✅（按 provider 能力差异） | `SupportsNativeFunctionCalling` |
| Tools | Tool Executor/ReAct/Web 搜索抓取 | ✅ | `llm/capabilities/tools/*` |
| Image | 文生图 | ✅ | `llm/capabilities/image/*`, `llm/providers/*/multimodal.go` |
| Image | 图像编辑 | ✅（统一入口已支持） | `gateway.ImageInput.Edit`, `image.Provider.Edit` |
| Video | 文生视频 | ✅（能力层）/ 部分 provider 未接通 | `llm/capabilities/video/*` |
| Video | 图生视频 | ✅（能力层）/ 需逐 provider 对齐 | `llm/capabilities/video/*` |
| Audio | TTS | ✅ | `llm/capabilities/audio/openai_tts.go`, `elevenlabs` |
| Audio | STT | ✅ | `llm/capabilities/audio/openai_stt.go`, `deepgram` |
| Audio | 口播 | ⚠️ 需统一定义为 TTS 场景模板 | 待补 `audio` 能力模型 |
| Avatar | 数字人生成 | ✅（接口槽位 + gateway 路由） | `llm/capabilities/avatar/types.go`, `gateway.AvatarInput` |
| Avatar | 数字人口播/驱动 | ⚠️ 需补资产编排协议 | 待补 |
| Embedding | 向量嵌入 | ✅ | `llm/capabilities/embedding/*` |
| Rerank | 重排序 | ✅ | `llm/capabilities/rerank/*` |
| Moderation | 内容审核 | ✅ | `llm/capabilities/moderation/*` |
| Music | 音乐生成 | ✅ | `llm/capabilities/music/*` |
| ThreeD | 3D 生成 | ✅ | `llm/capabilities/threed/*` |

## 3. 改造目标（单一事实源）

能力矩阵必须拆分为两个维度，避免再次混淆：

1. `Implemented Matrix`：项目当前已实现能力（代码事实）。
2. `Official Matrix`：官方已支持能力（外部事实）。

最终对外展示应同时给出 `官方支持` 与 `项目已实现` 两列状态，明确“可用性”与“待实现缺口”。

新增强制要求（对应你的目标）：

1. 每个 provider 维护 **官方能力清单**（文档证据链接 + 更新时间）。
2. 每个能力清单项都必须映射到一个代码实现或明确 `TODO`。
3. 禁止只在文档标记“支持”而没有可调用实现。

## 4. 分阶段修改计划

### Phase A（P0，立即执行）：修正文档错误，不改行为

改动项：

1. 修正以下文档矩阵为“代码事实”：
   - [`./llm层重构.md`](./llm层重构.md)
   - [`../../llm/providers/MULTIMODAL_ENDPOINTS.md`](../../llm/providers/MULTIMODAL_ENDPOINTS.md)
2. 同步修正 5 个确定冲突单元格（Qwen 图像、Grok 图像、Grok Embedding、Doubao 图像、Mistral 音频转录）。
3. 在文档标题处标注“这是实现矩阵，不等同官方能力矩阵”。

验收标准：

1. 两份文档矩阵与 `multimodal.go` 一致。
2. 不再出现“文档说不支持但代码已实现”的条目。

### Phase B（P1）：引入矩阵单一事实源与自动校验

改动项：

1. 新增 Provider 能力注册表（建议：`llm/providers/capability_matrix.go`）。
2. 每个 provider 显式声明 6 维能力布尔值（图像/视频/音频生成/音频转录/embedding/微调）。
3. 文档矩阵改为由注册表自动生成（脚本生成 Markdown），删除手工维护矩阵。
4. 新增测试确保“注册表声明”和“方法行为”一致（支持方法不得返回 not supported；不支持方法必须返回标准错误）。

验收标准：

1. `go test ./llm/providers/...` 通过。
2. CI 中出现矩阵一致性校验（防止未来回归）。

### Phase C（P2）：补齐官方已支持但项目未实现的能力

优先级（按业务价值+实现复杂度）：

1. OpenAI `GenerateVideo`：接入 `/v1/videos`，复用已有异步轮询基础（`llm/providers/async_poller.go`）。
2. Grok `GenerateVideo`：接入 xAI 视频生成端点与状态轮询。
3. Qwen `GenerateVideo`：接入万相视频生成 API（文本/图像到视频），统一到 `VideoGenerationRequest/Response`。

同步要求：

1. Provider 层实现完成后，更新 `Official vs Implemented` 对照矩阵。
2. 每新增能力必须配套 `httptest` 覆盖成功、上游错误、轮询超时三类路径。

### Phase C+（P2.5）：面向“官方全能力覆盖”的能力细分补齐

按你关心的能力优先级执行：

1. **视频细分**
   - 文生视频、图生视频拆分能力标识（至少在请求结构里可区分模式）。
   - 各 provider 端点能力映射清晰化（同一 `GenerateVideo` 内按 mode 路由）。
2. **口播**
   - 在 `audio` 里新增 `NarrationProfile`（语速、情感、角色、停顿、口播风格）。
   - 统一 OpenAI/ElevenLabs/其他 provider 的口播参数映射。
3. **数字人**
   - 在 `avatar` 能力中新增“语音驱动 / 文本驱动 / 视频资产驱动”请求模型。
   - 与 `audio`、`video` 打通资产引用协议（避免重复上传）。
4. **工具调用**
   - 统一 `native tool calling` 与 `react tool loop` 的策略配置，避免双语义。
5. **嵌入 + 重排**
   - 建立统一评测基线（召回率、NDCG、延迟、成本），用于 provider 选型。

### Phase D（P3）：收口与治理

1. 在架构文档中补充“矩阵定义规范”（官方能力 vs 实现能力）。
2. 将矩阵更新纳入发布检查清单。
3. 关闭旧的叙述性报告中不再可信的静态数字（避免再次过期）。
4. 将“官方全能力覆盖率”纳入发布 KPI（按 provider 与能力域双维度统计）。

## 5. 风险与边界

1. 部分供应商文档更新频繁（尤其 xAI、国内平台），官方能力可能先于 SDK/兼容层变化。
2. OpenAI-Compatible 路径不等于真实可用能力，必须以端到端集成测试确认为准。
3. 本计划不引入兼容双轨：旧矩阵定义将被单点替换，不保留并行口径。
4. “官方全能力”是动态目标，必须接受持续迭代；不可能一次性永久完成。

## 6. 执行顺序建议

1. 先做 P0（当天可完成，低风险）。
2. 再做 P1（防回归的基础设施，建议与 P0 同迭代完成）。
3. 最后做 P2（新增能力开发，按 OpenAI -> Grok -> Qwen 顺序）。
4. 持续执行 P2.5，直到各供应商主能力达到可用标准。

---

置信度标注：

- 高：文档与代码冲突清单（仓库内可直接验证）。
- 中高：OpenAI/xAI/Qwen 视频官方支持结论（有官方文档入口，需实现时再做接口级联调确认）。
- 中：个别供应商 Embedding/微调的公开说明粒度不一致，落地前需再做 endpoint 级验证。

## 7. 逐供应商官方文档 URL（固定入口 + 能力页）

说明：
1. 本节只收录“官方站点”URL。
2. 若能力页暂无稳定公开地址，标记为 `—`（不得用二手文章替代）。
3. 能力缩写：`C=chat`，`T=tools`，`I=image`，`V=video`，`TTS`，`STT`，`E=embedding`，`R=rerank`，`F=fine-tuning`。

| Provider | 官方文档主页 | 能力页 URL（C/T/I/V/TTS/STT/E/R/F） |
|---|---|---|
| OpenAI | https://platform.openai.com/docs | C: https://platform.openai.com/docs/guides/text<br>T: https://platform.openai.com/docs/guides/function-calling<br>I: https://platform.openai.com/docs/guides/image-generation<br>V: https://platform.openai.com/docs/guides/video-generation<br>TTS: https://platform.openai.com/docs/guides/text-to-speech<br>STT: https://platform.openai.com/docs/guides/speech-to-text<br>E: https://platform.openai.com/docs/guides/embeddings<br>R: —<br>F: https://platform.openai.com/docs/guides/fine-tuning |
| Anthropic | https://docs.anthropic.com | C: https://docs.anthropic.com/en/api/messages<br>T: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/overview<br>I: —<br>V: —<br>TTS: —<br>STT: —<br>E: —<br>R: —<br>F: — |
| Gemini (Google) | https://ai.google.dev/gemini-api/docs | C: https://ai.google.dev/gemini-api/docs/text-generation<br>T: https://ai.google.dev/gemini-api/docs/function-calling<br>I: https://ai.google.dev/gemini-api/docs/image-generation<br>V: https://ai.google.dev/gemini-api/docs/video-generation<br>TTS: https://ai.google.dev/gemini-api/docs/speech-generation<br>STT: https://ai.google.dev/gemini-api/docs/speech-to-text<br>E: https://ai.google.dev/gemini-api/docs/embeddings<br>R: —<br>F: https://ai.google.dev/gemini-api/docs/model-tuning |
| xAI (Grok) | https://docs.x.ai | C: https://docs.x.ai/docs/guides/chat<br>T: https://docs.x.ai/docs/guides/function-calling<br>I: https://docs.x.ai/docs/guides/image-generation<br>V: https://docs.x.ai/docs/guides/video-generation<br>TTS: —<br>STT: —<br>E: https://docs.x.ai/docs/guides/embeddings<br>R: —<br>F: — |
| DeepSeek | https://api-docs.deepseek.com | C: https://api-docs.deepseek.com/api/create-chat-completion<br>T: https://api-docs.deepseek.com/guides/function_calling<br>I: —<br>V: —<br>TTS: —<br>STT: —<br>E: —<br>R: —<br>F: — |
| Qwen（阿里云百炼） | https://help.aliyun.com/zh/model-studio/ | C: https://help.aliyun.com/zh/model-studio/developer-reference/text-generation<br>T: https://help.aliyun.com/zh/model-studio/developer-reference/qwen-function-calling<br>I: https://help.aliyun.com/zh/model-studio/developer-reference/text-to-image-api-reference<br>V: https://help.aliyun.com/zh/model-studio/developer-reference/text-to-video-api-reference<br>TTS: —<br>STT: —<br>E: https://help.aliyun.com/zh/model-studio/developer-reference/text-embedding-synchronous-api-reference<br>R: https://help.aliyun.com/zh/model-studio/developer-reference/text-rerank-api-reference<br>F: — |
| GLM（智谱） | https://docs.bigmodel.cn | C: https://docs.bigmodel.cn/cn/guide/models/chatglm<br>T: https://docs.bigmodel.cn/cn/guide/models/function_calling<br>I: https://docs.bigmodel.cn/cn/guide/models/cogview<br>V: https://docs.bigmodel.cn/cn/guide/models/cogvideo<br>TTS: https://docs.bigmodel.cn/cn/guide/models/voice<br>STT: —<br>E: https://docs.bigmodel.cn/cn/guide/models/embeddings<br>R: https://docs.bigmodel.cn/cn/guide/models/rerank<br>F: https://docs.bigmodel.cn/cn/guide/model-fine-tuning/intro |
| Doubao（火山方舟） | https://www.volcengine.com/docs/82379 | C: —<br>T: —<br>I: —<br>V: https://www.volcengine.com/docs/82379/1949118<br>TTS: —<br>STT: —<br>E: —<br>R: —<br>F: — |
| Kimi（Moonshot） | https://platform.moonshot.cn/docs | C: —<br>T: —<br>I: —<br>V: —<br>TTS: —<br>STT: —<br>E: —<br>R: —<br>F: — |
| Mistral | https://docs.mistral.ai | C: https://docs.mistral.ai/capabilities/completion/<br>T: https://docs.mistral.ai/capabilities/function_calling/<br>I: —<br>V: —<br>TTS: —<br>STT: https://docs.mistral.ai/capabilities/audio_transcription/<br>E: https://docs.mistral.ai/capabilities/embeddings/<br>R: —<br>F: https://docs.mistral.ai/capabilities/fine_tuning/ |
| Hunyuan（腾讯） | https://cloud.tencent.com/document/product/1729 | C: https://cloud.tencent.com/document/product/1729/111007<br>T: —<br>I: —<br>V: —<br>TTS: —<br>STT: —<br>E: —<br>R: —<br>F: — |
| MiniMax | https://platform.minimaxi.com/docs | C: —<br>T: —<br>I: —<br>V: —<br>TTS: —<br>STT: —<br>E: —<br>R: —<br>F: — |
| Llama（Meta） | https://llama.developer.meta.com/docs | C: —<br>T: —<br>I: —<br>V: —<br>TTS: —<br>STT: —<br>E: —<br>R: —<br>F: — |

> 注：部分国内平台文档路径会按产品线重构；若 URL 失效，以对应“官方文档主页”站内导航为准并在本表更新。

## 8. 三态矩阵（官方支持 / 代码实现 / 已验证）

判定口径：
1. `官方支持`：上表存在该能力官方页，且未标记为 `—`。
2. `代码实现`：当前仓库 provider 层存在可用实现（不返回 not-supported）。
3. `已验证`：仓库存在对应能力测试（包含成功路径或显式 not-supported 行为测试）。

单元格格式：`官/码/验`，取值 `✅` 或 `❌`。

| Provider | Chat | Tools | Image | Video | TTS | STT | Embedding | Rerank | Fine-tuning |
|---|---|---|---|---|---|---|---|---|---|
| OpenAI | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/❌ | ✅/✅/✅ |
| Anthropic | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/❌ | ❌/❌/✅ |
| Gemini | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/❌ | ✅/✅/✅ |
| DeepSeek | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/❌ | ❌/❌/✅ |
| Qwen | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ❌/✅/✅ | ❌/❌/✅ | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/✅ |
| GLM | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/❌ | ✅/✅/✅ | ❌/❌/✅ | ✅/✅/❌ | ✅/✅/✅ | ✅/✅/✅ |
| Grok | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ✅/✅/✅ | ❌/❌/❌ | ❌/❌/✅ |
| Doubao | ❌/✅/✅ | ❌/✅/✅ | ❌/✅/✅ | ✅/❌/✅ | ❌/✅/✅ | ❌/❌/✅ | ❌/✅/✅ | ❌/❌/❌ | ❌/❌/✅ |
| Kimi | ❌/✅/✅ | ❌/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/❌ | ❌/❌/✅ |
| Mistral | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ✅/✅/✅ | ✅/✅/✅ | ❌/❌/❌ | ✅/✅/✅ |
| Hunyuan | ✅/✅/✅ | ❌/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/❌ | ❌/❌/✅ |
| MiniMax | ❌/✅/✅ | ❌/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/❌ | ❌/❌/✅ |
| Llama | ❌/✅/✅ | ❌/✅/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/✅ | ❌/❌/❌ | ❌/❌/✅ |

补充说明（Rerank）：
1. 当前 `rerank` 在 `llm/capabilities/rerank/*` 独立实现（Cohere/Jina/Voyage），并非上述 chat provider 的 `multimodal.go` 能力。
2. 若要“逐供应商全能力对齐”，需把 rerank provider 与本 13 家供应商显式绑定，避免矩阵里长期出现 `官✅/码❌` 的空档。

## 9. 能力添加执行看板（供应商 x 能力）

使用规则：
1. 状态仅用 `[ ]`（未完成）和 `[x]`（已完成）。
2. 仅对“第 8 节里 `官✅/码❌` 的缺口”建任务，避免重复劳动。
3. 每打勾一项，必须同步更新第 8 节对应单元格（至少 `码` 与 `验`）。
4. 禁止兼容双轨：新实现落地后，删除旧分支/旧口径。

列说明：
1. `E2E`：端到端调用 + 回归测试 + 文档同步都已完成才可打勾。
2. `实现入口`：优先落在 `llm/providers/<provider>/multimodal.go`，并经 `gateway` 暴露。

| Provider | Capability | 当前差距（来自第 8 节） | 主要改动位置 | E2E |
|---|---|---|---|---|
| OpenAI | Video | 官✅/码✅/验✅ | `llm/providers/openai/multimodal.go`, `llm/providers/async_poller.go`, `llm/providers/openai/multimodal_test.go` | [x] |
| Grok | Video | 官✅/码✅/验✅ | `llm/providers/grok/multimodal.go`, `llm/providers/async_poller.go`, `llm/providers/grok/multimodal_test.go` | [x] |
| Qwen | Video | 官✅/码✅/验✅ | `llm/providers/qwen/multimodal.go`, `llm/providers/qwen/provider_test.go` | [x] |
| Qwen | Rerank | 官✅/码✅/验✅ | `llm/capabilities/rerank/*`, `llm/gateway/gateway.go`, provider 映射注册 | [x] |
| Gemini | STT | 官✅/码✅/验✅ | `llm/providers/gemini/multimodal.go`, `llm/providers/gemini/multimodal_test.go` | [x] |
| Gemini | Fine-tuning | 官✅/码✅/验✅ | `llm/providers/gemini/multimodal.go`, `llm/providers/gemini/multimodal_test.go` | [x] |
| GLM | TTS | 官✅/码✅/验✅ | `llm/providers/glm/multimodal.go`, `llm/providers/glm/multimodal_test.go` | [x] |
| GLM | Fine-tuning | 官✅/码✅/验✅ | `llm/providers/glm/multimodal.go`, `llm/providers/glm/multimodal_test.go` | [x] |
| GLM | Rerank | 官✅/码✅/验✅ | `llm/capabilities/rerank/*`, `llm/gateway/gateway.go`, provider 映射注册 | [x] |
| Mistral | Fine-tuning | 官✅/码✅/验✅ | `llm/providers/mistral/multimodal.go`, `llm/providers/mistral/provider_test.go` | [x] |

### 9.1 跨层公共任务（完成后可批量推进上表）

- [x] `types/`：补齐文生视频/图生视频、口播 profile、数字人驱动模式的统一类型。
- [x] `llm/core`：能力契约补齐，避免 provider 私有字段泄漏到上层。
- [x] `llm/gateway`：新增能力路由与参数校验，支持统一能力入口。
- [x] `llm/capabilities`：建立 chat-provider 到 rerank-provider 的显式绑定策略。
- [x] `tests`：新增统一用例模板（成功/上游错误/超时轮询）并在各 provider 套用。
- [x] `docs`：第 7 节 URL 与第 8 节矩阵在同一提交更新。

### 9.2 完成定义（DoD）

- [x] 代码：对应能力不再返回 `not supported`，并通过 provider 测试。
- [x] 联调：真实上游或 mock 协议级联调通过（含错误路径）。
- [x] 文档：第 8 节该格更新为 `官✅/码✅/验✅`（或显式修正 `官`）。
- [x] 架构：通过 `architecture_guard_test.go` / 架构守卫脚本检查。

### 9.3 本轮变更记录（2026-03-03）

- [x] `types` 统一类型落地：新增 `types/multimodal.go` 与 `types/multimodal_test.go`，补齐 `VideoGenerationMode(text_to_video/image_to_video)`、`NarrationProfile`、`AvatarDriveMode(text/audio/video)`。
- [x] `llm/capabilities` 请求模型对齐：`video.GenerateRequest` 新增 `mode`；`avatar.GenerateRequest` 新增 `drive_mode`、`drive_video_url`、`narration_profile`。
- [x] `llm/gateway` 参数校验落地：`validateRequest` 增加能力级 payload 校验与模式推断（video/avatar/audio 等），并新增 `llm/gateway/pipeline_validation_test.go` 覆盖错误与推断路径。
- [x] 回归通过：`go test ./types/... ./llm/gateway/... ./llm/capabilities/video/... ./llm/capabilities/avatar/...`。
- [x] `llm/core` + `llm/capabilities` 路由契约补齐：新增 `llm/core.CapabilityHints`（`chat_provider/rerank_provider`）与 `Entry.BindChatToRerank/ResolveRerankProvider`，`gateway` 统一通过 `req.Hints.ChatProvider` 解析 rerank 绑定。
- [x] 视频轮询模板补齐：新增 `llm/providers/grok/multimodal_test.go` 三路径（成功/上游错误/超时），并补 `llm/providers/qwen/provider_test.go` 超时路径。
- [x] GLM `not supported` 收敛：`GenerateAudio` 与 Fine-tuning 全链路实现（Create/List/Get/Cancel）并补 `llm/providers/glm/multimodal_test.go` 覆盖。
- [x] Rerank 供应商对齐：补 `llm/capabilities/rerank/rerank_test.go` 中 Qwen/GLM 成功与错误路径测试。

---

## 执行状态总览（自动补齐）

- [x] 已补齐章节结构
- [x] 已补齐任务状态行

