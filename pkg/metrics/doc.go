// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 metrics 提供基于 Prometheus 的全链路指标采集能力，覆盖
HTTP、LLM、Agent、缓存与数据库五大维度。

# 概述

本包通过 Collector 统一注册和记录 Prometheus 指标，使用 promauto
自动注册机制，避免手动管理 Registry。所有指标按 namespace 隔离，
支持多维度 label 分组，便于 Grafana 等工具进行可视化与告警。

# 核心类型

  - Collector：指标收集器，持有 Counter、Histogram、Gauge 等
    Prometheus 向量指标，按业务域分组管理。

# 主要能力

  - HTTP 指标：请求总数、请求耗时、请求/响应体大小，
    按 method/path/status 分组，状态码归类为 2xx/3xx/4xx/5xx。
  - LLM 指标：请求总数、请求耗时、Token 用量（prompt/completion）、
    调用成本，按 provider/model 分组。
  - Agent 指标：执行总数、执行耗时、状态转换计数，
    按 agent_id/agent_type 分组。
  - 缓存指标：命中与未命中计数，按 cache_type 分组。
  - 数据库指标：活跃/空闲连接数 Gauge、查询耗时 Histogram，
    按 database/operation 分组。
*/
package metrics
