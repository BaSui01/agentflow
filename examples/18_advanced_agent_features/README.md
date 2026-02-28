# 高级 Agent 功能 (Advanced Agent Features)

展示联邦编排、深思模式、长时运行执行器、技能注册表。

## 功能

- **联邦编排**：多节点注册和能力发现（Federated Orchestration）
- **深思模式**：Agent 在即时响应和深度思考之间切换（Deliberation Mode）
- **长时运行**：多步骤任务的检查点和自动恢复（Long-Running Executor）
- **技能注册表**：按类别和标签管理 Agent 技能（Skills Registry）

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/18_advanced_agent_features
go run main.go
```

## 代码说明

`federation.NewOrchestrator` 管理联邦节点；`deliberation.NewEngine` 支持 Immediate/Deliberate 模式切换；`longrunning.NewExecutor` 管理多步骤执行和检查点；`skills.NewRegistry` 提供技能的注册、分类查询和标签搜索。
