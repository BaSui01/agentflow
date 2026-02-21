// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 llm 提供统一的大语言模型接入层，包括 Provider 抽象、路由、
缓存、重试、可观测与工具调用等能力。

# 概述

本包目标是屏蔽不同模型服务商在接口、鉴权、错误语义和流式协议上的差异，
对上层业务暴露一致的请求与响应模型，降低多模型接入和切换成本。

你可以使用它完成以下典型场景：

- 单一 Provider 的快速接入与调用。
- 多 Provider 路由与故障转移。
- 流式输出、函数调用与工具编排。
- 缓存、重试、熔断、限流与成本观测。

# Provider 抽象

核心接口是 [Provider]，包含补全、流式输出、健康检查与能力声明。
基于该接口，系统可以在保持上层调用不变的前提下切换底层模型服务。

# 核心接口

  - [Provider]：LLM 提供者接口，提供 Completion / Stream / HealthCheck /
    Name / SupportsNativeFunctionCalling
  - [SecurityProvider]：安全认证接口，提供 Authenticate / Authorize
  - [AuditLogger]：审计日志接口，提供 Log / Query
  - [RateLimiter]：限流接口，提供 Allow / Wait / Limit
  - [Tracer] / [Span]：分布式追踪接口
  - [ProviderMiddleware]：Provider 中间件接口，支持链式增强

# 核心类型

  - [LLMModel]：抽象模型定义（如 gpt-4、claude-3-opus）
  - [LLMProvider]：提供商定义（如 OpenAI、Anthropic、DeepSeek）
  - [LLMProviderModel]：提供商-模型多对多映射
  - [LLMProviderAPIKey]：API 密钥池条目
  - [ChatRequest] / [ChatResponse]：聊天请求与响应
  - [StreamChunk]：流式输出分片
  - [HealthStatus]：健康检查状态
  - [Identity]：代理或用户身份信息
  - [AuditEvent]：审计事件
  - [CredentialOverride]：单次请求凭据覆盖，通过 context 传递

# 运维能力

  - [HealthMonitor]：Provider 健康监控，周期性探测与评分
  - [APIKeyPool]：API 密钥池，支持轮询、优先级与限流策略
  - [CanaryConfig] / [CanaryDeployment]：金丝雀发布，支持灰度流量与自动回滚

# 主要能力

- 路由与策略：支持按模型、标签、健康状态或策略选择目标 Provider。
- 流式处理：统一流式分片结构，便于实时输出与增量聚合。
- 韧性机制：支持重试、退避、熔断、超时与降级。
- 多级缓存：支持本地与远端缓存协同，减少重复调用。
- 可观测性：支持指标、追踪与成本统计。
- 工具调用：支持函数调用与工具执行闭环。
- 中间件链：通过 [ChainProviderMiddleware] 组合多个中间件增强 Provider。

# 集成建议

- 单 Provider 场景，优先直接使用对应实现并开启基础重试。
- 多 Provider 场景，优先通过路由器统一入口并配置回退策略。
- 高并发场景，建议叠加限流、缓存和熔断配置。
- 生产场景，建议启用观测能力并接入统一告警。

# 相关子包

- llm/providers：各模型服务商适配实现。
- llm/router：路由策略与选择逻辑。
- llm/cache：缓存实现与策略。
- llm/retry：重试与退避策略。
- llm/observability：指标、追踪与成本观测。
- llm/tools：工具调用、ReAct 执行与外部工具集成。
- llm/embedding：文本嵌入 Provider 接口与实现。
- llm/image：图像生成与编辑 Provider 接口与实现。
- llm/video：视频分析与生成 Provider 接口与实现。
- llm/rerank：文档重排序 Provider 接口与实现。
- llm/batch：批量请求处理器。
- llm/budget：Token 预算管理与告警。
- llm/circuitbreaker：熔断器实现。
- llm/config：配置类型与路由权重定义。
- llm/factory：Provider 工厂与注册表。
*/
package llm
