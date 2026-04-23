// Package skills 提供 Agent 技能发现、加载与执行能力。
//
// # 职责划分
//
// 本包存在两套并行体系，职责边界如下：
//
// ## Registry（运行时注册 + 执行）
//
//   - 职责：技能运行时注册、按 ID 查找、按类别/标签搜索、执行调用（Invoke）
//   - 数据结构：SkillDefinition + SkillHandler，以 SkillInstance 形式存储
//   - 典型用法：Agent 在运行时通过 Register 注册技能，通过 Invoke 执行
//   - 生命周期：进程内内存，无持久化
//
// ## SkillManager（发现 + 加载 + 评分）
//
//   - 职责：技能发现（DiscoverSkills）、目录扫描（ScanDirectory）、索引刷新（RefreshIndex）、
//     按任务匹配与评分、加载/卸载技能
//   - 数据结构：Skill + SkillMetadata，支持磁盘目录与内存注册
//   - 典型用法：根据任务描述发现并加载最匹配的技能，支持依赖加载与缓存
//   - 生命周期：可扫描目录、可持久化索引
//
// ## 协作关系
//
// Registry 与 SkillManager 可独立使用，也可通过 DiscoveryBridge 桥接：
// SkillManager 负责发现与加载，Registry 负责注册与执行。
package tools
