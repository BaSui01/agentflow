// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 k8s 提供 Kubernetes 场景下的智能体生命周期管理与运维能力。

# 概述

k8s 解决的核心问题是：如何以 Kubernetes 原生方式管理 Agent 的部署、
扩缩容、健康检查与自愈。它通过自定义 CRD（AgentCRD）和 Operator 模式，
将 Agent 的期望状态声明式地映射为 Kubernetes 资源，由 Operator 持续
调谐（reconcile）实际状态使其趋近期望状态。

# 核心模型

本包围绕以下类型展开：

  - AgentCRD：Agent 自定义资源定义，包含 Spec（期望状态）与 Status（观测状态）
  - AgentSpec：期望状态声明，涵盖副本数、模型配置、资源配额与扩缩容策略
  - AgentCRDStatus：观测状态，记录副本就绪数、当前指标与 Condition 列表
  - AgentOperator：Operator 控制器，驱动 reconcile / scale / healthCheck 循环
  - AgentInstance：运行中的 Agent 实例，携带状态与运行时指标

Agent 生命周期通过 AgentPhase 枚举管理：
Pending -> Running -> Scaling / Degraded / Failed / Terminating。

# 主要能力

  - 声明式管理：通过 AgentCRD 描述 Agent 期望状态，Operator 自动调谐
  - 自动扩缩容：基于 CPU / 内存 / QPS / 延迟等指标动态调整副本数
  - 健康检查与自愈：周期性探测实例健康，自动替换不健康实例
  - 指标采集：持续收集实例级与 Operator 级运行指标
  - CRD 导入导出：支持 JSON 格式的 AgentCRD 序列化与反序列化
  - Leader Election：支持多副本 Operator 的主节点选举

# 与其他包协同

k8s 可与 agent/federation 配合，在多集群间构建联邦 Agent 网络。
Agent 实例的任务委派可通过 agent/handoff 协议完成。
*/
package k8s
