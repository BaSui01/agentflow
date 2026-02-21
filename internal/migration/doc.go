// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 migration 提供数据库 Schema 迁移管理能力，支持 PostgreSQL、
MySQL 与 SQLite 三种数据库，基于 golang-migrate 实现。

# 概述

本包通过 embed.FS 内嵌各数据库方言的 SQL 迁移文件，结合
golang-migrate 引擎实现版本化的 Schema 变更管理。支持正向迁移、
回滚、按步执行、跳转到指定版本以及强制设置版本号等操作。

# 核心接口与类型

  - Migrator：迁移器接口，定义 Up/Down/DownAll/Steps/Goto/Force/
    Version/Status/Info/Close 等完整操作集。
  - DefaultMigrator：Migrator 的默认实现，封装 golang-migrate 实例
    与数据库连接管理。
  - Config：迁移配置，包含数据库类型、连接 URL、迁移表名与锁超时。
  - DatabaseType：数据库类型枚举（postgres/mysql/sqlite）。
  - MigrationStatus / MigrationInfo：迁移状态与摘要信息。
  - CLI：命令行交互层，封装 Migrator 提供格式化输出。

# 主要能力

  - 多数据库支持：通过 DatabaseType 与内嵌 SQL 文件自动适配方言。
  - 工厂函数：NewMigratorFromConfig / NewMigratorFromDatabaseConfig /
    NewMigratorFromURL 支持从不同配置源快速创建迁移器。
  - CLI 集成：CLI 类型提供 RunUp/RunDown/RunStatus/RunInfo 等
    面向终端的格式化操作。
  - 辅助工具：ParseDatabaseType 解析类型字符串，BuildDatabaseURL
    按方言拼接连接 URL。
*/
package migration
