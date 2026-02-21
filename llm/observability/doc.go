// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 observability 提供 LLM 调用的可观测性能力，涵盖指标采集、
分布式追踪与成本核算三大模块。

# 概述

本包基于 OpenTelemetry 标准，为 LLM 请求全生命周期提供统一的
观测手段。从请求发起到响应结束，自动记录延迟、Token 消耗、
错误率、缓存命中与降级事件，并支持将追踪数据导出至外部系统。

典型使用场景：

  - 实时监控 LLM 请求量、延迟分布与错误率。
  - 按 Provider、Model、Tenant 维度统计 Token 消耗与成本。
  - 追踪多步 Agent 链路中每一次 LLM 调用与工具调用。
  - 多轮对话级别的端到端追踪与反馈收集。

# 核心接口

  - Metrics：基于 OpenTelemetry Meter 的指标收集器，提供请求计数、
    Token 计数、延迟直方图、成本直方图与活跃请求 Gauge 等指标。
  - Tracer：追踪器，管理 Run 与 Trace 的生命周期，支持 LLM 调用、
    工具调用与自定义 Span 的嵌套追踪。
  - TraceExporter：追踪导出接口，用于将 Run/Trace 数据推送至外部系统。
  - CostCalculator：成本计算器，内置主流模型价格表，支持动态更新。
  - CostTracker：会话级成本追踪器，实时汇总 Token 与费用统计。
  - ConversationTracer：多轮对话追踪器，记录每个回合的输入、输出、
    Token 用量与工具调用详情。

# 主要能力

  - 指标采集：请求总量、Token 总量、错误数、降级数、缓存命中/未命中、
    请求延迟、Token 分布与单次请求成本。
  - 分布式追踪：与 OpenTelemetry Span 集成，支持 LLM、Tool、Chain、
    Agent、Retriever 五种 TraceType。
  - 成本核算：内置 OpenAI、Claude、Gemini、Qwen、ERNIE、GLM 等
    模型价格，支持批量更新与会话级汇总。
*/
package observability
