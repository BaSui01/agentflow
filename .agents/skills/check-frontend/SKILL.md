---
name: check-frontend
description: "检查你刚写的代码是否符合前端开发规范。"
---

检查你刚写的代码是否符合前端开发规范。

执行以下步骤：
1. 运行 `git status` 查看修改的文件
2. 阅读 `.trellis/spec/frontend/index.md` 了解适用的规范
3. 根据你的修改内容，阅读相关的规范文件：
   - 组件变更 → `.trellis/spec/frontend/component-guidelines.md`
   - Hook 变更 → `.trellis/spec/frontend/hook-guidelines.md`
   - 状态变更 → `.trellis/spec/frontend/state-management.md`
   - 类型变更 → `.trellis/spec/frontend/type-safety.md`
   - 任何变更 → `.trellis/spec/frontend/quality-guidelines.md`
4. 对照规范审查你的代码
5. 报告发现的违规项并修复
