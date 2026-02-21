// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 skills 提供 Agent 技能的注册、发现、加载与调用管理能力。

# 概述

skills 实现了一套标准化的技能生命周期管理体系，使 Agent 能够按需
发现并加载可复用的能力模块。每个技能包含指令、工具列表、资源文件
和使用示例，支持从目录加载（SKILL.json 清单）或通过 Builder 模式
在内存中构建。

# 核心接口

  - SkillManager：技能管理器接口，定义发现、加载、查询、注册与目录扫描能力
  - SkillHandler：技能执行函数签名，接收 JSON 输入并返回 JSON 输出
  - Registry：并发安全的技能注册表，支持按 ID、名称、分类和标签检索

# 主要能力

  - 技能发现：根据任务描述自动匹配并按相关度排序返回最佳技能
  - 目录扫描：递归扫描文件系统中的 SKILL.json 清单，建立技能索引
  - 依赖管理：加载技能时自动解析并加载依赖项，检测循环依赖
  - 调用统计：Registry 自动跟踪每个技能的调用次数、成功率和平均延迟
  - 导入导出：支持技能定义的 JSON 序列化，便于跨实例迁移
  - Builder 模式：通过 SkillBuilder 链式构建技能，简化编程式注册

# 与其他包协同

  - agent/voice：技能可作为语音 Agent 的后端处理能力
  - llm：通过 ToToolSchema 将技能转换为 LLM 工具调用格式
  - agent/guardrails：技能指令可接入安全校验链路
*/
package skills
