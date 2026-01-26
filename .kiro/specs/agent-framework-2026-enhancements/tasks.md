# 实现计划: Agent Framework 2026 Enhancements

## 进度概览

| 模块 | 状态 | 备注 |
|------|------|------|
| 1. Guardrails | ⬜ 未开始 | 需创建 `agent/guardrails/` |
| 2. Structured Output | ⬜ 未开始 | 需创建 `agent/structured/` |
| 3. A2A Protocol | ⬜ 未开始 | 需创建 `agent/a2a/` |
| 4. Persistent Execution | ✅ 已完成 | 实现在 `agent/checkpoint.go` |
| 5. Evaluation | ⬜ 未开始 | 目录已创建 `agent/evaluation/` |
| 6. 集成和文档 | 🔄 部分完成 | Checkpoint 已可集成 |

## 概述

本计划将 2026 年增强功能分为 5 个主要模块实现，按依赖关系和优先级排序。使用 Go 1.24+，属性测试使用 `pgregory.net/rapid`。

## 任务

- [x] 1. Guardrails 模块实现
  - [x] 1.1 实现核心接口和类型定义
    - 创建 `agent/guardrails/` 目录
    - 实现 `Validator`、`Filter` 接口
    - 实现 `ValidationResult`、`ValidationError`、`GuardrailsConfig` 类型
    - _Requirements: 1.5, 1.6, 1.7_

  - [x] 1.2 实现 PII 检测器
    - 实现 `PIIDetector` 结构体
    - 支持手机号、邮箱、身份证、银行卡等模式
    - 实现脱敏、拒绝、警告三种处理模式
    - _Requirements: 1.2, 2.1_

  - [x]* 1.3 编写 PII 检测属性测试
    - **Property 1: 输入验证检测**
    - **Property 5: 输出敏感信息脱敏**
    - **Validates: Requirements 1.2, 2.1**

  - [x] 1.4 实现提示注入检测器
    - 实现 `InjectionDetector` 结构体
    - 支持中英文注入模式检测
    - 实现分隔符隔离和角色隔离
    - _Requirements: 1.1_

  - [x]* 1.5 编写注入检测属性测试
    - **Property 1: 输入验证检测**
    - **Validates: Requirements 1.1**

  - [x] 1.6 实现长度和关键词验证器
    - 实现 `LengthValidator` 结构体
    - 实现 `KeywordValidator` 结构体
    - _Requirements: 1.3, 1.4_

  - [x]* 1.7 编写长度限制属性测试
    - **Property 2: 输入长度限制**
    - **Validates: Requirements 1.3**

  - [x] 1.8 实现验证器链和优先级执行
    - 实现 `ValidatorChain` 结构体
    - 按优先级排序执行验证器
    - 聚合所有验证结果
    - _Requirements: 1.5, 1.6_

  - [x]* 1.9 编写验证器优先级属性测试
    - **Property 3: 验证器优先级执行顺序**
    - **Property 4: 验证错误信息完整性**
    - **Validates: Requirements 1.5, 1.6**

  - [x] 1.10 实现输出验证和内容过滤
    - 实现 `OutputValidator` 结构体
    - 实现 `ContentFilter` 结构体
    - 实现审计日志记录
    - _Requirements: 2.1, 2.2, 2.3, 2.5_

  - [x]* 1.11 编写输出验证属性测试
    - **Property 6: 输出验证失败日志记录**
    - **Validates: Requirements 2.5**

- [x] 2. Checkpoint - 确保 Guardrails 模块测试通过
  - 运行 `go test ./agent/guardrails/...`
  - 确保所有测试通过，如有问题请询问用户

- [x] 3. Structured Output 模块实现
  - [x] 3.1 实现 JSON Schema 类型定义
    - 创建 `agent/structured/` 目录
    - 实现 `JSONSchema` 结构体
    - 支持嵌套对象、数组、枚举等类型
    - _Requirements: 3.5_

  - [x] 3.2 实现 Schema 生成器
    - 实现 `SchemaGenerator` 结构体
    - 从 Go 结构体反射生成 Schema
    - 支持 `jsonschema` 标签
    - _Requirements: 4.3_

  - [x] 3.3 实现 Schema 验证器
    - 实现 `SchemaValidator` 接口
    - 验证 JSON 数据符合 Schema
    - 返回字段级错误信息
    - _Requirements: 3.1, 3.2, 3.6_

  - [x]* 3.4 编写 Schema 验证属性测试
    - **Property 8: Schema 验证错误定位**
    - **Validates: Requirements 3.2**

  - [x] 3.5 实现泛型结构化输出处理器
    - 实现 `StructuredOutput[T]` 泛型结构体
    - 实现 `Generate` 和 `GenerateWithMessages` 方法
    - 支持原生和提示工程两种模式
    - _Requirements: 3.3, 3.4, 3.7, 4.1_

  - [x]* 3.6 编写 Schema Round-Trip 属性测试
    - **Property 7: Schema 生成与解析 Round-Trip**
    - **Validates: Requirements 3.1, 3.5, 3.6, 4.1, 4.3**

- [x] 4. Checkpoint - 确保 Structured Output 模块测试通过
  - 运行 `go test ./agent/structured/...`
  - 确保所有测试通过，如有问题请询问用户

- [x] 5. A2A Protocol 模块实现
  - [x] 5.1 实现 Agent Card 类型定义
    - 创建 `agent/a2a/` 目录
    - 实现 `AgentCard`、`Capability`、`ToolDefinition` 类型
    - _Requirements: 5.1, 5.3_

  - [x] 5.2 实现 Agent Card 生成器
    - 实现 `AgentCardGenerator` 结构体
    - 从 Agent 配置自动生成 Card
    - _Requirements: 5.2, 5.4_

  - [x]* 5.3 编写 Agent Card 属性测试
    - **Property 9: Agent Card 完整性**
    - **Validates: Requirements 5.1, 5.2, 5.3**

  - [x] 5.4 实现 A2A 消息类型
    - 实现 `A2AMessage`、`A2AMessageType` 类型
    - 实现消息序列化/反序列化
    - _Requirements: 6.1_

  - [x]* 5.5 编写 A2A 消息 Round-Trip 属性测试
    - **Property 10: A2A 消息 Round-Trip**
    - **Validates: Requirements 6.1**

  - [x] 5.6 实现 A2A 客户端
    - 实现 `A2AClient` 接口
    - 实现 `Discover`、`Send`、`SendAsync`、`GetResult` 方法
    - _Requirements: 6.3, 6.4, 6.5_

  - [x] 5.7 实现 A2A 服务端和路由
    - 实现 `A2AServer` 接口
    - 实现 HTTP 端点
    - 实现任务路由到本地 Agent
    - _Requirements: 5.5, 6.2, 6.6_

  - [x]* 5.8 编写 A2A 路由属性测试
    - **Property 11: A2A 任务路由正确性**
    - **Validates: Requirements 6.2**

- [x] 6. Checkpoint - 确保 A2A 模块测试通过
  - 运行 `go test ./agent/a2a/...`
  - 确保所有测试通过，如有问题请询问用户

- [x] 7. Persistent Execution 模块实现 _(已在 `agent/checkpoint.go` 中实现)_
  - [x] 7.1 实现检查点类型定义
    - ~~创建 `agent/persistent/` 目录~~ (实现在 `agent/checkpoint.go`)
    - 实现 `Checkpoint`、`ExecutionState`、`ToolCall` 类型
    - _Requirements: 7.3_

  - [x] 7.2 实现 CheckpointStore 接口和内存实现
    - 实现 `CheckpointStore` 接口
    - ~~实现 `MemoryCheckpointStore` 用于测试~~ (使用 FileCheckpointStore)
    - _Requirements: 7.5_

  - [x] 7.3 实现文件和 Redis 存储后端
    - 实现 `FileCheckpointStore`
    - 实现 `RedisCheckpointStore`
    - 实现 `PostgreSQLCheckpointStore`
    - _Requirements: 7.5_

  - [x]* 7.4 编写检查点 Round-Trip 属性测试
    - **Property 12: 检查点 Round-Trip** _(在 `agent/checkpoint_property_test.go`)_
    - **Validates: Requirements 7.3, 8.1, 8.2**

  - [x] 7.5 实现 CheckpointManager
    - 实现 `CheckpointManager` 结构体
    - 实现 `CreateCheckpoint`、`ResumeFromCheckpoint`、`RollbackToVersion` 方法
    - 实现版本管理和清理逻辑
    - _Requirements: 7.1, 7.2, 7.4, 7.6, 8.1, 8.4_

  - [x]* 7.6 编写检查点版本管理属性测试
    - **Property 13: 检查点版本管理** _(在 `agent/checkpoint_manager_test.go`)_
    - **Validates: Requirements 7.6**

  - [x] 7.7 实现恢复执行逻辑
    - 实现 `ResumeFromCheckpoint` 方法
    - 实现 `RollbackToVersion` 方法
    - _Requirements: 8.3, 8.5, 8.6_

  - [x]* 7.8 编写恢复步骤跳过属性测试
    - **Property 14: 检查点恢复步骤跳过**
    - **Validates: Requirements 8.5**

- [x] 8. Checkpoint - 确保 Persistent Execution 模块测试通过
  - 运行 `go test ./agent/checkpoint*.go -v`
  - 测试文件: `checkpoint_file_test.go`, `checkpoint_manager_test.go`, `checkpoint_property_test.go`

- [x] 9. Evaluation 模块实现 _(目录 `agent/evaluation/` 已创建，待实现)_
  - [x] 9.1 实现评估指标类型定义
    - 创建 `agent/evaluation/` 目录
    - 实现 `Metric`、`EvalInput`、`EvalOutput`、`EvalResult` 类型
    - _Requirements: 9.1_

  - [x] 9.2 实现内置评估指标
    - 实现 `AccuracyMetric`、`LatencyMetric`、`TokenUsageMetric`、`CostMetric`
    - _Requirements: 9.3_

  - [x] 9.3 实现评估执行器
    - 实现 `Evaluator` 结构体
    - 实现批量评估和报告生成
    - _Requirements: 9.2, 9.4, 9.5, 9.6_

  - [x]* 9.4 编写评估指标收集属性测试
    - **Property 15: 评估指标收集完整性**
    - **Validates: Requirements 9.1, 9.2**

  - [x] 9.5 实现 LLM-as-Judge
    - 实现 `LLMJudge` 结构体
    - 实现 `LLMJudgeConfig`、`JudgeDimension`、`JudgeResult` 类型
    - 实现 `Judge` 和 `JudgeBatch` 方法
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5_

  - [x]* 9.6 编写 LLM-as-Judge 属性测试
    - **Property 16: LLM-as-Judge 结果结构**
    - **Validates: Requirements 10.1, 10.3, 10.4**

  - [x] 9.7 实现 A/B 测试器
    - 实现 `ABTester`、`Experiment`、`Variant` 类型
    - 实现流量分配和结果记录
    - _Requirements: 11.1, 11.2, 11.3, 11.5_

  - [x]* 9.8 编写 A/B 测试流量分配属性测试
    - **Property 17: A/B 测试流量分配**
    - **Validates: Requirements 11.2**

  - [x] 9.9 实现统计分析和报告
    - 实现 `ExperimentResult`、`VariantResult` 类型
    - 实现统计显著性分析
    - 实现自动选择优胜配置
    - _Requirements: 11.4, 11.6_

  - [x]* 9.10 编写 A/B 测试统计分析属性测试
    - **Property 18: A/B 测试统计分析**
    - **Validates: Requirements 11.3, 11.4**

- [x] 10. Checkpoint - 确保 Evaluation 模块测试通过
  - 运行 `go test ./agent/evaluation/...`
  - 确保所有测试通过，如有问题请询问用户

- [x] 11. 集成和文档
  - [x] 11.1 集成 Guardrails 到 BaseAgent
    - 在 BaseAgent 中添加 Guardrails 配置
    - 在 Execute 方法中集成输入/输出验证
    - _Requirements: 1.7, 2.4_

  - [x] 11.2 集成 Persistent Execution 到 BaseAgent _(CheckpointManager 已可用)_
    - 在 BaseAgent 中添加 CheckpointManager
    - 在 Execute 方法中集成检查点保存
    - _Requirements: 7.1, 7.4_

  - [x] 11.3 更新 examples 目录
    - 添加 Guardrails 使用示例
    - 添加 Structured Output 使用示例
    - 添加 A2A Protocol 使用示例

- [x] 12. Final Checkpoint - 确保所有测试通过
  - 运行 `go test ./...`
  - 确保所有测试通过，如有问题请询问用户

## 备注

- 标记 `*` 的任务为可选属性测试任务
- 每个模块完成后有检查点确保质量
- 属性测试使用 `pgregory.net/rapid` 库
- 每个属性测试最少运行 100 次迭代
