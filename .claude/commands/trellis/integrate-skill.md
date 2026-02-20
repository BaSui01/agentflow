# 集成 Claude Skill 到项目规范

将 Claude 全局 Skill 适配并集成到你的项目开发规范中（不是直接集成到项目代码中）。

## 用法

```
/trellis:integrate-skill <skill-name>
```

**示例**：
```
/trellis:integrate-skill frontend-design
/trellis:integrate-skill mcp-builder
```

## 核心原则

> [!] **重要**：Skill 集成的目标是更新**开发规范**，而不是直接生成项目代码。
>
> - 规范内容 -> 写入 `.trellis/spec/{target}/doc.md`
> - 代码示例 -> 放在 `.trellis/spec/{target}/examples/skills/<skill-name>/`
> - 示例文件 -> 使用 `.template` 后缀（例如 `component.tsx.template`）以避免 IDE 报错
>
> 其中 `{target}` 是 `frontend` 或 `backend`，由 Skill 类型决定。

## 执行步骤

### 1. 读取 Skill 内容

```bash
openskills read <skill-name>
```

如果 Skill 不存在，提示用户检查可用的 Skill：
```bash
# 可用的 Skill 列在 AGENTS.md 的 <available_skills> 下
```

### 2. 确定集成目标

根据 Skill 类型，确定要更新哪些规范：

| Skill 类别 | 集成目标 |
|------------|----------|
| UI/前端（`frontend-design`, `web-artifacts-builder`） | `.trellis/spec/frontend/` |
| 后端/API（`mcp-builder`） | `.trellis/spec/backend/` |
| 文档（`doc-coauthoring`, `docx`, `pdf`） | `.trellis/` 或创建专用规范 |
| 测试（`webapp-testing`） | `.trellis/spec/frontend/`（E2E） |

### 3. 分析 Skill 内容

从 Skill 中提取：
- **核心概念**：Skill 如何工作及关键概念
- **最佳实践**：推荐的方法
- **代码模式**：可复用的代码模板
- **注意事项**：常见问题和解决方案

### 4. 执行集成

#### 4.1 更新规范文档

在对应的 `doc.md` 中添加新章节：

```markdown
@@@section:skill-<skill-name>
## # <Skill 名称> 集成指南

### 概述
[Skill 的核心功能和使用场景]

### 项目适配
[如何在当前项目中使用此 Skill]

### 使用步骤
1. [步骤 1]
2. [步骤 2]

### 注意事项
- [项目特定的约束]
- [与默认行为的差异]

### 参考示例
参见 `examples/skills/<skill-name>/`

@@@/section:skill-<skill-name>
```

#### 4.2 创建示例目录（如有代码示例）

```bash
# 目录结构（{target} = frontend 或 backend）
.trellis/spec/{target}/
|-- doc.md                      # 添加 Skill 相关章节
|-- index.md                    # 更新索引
+-- examples/
    +-- skills/
        +-- <skill-name>/
            |-- README.md               # 示例文档
            |-- example-1.ts.template   # 代码示例（使用 .template 后缀）
            +-- example-2.tsx.template
```

**文件命名约定**：
- 代码文件：`<name>.<ext>.template`（例如 `component.tsx.template`）
- 配置文件：`<name>.config.template`（例如 `tailwind.config.template`）
- 文档：`README.md`（正常后缀）

#### 4.3 更新索引文件

在 `index.md` 的快速导航表中添加：

```markdown
| <Skill 相关任务> | <章节名称> | `skill-<skill-name>` |
```

### 5. 生成集成报告

---

## Skill 集成报告：`<skill-name>`

### # 概述
- **Skill 描述**：[功能描述]
- **集成目标**：`.trellis/spec/{target}/`

### # 技术栈兼容性

| Skill 要求 | 项目状态 | 兼容性 |
|-----------|----------|--------|
| [技术 1] | [项目技术] | [OK]/[!]/[X] |

### # 集成位置

| 类型 | 路径 |
|------|------|
| 规范文档 | `.trellis/spec/{target}/doc.md`（章节：`skill-<name>`） |
| 代码示例 | `.trellis/spec/{target}/examples/skills/<name>/` |
| 索引更新 | `.trellis/spec/{target}/index.md` |

> `{target}` = `frontend` 或 `backend`

### # 依赖（如需要）

```bash
# 安装所需依赖（根据你的包管理器调整）
npm install <package>
# 或
pnpm add <package>
# 或
yarn add <package>
```

### [OK] 已完成的变更

- [ ] 在 `doc.md` 中添加了 `@@@section:skill-<name>` 章节
- [ ] 在 `index.md` 中添加了索引条目
- [ ] 在 `examples/skills/<name>/` 中创建了示例文件
- [ ] 示例文件使用了 `.template` 后缀

### # 相关规范

- [现有的相关章节 ID]

---

## 6. 可选：创建使用命令

如果此 Skill 经常使用，创建一个快捷命令：

```bash
/trellis:create-command use-<skill-name> 按照项目规范使用 <skill-name> Skill
```

## 常见 Skill 集成参考

| Skill | 集成目标 | 示例目录 |
|-------|----------|----------|
| `frontend-design` | `frontend` | `examples/skills/frontend-design/` |
| `mcp-builder` | `backend` | `examples/skills/mcp-builder/` |
| `webapp-testing` | `frontend` | `examples/skills/webapp-testing/` |
| `doc-coauthoring` | `.trellis/` | 无（仅文档工作流） |

## 示例：集成 `mcp-builder` Skill

### 目录结构

```
.trellis/spec/backend/
|-- doc.md                           # 添加 MCP 章节
|-- index.md                         # 添加索引条目
+-- examples/
    +-- skills/
        +-- mcp-builder/
            |-- README.md
            |-- server.ts.template
            |-- tools.ts.template
            +-- types.ts.template
```

### doc.md 中的新章节

```markdown
@@@section:skill-mcp-builder
## # MCP Server 开发指南

### 概述
使用 MCP（Model Context Protocol）创建 LLM 可调用的工具服务。

### 项目适配
- 将服务放在专用目录中
- 遵循现有的 TypeScript 和类型定义约定
- 使用项目的日志系统

### 参考示例
参见 `examples/skills/mcp-builder/`

@@@/section:skill-mcp-builder
```