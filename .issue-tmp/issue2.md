## 🟡 优先级：MEDIUM（架构职责过载）

## 问题描述

`agent/capabilities/tools/` 包当前包含了过多职责：

- Registry（工具注册）
- Matcher（匹配）
- Composer（组合）
- Protocol（协议适配）
- Store（存储）
- Executor（执行）
- RemoteTransport（远程传输）
- Discovery（发现）
- SkillManager（技能管理）

共 13+ 文件、20+ 接口集中在一个 sub-tree 内，违反单一职责原则（SRP），新开发者上手成本高，并发修改易冲突。

## 建议拆分

```
agent/capabilities/tools/
├── registry/         # 工具注册、查询、健康
├── discovery/        # 发现、匹配、技能管理
├── execution/        # 调用、Composer、超时
├── remote/           # RemoteTransport、Protocol 适配
└── store/            # 存储后端
```

## 影响评估

- 影响包内现有 import 路径，需要相应调整调用方
- `architecture_guard_test.go` 需要更新依赖规则
- 公开 API 通过 `tools.Registry` 等保持向后兼容（用 type alias 过渡）

## TDD 流程建议

1. **Red**：先写 `architecture_guard_test.go` 新规则，要求 `tools/registry/` 不依赖 `tools/execution/`，当前应失败
2. **Green**：按子包拆分文件、修复 import
3. **Refactor**：在 `tools/doc.go` 中绘制新的依赖图

## 标签
`enhancement` `tech-debt`
