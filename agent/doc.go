// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 agent 提供 AgentFlow 的核心智能体框架。

# 概述

本包面向"可编排、可扩展、可观测"的智能体开发场景，统一了
智能体生命周期管理、任务执行流程、工具调用、记忆管理与状态控制。

框架支持多种推理与执行模式，包括 ReAct、链式推理、计划后执行、
反思式迭代以及自定义策略，适合从简单任务型 Agent 到复杂多阶段
协作流程的实现。

# 架构分层

整体能力由以下层次组成：

- [Agent] 接口层：定义统一能力边界，如 Init、Plan、Execute。
- [BaseAgent] 基础层：提供状态机、钩子、并发保护等通用能力。
- 组件层：记忆、工具、护栏、观测、持久化等可插拔模块。
- LLM 适配层：通过 llm.Provider 抽象接入不同模型提供商。

# 核心接口

  - [Agent]：智能体统一接口，提供 ID / Name / Type / Init / Plan / Execute / Observe
  - [ContextManager]：上下文管理接口，提供 GetContext / UpdateContext
  - [EventBus]：事件总线接口，提供 Publish / Subscribe / Unsubscribe
  - [ToolManager]：工具管理接口，提供 GetAllowedTools / ExecuteForAgent
  - [ToolSelector]：工具选择接口，根据上下文动态选择最优工具集
  - [MemoryWriter] / [MemoryReader] / [MemoryManager]：记忆读写与管理接口
  - [Plugin]：插件接口，支持 PreProcess / PostProcess / Middleware 扩展点

# 核心类型

  - [BaseAgent]：Agent 基础实现，内置状态机、护栏、记忆与工具管理
  - [Config]：Agent 配置，包含执行参数、模型绑定与能力开关
  - [Input] / [Output]：执行输入与输出模型
  - [PlanResult]：规划结果，包含步骤列表与推理过程
  - [Feedback]：观测反馈，用于反思与学习
  - [Event] / [EventType] / [EventHandler]：事件模型与处理器
  - [SimpleEventBus]：默认事件总线实现
  - [GuardrailsError]：护栏校验错误，包含错误类型与详情

# 异步与协作

  - [AsyncExecutor]：异步任务执行器，支持后台执行与结果回调
  - [SubagentManager]：子 Agent 管理器，支持动态注册与任务分发
  - [RealtimeCoordinator]：实时协调器，基于事件总线协调多 Agent 执行

# Prompt 工程

  - [PromptEnhancer]：Prompt 增强器，自动优化与丰富提示词
  - [PromptBundle]：Prompt 配置包，组合系统提示、示例与记忆配置
  - [PromptTemplateLibrary]：Prompt 模板库，管理可复用模板

# 插件系统

  - [PluginRegistry]：插件注册表，管理插件生命周期
  - [PluginEnabledAgent]：插件增强 Agent，在执行流程中自动触发插件

# 工具集成

  - [AgentTool]：将 Agent 封装为工具，支持 Agent 间嵌套调用
  - [DynamicToolSelector]：动态工具选择器，基于历史统计选择最优工具

# 核心能力

- 生命周期管理：统一初始化、执行、收尾与状态转换流程。
- 状态机约束：保证状态流转合法，减少异常路径下的行为漂移。
- 组件注入：通过构建器按需启用记忆、工具、护栏、反思与观测能力。
- 错误语义化：通过统一错误类型与错误码提升调试与恢复效率。
- 并发安全：基础实现内置必要同步机制，保护共享状态一致性。

# 使用方式

推荐通过构建器创建 Agent，并按需开启增强能力：

- 使用 NewAgentBuilder 配置名称、类型与执行参数。
- 通过 WithProvider 绑定主模型，必要时通过 WithToolProvider 分离工具调用模型。
- 通过 WithMemory、WithToolManager、WithReflection 等接口启用扩展能力。
- 通过 Build 生成可执行实例，再调用 Execute 处理输入任务。

# 相关子包

- agent/guardrails：输入与输出约束、注入防护、安全校验。
- agent/memory：多层记忆与检索能力。
- agent/discovery：多 Agent 能力发现与匹配。
- agent/evaluation：效果评测、指标采集与实验能力。
- agent/structured：结构化输出生成与校验。
- agent/protocol/a2a：智能体间通信协议与实现。
- agent/persistence：状态持久化与任务存储。
- agent/collaboration：多 Agent 协作角色与流水线。
- agent/deliberation：深度推理与决策引擎。
- agent/voice：语音交互与实时对话。
- agent/artifacts：产物管理与存储。
*/
package agent
