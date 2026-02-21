// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 guardrails 为智能体提供输入与输出安全防护能力。

# 概述

guardrails 聚焦于"在不破坏业务流程的前提下，降低内容风险"。
它支持在 Agent 执行前后进行统一校验，用于识别并处理：

- 提示词注入与越权指令
- PII（个人敏感信息）泄露
- 不安全或违规输出
- 自定义业务规则违规内容

# 核心接口

  - [Validator]：单项规则校验器接口，提供 Validate / Name / Priority
  - [Filter]：内容过滤器接口，提供 Filter / Name，用于转换或脱敏内容
  - [AuditLogger]：审计日志接口，提供 Log / Query / Count，记录校验事件

# 核心模型

本包围绕 Validator 与 ValidatorChain 展开：

- [ValidatorChain]：多校验器编排器，统一执行顺序与结果汇总
- [OutputValidator]：输出验证器，对 Agent 输出执行安全与合规检查

链路执行支持三种模式：

- [ChainModeFailFast]：遇到首个错误立即返回，延迟更低
- [ChainModeCollectAll]：执行全部校验并汇总错误，诊断更全面
- [ChainModeParallel]：并行执行所有校验器并收集结果

# 结果与错误

  - [ValidationResult]：校验结果，包含有效性、Tripwire 标记、错误列表、
    警告列表与附加元数据
  - [ValidationError]：结构化校验错误，包含错误码、消息与严重级别
  - [TripwireError]：Tripwire 触发错误，表示应立即中断整个 Agent 执行链
  - 错误码常量：ErrCodeInjectionDetected / ErrCodePIIDetected /
    ErrCodeMaxLengthExceeded / ErrCodeBlockedKeyword 等
  - 严重级别：SeverityCritical / SeverityHigh / SeverityMedium / SeverityLow

# 内置校验器

- [LengthValidator]：防止超长输入导致资源消耗异常，支持截断或拒绝
- [PIIDetector]：识别邮箱、手机号、身份证号、银行卡号等敏感信息
- [InjectionDetector]：识别常见 Prompt Injection 模式，支持中英文
- [ShadowAIDetector]：检测未经授权的 AI 服务使用

# 注册表

  - [ValidatorRegistry]：校验器注册表，支持自定义校验规则的注册与扩展
  - [FilterRegistry]：过滤器注册表，支持自定义过滤器的注册与扩展

# 配置

  - [GuardrailsConfig]：全局护栏配置，包含输入/输出校验器、过滤器与失败策略
  - [FailureAction]：失败处理动作（Reject / Warn / Retry）

# 扩展方式

你可以通过实现 Validator 接口接入自定义规则，例如：

- 行业术语合规校验
- 组织内部敏感词校验
- 多租户隔离策略校验

# 与 agent 包集成

guardrails 可接入 Agent 的输入和输出环节，实现全链路防护：

- 输入侧：在规划或执行前拦截高风险内容
- 输出侧：在返回用户前进行安全校验和必要处理

这使得智能体在保持可用性的同时具备更稳定的安全边界。
*/
package guardrails
