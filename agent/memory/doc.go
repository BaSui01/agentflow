// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 memory 提供面向智能体的分层记忆系统。

# 概述

本包用于解决智能体"如何记住、检索、遗忘和沉淀信息"的问题，
支持从短期上下文到长期知识的多层记忆建模，并提供衰减、整合、
向量检索与存储抽象等能力。

# 记忆层次

典型场景下可组合以下记忆形态：

- 工作记忆：短期、快速、可过期，适合当前回合上下文。
- 情节记忆：按事件序列记录交互过程与执行结果。
- 语义记忆：沉淀事实知识，支持基于语义相似度检索。
- 程序记忆：沉淀"如何做事"的策略与步骤经验。

# 核心接口

  - [MemoryStore]：通用记忆存储接口，提供 Save / Load / Delete / List / Clear
  - [EpisodicStore]：情节记忆存储接口，提供 RecordEvent / QueryEvents / GetTimeline
  - [KnowledgeGraph]：知识图谱接口，提供实体与关系的增删查操作
  - [BatchVectorStore]：扩展向量存储接口，支持批量写入
  - [Embedder]：向量嵌入接口，将文本转换为向量表示

# 核心类型

  - [EnhancedMemorySystem]：增强型多层记忆系统，整合短期、工作、长期、
    情节与语义记忆，并支持记忆整合
  - [LayeredMemory]：分层记忆管理器，组合情节、语义、工作与程序记忆
  - [IntelligentDecay]：智能衰减管理器，基于时间、访问频次与相关性
    自动淘汰低价值记忆
  - [MemoryConsolidator]：记忆整合器，将短期高价值信息迁移到长期层
  - [MemoryEntry]：记忆条目，包含内容、嵌入向量、重要性与元数据
  - [MemoryItem]：带衰减元数据的记忆项，用于智能衰减场景
  - [EpisodicEvent] / [EpisodicQuery]：情节事件与查询模型
  - [Entity] / [Relation]：知识图谱实体与关系
  - [Episode]：情节/事件记录
  - [Fact]：语义记忆中的事实条目
  - [DecayConfig] / [DecayResult] / [DecayStats]：衰减配置、结果与统计

# 默认实现

  - [InMemoryStore]：基于内存的通用存储实现
  - [InMemoryEpisodicStore]：基于内存的情节记忆存储
  - [InMemoryKnowledgeGraph]：基于内存的知识图谱实现
  - [InMemoryVectorStore]：基于内存的向量存储实现
  - [EpisodicMemory]：基于内存的情节记忆（简化版）
  - [SemanticMemory]：基于内存的语义记忆
  - [WorkingMemory]：基于内存的工作记忆
  - [ProceduralMemory]：基于内存的程序记忆

# 核心能力

- 统一存储接口：支持内存存储与可扩展后端实现。
- 重要性建模：通过分数管理记忆优先级。
- 衰减机制：按时间与访问频次衰减低价值记忆。
- 记忆整合：将短期高价值信息迁移到长期层。
- 语义检索：结合向量表示执行相似记忆召回。

# 适用场景

- 长对话上下文保持与回溯
- 用户偏好持续学习
- 任务执行经验沉淀
- 多轮规划中的状态连续性维护

# 与 agent 包协同

memory 可与 `agent` 执行流程直接集成：

- 执行前：检索相关历史经验与知识片段
- 执行中：记录关键步骤、工具结果与中间状态
- 执行后：整合高价值记忆并触发衰减治理

这样可在保证上下文稳定性的同时，逐步提升智能体长期表现。
*/
package memory
