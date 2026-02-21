// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 federation 提供跨组织智能体联合协作与任务编排能力。

# 概述

federation 解决的核心问题是：当多个独立部署的 Agent 节点需要协同完成任务时，
如何安全地发现彼此、分发任务并汇总结果。它通过联邦网络将分布式节点组织为
一个逻辑整体，支持跨组织边界的智能体协作。

# 核心模型

本包围绕以下类型展开：

  - FederatedNode：联邦网络中的节点，携带 endpoint、公钥、能力列表与在线状态
  - FederatedTask：跨节点分发的任务，包含优先级、超时、所需能力与执行结果
  - Orchestrator：联邦编排器，负责节点注册、任务提交、分发与结果收集
  - TaskHandler：任务处理函数，由各节点注册以响应特定类型的联邦任务

节点状态通过 NodeStatus 枚举管理（online / offline / degraded），
任务生命周期通过 TaskStatus 枚举跟踪（pending / running / completed / failed）。

# 主要能力

  - 节点发现与注册：动态加入或移除联邦节点
  - 能力匹配路由：根据 RequiredCaps 自动筛选具备对应能力的目标节点
  - 并行任务分发：向多个目标节点并发下发任务，汇总各节点执行结果
  - 心跳健康检测：周期性检查节点存活状态，自动标记离线节点
  - 本地/远程透明执行：本地节点直接调用 handler，远程节点通过 HTTPS 转发
  - TLS 安全通信：节点间通信默认启用 TLS 加密

# 与其他包协同

federation 可与 agent/handoff 配合，将跨组织的任务交接建模为联邦任务分发。
同时可与 agent/k8s 结合，在 Kubernetes 集群间构建联邦网络。
*/
package federation
