// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 hitl 提供 Human-in-the-Loop 工作流中断与恢复能力。

# 概述

hitl 用于在 Agent 执行过程中注入人工确认节点。当工作流到达需要
人工介入的关键决策点时，InterruptManager 会暂停执行并等待人类
响应，支持审批、补充输入、代码审查、断点调试和错误处理五种中断
类型，适用于高风险决策和关键业务流程。

# 核心接口

  - InterruptStore：中断持久化存储接口，定义 Save / Load / List /
    Update 四个方法，包内提供 InMemoryInterruptStore 默认实现
  - InterruptHandler：中断事件回调函数，用于通知外部系统（如 UI、
    消息队列）有新的中断等待处理

# 主要能力

  - InterruptManager：中断管理器核心，负责创建中断、等待响应、
    解决/拒绝/取消/超时处理，内部通过 channel 实现阻塞等待。
    使用 NewInterruptManager 创建实例
  - 五种中断类型（InterruptType）：Approval（审批）、Input（补充输入）、
    Review（审查）、Breakpoint（断点）、Error（错误处理）
  - 五种中断状态（InterruptStatus）：Pending（等待中）、Resolved（已解决）、
    Rejected（已拒绝）、Timeout（已超时）、Canceled（已取消）
  - 超时机制：每个中断可配置独立超时（默认 24 小时），超时后自动
    标记状态并释放等待的 goroutine
  - 并发安全：所有操作通过 RWMutex 保护，resolveOnce 确保中断
    只被解决一次

# 数据类型

  - Interrupt：中断核心数据结构，包含工作流 ID、节点 ID、中断类型、
    状态、可选项列表、输入 JSON Schema、人工响应等字段
  - InterruptOptions：中断创建参数，包括 WorkflowID、NodeID、Type、
    Title、Description、Timeout、CheckpointID 和自定义 Metadata
  - Option：审批中断的可选项，包含 ID、Label、Description 和 IsDefault
  - Response：人工对中断的响应，包含 OptionID、Input、Comment、
    Approved 标志、UserID 和 Metadata

# 与其他包协同

  - agent/streaming：流式交互中可在关键节点插入人工中断
  - agent/voice：语音 Agent 的高风险指令可触发审批流程
  - agent/guardrails：安全校验失败时可升级为人工审查中断
*/
package hitl
