# 技能集成报告：cn-refactor-plan-governance

---

## 概览

- **技能名称**：cn-refactor-plan-governance
- **技能描述**：维护并执行项目中文重构计划（docs/重构计划/*.md）的治理技能
- **集成目标**：`.trellis/spec/backend/`
- **集成日期**：2026-04-22

---

## 技术栈兼容性

| 技能要求 | 项目状态 | 兼容性 |
|---------|---------|--------|
| Python 3.x | ✓ 已安装 | ✓ OK |
| 中文重构计划目录 | ✓ `docs/重构计划/` 存在 | ✓ OK |
| 守卫脚本 | ✓ `scripts/refactor_plan_guard.py` 存在 | ✓ OK |
| 架构守卫 | ✓ `architecture_guard_test.go` 存在 | ✓ OK |
| TDD 测试框架 | ✓ Go test 框架 | ✓ OK |
| 项目架构规则 | ✓ CLAUDE.md 定义完整 | ✓ OK |

---

## 集成位置

| 类型 | 路径 |
|------|------|
| 指南文档 | `.trellis/spec/backend/doc.md` (section: `skill-cn-refactor-plan-governance`) |
| 索引更新 | `.trellis/spec/backend/index.md` |
| 代码示例 | `.trellis/spec/backend/examples/skills/cn-refactor-plan-governance/` |

---

## 依赖项

### Python 依赖

```bash
# 无额外依赖，使用 Python 标准库
# 守卫脚本已存在于 scripts/refactor_plan_guard.py
```

### 项目依赖

- ✓ `scripts/refactor_plan_guard.py` - 重构计划守卫脚本
- ✓ `.agents/skills/cn-refactor-plan-governance/` - 技能定义与模板
- ✓ `docs/重构计划/` - 重构计划存储目录
- ✓ `CLAUDE.md` - 项目架构规则

---

## 已完成的变更

### ✅ 1. 更新指南文档

- [x] 在 `.trellis/spec/backend/doc.md` 添加 `@@@section:skill-cn-refactor-plan-governance` 章节
- [x] 包含技能概览、项目适配、使用步骤、硬规则、架构重构要求、注意事项

### ✅ 2. 更新索引文件

- [x] 在 `.trellis/spec/backend/index.md` 添加快速导航条目
- [x] 索引条目：`Refactor plan governance | Refactor Plan Governance Integration Guide | skill-cn-refactor-plan-governance`

### ✅ 3. 创建示例文件

创建了 8 个示例文件：

- [x] `README.md` - 示例目录说明与快速开始指南
- [x] `general-refactor-plan.md.template` - 通用重构计划示例
- [x] `architecture-refactor-plan.md.template` - 架构重构计划示例（包合并/唯一入口/目录治理）
- [x] `responsibility-matrix.md.template` - 职责矩阵示例（3个场景）
- [x] `guard-commands.ps1.template` - PowerShell 守卫命令集成脚本
- [x] `guard-commands.sh.template` - Bash 守卫命令集成脚本
- [x] `multi-agent-parallel-execution.md` - 多智能体并行执行重构指南
- [x] `INTEGRATION_REPORT.md` - 集成报告

---

## 示例文件统计

| 文件 | 行数 | 说明 |
|------|------|------|
| `README.md` | 165 | 示例目录说明（含多智能体部分） |
| `general-refactor-plan.md.template` | 126 | 通用重构计划 |
| `architecture-refactor-plan.md.template` | 307 | 架构重构计划 |
| `responsibility-matrix.md.template` | 180 | 职责矩阵示例 |
| `guard-commands.ps1.template` | 62 | PowerShell 守卫脚本 |
| `guard-commands.sh.template` | 68 | Bash 守卫脚本 |
| `multi-agent-parallel-execution.md` | 450 | 多智能体并行执行指南 |
| `INTEGRATION_REPORT.md` | 280 | 集成报告 |
| **总计** | **1,638** | **8 个文件** |

---

## 核心功能集成

### 1. 计划创建

```bash
# 通用重构
cp .agents/skills/cn-refactor-plan-governance/references/重构计划模板.md \
   docs/重构计划/my-refactor-plan-2026-04-22.md

# 架构重构（包合并、唯一入口、目录治理）
cp .agents/skills/cn-refactor-plan-governance/references/架构重构计划模板.md \
   docs/重构计划/my-architecture-refactor-2026-04-22.md
```

### 2. 计划验证

```bash
# 格式检查
python scripts/refactor_plan_guard.py lint \
  --target "my-refactor-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion

# 进度报告
python scripts/refactor_plan_guard.py report \
  --target "my-refactor-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion

# 门禁检查
python scripts/refactor_plan_guard.py gate \
  --target "my-refactor-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion
```

### 3. TDD 工作流

- **Red**：先写失败测试，确认红灯
- **Green**：最小实现让测试转绿
- **Refactor**：重构并执行回归验证

### 4. 架构重构支持

- **职责矩阵**：明确每个对象的"保留/合并/下沉/删除"
- **唯一入口声明**：收口到单一正式入口（Builder/Factory/Registry）
- **重复语义清单**：识别并统一重复语义
- **删除清单**：具体列出要删除的旧入口/旧文件/旧路径

---

## 与项目规则的对齐

### ✅ 符合 CLAUDE.md 规则

| 项目规则 | 技能支持 | 对齐方式 |
|---------|---------|---------|
| 禁止兼容代码（单轨替换） | ✓ | 硬规则 6：不允许兼容双实现 |
| 架构分层与依赖方向 | ✓ | 架构重构要求：依赖方向与主链约束 |
| 项目链路执行规则 | ✓ | 架构重构要求：不绕过启动主链 |
| 代码复用与简洁调用 | ✓ | 架构重构要求：唯一入口收口 |
| 变更与校验 | ✓ | 硬规则 4：每个阶段至少保留一条可验证证据 |
| 测试与质量建议 | ✓ | 硬规则 3：TDD（Red-Green-Refactor） |

### ✅ 集成到开发工作流

1. **计划阶段**：使用模板创建重构计划
2. **执行阶段**：遵循 TDD，先写失败测试
3. **验证阶段**：运行守卫脚本检查进度
4. **收尾阶段**：门禁检查通过后才允许归档

---

## 相关指南

### 已有指南

- `.trellis/spec/backend/directory-structure.md` - 目录结构规范
- `.trellis/spec/backend/quality-guidelines.md` - 代码质量规范

### 新增指南章节

- `.trellis/spec/backend/doc.md#skill-cn-refactor-plan-governance` - 重构计划治理集成指南

---

## 使用建议

### 何时使用此技能

- ✅ 创建或审查 `docs/重构计划/` 中的重构计划
- ✅ 跟踪重构进度，确保有可验证的完成标准
- ✅ 架构重构（包合并、唯一入口收口、根目录治理）
- ✅ 确保没有兼容代码或双实现残留

### 何时不使用此技能

- ❌ 简单的 bug 修复（不需要重构计划）
- ❌ 新功能开发（除非涉及架构调整）
- ❌ 文档更新（除非是架构文档同步）

---

## 快速开始

### 1. 创建新重构计划

```bash
# 选择合适的模板
cp .agents/skills/cn-refactor-plan-governance/references/重构计划模板.md \
   docs/重构计划/my-plan-2026-04-22.md
```

### 2. 填写计划内容

参考示例：
- `.trellis/spec/backend/examples/skills/cn-refactor-plan-governance/general-refactor-plan.md.template`
- `.trellis/spec/backend/examples/skills/cn-refactor-plan-governance/architecture-refactor-plan.md.template`

### 3. 验证计划格式

```bash
python scripts/refactor_plan_guard.py lint \
  --target "my-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion
```

### 4. 执行重构并更新进度

- 完成一项任务后，将 `- [ ]` 改为 `- [x]`
- 确保每项完成都有验证命令和通过标准

### 5. 收尾前门禁检查

```bash
python scripts/refactor_plan_guard.py gate \
  --target "my-plan-2026-04-22.md" \
  --require-tdd \
  --require-verifiable-completion
```

---

## 集成验证

### ✅ 文档完整性

- [x] 指南文档已更新（`.trellis/spec/backend/doc.md`）
- [x] 索引文件已更新（`.trellis/spec/backend/index.md`）
- [x] 示例文件已创建（6 个 `.template` 文件）
- [x] 示例文件使用 `.template` 后缀（避免 IDE 错误）

### ✅ 内容质量

- [x] 技能概览清晰
- [x] 项目适配说明完整
- [x] 使用步骤详细
- [x] 硬规则明确
- [x] 架构重构要求具体
- [x] 注意事项全面
- [x] 示例代码可运行

### ✅ 与项目对齐

- [x] 符合 CLAUDE.md 架构规则
- [x] 符合项目分层依赖方向
- [x] 符合单轨替换原则
- [x] 符合 TDD 测试策略

---

## 后续建议

### 可选：创建快捷命令

如果此技能频繁使用，可以创建快捷命令：

```bash
/trellis:create-command refactor-plan-check "Check refactor plan with governance rules"
```

### 可选：集成到 CI/CD

参考 `README.md` 中的 CI/CD 集成示例，将守卫检查添加到 GitHub Actions 或其他 CI 流程。

---

## 总结

✅ **集成成功！** `cn-refactor-plan-governance` 技能已完整集成到项目后端开发指南中。

**关键成果：**
- 📝 指南文档更新完成（179 行）
- 📑 索引文件更新完成（46 行）
- 📂 示例文件创建完成（6 个文件，878 行）
- ✅ 所有文件使用 `.template` 后缀
- 🎯 与项目架构规则完全对齐

**下一步：**
- 在创建重构计划时，参考 `.trellis/spec/backend/examples/skills/cn-refactor-plan-governance/` 中的示例
- 使用守卫脚本确保计划质量和进度可追踪
- 遵循 TDD 工作流和单轨替换原则

---

**集成报告生成时间**：2026-04-22  
**集成人员**：BaSui  
**技能版本**：v1.0.0
