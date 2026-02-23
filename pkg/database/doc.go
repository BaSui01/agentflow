// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 database 提供基于 GORM 的数据库连接池管理，支持健康检查、
统计信息采集与事务重试。

# 概述

本包通过 PoolManager 封装 GORM 与 database/sql 的连接池配置，
统一管理连接生命周期、空闲回收与最大连接数限制。后台健康检查
定时探活，异常时通过 zap 日志输出诊断信息。

# 核心类型

  - PoolManager：连接池管理器，持有 GORM DB 实例与底层 sql.DB，
    提供 DB()、Ping()、Stats()、Close() 等生命周期方法。
  - PoolConfig：连接池配置，包含最大空闲连接数、最大打开连接数、
    连接最大生命周期、空闲超时与健康检查间隔。
  - PoolStats：友好格式的连接池统计信息。
  - TransactionFunc：事务回调函数类型。

# 主要能力

  - 连接池调优：通过 MaxIdleConns/MaxOpenConns/ConnMaxLifetime 精细控制。
  - 健康检查：后台定时 PingContext 探活，输出连接数与空闲数。
  - 事务管理：WithTransaction 提供单次事务执行，
    WithTransactionRetry 支持指数退避重试（死锁、序列化失败等场景）。
  - 统计采集：GetStats 返回结构化的连接池运行指标。
*/
package database
