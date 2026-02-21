// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 deliberation 提供智能体行动前的多步推理与审议引擎。

# 概述

本包用于解决"智能体在执行工具调用前如何进行深度思考"的问题。
通过可配置的审议模式，让智能体在行动前经历理解、评估、规划、
自我批判等推理步骤，从而提升决策质量与置信度。

# 核心模型

  - DeliberationMode：审议模式枚举，支持 immediate（直接执行）、
    deliberate（完整推理循环）和 adaptive（上下文自适应）。
  - Engine：审议引擎，驱动多轮推理循环并管理置信度阈值。
  - ThoughtProcess：单步推理记录，包含类型（understand / evaluate /
    plan / critique）、内容与置信度。
  - Decision：审议最终产出，包含动作、工具选择、参数与推理依据。
  - Reasoner：基于 LLM 的推理接口，由外部注入具体实现。

# 主要能力

  - 多轮迭代推理：在置信度未达阈值时自动触发新一轮思考。
  - 自我批判：当 EnableSelfCritique 开启时，对低置信度决策进行反思。
  - 超时控制：通过 MaxThinkingTime 限制单次审议的最大耗时。
  - 运行时模式切换：支持在运行期间动态调整审议模式。

# 与 agent 包协同

deliberation 作为 agent 决策管线的前置环节：agent 在调用工具前
将任务提交给 Engine.Deliberate，获取经过推理验证的 Decision 后
再执行实际操作，从而在效率与决策质量之间取得平衡。
*/
package deliberation
