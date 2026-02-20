---
name: check-backend
description: "检查你刚写的代码是否符合后端开发规范。"
---

检查你刚写的代码是否符合后端开发规范。

执行以下步骤：
1. 运行 `git status` 查看修改的文件
2. 阅读 `.trellis/spec/backend/index.md` 了解适用的规范
3. 根据你的修改内容，阅读相关的规范文件：
   - 数据库变更 → `.trellis/spec/backend/database-guidelines.md`
   - 错误处理 → `.trellis/spec/backend/error-handling.md`
   - 日志变更 → `.trellis/spec/backend/logging-guidelines.md`
   - 类型变更 → `.trellis/spec/backend/type-safety.md`
   - 任何变更 → `.trellis/spec/backend/quality-guidelines.md`
4. 对照规范审查你的代码
5. 报告发现的违规项并修复
