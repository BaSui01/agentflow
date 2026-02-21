// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 context 为智能体提供上下文窗口管理、压缩与自适应调控能力。

# 概述

context 解决的核心问题是：LLM 的上下文窗口有限，而智能体的
对话历史会持续增长。本包通过多级压缩策略与窗口管理机制，
确保消息始终适配模型的 token 预算，同时尽可能保留关键信息。

# 核心模型

  - Engineer：统一上下文管理引擎，根据 token 使用率自动选择
    压缩级别（None / Normal / Aggressive / Emergency），
    提供 Manage / MustFit / GetStatus 等核心方法
  - WindowManager：上下文窗口管理器，实现三种裁剪策略，
    满足 agent.ContextManager 接口约定
  - AgentContextManager：面向 Agent 的集成组件，封装 Engineer
    并提供 PrepareMessages / ShouldCompress / GetRecommendation
    等便捷方法

# 主要能力

  - 多级压缩：基于 SoftLimit(70%) / WarnLimit(85%) / HardLimit(95%)
    三道阈值，自动触发 Normal → Aggressive → Emergency 压缩
  - 窗口策略：WindowManager 支持 SlidingWindow（保留最近 N 条）、
    TokenBudget（按 token 预算从新到旧裁剪）、Summarize（LLM
    摘要压缩旧消息）三种策略
  - 紧急兜底：MustFit 最多重试 5 轮压缩，最终通过 hardTruncate
    强制裁剪，保证消息永远不会超出上下文窗口
  - 模型适配：DefaultAgentContextConfig 为 GPT-4 / Claude-3 /
    Gemini 等主流模型提供预设参数
  - 统计追踪：Stats 记录压缩次数、紧急压缩次数、平均压缩比、
    节省 token 数等运行指标

# 扩展方式

  - 实现 Summarizer 接口接入自定义 LLM 摘要能力
  - 通过 SetSummaryProvider 为 AgentContextManager 注入摘要函数
  - 调整 Config 中的阈值与策略参数适配不同场景

# 与其他包协同

context 是 Agent 执行链路的核心前置环节。每次调用 LLM 前，
Agent 通过 PrepareMessages 或 MustFit 确保消息适配上下文窗口，
与 agent/conversation 的多轮对话和 agent/tools 的工具调用
产生的长输出协同工作。
*/
package context
