---
description: 中文重构计划执行与门禁（校验格式、推进状态、未全 [x] 禁止收尾）
argument-hint: （可选）计划文件名，例如：workflow层重构.md
---

$ARGUMENTS

## 目标

维护并执行 `docs/重构计划/*.md`，强制使用 `- [ ]` / `- [x]` 状态，且在全部任务完成前禁止停止。

## 执行顺序（固定）

1. 格式校验
```bash
python scripts/refactor_plan_guard.py lint --target ${ARGUMENTS:-all}
```

2. 进度汇总
```bash
python scripts/refactor_plan_guard.py report --target ${ARGUMENTS:-all}
```

3. 推进执行
- 只从现有 `[ ]` 中选任务推进
- 完成后改为 `[x]`
- 同步补证据（测试命令、文件路径、守卫结果）

4. 收尾门禁（必须）
```bash
python scripts/refactor_plan_guard.py gate --target ${ARGUMENTS:-all}
```

## 强制规则

- 任意计划文档缺少 `- [ ]` / `- [x]`：判定失败
- 任意计划文档缺少“执行状态总览/执行计划(Phase)/完成定义(DoD)”章节：判定失败
- 仍有 `[ ]` 时：禁止输出“完成/停止/归档”

