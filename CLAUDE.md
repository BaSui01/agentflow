# Project Rules

## 禁止命令

- **禁止使用 `git stash pop`**：除非用户明确要求，禁止执行 `git stash pop`。
- **禁止使用 `git checkout -- .`**：除非用户明确要求，禁止批量还原工作区修改。
- **禁止编写兼容代码**：修改时禁止保留旧逻辑分支、兜底或双实现；删掉被替代实现，只留唯一正确实现。

## 外部参考目录

- `CC-Source/` 与 `docs/claude-code/` 仅作 AgentFlow 外部参考。
- 这两个目录不属于正式实现、README/ADR/架构守卫范围；设计、实现、评审、文档同步时默认排除。
