# Recent Model Families and Multimodal Matrix (Last 12 Months)

> Updated: 2026-04-21  
> Scope: mainstream model families that remained active in official docs between 2025-04-21 and 2026-04-21.  
> Note: this document is a documentation/reference baseline, not a guarantee that every upstream model is wired into every AgentFlow runtime path.

---

## 1. Chat / Reasoning Model Families

| Vendor / product | Recent mainstream families | Recommended AgentFlow path | Notes | Official sources |
|---|---|---|---|---|
| OpenAI GPT-5 | `gpt-5.4`, `gpt-5.4-pro` | `llm/providers/openai` | Prefer Responses API; `reasoning.effort`, `verbosity`, `previous_response_id`, `conversation_id` matter more than legacy sampling knobs | [Latest model guide](https://developers.openai.com/api/docs/guides/latest-model), [All models](https://developers.openai.com/api/docs/models/all-models) |
| Anthropic Claude 4 | `claude-opus-4-7`, `claude-sonnet-4-6`, `claude-haiku-4-5` | `llm/providers/anthropic` | `system` is separate; thinking/output config differs from OpenAI; tool/result blocks are provider-native | [Models overview](https://platform.claude.com/docs/en/about-claude/models/overview), [Messages API](https://platform.claude.com/docs/en/api/messages), [Extended thinking](https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking) |
| Google Gemini 3 / 2.5 | `gemini-3.1-pro-preview`, `gemini-3.1-flash-preview`, `gemini-2.5-pro`, `gemini-2.5-flash` | `llm/providers/gemini` | Gemini 3.x uses `thinkingLevel`; Gemini 2.5 uses `thinkingBudget`; structured output uses `responseMimeType` + `responseJsonSchema` | [Models](https://ai.google.dev/gemini-api/docs/models), [Thinking](https://ai.google.dev/gemini-api/docs/thinking), [GenerationConfig](https://ai.google.dev/api/rest/v1beta/GenerationConfig) |
| DeepSeek | `deepseek-chat`, `deepseek-reasoner` | `llm/providers/vendor` + `openaicompat` | Chat and reasoning are split; reasoner returns dedicated reasoning content | [Updates](https://api-docs.deepseek.com/updates/), [Reasoning model](https://api-docs.deepseek.com/guides/reasoning_model), [Chat completion](https://api-docs.deepseek.com/api/create-chat-completion) |
| Qwen 3 | `qwen3-max`, `qwen3-max-2026-01-23` | `llm/providers/vendor` + `openaicompat` | Thinking mode is explicit; structured JSON and forced tool selection need extra care | [Model list](https://www.alibabacloud.com/help/en/model-studio/user-guide/model/), [API reference](https://www.alibabacloud.com/help/en/model-studio/use-qwen-by-calling-api) |
| GLM | `glm-5.1`, `glm-4.7`, `glm-4.6`, `glm-z1-flash` | `llm/providers/vendor` + `openaicompat` | Current project keeps GLM on the OpenAI-compatible chat path | [Model center](https://www.bigmodel.cn/dev/howuse/model), [API](https://www.bigmodel.cn/dev/api) |
| xAI Grok | `grok-4.20`, `grok-4.20-reasoning` | `llm/providers/vendor` + `openaicompat` | Reasoning models reject `stop`, `presence_penalty`, `frequency_penalty`, and `reasoning_effort` | [Models](https://docs.x.ai/docs/models/), [Reasoning](https://docs.x.ai/developers/model-capabilities/text/reasoning) |
| Mistral / Magistral | `mistral-medium-latest`, `mistral-medium-2508+1`, `magistral-medium-latest` | `llm/providers/vendor` + `openaicompat` | Keep general-purpose and reasoning families explicit in configuration | [Models overview](https://docs.mistral.ai/models/overview), [API](https://docs.mistral.ai/api) |
| Kimi / Moonshot | `kimi-k2.5`, `kimi-k2-thinking` | `llm/providers/vendor` + `openaicompat` | Thinking mode is more restrictive than plain chat for `tool_choice` and sampling fields | [Intro](https://platform.moonshot.cn/docs/intro), [Kimi K2.5](https://platform.moonshot.cn/docs/guide/kimi-k2-5), [API reference](https://platform.moonshot.cn/docs/api-reference) |
| Tencent Hunyuan | `hunyuan-t1-latest`, `hunyuan-turbos-latest` | `llm/providers/vendor` + `openaicompat` | Separate reasoning (`t1`) and function-calling model families remain useful in routing | [Tencent Cloud Hunyuan docs](https://cloud.tencent.com/document/product/1729) |
| MiniMax | `MiniMax-M2.7`, `MiniMax-M2.5` | `llm/providers/vendor` + `openaicompat` | Older `abab*` models remain legacy XML-tool-call models; newer M-series models are JSON tool-call capable | [MiniMax docs](https://platform.minimaxi.com/document) |
| Doubao / Volcano Engine | `doubao-seed-1.6`, `Doubao-1.5-pro-32k` | `llm/providers/vendor` + `openaicompat` | Project fallback is still conservative for stable compatibility | [Volcano Engine Ark docs](https://www.volcengine.com/docs) |

---

## 2. Image Model Families

| Provider | Recent mainstream image models | Typical API / path | Notes | Official sources |
|---|---|---|---|---|
| OpenAI | `gpt-image-1` | Images API | Replaces older DALL·E-first mental model in most current docs | [All models](https://developers.openai.com/api/docs/models/all-models), [Image generation guide](https://developers.openai.com/api/docs/guides/image-generation) |
| Google | `imagen-4.0-generate-001`, Imagen 4 Ultra / Standard / Fast | Gemini / Imagen API | Imagen 4 is the current image generation family in official Gemini docs | [Imagen guide](https://ai.google.dev/gemini-api/docs/imagen) |
| GLM | CogView 4 family | BigModel image generation API | Official GLM model center now centers newer CogView families, not only CogView 3 | [Model center](https://www.bigmodel.cn/dev/howuse/model), [API](https://www.bigmodel.cn/dev/api) |
| FLUX / BFL | FLUX 1.1 Pro / Ultra | BFL image API | Strong external image provider in project multimodal support | [BFL docs](https://docs.bfl.ai/) |
| Ideogram | Ideogram V3 | Ideogram generate API | Current official generation family in Ideogram docs | [Ideogram API](https://developer.ideogram.ai/api-reference/api-reference/generate) |
| Stability | Stable Image Ultra | Stability image API | Common mainstream image generation family | [Stability API](https://platform.stability.ai/docs/api-reference) |
| Doubao | Seedream 3.0 | Volcano Engine image API | Used for Chinese image-generation routes in project multimodal docs | [Volcano Engine image docs](https://www.volcengine.com/docs) |

---

## 3. Video Model Families

| Provider | Recent mainstream video models | Typical API / path | Notes | Official sources |
|---|---|---|---|---|
| OpenAI | `sora-2` | Video generation API | OpenAI’s current official video generation family | [All models](https://developers.openai.com/api/docs/models/all-models), [Video generation guide](https://developers.openai.com/api/docs/guides/video-generation) |
| Google | `veo-3.1-generate-preview`, `veo-3.1-fast-generate-preview` | Gemini video API | Veo 3.1 is the active official family in Gemini docs | [Video guide](https://ai.google.dev/gemini-api/docs/video) |
| Runway | Gen-4.5, `gen4_turbo` | Runway image/video API | Mainstream external video provider in project multimodal surface | [Runway API docs](https://docs.dev.runwayml.com/) |
| Luma | Ray 2, Ray Flash 2 | Luma video API | Current Luma video families | [Luma docs](https://docs.lumalabs.ai/) |
| MiniMax | Hailuo 2.3 / 2.3 Fast | MiniMax video API | Project multimodal layer already includes MiniMax video provider support | [MiniMax docs](https://platform.minimaxi.com/document) |
| GLM | CogVideoX / CogVideoX-Flash | BigModel video API | Chinese mainstream video generation family | [Model center](https://www.bigmodel.cn/dev/howuse/model), [API](https://www.bigmodel.cn/dev/api) |

---

## 4. Speech / Audio / Realtime Model Families

| Provider | Recent mainstream speech models | Use case | Official sources |
|---|---|---|---|
| OpenAI | `gpt-realtime`, `gpt-realtime-1.5`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`, `gpt-4o-transcribe-diarize`, `gpt-4o-mini-tts` | Realtime, STT, diarized STT, TTS | [All models](https://developers.openai.com/api/docs/models/all-models), [Speech-to-text guide](https://developers.openai.com/api/docs/guides/speech-to-text), [Text-to-speech guide](https://developers.openai.com/api/docs/guides/text-to-speech), [Realtime guide](https://developers.openai.com/api/docs/guides/realtime-model-capabilities) |
| Google | `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`, `gemini-2.5-flash-native-audio-preview-*` | TTS and native audio conversations | [Speech generation](https://ai.google.dev/gemini-api/docs/speech-generation), [Native audio](https://ai.google.dev/gemini-api/docs/native-audio) |
| ElevenLabs | `eleven_v3`, `eleven_flash_v2_5`, `eleven_turbo_v2_5` | TTS | [ElevenLabs models](https://elevenlabs.io/docs/models/) |
| Deepgram | `nova-3`, `nova-3-medical`, Aura-2 family | STT and TTS | [Deepgram STT models](https://developers.deepgram.com/docs/models-languages-overview), [Aura-2 TTS](https://developers.deepgram.com/docs/text-to-speech) |
| MiniMax | `speech-2.5-hd`, `speech-2.8-turbo` | TTS | [MiniMax docs](https://platform.minimaxi.com/document) |
| Qwen | Qwen Audio family | Audio understanding | [Qwen API reference](https://www.alibabacloud.com/help/en/model-studio/use-qwen-by-calling-api) |

---

## 5. Maintenance Rules

1. Treat "latest" as date-scoped. Always add an absolute date.
2. Keep project fallback models separate from official-latest wording.
3. Prefer provider-local request mapping and validation over business-layer ad-hoc payload assembly.
4. When a model family splits into standard vs reasoning variants, keep the routing rule explicit in code and docs.
5. For new multimodal families, update both:
   - `docs/en/tutorials/02.ProviderConfiguration.md`
   - `docs/cn/guides/模型与媒体端点参考.md`
