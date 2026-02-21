// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 conversation 提供多智能体对话编排与对话状态管理能力。

# 概述

conversation 解决两个核心问题：一是如何让多个 Agent 在同一对话中
协作（谁先说、谁后说、何时终止）；二是如何管理对话的分支、回滚与
快照，使对话历史具备版本控制能力。

# 核心接口

  - ConversationAgent：对话参与者接口，定义 ID / Name /
    SystemPrompt / Reply / ShouldTerminate 五个方法
  - SpeakerSelector：发言人选择器接口，决定下一轮由哪个 Agent 发言
  - LLMClient：LLM 调用接口，供 LLMSelector 使用

# 主要能力

  - 多种对话模式：通过 ConversationMode 枚举支持 RoundRobin（轮询）、
    Selector（LLM 选择）、GroupChat（自由讨论）、Hierarchical（层级委派）、
    AutoReply（自动应答链）五种编排模式
  - 对话生命周期：Conversation.Start 驱动完整的对话循环，
    支持最大轮次、最大消息数、超时、终止词等多维终止条件
  - 分支管理：ConversationTree 提供 Fork / SwitchBranch /
    MergeBranch / DeleteBranch 等 Git 风格的对话分支操作
  - 状态回滚：Rollback / RollbackN 支持按状态 ID 或步数
    回退到历史节点
  - 快照机制：Snapshot / FindSnapshot / RestoreSnapshot 支持
    为关键对话节点打标签并随时恢复
  - 序列化：Export / Import 支持对话树的 JSON 持久化与恢复
  - 群聊管理：GroupChatManager 统一管理多个并行对话实例

# 内置实现

  - RoundRobinSelector：按 Agent 列表顺序轮流选择发言人
  - LLMSelector：基于 LLM 智能选择下一位发言人（无 LLM 时
    回退为轮询）

# 与其他包协同

conversation 是多 Agent 协作的编排层。它依赖 agent/context
进行上下文窗口管理，与 agent/tools 配合实现对话中的工具调用，
对话产生的中间结果可通过 agent/artifacts 持久化。
*/
package conversation
