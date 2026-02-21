// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 handoff 提供智能体间的任务交接协议与上下文传递能力。

# 概述

handoff 解决的核心问题是：当一个 Agent 无法独立完成任务或需要将子任务
委派给更合适的 Agent 时，如何安全、可靠地完成任务移交并保持上下文连续性。
它定义了一套完整的交接协议，涵盖任务发起、能力匹配、接受确认、执行跟踪
与结果回传的全生命周期。

# 核心模型

本包围绕以下类型展开：

  - Handoff：交接记录，包含来源/目标 Agent、任务、状态、上下文与结果
  - Task：被交接的任务，携带类型、描述、输入数据与优先级
  - HandoffContext：交接上下文，传递对话历史、变量与父级交接链
  - HandoffResult：交接执行结果，包含输出数据、错误信息与耗时
  - HandoffAgent：支持交接的 Agent 接口，声明能力并处理交接请求
  - HandoffManager：交接管理器，协调 Agent 注册、能力匹配与交接执行

交接状态通过 HandoffStatus 枚举管理：
pending -> accepted -> in_progress -> completed / failed / rejected。

# 主要能力

  - Agent 能力注册：通过 AgentCapability 声明可处理的任务类型与优先级
  - 智能路由：根据任务类型自动匹配最优 Agent（按优先级排序）
  - 同步/异步交接：通过 HandoffOptions.Wait 控制是否等待执行完成
  - 超时与重试：支持交接超时控制与最大重试次数配置
  - 上下文传递：在交接过程中保持对话历史与变量的连续性

# 与其他包协同

handoff 可与 agent/federation 配合实现跨组织的任务委派，
也可与核心 agent 包集成，作为多 Agent 协作的基础通信协议。
*/
package handoff
