// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 observability 提供面向智能体的可观测性与可解释性系统。

# 概述

本包用于解决智能体"运行过程不透明、难以调试和审计"的问题。
它从三个维度构建可观测能力：指标采集（Metrics）、分布式追踪（Tracing）
和可解释性（Explainability），使智能体的每一次决策、每一步推理
都可被记录、量化和回溯。

# 核心接口

  - EvaluationStrategy: 评估策略接口，对智能体输入输出进行质量评分。
  - ObservabilitySystem: 顶层门面，聚合 MetricsCollector、Tracer 和 Evaluator。

# 核心模型

  - AgentMetrics: 智能体级指标，涵盖任务成功率、延迟分位数（P50/P95/P99）、
    Token 消耗效率、输出质量均值与成本统计。
  - Trace / Span / SpanEvent: 分布式追踪模型，支持父子 Span 嵌套与事件标注。
  - Decision / Alternative / Factor: 决策记录模型，记录决策类型、推理依据、
    备选方案及影响因子。
  - ReasoningTrace / ReasoningStep: 完整推理链路追踪，关联 Session 与 Agent。
  - AuditReport / TimelineEvent: 审计报告，按时间线聚合步骤与决策事件。
  - Benchmark / BenchmarkCase / BenchmarkResult: 基准测试框架，
    支持注册数据集并对智能体进行批量评估。

# 主要能力

  - 指标采集: 按 Agent 维度记录任务执行指标，支持延迟百分位与成本追踪。
  - 分布式追踪: 通过 Tracer 记录执行 Span 树，支持跨步骤关联分析。
  - 可解释性追踪: 通过 ExplainabilityTracker 记录推理步骤与决策链路，
    生成人类可读的决策解释和审计报告。
  - 质量评估: 通过可插拔的 EvaluationStrategy 对输出进行多维度评分。
  - 基准测试: 注册 Benchmark 数据集，批量运行并统计成功率、均分与延迟。

# 与 agent 包协同

observability 可嵌入 agent 执行流程的各阶段：

  - 执行前: StartTrace 开启追踪，记录输入上下文
  - 执行中: AddSpan 标注工具调用，RecordDecision 记录路由与策略选择
  - 执行后: EndTrace 结束追踪，RecordTask 汇总指标，GenerateAuditReport 输出审计
*/
package observability
