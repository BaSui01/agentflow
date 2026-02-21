// 版权所有 2024 AgentFlow Authors. 保留所有权利。
// 此源代码的使用由 MIT 许可规范，该许可可以
// 在 LICENSE 文件中找到。

/*
示例 18_advanced_agent_features 演示了 AgentFlow 的高级 Agent 特性。

# 演示内容

本示例展示四项进阶能力：

  - Federated Orchestration：联邦编排，支持多节点注册与能力发现，
    实现跨节点的分布式 Agent 协调
  - Deliberation Mode：深度思考模式，支持 Immediate 与 Deliberate 两种推理策略，
    可配置最大思考时间、最低置信度及自我批判机制
  - Long-Running Executor：长时任务执行器，支持多步骤流水线、
    Checkpoint 持久化及自动恢复
  - Skills Registry：技能注册中心，支持按 Category 和 Tag 检索，
    提供 Research、Coding、Data 等技能分类

# 运行方式

	go run .
*/
package main
