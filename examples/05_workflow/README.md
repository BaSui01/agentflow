# 工作流引擎 (Workflow)

展示 AgentFlow 当前推荐的 DAG 工作流主链（单入口）。

## 功能

- **DAG 节点编排**：通过有向无环图表达节点依赖
- **Action Step 执行**：每个节点通过 `workflow.Step` 执行任务
- **单入口执行**：统一通过 `DAGWorkflow.Execute` 触发

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`

## 运行

```bash
cd examples/05_workflow
go run main.go
```

## 代码说明

示例使用 `workflow.NewDAGGraph` + `workflow.NewDAGWorkflow` 构建并执行 “翻译 -> 总结” 的两节点 DAG。  
`translate` 节点输出作为 `summarize` 节点输入，符合当前 Workflow 单入口执行模型。  
