// 版权所有 2024 AgentFlow Authors. 保留所有权利。
// 此源代码的使用由 MIT 许可规范，该许可可以
// 在 LICENSE 文件中找到。

/*
示例 14_guardrails 演示了 AgentFlow 的安全防护（Guardrails）能力。

# 演示内容

本示例展示三层输入安全校验机制：

  - PII Detection：个人隐私信息检测与脱敏，支持手机号、邮箱等模式识别，
    提供 Mask 和 Warn 两种处理策略
  - Injection Detection：Prompt 注入攻击检测，覆盖英文、中文及通用注入模式，
    支持角色劫持、指令覆盖等攻击向量识别
  - Validator Chain：多验证器链式编排，支持按优先级排序执行，
    内置 Length、Injection、Keyword、PII 四种验证器，
    提供 CollectAll 和 FailFast 两种链模式

# 运行方式

	go run .
*/
package main
