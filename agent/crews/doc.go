// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 crews 提供基于角色分工的多智能体团队协作框架。

# 概述

本包用于解决"多个智能体如何以团队形式协同完成复杂任务"的问题。
通过定义角色（Role）、成员（CrewMember）和任务（CrewTask），
将多个智能体组织为一个 Crew，并支持多种任务分配与执行策略。

# 核心模型

  - Role：定义成员的角色名称、目标、技能列表与委派权限。
  - CrewMember：持有角色定义与底层 CrewAgent 接口的成员实例。
  - CrewTask：描述任务内容、期望输出、优先级与依赖关系。
  - Proposal / NegotiationResult：成员间的协商提案与应答机制。
  - Crew：团队容器，管理成员注册、任务队列与执行流程。

# 主要能力

  - 顺序执行（Sequential）：按任务列表顺序逐一分配并执行。
  - 层级执行（Hierarchical）：由管理者角色委派任务，支持协商拒绝与回退。
  - 共识执行（Consensus）：全体成员投票决定任务归属。
  - 自动成员匹配：根据指定分配或空闲状态选择最佳执行者。
  - 协商协议：支持 delegate、assist、inform、request 四种提案类型。

# 与 agent 包协同

crews 通过 CrewAgent 接口与上层 agent 解耦：任何实现了
Execute 和 Negotiate 方法的智能体均可作为团队成员加入 Crew，
从而在不修改核心 agent 逻辑的前提下实现多智能体协作。
*/
package crews
