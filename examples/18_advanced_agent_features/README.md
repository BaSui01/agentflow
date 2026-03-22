# 高级 Agent 功能 (Advanced Agent Features)

展示联邦编排、深思模式、长时运行执行器、技能注册表。

## 功能

- **联邦编排**：多节点注册和能力发现（Federated Orchestration）
- **深思模式**：Agent 在即时响应和深度思考之间切换（Deliberation Mode）
- **长时运行**：多步骤任务的检查点和自动恢复（Long-Running Executor）
- **技能注册表**：按类别和标签管理 Agent 技能（Skills Registry）
- **异步执行与子 Agent 协调**：Async Execution & Subagent Coordination
- **防御性提示增强**：Defensive Prompt Enhancer
- **运行时配置覆盖**：RunConfig Runtime Overrides
- **生命周期与作用域存储**：Lifecycle + AgentTool + Scoped Stores
- **多 Agent 模式与聚合**：Multi-Agent Modes & Aggregation
- **A2A/MCP 协议可达性**：协议子系统验证
- **推理模式可达性**：Reasoning Patterns 子系统验证
- **流式/结构化输出/语音子系统**：Streaming/StructuredOutput/Voice 可达性
- **LLM 金丝雀/核心扩展/高级运行时/工具缓存**：LLM 子系统全模块可达性验证
- **全模块集成可达性**：Full Module Integration Reachability

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/18_advanced_agent_features
go run main.go
```

## 代码说明

`federation.NewOrchestrator` 管理联邦节点；`deliberation.NewEngine` 支持 Immediate/Deliberate 模式切换；`longrunning.NewExecutor` 管理多步骤执行和检查点；`skills.NewRegistry` 提供技能的注册、分类查询和标签搜索。LLM Canary 示例默认以内存模式运行，不依赖本地 SQLite/CGO。
