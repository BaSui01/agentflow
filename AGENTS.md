<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

Use the `/trellis:start` command when starting a new session to:
- Initialize your developer identity
- Understand current project context
- Read relevant guidelines

Use `@/.trellis/` to learn:
- Development workflow (`workflow.md`)
- Project structure guidelines (`spec/`)
- Developer workspace (`workspace/`)

Keep this managed block so 'trellis update' can refresh the instructions.

<!-- TRELLIS:END -->

## Project Rules

- **禁止编写兼容代码**：代码修改时不允许为兼容旧逻辑保留分支、兜底或双实现；必须删除被替代的旧实现，只保留修改后唯一且最正确的实现。
