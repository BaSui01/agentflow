// Copyright (c) AgentFlow Authors.
// Licensed under the MIT License.

/*
Package workflow 提供工作流编排与执行引擎。

# 概述

workflow 包实现了 AgentFlow 的工作流系统，支持链式、路由、并行和 DAG
四种编排模式。DAG 引擎提供条件分支、循环、并行执行、子图嵌套、熔断器、
Checkpoint 时间旅行以及可视化构建器等高级能力。

# 核心接口与类型

  - Runnable           — 通用执行接口 Execute(ctx, input) (output, error)
  - Workflow           — 工作流接口（Runnable + Name + Description）
  - Step / Handler     — 工作流步骤与处理器接口
  - Router             — 路由决策接口
  - ChainWorkflow      — 顺序链式工作流
  - RoutingWorkflow    — 基于路由器的分发工作流
  - ParallelWorkflow   — 并行执行 + Aggregator 聚合
  - DAGWorkflow        — DAG 有向无环图工作流
  - DAGBuilder         — Fluent API 构建 DAG（含环检测与孤立节点检测）
  - DAGExecutor        — DAG 执行器（依赖解析、重试、熔断、历史记录）
  - VisualBuilder      — 可视化画布 JSON → DAG 转换器

# 主要能力

  - DAG 节点类型：Action、Condition、Loop（while/for/foreach）、Parallel、
    SubGraph、Checkpoint
  - 错误策略：FailFast / Skip（含 FallbackValue）/ Retry（指数退避）
  - 熔断器：CircuitBreaker + CircuitBreakerRegistry（Closed/Open/HalfOpen）
  - Checkpoint：EnhancedCheckpointManager 支持 Save / Resume / Rollback / Compare
  - 状态管理：Channel[T] + StateGraph + Reducer（泛型状态通道与合并策略）
  - Agent 适配：AgentStep / AgentAdapter / NativeAgentAdapter / AgentRouter
  - 序列化：DAGDefinition 支持 JSON / YAML 导入导出与校验
  - 执行历史：ExecutionHistory + ExecutionHistoryStore 全链路追踪
*/
package workflow
