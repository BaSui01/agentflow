# 高优先级功能 (High Priority Features)

展示产物管理、HITL 中断、OpenAPI 工具生成、部署、增强检查点、可视化工作流构建。

## 功能

- **Artifacts**：文件产物的创建、标签管理和查询
- **HITL**：Human-in-the-Loop 中断审批机制（异步等待人工确认）
- **OpenAPI Tools**：从 OpenAPI Spec 自动生成 LLM 可调用的工具
- **Deployment**：K8s 部署清单生成和预览
- **Enhanced Checkpoints**：工作流检查点的版本管理和时间旅行对比
- **Visual Builder**：可视化工作流构建器，生成可执行 DAG

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/17_high_priority_features
go run main.go
```

## 代码说明

各功能模块独立演示：`artifacts.NewManager` 管理产物；`hitl.NewInterruptManager` 处理审批中断；`openapi.NewGenerator` 从 Spec 生成工具；`workflow.NewVisualBuilder` 将可视化节点编译为 DAG。
