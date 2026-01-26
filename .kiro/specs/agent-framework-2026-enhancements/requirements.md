# 需求文档

## 简介

本文档定义了 AgentFlow 框架 2026 年增强功能的需求规格。这些增强功能旨在将框架提升至生产级别，涵盖安全护栏、结构化输出、跨系统互操作、持久化执行和自动化评估等关键能力。

## 术语表

- **Guardrails（安全护栏）**: 用于验证和过滤 Agent 输入/输出的安全机制
- **PII（个人身份信息）**: 可用于识别个人身份的敏感数据，如姓名、身份证号、电话号码等
- **Prompt_Injection（提示注入）**: 恶意用户通过构造特殊输入试图操控 LLM 行为的攻击方式
- **Structured_Output（结构化输出）**: 符合预定义 Schema 的类型安全输出格式
- **JSON_Schema**: 用于描述 JSON 数据结构的规范
- **A2A_Protocol（Agent-to-Agent 协议）**: Google 提出的跨系统 Agent 互操作标准协议
- **Agent_Card**: A2A 协议中描述 Agent 能力和元数据的标准格式
- **Persistent_Execution（持久化执行）**: 支持状态保存、故障恢复的长时间运行机制
- **Checkpoint（检查点）**: 执行过程中保存的状态快照，用于故障恢复
- **Evaluation_Framework（评估框架）**: 用于自动化测试和评估 Agent 性能的系统
- **LLM_as_Judge**: 使用 LLM 作为评判者来评估 Agent 输出质量的方法
- **BaseAgent**: 框架中的基础 Agent 实现类
- **Validator（验证器）**: 执行输入/输出验证的组件
- **Filter（过滤器）**: 执行内容过滤的组件
- **Detector（检测器）**: 执行特定模式检测的组件

## 需求

### 需求 1：输入验证护栏

**用户故事：** 作为框架使用者，我希望能够验证 Agent 输入的安全性，以便防止恶意输入和提示注入攻击。

#### 验收标准

1. WHEN 用户输入包含提示注入模式 THEN Guardrails 系统 SHALL 检测并拒绝该输入，返回安全错误
2. WHEN 用户输入包含 PII 数据 THEN Guardrails 系统 SHALL 检测并根据配置进行脱敏或拒绝处理
3. WHEN 用户输入超过配置的最大长度 THEN Guardrails 系统 SHALL 截断或拒绝该输入
4. WHEN 用户输入包含禁止的关键词 THEN Guardrails 系统 SHALL 根据配置的严重级别进行处理
5. WHEN 验证器配置为多个规则 THEN Guardrails 系统 SHALL 按优先级顺序执行所有规则
6. WHEN 任一验证规则失败 THEN Guardrails 系统 SHALL 返回包含失败原因的详细错误信息
7. THE Guardrails 系统 SHALL 支持自定义验证规则的注册和扩展

### 需求 2：输出验证护栏

**用户故事：** 作为框架使用者，我希望能够验证 Agent 输出的安全性和合规性，以便确保输出内容符合业务规则。

#### 验收标准

1. WHEN Agent 输出包含敏感信息 THEN Guardrails 系统 SHALL 检测并进行脱敏处理
2. WHEN Agent 输出包含有害内容 THEN Guardrails 系统 SHALL 拦截并返回安全替代响应
3. WHEN Agent 输出不符合预定义格式 THEN Guardrails 系统 SHALL 尝试修复或返回格式错误
4. WHEN 输出验证失败且配置了重试 THEN Guardrails 系统 SHALL 请求 Agent 重新生成输出
5. THE Guardrails 系统 SHALL 记录所有验证失败事件用于审计
6. WHEN 配置了内容分类器 THEN Guardrails 系统 SHALL 对输出进行分类并标记风险等级

### 需求 3：结构化输出生成

**用户故事：** 作为框架使用者，我希望 Agent 能够生成符合预定义 Schema 的结构化输出，以便下游系统能够可靠地解析和处理。

#### 验收标准

1. WHEN 配置了 JSON Schema THEN Structured_Output 系统 SHALL 强制 Agent 输出符合该 Schema
2. WHEN Agent 输出不符合 Schema THEN Structured_Output 系统 SHALL 返回验证错误并包含具体违规字段
3. WHEN 使用支持原生结构化输出的 Provider THEN Structured_Output 系统 SHALL 使用 Provider 的原生能力
4. WHEN Provider 不支持原生结构化输出 THEN Structured_Output 系统 SHALL 通过提示工程和后处理实现
5. THE Structured_Output 系统 SHALL 支持嵌套对象、数组、枚举等复杂 Schema 类型
6. WHEN Schema 定义了必需字段 THEN Structured_Output 系统 SHALL 验证所有必需字段存在且非空
7. THE Structured_Output 系统 SHALL 提供 Go 类型安全的泛型 API

### 需求 4：结构化输出解析

**用户故事：** 作为框架使用者，我希望能够将 Agent 的结构化输出自动解析为 Go 结构体，以便在代码中类型安全地使用。

#### 验收标准

1. WHEN 输出符合 Schema THEN Structured_Output 系统 SHALL 自动解析为对应的 Go 结构体
2. WHEN 解析失败 THEN Structured_Output 系统 SHALL 返回详细的解析错误信息
3. THE Structured_Output 系统 SHALL 支持从 Go 结构体自动生成 JSON Schema
4. WHEN 结构体包含验证标签 THEN Structured_Output 系统 SHALL 执行字段级验证
5. THE Structured_Output 系统 SHALL 支持自定义类型的序列化和反序列化

### 需求 5：A2A 协议 Agent Card

**用户故事：** 作为框架使用者，我希望能够为 Agent 生成标准的 Agent Card，以便其他系统能够发现和理解 Agent 的能力。

#### 验收标准

1. THE A2A_Protocol 系统 SHALL 支持生成符合 Google A2A 规范的 Agent Card
2. WHEN Agent 注册时 THEN A2A_Protocol 系统 SHALL 自动生成包含能力描述的 Agent Card
3. THE Agent_Card SHALL 包含 Agent 名称、描述、支持的输入/输出格式、可用工具列表
4. WHEN Agent 能力变更时 THEN A2A_Protocol 系统 SHALL 自动更新 Agent Card
5. THE A2A_Protocol 系统 SHALL 提供 HTTP 端点用于 Agent Card 的发现和获取

### 需求 6：A2A 协议消息交换

**用户故事：** 作为框架使用者，我希望 Agent 能够通过标准协议与外部 Agent 通信，以便实现跨系统的 Agent 协作。

#### 验收标准

1. THE A2A_Protocol 系统 SHALL 支持 A2A 标准消息格式的发送和接收
2. WHEN 接收到 A2A 任务请求 THEN A2A_Protocol 系统 SHALL 路由到对应的本地 Agent 处理
3. WHEN 本地 Agent 需要调用远程 Agent THEN A2A_Protocol 系统 SHALL 构造标准 A2A 请求
4. THE A2A_Protocol 系统 SHALL 支持同步和异步两种消息交换模式
5. WHEN 远程 Agent 不可用 THEN A2A_Protocol 系统 SHALL 返回标准错误响应
6. THE A2A_Protocol 系统 SHALL 支持消息的认证和授权

### 需求 7：执行状态持久化

**用户故事：** 作为框架使用者，我希望能够保存 Agent 执行状态，以便在故障后能够恢复执行。

#### 验收标准

1. WHEN Agent 执行过程中 THEN Persistent_Execution 系统 SHALL 定期保存执行状态检查点
2. WHEN 检查点保存失败 THEN Persistent_Execution 系统 SHALL 记录错误并继续执行
3. THE Checkpoint SHALL 包含当前步骤、中间结果、上下文变量、工具调用历史
4. WHEN 配置了检查点间隔 THEN Persistent_Execution 系统 SHALL 按配置的间隔保存检查点
5. THE Persistent_Execution 系统 SHALL 支持多种存储后端（内存、文件、Redis、数据库）
6. WHEN 检查点数量超过配置上限 THEN Persistent_Execution 系统 SHALL 清理旧检查点

### 需求 8：故障恢复执行

**用户故事：** 作为框架使用者，我希望能够从检查点恢复 Agent 执行，以便处理长时间运行的任务。

#### 验收标准

1. WHEN 提供执行 ID THEN Persistent_Execution 系统 SHALL 加载最近的有效检查点
2. WHEN 检查点加载成功 THEN Persistent_Execution 系统 SHALL 从检查点状态恢复执行
3. WHEN 检查点损坏或不存在 THEN Persistent_Execution 系统 SHALL 返回恢复失败错误
4. THE Persistent_Execution 系统 SHALL 支持从指定检查点版本恢复
5. WHEN 恢复执行时 THEN Persistent_Execution 系统 SHALL 跳过已完成的步骤
6. THE Persistent_Execution 系统 SHALL 记录恢复事件用于审计追踪

### 需求 9：Agent 评估指标

**用户故事：** 作为框架使用者，我希望能够定义和收集 Agent 性能指标，以便量化评估 Agent 质量。

#### 验收标准

1. THE Evaluation_Framework SHALL 支持定义自定义评估指标
2. WHEN Agent 执行完成 THEN Evaluation_Framework SHALL 自动收集配置的指标
3. THE Evaluation_Framework SHALL 内置常用指标：准确率、延迟、Token 使用量、成本
4. WHEN 配置了基准测试集 THEN Evaluation_Framework SHALL 支持批量评估
5. THE Evaluation_Framework SHALL 生成评估报告包含统计摘要和详细结果
6. WHEN 指标超过阈值 THEN Evaluation_Framework SHALL 触发告警

### 需求 10：LLM-as-Judge 评估

**用户故事：** 作为框架使用者，我希望能够使用 LLM 作为评判者来评估 Agent 输出质量，以便进行主观质量评估。

#### 验收标准

1. THE Evaluation_Framework SHALL 支持配置 LLM 作为评估者
2. WHEN 执行 LLM-as-Judge 评估 THEN Evaluation_Framework SHALL 使用标准评估提示模板
3. THE Evaluation_Framework SHALL 支持自定义评估维度和评分标准
4. WHEN 评估完成 THEN Evaluation_Framework SHALL 返回结构化的评估结果
5. THE Evaluation_Framework SHALL 支持多个 LLM 评估者的结果聚合
6. WHEN 评估结果差异过大 THEN Evaluation_Framework SHALL 标记需要人工复核

### 需求 11：A/B 测试支持

**用户故事：** 作为框架使用者，我希望能够对不同 Agent 配置进行 A/B 测试，以便选择最优配置。

#### 验收标准

1. THE Evaluation_Framework SHALL 支持定义 A/B 测试实验
2. WHEN 配置了 A/B 测试 THEN Evaluation_Framework SHALL 按配置比例分流请求
3. THE Evaluation_Framework SHALL 收集各组的性能指标用于对比
4. WHEN 实验结束 THEN Evaluation_Framework SHALL 生成统计显著性分析报告
5. THE Evaluation_Framework SHALL 支持多变量测试（超过两组）
6. WHEN 检测到显著差异 THEN Evaluation_Framework SHALL 支持自动选择优胜配置
