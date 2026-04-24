# 多模态实现总结

> 文档定位：当前基线说明 + 历史实现摘要
> 更新时间：2026-04-21

本文档不再按"初次开发时的实现流水账"展开，而是收口为三层：

1. **当前基线**：今天这个仓库里，多模态能力是怎么组织的、看哪些文档、改哪些位置。
2. **历史说明**：2026-02 那一轮多模态接口覆盖工作到底完成了什么，供追溯与历史理解。
3. **配合使用建议**：当前文档和历史文档的协作阅读方式。

---

## 一、当前基线

### 1.1 阅读顺序

如果你现在要做多模态相关开发或评审，建议按这个顺序看：

1. `docs/cn/guides/近12个月主流多模态模型总表.md`
   - 看最近 12 个月主流官方模型家族
2. `./模型与媒体端点参考.md`
   - 看 provider `/models`、chat / image / video / speech 总览
3. `./能力端点参考.md`
   - 看项目当前**已实现**的多模态能力矩阵和端点
4. `./视频与图像厂商及端点说明.md`
   - 看图像 / 视频厂商接入、共享 key、配置关系
5. 本文档
   - 看当前实现边界与历史背景

### 1.2 当前架构边界

- **OpenAI / Anthropic Claude / Gemini** 保持原生 provider 主路径
- **compat 厂商 chat 主链** 统一走 `llm/providers/vendor.NewChatProviderFromConfig(...)`
- `llm/providers/<vendor>/multimodal.go` 只表示**厂商能力实现位置**，不再表示公共 chat 构造入口
- `deepseek / kimi / llama / hunyuan` 的独立 compat chat 目录已删除，chat 已收敛到 `vendor + openaicompat`

### 1.3 当前主要实现落点

| 能力 | 当前主要代码位置 | 说明 |
|---|---|---|
| 多模态接口与类型 | `llm/multimodal.go` | `MultiModalProvider` / `EmbeddingProvider` / `FineTuningProvider` 等契约 |
| OpenAI-compatible 辅助函数 | `llm/providers/multimodal_helpers.go` | 通用 image / video / audio / embedding helper |
| 原生 OpenAI 多模态 | `llm/providers/openai/multimodal.go` | 图像 / 音频 / 转录 / embedding / 微调 |
| 原生 Gemini 多模态 | `llm/providers/gemini/multimodal.go` | 图像 / 视频 / 音频 / 转录 / embedding |
| GLM / Qwen / Doubao / Grok / MiniMax / Mistral 多模态 | `llm/providers/<vendor>/multimodal.go` | provider-local 能力实现 |
| 图像能力层 | `llm/capabilities/image/*` | 多厂商图像生成能力 |
| 视频能力层 | `llm/capabilities/video/*` | Runway / Veo / Sora / Kling / Luma / MiniMax / Seedance |
| 语音能力层 | `llm/capabilities/audio/*` | OpenAI / ElevenLabs / Deepgram 等 |

### 1.4 当前维护原则

1. **看代码事实，不看旧报告措辞。**
2. **优先区分"当前已实现能力"和"官方主流模型"。**
3. **chat 主链与多模态能力层分开理解。**
4. **新增厂商能力先补文档入口，再补能力矩阵，再补实现。**
5. **不要再把 provider 目录下的说明文档当成主文档源；主文档统一收口到 `docs/multimodal/`。**

### 1.5 当前维护建议

1. **把"当前代码已实现"与"官方最近 12 个月主流模型"分开。**
2. **多模态说明统一收口到 `docs/multimodal/`，不要再新增散落在 provider 目录的主文档。**
3. **当仓库结构变了，优先修正文档基线，而不是继续堆历史过程描述。**

---

## 二、历史说明（2026-02 多模态接口覆盖工作）

### 2.1 当时完成了什么

那一轮工作的核心目标是：

- 为 AgentFlow 的多 provider 框架补齐统一多模态接口
- 让所有 provider 至少完成接口覆盖（支持或标准化返回 not supported）
- 建立统一请求/响应类型与通用 helper
- 给主要 provider 补上图像 / 视频 / 音频 / embedding / 微调入口

### 2.2 历史成果摘要

| 项目 | 历史结果 |
|---|---|
| 接口定义 | 完成 `MultiModalProvider` / `EmbeddingProvider` / `FineTuningProvider` |
| 请求/响应类型 | 完成图像、视频、音频、转录、embedding、微调相关 DTO |
| 通用 helper | 完成 OpenAI-compatible helper |
| Provider 覆盖 | 13/13 provider 达到"接口覆盖完成" |
| 文档 | 完成当时版本的端点文档、实现总结、报告 |

### 2.3 历史遗产（仍然有价值）

| 历史产物 | 现在的价值 |
|---|---|
| `llm/multimodal.go` | 仍是多模态契约核心 |
| `llm/providers/multimodal_helpers.go` | 仍是 compat helper 基线 |
| provider `multimodal.go` 文件 | 仍是各厂商能力实现主位置 |
| 首轮文档 | 现在可作为历史背景，但不应直接当成当前基线 |

### 2.4 为什么要压缩旧描述

旧报告的问题不是"错"，而是：

- 太像阶段性庆功稿，不像当前维护文档
- 容易把"接口覆盖完成"和"厂商能力完整支持"混为一谈
- 容易把旧模型名、旧路径、旧入口继续带到现在

所以现在把它收缩成"历史里程碑说明"，只保留必要背景。

---

## 三、当前和历史怎么配合使用

- **看今天怎么做**：优先看"当前基线"部分（1.1 ~ 1.5）
- **看当年为什么这么设计**：再看"历史说明"部分（2.1 ~ 2.4）
- **看具体能力与端点**：回到 `./能力端点参考.md`
- **看厂商和模型总览**：回到 `./模型与媒体端点参考.md` 与 `docs/cn/guides/近12个月主流多模态模型总表.md`

---

## 四、当前建议怎么用这份文档

- **做实现**：先回到 `./能力端点参考.md` 和真实代码
- **做模型选型**：先看 `docs/cn/guides/近12个月主流多模态模型总表.md`
- **做厂商接入**：先看 `./视频与图像厂商及端点说明.md`
- **做历史追溯**：再看本文的"历史说明"部分
