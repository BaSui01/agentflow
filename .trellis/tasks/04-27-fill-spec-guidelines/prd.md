# 填写开发规范指南文件

## 目标
填写 `.trellis/spec/backend/` 目录下的 4 个空模板指南文件：
1. `directory-structure.md` - 目录结构约定
2. `database-guidelines.md` - 数据库规范
3. `error-handling.md` - 错误处理标准
4. `logging-guidelines.md` - 日志规范

## 要求

### Directory Structure
- 文档化项目的分层架构（types/llm/agent/rag/workflow/api/cmd/pkg）
- 说明每一层的职责和依赖方向
- 提供命名约定和代码组织示例

### Database Guidelines
- 文档化 GORM 使用模式
- 说明迁移管理（golang-migrate）
- 提供查询模式和命名约定

### Error Handling
- 文档化 types.Error 结构化错误
- 说明错误码定义和使用
- 提供错误包装和转换模式
- 说明 HTTP 状态码映射

### Logging Guidelines
- 文档化 zap 日志库使用
- 说明日志级别和结构化字段
- 提供命名约定和最佳实践

## 验收标准
- [x] 所有 4 个指南文件都包含项目特定的实际约定（不是通用模板）
- [x] 每个指南都包含代码示例，引用项目中的实际文件
- [x] 每个指南都列出禁止模式和常见错误
- [x] 指南之间保持一致性
