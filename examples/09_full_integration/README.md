# 全功能集成 (Full Integration)

展示如何将 AgentFlow 所有功能集成到一个完整的项目中。

## 功能

- **增强单 Agent**：启用反射、工具选择、提示词增强、技能系统、MCP、增强记忆、可观测性
- **层次化多 Agent**：Supervisor + 多 Worker 的任务分解与并行执行
- **协作多 Agent**：辩论模式的多专家协作系统
- **生产配置**：展示各子系统的推荐生产配置和渐进式上线策略

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`（可选，无 Key 时跳过实际执行，仅演示初始化）

## 运行

```bash
cd examples/09_full_integration
go run main.go
```

## 代码说明

四个场景逐步展示从单 Agent 增强到多 Agent 协作的完整架构。无 API Key 时所有子系统仍会正常初始化，仅跳过 LLM 调用部分。
