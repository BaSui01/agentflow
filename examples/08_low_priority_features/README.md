# 低优先级功能 (Low Priority Features)

展示 legacy 多 Agent surface（层次化架构、协作系统）以及可观测性系统。

## 功能

- **legacy 层次化架构**：Supervisor-Worker 模式，创建主管 Agent 和多个工作 Agent
- **legacy 多 Agent 协作**：辩论、共识、流水线、广播、网络五种协作模式 + 角色流水线（RolePipeline）
- **可观测性系统**：MetricsCollector 指标收集 + Tracer 追踪系统
- **消息中心**：带持久化的 MessageHub（MessageHubWithStore）

## 前置条件

- Go 1.24+
- 无需 API Key（使用 nil Provider 演示结构，不执行真实 LLM 调用）

## 运行

```bash
cd examples/08_low_priority_features
go run main.go
```

## 代码说明

该示例故意展示 legacy surface：层次化架构通过 `hierarchical.NewHierarchicalAgent` 创建，协作系统通过 `collaboration.NewMultiAgentSystem` 配置不同模式；新的多 Agent 接入默认应优先使用 `agent/team`。可观测性部分通过 `observability.NewMetricsCollector` 和 `observability.NewTracer` 记录真实指标与追踪数据。
