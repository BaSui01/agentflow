// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 reasoning 提供面向智能体的多种高级推理模式。

# 概述

本包用于解决智能体"如何系统性地思考、规划和解决复杂问题"的需求。
它将学术界和工程实践中的主流推理范式封装为统一接口，使智能体可以
根据任务特征选择最合适的推理策略，并通过注册表机制实现动态发现与切换。

# 核心接口

  - ReasoningPattern: 推理模式统一接口，定义 Execute(ctx, task) 和 Name() 方法。
  - PatternRegistry: 推理模式注册表，支持注册、查询、列举与注销。

# 核心模型

  - ReasoningResult: 推理结果，包含最终答案、置信度、步骤链、Token 消耗与延迟。
  - ReasoningStep: 推理步骤，类型涵盖 thought、action、observation、
    evaluation、backtrack，支持嵌套子步骤与评分。

# 推理模式

  - Tree-of-Thought (ToT): 思维树模式，并行探索多条推理路径，
    通过 Beam Search 剪枝保留最优分支，适合需要创造性探索的开放问题。
  - Plan-and-Execute: 先规划后执行模式，支持执行中自适应重规划，
    适合目标明确但路径不确定的结构化任务。
  - ReWOO: 无观测推理模式，先生成完整计划再批量执行所有步骤，
    最后综合观测结果，适合步骤间依赖明确的高效批处理场景。
  - Reflexion: 反思模式，通过"尝试 → 评估 → 反思 → 改进"的迭代循环
    逐步提升输出质量，适合需要自我纠错的任务。
  - Dynamic Planner: 动态规划模式，构建计划树并支持回溯与备选路径切换，
    适合不确定性高、需要灵活应变的复杂任务。
  - Iterative Deepening: 迭代加深研究模式，以宽度优先生成查询、
    深度优先递归探索，逐层加深理解，适合开放式研究与信息综合。

# 与 agent 包协同

reasoning 可与 agent 执行流程集成：

  - 通过 PatternRegistry 注册可用推理模式，运行时按任务类型动态选择
  - 推理过程中调用 agent 的工具执行器（ToolExecutor）与 LLM Provider
  - ReasoningResult 的步骤链可输出至 observability 包进行追踪与审计
*/
package reasoning
