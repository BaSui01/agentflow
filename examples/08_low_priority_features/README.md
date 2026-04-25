# 低优先级功能 (Low Priority Features)

展示官方 `agent/team` 多 Agent surface（TeamBuilder、执行模式枚举）以及可观测性系统。

## 功能

- **层次化团队**：通过 `team.NewTeamBuilder(...).WithMode(team.ModeSupervisor)` 创建 Supervisor-Worker 团队
- **多 Agent 协作**：展示 `team.SupportedExecutionModes()` 与 TeamBuilder 协作模式
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

该示例只展示官方入口：多 Agent 团队通过 `agent/team` 创建，执行模式通过 `team.SupportedExecutionModes()` 暴露；内部 engine 不作为示例入口。可观测性部分通过 `observability.NewMetricsCollector` 和 `observability.NewTracer` 记录真实指标与追踪数据。
