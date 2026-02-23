// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 cache 提供基于 Redis 的缓存管理能力，支持连接池、健康检查、
JSON 序列化与统计信息采集。

# 概述

本包封装 go-redis 客户端，为上层业务提供统一的缓存读写接口。
Manager 负责连接生命周期管理，包括初始化、健康检查与优雅关闭。
支持可选 TLS 加密连接，适用于生产环境安全要求。

# 核心类型

  - Manager：缓存管理器，持有 Redis 客户端与连接池配置，
    提供 Get/Set/Delete/Exists/Expire 等基础操作，
    以及 GetJSON/SetJSON 便捷序列化方法。
  - Config：缓存配置，包含地址、密码、连接池大小、默认 TTL、
    TLS 开关与健康检查间隔等参数。
  - Stats：缓存统计信息，包含命中率、键数量、内存使用与连接数。

# 主要能力

  - 键值读写：支持字符串与 JSON 两种模式的缓存存取。
  - 连接池管理：通过 PoolSize 与 MinIdleConns 控制连接复用。
  - 健康检查：后台定时 Ping 检测，异常时通过 zap 日志告警。
  - 优雅关闭：Close 方法安全释放底层 Redis 连接。
  - 错误语义：提供 ErrCacheMiss 哨兵错误与 IsCacheMiss 判断函数。
*/
package cache
