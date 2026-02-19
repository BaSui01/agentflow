# 📚 AgentFlow 中文文档

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

### 📚 教程

| 文档 | 描述 | 难度 |
|------|------|------|
| [🚀 快速开始](./tutorials/01.快速开始.md) | 核心概念和基础使用 | ⭐ |
| [🔌 Provider 配置指南](./tutorials/02.Provider配置指南.md) | 13+ LLM 提供商配置详解 | ⭐⭐ |
| [🤖 Agent 开发教程](./tutorials/03.Agent开发教程.md) | 创建智能体的完整指南 | ⭐⭐ |
| [🔧 工具集成说明](./tutorials/04.工具集成说明.md) | 工具注册、执行和 ReAct 循环 | ⭐⭐⭐ |
| [📊 工作流编排](./tutorials/05.工作流编排.md) | 链式、并行、DAG 工作流 | ⭐⭐⭐ |
| [🖼️ 多模态处理](./tutorials/06.多模态处理.md) | 图像、音频、视频处理 | ⭐⭐⭐ |
| [🔍 检索增强 RAG](./tutorials/07.检索增强RAG.md) | 向量存储和知识检索 | ⭐⭐⭐⭐ |
| [👥 多 Agent 协作](./tutorials/08.多Agent协作.md) | 多智能体协同工作 | ⭐⭐⭐⭐ |

---

## 🌟 核心特性

### 🔌 统一 LLM 抽象层
- **13+ 提供商支持**: OpenAI、Claude、Gemini、DeepSeek、通义千问、智谱 GLM、Grok、Kimi 等
- **统一接口**: 一套代码适配所有 LLM
- **弹性容错**: 自动重试、熔断器、幂等性保证
- **A/B 测试路由**: 多变体流量分配、粘性路由、动态权重调整
- **统一 Token 计数器**: tiktoken 适配器 + CJK 估算器
- **Provider 重试包装器**: 指数退避重试，仅重试可恢复错误

### 🤖 智能 Agent 系统
- **状态管理**: 完整的生命周期管理
- **记忆系统**: 短期/长期记忆、向量检索
- **工具调用**: 原生 Function Calling + ReAct 循环
- **双模型架构 (toolProvider)**: 便宜模型做工具调用，贵模型做内容生成
- **Browser Automation**: 浏览器自动化（chromedp 驱动、连接池、视觉适配器）

### 📊 工作流编排
- **多种模式**: 链式、并行、DAG、条件路由
- **高级特性**: 循环、子图、检查点、错误恢复
- **可视化**: Mermaid/DOT 图生成
- **熔断器**: DAG 节点级熔断保护（Closed/Open/HalfOpen 三态机）
- **YAML DSL 编排语言**: 声明式工作流定义，支持变量插值、条件分支、循环

### 🔍 RAG 检索增强
- **混合检索**: 向量搜索 + 关键词搜索
- **BM25 Contextual Retrieval**: 上下文检索，BM25 参数可调，IDF 缓存
- **Multi-hop 去重**: 多跳推理链，四阶段去重流程，DedupStats 统计

### 🖼️ 多模态能力
- **输入理解**: 图像、音频、视频分析
- **内容生成**: DALL-E、Flux 图像生成；TTS/STT 语音处理

### 🛡️ 企业级能力
- **API 安全中间件**: API Key 认证、IP 限流、CORS、Panic 恢复
- **成本控制与预算管理**: Token 计数、周期重置、成本报告
- **配置热重载与回滚**: 文件监听自动重载、版本化历史、一键回滚

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
        APIKey: os.Getenv("OPENAI_API_KEY"),
        Model:  "gpt-4o-mini",
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
