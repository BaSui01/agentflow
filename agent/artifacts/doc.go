// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 artifacts 为智能体提供产物的生成、存储与生命周期管理能力。

# 概述

artifacts 解决的核心问题是：智能体在执行过程中产生的文件、数据、
代码片段、模型输出等中间或最终产物，需要统一的存储、检索、版本控制
与自动清理机制。本包提供从创建到归档的完整生命周期管理。

# 核心接口

  - ArtifactStore：产物存储抽象接口，定义 Save / Load / Delete /
    List / Archive 等操作，支持可插拔的后端实现
  - Artifact：产物元数据模型，包含 ID、类型、状态、校验和、
    标签、版本号、过期时间等完整描述信息

# 主要能力

  - 类型化存储：支持 file / data / image / code / output / model
    六种产物类型，通过 ArtifactType 枚举区分
  - 生命周期状态：pending → uploading → ready → archived → deleted，
    由 ArtifactStatus 驱动状态流转
  - 版本管理：通过 CreateVersion 基于已有产物创建新版本，
    自动维护 ParentID 与 Version 链
  - 自动清理：Manager.Cleanup 定期扫描并删除过期产物，
    基于 TTL 与 ExpiresAt 判定
  - 查询过滤：ArtifactQuery 支持按 SessionID、Type、Status、
    Tags、CreatedBy 等维度组合检索
  - Functional Options：通过 WithMetadata / WithTags / WithTTL
    等选项灵活配置产物创建参数

# 内置实现

FileStore 提供基于本地文件系统的 ArtifactStore 实现，
每个产物存储为独立目录（含 data 文件与 metadata.json），
并维护全局 index.json 索引。适用于单机部署与开发调试场景。

# 与其他包协同

artifacts 通常由 Agent 执行流程中的工具调用产生，
Manager 作为统一入口注入到 Agent 运行时，
使得产物管理与业务逻辑解耦。
*/
package artifacts
