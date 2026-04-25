# 📚 AgentFlow 中文文档

> 官方入口：sdk.New(opts).Build(ctx)；单 Agent：agent/runtime；多 Agent：agent/team；显式编排：workflow/runtime。

> 高性能 Go 语言 AI Agent 框架 - 统一 LLM 抽象、智能路由、工具调用、工作流编排

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/License-MIT-green?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Providers-13+-blue?style=flat-square" alt="Providers">
</p>

---

## 🚀 快速导航

### 📖 入门指南

| 文档 | 描述 | 预计阅读 |
|------|------|----------|
| [⚡ 五分钟快速开始](./getting-started/00.五分钟快速开始.md) | 从零运行第一个程序 | 5 分钟 |
| [📦 安装与配置](./getting-started/01.安装与配置.md) | 详细安装步骤和配置选项 | 10 分钟 |
| [🚀 框架入口与快速开始](./getting-started/02.框架入口与快速开始.md) | 官方入口、最小可用示例与并发策略 | 10 分钟 |
| [🏖️ 沙箱环境配置](./getting-started/03.沙箱环境配置.md) | 启用代码执行与隔离环境 | 10 分钟 |
| [🧰 SDK 工具注册与编排示例](./getting-started/04.SDK工具注册与编排示例.md) | ToolManager、RetrievalProvider、Team 与 Workflow 官方示例 | 15 分钟 |

### 📚 教程

| 文档 | 描述 | 难度 |
|------|------|------|
| [🚀 快速开始](./tutorials/01.快速开始.md) | 核心概念和基础使用 | ⭐ |
| [🔌 Provider 配置指南](./tutorials/02.Provider配置指南.md) | 13+ LLM 提供商配置详解 | ⭐⭐ |
| [🤖 Agent 开发教程](./tutorials/03.Agent开发教程.md) | 创建智能体的完整指南 | ⭐⭐ |
| [🔧 工具集成说明](./tutorials/04.工具集成说明.md) | 工具注册、执行和 ReAct 循环 | ⭐⭐⭐ |
| [📊 工作流编排](./tutorials/05.工作流编排.md) | 链式、并行、DAG 工作流 | ⭐⭐⭐ |
| [🖼️ 多模态处理](./tutorials/06.多模态处理.md) | 图像、音频、视频处理 | ⭐⭐⭐ |
| [🎬 多模态框架 API](./tutorials/21.多模态框架API.md) | 能力层多模态 HTTP 接口 | ⭐⭐⭐ |
| [🔍 检索增强 RAG](./tutorials/07.检索增强RAG.md) | 向量存储和知识检索 | ⭐⭐⭐⭐ |
| [👥 Team 多 Agent 协作](./tutorials/08.多Agent协作.md) | 官方 team 门面与多 Agent 协作模式 | ⭐⭐⭐⭐ |
| [🔗 Hosted 工具与 MCP](./tutorials/09.Hosted工具与MCP.md) | 托管工具和 MCP 协议集成 | ⭐⭐⭐ |
| [📊 工作流编排进阶](./tutorials/10.工作流编排进阶.md) | 高级工作流模式与 DSL | ⭐⭐⭐⭐ |
| [💰 成本追踪](./tutorials/11.成本追踪.md) | Token 计数与成本管理 | ⭐⭐ |

### 🏗️ 架构与框架设计

| 文档 | 描述 | 适用场景 |
|------|------|----------|
| [`../architecture/README.md`](../architecture/README.md) | 当前架构文档索引与官方入口总览 | 不确定该看哪份架构文档时先看这里 |
| [`../architecture/Agent框架现状与收口改进计划-2026-04-25.md`](../architecture/Agent框架现状与收口改进计划-2026-04-25.md) | 当前 Agent 框架能力盘点、缺口与收口 checklist | 想判断项目完善程度、安排后续 Agent 框架收口 |
| [`../architecture/Workflow-Agent与Agentic-Agent现状建议补充-2026-04-25.md`](../architecture/Workflow-Agent与Agentic-Agent现状建议补充-2026-04-25.md) | Workflow Agent / Agentic Agent 完成度与 `[X]` / `[ ]` 补充建议 | 想快速看已完成、未完成和下一步优先级 |
| [`../architecture/ADRs/004-多Agent团队抽象.md`](../architecture/ADRs/004-多Agent团队抽象.md) | `agent/team` public surface 与多 Agent 边界契约 | 想修改 TeamBuilder、执行模式或多 Agent facade |
| [`../architecture/我的Agent框架设计参考-2026-04-23.md`](../architecture/我的Agent框架设计参考-2026-04-23.md) | 面向自定义 Agent 框架的设计参考 | 想基于外部框架经验设计自己的 Agent 框架 |
| [`../architecture/权限控制系统重构与引入方案-2026-04-24.md`](../architecture/权限控制系统重构与引入方案-2026-04-24.md) | 统一鉴权、授权、审批、审计的重构方案 | 想引入权限控制系统或完善工具审批链路 |
| [`../architecture/权限控制系统详细设计-2026-04-24.md`](../architecture/权限控制系统详细设计-2026-04-24.md) | package / 接口 / 数据结构级权限设计 | 要开始实现权限控制系统时优先阅读 |
| [`../architecture/启动装配链路与组合根说明.md`](../architecture/启动装配链路与组合根说明.md) | 服务启动链路、组合根边界与热更新真相 | 想理解 `cmd -> bootstrap -> api -> domain` 主链 |
| [`../architecture/原生Provider与SDK边界说明.md`](../architecture/原生Provider与SDK边界说明.md) | OpenAI / Anthropic / Gemini 原生 SDK 边界 | 想改 Provider 或 SDK 接入边界 |
| [`../architecture/Provider原生Token计数说明.md`](../architecture/Provider原生Token计数说明.md) | 原生 token counting 约束与预算准入边界 | 想改预算、token counting 或 provider admission |
| [`../architecture/Provider工具负载映射说明.md`](../architecture/Provider工具负载映射说明.md) | tool payload 在 gateway / provider / sdk 之间的映射规则 | 想改 function calling / tool payload 语义 |
| [`../architecture/FunctionCalling回归矩阵说明-2026-04-25.md`](../architecture/FunctionCalling回归矩阵说明-2026-04-25.md) | provider tool/function calling 回归矩阵与验收命令 | 想补 OpenAI / Anthropic / Gemini / XML fallback 工具调用回归 |
| [`../architecture/Channel路由扩展架构说明.md`](../architecture/Channel路由扩展架构说明.md) | channel-based routing 的设计与迁移说明 | 想做渠道路由扩展或替换 `MultiProviderRouter` |
| [`../architecture/Channel路由外部接入模板-中文版.md`](../architecture/Channel路由外部接入模板-中文版.md) | 外部项目最小接入模板（中文） | 想复用 `ChannelRoutedProvider` 接业务侧 channel/key/mapping 系统 |

### 🗄️ 历史归档

| 文档 | 描述 |
|------|------|
| [`../archive/agent-framework-legacy-2026-04/Agent框架现状评估与主流框架调研-2026-04-23.md`](../archive/agent-framework-legacy-2026-04/Agent框架现状评估与主流框架调研-2026-04-23.md) | 历史可用性评估与主流框架调研，不作为当前契约 |
| [`../archive/agent-framework-legacy-2026-04/AgentFlow收口改造方案与实施清单-2026-04-23.md`](../archive/agent-framework-legacy-2026-04/AgentFlow收口改造方案与实施清单-2026-04-23.md) | 历史收口路线，不作为当前实施清单 |
| [`../archive/refactor-plans-2026-04/我的Agent框架一次性硬切换重构计划-2026-04-24.md`](../archive/refactor-plans-2026-04/我的Agent框架一次性硬切换重构计划-2026-04-24.md) | 历史硬切换总计划，用于追溯背景 |
| [`../archive/Gemini官方SDK迁移清理计划.md`](../archive/Gemini官方SDK迁移清理计划.md) | Gemini 官方 SDK 迁移历史 |
| [`../archive/LLM供应商维度重构分析.md`](../archive/LLM供应商维度重构分析.md) | vendor profile 重构历史 |
| [`../archive/归档说明.md`](../archive/归档说明.md) | 历史快照与归档文档说明，不作为当前契约真相 |

### 📘 指南

| 文档 | 描述 | 难度 |
|------|------|------|
| [🧭 模型厂商与模型中文命名规范](./guides/模型厂商与模型中文命名规范.md) | 统一厂商名、模型名、latest 写法与引用口径 | ⭐ |
| [🗂️ 近12个月主流多模态模型总表](./guides/近12个月主流多模态模型总表.md) | 统一近 12 个月 chat / image / video / TTS / STT 主流模型口径 | ⭐ |
| [🧩 模型字段与 Agent 框架接入指南](./guides/模型字段与Agent框架接入指南.md) | 说明上游模型字段如何落到 `Model / Control / Tools` 主面，以及当前实现缺口 | ⭐⭐ |
| [✅ 最佳实践](./guides/best-practices.md) | AgentFlow 使用建议与常见设计约束 | ⭐⭐ |

### 🧭 文档分层导航

| 层次 | 先看什么 | 适用场景 |
|------|----------|----------|
| 官方主流模型 | [近12个月主流多模态模型总表](./guides/近12个月主流多模态模型总表.md) | 需要确认最新一年的主流 chat / image / video / speech 模型 |
| 字段映射 / 运行时主面 | [模型字段与 Agent 框架接入指南](./guides/模型字段与Agent框架接入指南.md) | 需要把上游模型字段对齐到 `Model / Control / Tools`，或评估当前实现缺口 |
| 项目统一总览 | [`./guides/模型与媒体端点参考.md`](./guides/模型与媒体端点参考.md) | 需要看 provider `/models`、chat / image / video / speech 总览 |
| 当前代码能力 | [`./guides/多模态能力端点参考.md`](./guides/多模态能力端点参考.md) | 需要确认项目当前真正已实现哪些多模态能力 |
| 厂商接入与配置 | [`./guides/视频与图像厂商及端点说明.md`](./guides/视频与图像厂商及端点说明.md) | 需要接入图像 / 视频厂商、看共享 key / endpoint / 配置关系 |
| 教程示例 | [Provider 配置指南](./tutorials/02.Provider配置指南.md) / [多模态处理](./tutorials/06.多模态处理.md) | 需要复制示例、快速上手 |
| 历史背景 | [`../archive/多模态实现总结.md`](../archive/多模态实现总结.md) / [`../archive/多模态功能实现报告.md`](../archive/多模态功能实现报告.md) | 需要追溯历史设计与阶段性实现背景 |

---

## 🌟 核心特性

### 🔌 统一 LLM 抽象层
- **13+ 提供商支持**: OpenAI、Anthropic Claude、Google Gemini、DeepSeek、通义千问 Qwen、智谱 GLM、xAI Grok、Kimi 等
- **统一接口**: 一套代码适配所有 LLM
- **弹性容错**: 自动重试、熔断器、幂等性保证
- **A/B 测试路由**: 多变体流量分配、粘性路由、动态权重调整
- **统一 Token 计数器**: tiktoken 适配器 + CJK 估算器
- **Provider 重试包装器**: 指数退避重试，仅重试可恢复错误
- **API Key 池**: 多 Key 轮询、限流检测
- **OpenAI 兼容层**: 统一适配 OpenAI 兼容 API

### 🤖 智能 Agent 系统
- **状态管理**: 完整的生命周期管理
- **Reflection 机制**: 自我评估与迭代改进
- **官方单 Agent 主链**: 默认只走 `react`，`reflection` 作为可选质量增强
- **高级 / 实验策略**: `Reflexion`、`ReWOO`、`Plan-Execute` 需显式启用；`ToT`、`Dynamic Planner`、`Iterative Deepening` 属于实验能力
- **多层记忆**: 短期/工作记忆、长期记忆、情节记忆、语义记忆、程序性记忆
- **工具调用**: 原生 Function Calling + ReAct 循环；非原生 provider 自动走 XML tool-calling fallback
- **双模型架构 (toolProvider)**: 便宜模型优先处理工具调用链路，贵模型做内容生成
- **MCP/A2A 协议**: 完整 Agent 互操作协议栈 (Google A2A & Anthropic MCP)
- **Guardrails**: 输入/输出验证、PII 检测、注入防护
- **Skills 系统**: 动态技能加载
- **Human-in-the-Loop**: 人工审批节点
- **Thought Signatures**: 推理链签名，保持多轮推理连续性
- **声明式 Agent 加载器**: YAML/JSON 定义 Agent，工厂自动装配

### 📊 工作流编排
- **多种模式**: 链式、并行、DAG、条件路由
- **高级特性**: 循环、子图、检查点、错误恢复
- **可视化**: Mermaid/DOT 图生成
- **熔断器**: DAG 节点级熔断保护（Closed/Open/HalfOpen 三态机）
- **YAML DSL 编排语言**: 声明式工作流定义，支持变量插值、条件分支、循环
- **DAG 节点并行执行**: 分支并发执行与结果聚合
- **状态持久化**: 检查点 (Checkpoint) 的保存与恢复

### 🔍 RAG 检索增强
- **混合检索**: 向量搜索 + 关键词搜索
- **BM25 Contextual Retrieval**: 上下文检索，BM25 参数可调，IDF 缓存
- **Multi-hop 去重**: 多跳推理链，四阶段去重流程，DedupStats 统计
- **Web 增强检索**: 本地 RAG + 实时 Web 搜索混合检索
- **语义缓存**: 基于向量相似度的响应缓存
- **多向量库**: Qdrant、Pinecone、Milvus、Weaviate 及内置 InMemoryStore
- **Graph RAG**: 知识图谱检索增强
- **查询路由**: 智能查询分发与改写

### 🖼️ 多模态能力
- **输入理解**: 图像、音频、视频分析
- **Embedding**: OpenAI、Gemini、Cohere、Jina、Voyage
- **Image**: `gpt-image-1`、Imagen 4、Flux、Stability、Ideogram、通义万相、智谱、文心一格、豆包、腾讯混元、可灵
- **Video**: `sora-2`、Runway Gen-4.5 / `gen4_turbo`、Veo 3.1、Gemini、可灵、Luma、MiniMax、即梦 Seedance
- **Audio**: `gpt-4o-mini-tts`、`gpt-4o-transcribe`、ElevenLabs、Deepgram
- **Music**: Suno、MiniMax
- **3D**: Meshy、Tripo
- **Rerank**: Cohere、Qwen、GLM

### 🛡️ 企业级能力
- **API 安全中间件**: API Key 认证、IP 限流、CORS、Panic 恢复
- **可观测性**: Prometheus 指标、OpenTelemetry 追踪
- **成本控制与预算管理**: Token 计数、周期重置、成本报告
- **配置热重载与回滚**: 文件监听自动重载、版本化历史、一键回滚
- **MCP WebSocket 心跳重连**: 指数退避重连、连接状态监控
- **金丝雀发布 (Canary)**: 分阶段流量切换（10%→50%→100%）、自动回滚、错误率/延迟监控

---

## HTTP API 概览

| 分组 | 主要端点 |
|------|----------|
| **System** | `GET /health`, `/healthz`, `/ready`, `/readyz`, `/version` |
| **Chat** | `POST /api/v1/chat/completions`, `/completions/stream`, `POST /v1/chat/completions` (OpenAI Chat 兼容), `POST /v1/responses` (OpenAI Responses 兼容), `POST /v1/messages` (Anthropic Messages 兼容) |
| **Agent** | `GET /api/v1/agents`, `POST /api/v1/agents/execute`, `/execute/stream`, `/plan` |
| **Provider** | `GET /api/v1/providers`, `GET/POST /api/v1/providers/{id}/api-keys` |
| **Tools** | `GET/POST /api/v1/tools`, `POST /api/v1/tools/reload`, `PUT/DELETE /api/v1/tools/{id}` |
| **Multimodal** | `POST /api/v1/multimodal/image`, `/video`, `/chat`, `/plan` |
| **Protocol** | `GET /api/v1/mcp/resources`, `POST /api/v1/mcp/tools`, `GET /api/v1/a2a/.well-known/agent.json`, `POST /api/v1/a2a/tasks` |
| **RAG** | `POST /api/v1/rag/query`, `POST /api/v1/rag/index` |
| **Workflow** | `POST /api/v1/workflows/execute`, `POST /api/v1/workflows/parse`, `GET /api/v1/workflows` |
| **Config** | `GET/PUT /api/v1/config`, `POST /api/v1/config/reload`, `/rollback` |

说明：Google Gemini Developer API `POST /v1beta/models/{model}:generateContent`、`POST /v1beta/models/{model}:streamGenerateContent` 以及 Vertex AI `POST /v1/projects/{project}/locations/{location}/publishers/google/models/{model}:generateContent` 等路径属于 provider 出站协议，由 `llm/providers/gemini` / `llm/providers/vendor` 负责，不是本项目新增 HTTP 入站路由。

---

## 📦 快速安装

```bash
# 安装 AgentFlow
go get github.com/BaSui01/agentflow
```

## 🎯 最小示例

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/llm/providers"
    "github.com/BaSui01/agentflow/llm/providers/openai"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    
    provider := openai.NewOpenAIProvider(providers.OpenAIConfig{
        BaseProviderConfig: providers.BaseProviderConfig{
            APIKey: os.Getenv("OPENAI_API_KEY"),
            Model:  "gpt-5.4",
        },
    }, logger)

    resp, _ := provider.Completion(context.Background(), &llm.ChatRequest{
        Messages: []llm.Message{
            {Role: llm.RoleUser, Content: "你好！"},
        },
    })

    fmt.Println("🤖", resp.Choices[0].Message.Content)
}
```

---

## 🗺️ 学习路径

```
新手入门                    进阶开发                    高级应用
   │                          │                          │
   ▼                          ▼                          ▼
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│ 五分钟快速开始 │ ──────▶ │ Provider 配置 │ ──────▶ │ 工作流编排   │
└─────────────┘         └─────────────┘         └─────────────┘
       │                      │                        │
       ▼                      ▼                        ▼
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│ 安装与配置   │ ──────▶ │ Agent 开发   │ ──────▶ │ 多 Agent 协作│
└─────────────┘         └─────────────┘         └─────────────┘
       │                      │                        │
       ▼                      ▼                        ▼
┌─────────────┐         ┌─────────────┐         ┌─────────────┐
│ 快速开始     │ ──────▶ │ 工具集成     │ ──────▶ │ RAG 检索增强 │
└─────────────┘         └─────────────┘         └─────────────┘
```

---

## 🔗 相关链接

- 📦 [GitHub 仓库](https://github.com/BaSui01/agentflow)
- 🌐 [English Documentation](../en/README.md)
- 💬 [问题反馈](https://github.com/BaSui01/agentflow/issues)

---

## 📄 许可证

AgentFlow 采用 [MIT 许可证](../../LICENSE) 开源。

---

<p align="center">
  <sub>Made with ❤️ by AgentFlow Team</sub>
</p>
