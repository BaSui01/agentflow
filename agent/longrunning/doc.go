// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 longrunning 提供面向智能体的长时任务执行与断点恢复能力。

# 概述

本包用于解决智能体"如何可靠地执行耗时较长的多步骤任务"的问题。
当任务包含多个顺序步骤且可能因超时、故障或人为干预而中断时，
longrunning 提供状态管理、检查点持久化与自动恢复机制，
确保任务可以从中断处继续执行而非从头开始。

# 核心模型

  - ExecutionState: 执行状态枚举，涵盖 initialized、running、paused、
    resuming、completed、failed、cancelled 七种生命周期状态。
  - Execution: 长时执行实例，记录进度、当前步骤、检查点列表与元数据。
  - Checkpoint: 可恢复的检查点快照，包含步骤编号与序列化状态。
  - StepFunc: 单步执行函数签名 func(ctx, state) (state, error)。

# 主要能力

  - 多步骤编排: 将复杂任务拆分为有序的 StepFunc 序列，逐步执行。
  - 检查点持久化: 按可配置间隔自动保存执行状态到磁盘 JSON 文件。
  - 暂停与恢复: 支持运行时 Pause/Resume，配合检查点实现断点续跑。
  - 自动重试: 单步失败时按配置次数重试，带指数退避。
  - 心跳监控: 定期更新 LastUpdate 时间戳，便于外部健康检测。
  - 执行管理: 通过 Executor 统一创建、启动、查询和加载执行实例。

# 与 agent 包协同

longrunning 可与 agent 执行流程集成：

  - 将智能体的多轮推理或工具调用链建模为 StepFunc 序列
  - 在每个检查点保存中间推理状态，支持故障后从最近检查点恢复
  - 通过 ListExecutions/GetExecution 提供任务进度查询能力
*/
package longrunning
