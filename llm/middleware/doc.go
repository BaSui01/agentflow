// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 middleware 提供 LLM 请求处理的中间件链机制，支持在请求发送到
上游模型服务之前和响应返回之后插入可组合的横切逻辑。

# 概述

本包采用经典的 Handler / Middleware 函数式组合模式，将日志、超时、
重试、缓存、限流、追踪等横切关注点从业务逻辑中解耦。同时提供
RequestRewriter 改写器链，用于在请求发送前进行参数清理与转换。

# 核心接口

  - Handler：func(ctx, *ChatRequest) (*ChatResponse, error)，
    表示一个请求处理函数。
  - Middleware：func(Handler) Handler，表示一个中间件装饰器。
  - Chain：中间件链，支持 Use / UseFront / Then 组合与执行。
  - RequestRewriter：请求改写器接口，包含 Rewrite 与 Name 方法。
  - RewriterChain：改写器链，按顺序执行多个 RequestRewriter。
  - MetricsCollector / Cache / BlockingRateLimiter / Validator：
    各中间件依赖的辅助接口。

# 主要能力

  - 日志记录：LoggingMiddleware 记录请求模型、耗时与 Token 用量。
  - 超时控制：TimeoutMiddleware 为请求添加 context 超时。
  - 自动重试：RetryMiddleware 支持指数退避重试。
  - 响应缓存：CacheMiddleware 基于 Cache 接口缓存响应。
  - 速率限制：RateLimitMiddleware 基于阻塞式限流器等待。
  - 指标采集：MetricsMiddleware 收集请求耗时与 Token 统计。
  - 分布式追踪：TracingMiddleware 集成 Tracer 接口。
  - Panic 恢复：RecoveryMiddleware 捕获 panic 并转为错误。
  - 请求验证：ValidatorMiddleware 在处理前校验请求合法性。
  - 请求改写：EmptyToolsCleaner 等改写器清理无效参数。
*/
package middleware
