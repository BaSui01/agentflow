# 工作流引擎 (Workflow)

展示 AgentFlow 工作流引擎的三种核心模式：Prompt Chaining、Routing、Parallelization。

## 功能

- **Prompt Chaining**：翻译 -> 总结，前一步输出作为后一步输入
- **Routing**：LLM 分类问题类型，路由到不同专家 Agent 处理
- **Parallelization**：并行执行情感分析、主题提取、关键词提取，聚合结果

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`

## 运行

```bash
cd examples/05_workflow
go run main.go
```

## 代码说明

使用 `workflow.NewChainWorkflow` 创建链式工作流，`workflow.NewRoutingWorkflow` 创建路由工作流，`workflow.NewParallelWorkflow` 创建并行工作流。每种模式通过 `FuncStep`/`FuncRouter`/`FuncTask` 封装 LLM 调用逻辑。
