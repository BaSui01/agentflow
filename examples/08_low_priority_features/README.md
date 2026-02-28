# 低优先级功能 (Low Priority Features)

展示层次化架构、多 Agent 协作、可观测性系统。

## 功能

- **层次化架构**：Supervisor-Worker 模式，创建主管 Agent 和多个工作 Agent
- **多 Agent 协作**：辩论、共识、流水线、广播、网络五种协作模式
- **可观测性系统**：MetricsCollector 指标收集 + Tracer 追踪系统

## 前置条件

- Go 1.24+
- 无需 API Key（使用 nil Provider 演示结构，不执行真实 LLM 调用）

## 运行

```bash
cd examples/08_low_priority_features
go run main.go
```

## 代码说明

层次化架构通过 `hierarchical.NewHierarchicalAgent` 创建；协作系统通过 `collaboration.NewMultiAgentSystem` 配置不同模式；可观测性通过 `observability.NewMetricsCollector` 和 `observability.NewTracer` 记录真实的指标和追踪数据。
