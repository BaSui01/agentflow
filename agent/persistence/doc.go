// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 persistence 提供面向智能体的消息与任务持久化存储抽象及多后端实现。

# 概述

本包用于解决智能体系统中"消息可靠投递"与"异步任务状态管理"两大持久化需求。
通过统一的接口抽象与可插拔的后端实现，使上层业务无需关心底层存储细节，
同时支持从开发测试到分布式生产的平滑切换。

# 核心接口

  - Store: 所有存储的基础接口，提供 Close 和 Ping 健康检查。
  - MessageStore: 消息持久化接口，支持保存、查询、确认（Ack）、
    重试与过期清理，适用于智能体间的可靠消息传递。
  - TaskStore: 异步任务持久化接口，支持任务创建、状态流转、
    进度更新、故障恢复与定期清理。

# 核心模型

  - Message: 持久化消息，包含 Topic 路由、发送/接收方、确认状态、
    重试计数与过期时间。
  - AsyncTask: 异步任务，包含状态机（pending → running → completed/failed）、
    优先级、进度、超时与父子任务关系。
  - StoreConfig / RetryConfig / CleanupConfig: 统一配置体系，
    涵盖存储类型选择、指数退避重试策略与自动清理策略。

# 后端实现

  - Memory: 内存实现，适合开发与测试，重启后数据丢失。
  - File: 基于文件的实现，原子写入 JSON 索引，适合单节点生产部署。
  - Redis: 基于 Redis 的实现，利用 Sorted Set 索引与 Pipeline 批量操作，
    适合分布式生产部署。

# 使用方式

通过工厂函数按配置创建存储实例：

	store, err := persistence.NewMessageStore(config)
	taskStore, err := persistence.NewTaskStore(config)

也可使用 MustNewMessageStore / MustNewTaskStore 在初始化阶段快速创建。

# 与 agent 包协同

persistence 为 agent 执行流程提供底层存储支撑：

  - 消息存储: 持久化智能体间通信消息，保证消息不丢失
  - 任务存储: 跟踪异步任务生命周期，支持服务重启后自动恢复未完成任务
*/
package persistence
