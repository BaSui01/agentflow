#!/usr/bin/env python3
"""
Refactor plan guard for docs/重构计划.

Usage:
  python scripts/refactor_plan_guard.py lint
  python scripts/refactor_plan_guard.py report
  python scripts/refactor_plan_guard.py gate
  python scripts/refactor_plan_guard.py autofix
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


CHECKBOX_RE = re.compile(r"^\s*[-*]\s+\[( |x|X)\]\s+")
HEADING_RE = re.compile(r"^\s*##+\s+")
LABEL_RE = {
    "验证命令": re.compile(r"验证命令\s*[:：]\s*\S+"),
    "通过标准": re.compile(r"通过标准\s*[:：]\s*\S+"),
}

REQUIRED_HEADING_GROUPS = (
    ("执行状态总览",),
    ("执行计划", "Phase"),
    ("完成定义", "DoD"),
)
TDD_HEADING_GROUP = ("测试策略（TDD）", "测试计划（TDD）")
TDD_REQUIRED_STEPS = (
    ("先写失败测试", "失败测试", "红灯"),
    ("最小实现让测试转绿", "转绿", "绿灯"),
    ("重构并回归验证", "重构"),
)
VERIFIABLE_SECTION_GROUPS = (
    TDD_HEADING_GROUP,
    ("执行计划", "Phase"),
    ("完成定义", "DoD"),
)

SKIP_FILES = {
    # 审核报告不作为执行计划模板检查对象
    "重构计划审核与完善-2026-03-02.md",
}


@dataclass
class PlanStats:
    path: Path
    total: int
    done: int
    todo: int
    errors: list[str]


@dataclass(frozen=True)
class PlanRequirements:
    require_tdd: bool = False
    require_verifiable_completion: bool = False


def iter_plan_files(root: Path, target: str) -> Iterable[Path]:
    if target == "all":
        files = sorted(root.glob("*.md"))
    else:
        path = root / target
        files = [path] if path.exists() else []
    for path in files:
        if path.name in SKIP_FILES:
            continue
        yield path


def collect_sections(lines: list[str]) -> list[tuple[str, list[str]]]:
    sections: list[tuple[str, list[str]]] = []
    current_heading: str | None = None
    current_lines: list[str] = []

    for line in lines:
        if HEADING_RE.match(line):
            if current_heading is not None:
                sections.append((current_heading, current_lines))
            current_heading = line.strip()
            current_lines = []
            continue
        if current_heading is not None:
            current_lines.append(line)

    if current_heading is not None:
        sections.append((current_heading, current_lines))
    return sections


def find_section(
    sections: list[tuple[str, list[str]]], group: tuple[str, ...]
) -> tuple[str, list[str]] | None:
    for heading, content in sections:
        if any(token in heading for token in group):
            return heading, content
    return None


def has_label_value(line: str, label: str) -> bool:
    return LABEL_RE[label].search(line) is not None


def validate_tdd_section(lines: list[str]) -> list[str]:
    errors: list[str] = []
    checkbox_lines = [line.strip() for line in lines if CHECKBOX_RE.match(line)]
    if not checkbox_lines:
        return ["测试策略（TDD）章节缺少任务状态行。"]

    for step_group in TDD_REQUIRED_STEPS:
        if not any(any(token in line for token in step_group) for line in checkbox_lines):
            errors.append(
                "测试策略（TDD）章节缺少关键步骤："
                + " / ".join(step_group)
            )
    return errors


def validate_verifiable_section(section_name: str, lines: list[str]) -> list[str]:
    errors: list[str] = []
    checkbox_lines = [line.strip() for line in lines if CHECKBOX_RE.match(line)]
    if not checkbox_lines:
        return [f"{section_name}章节缺少任务状态行。"]

    for line in checkbox_lines:
        if not has_label_value(line, "验证命令"):
            errors.append(f"{section_name}章节存在未声明验证命令的任务：{line}")
        if not has_label_value(line, "通过标准"):
            errors.append(f"{section_name}章节存在未声明通过标准的任务：{line}")
    return errors


def collect_stats(
    path: Path, requirements: PlanRequirements | None = None
) -> PlanStats:
    requirements = requirements or PlanRequirements()
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()
    errors: list[str] = []

    checkbox_lines = [line for line in lines if CHECKBOX_RE.match(line)]
    total = len(checkbox_lines)
    done = sum("[x]" in line.lower() for line in checkbox_lines)
    todo = total - done

    if total == 0:
        errors.append("缺少任务状态：至少需要一条 `- [ ]` 或 `- [x]`。")

    headings = [line.strip() for line in lines if HEADING_RE.match(line)]
    required_heading_groups = list(REQUIRED_HEADING_GROUPS)
    if requirements.require_tdd:
        required_heading_groups.append(TDD_HEADING_GROUP)

    for group in required_heading_groups:
        if not any(any(token in h for token in group) for h in headings):
            errors.append(
                "缺少必需章节："
                + "/".join(group)
                + "（要求包含执行状态、执行计划、完成定义"
                + ("、测试策略（TDD）" if requirements.require_tdd else "")
                + "）"
            )

    sections = collect_sections(lines)

    if requirements.require_tdd:
        tdd_section = find_section(sections, TDD_HEADING_GROUP)
        if tdd_section is not None:
            _, tdd_lines = tdd_section
            errors.extend(validate_tdd_section(tdd_lines))

    if requirements.require_verifiable_completion:
        for group in VERIFIABLE_SECTION_GROUPS:
            section = find_section(sections, group)
            if section is None:
                continue
            heading, section_lines = section
            errors.extend(validate_verifiable_section(heading, section_lines))

    return PlanStats(path=path, total=total, done=done, todo=todo, errors=errors)


def print_report(stats_list: list[PlanStats]) -> None:
    print("Refactor Plan Progress")
    print("=" * 80)
    print(f"{'File':46} {'Done':>6} {'Todo':>6} {'Total':>6}")
    print("-" * 80)
    for s in stats_list:
        rel = str(s.path).replace("\\", "/")
        print(f"{rel[-46:]:46} {s.done:6d} {s.todo:6d} {s.total:6d}")
    print("-" * 80)
    total_done = sum(s.done for s in stats_list)
    total_todo = sum(s.todo for s in stats_list)
    total_all = sum(s.total for s in stats_list)
    print(f"{'TOTAL':46} {total_done:6d} {total_todo:6d} {total_all:6d}")


def run_lint(stats_list: list[PlanStats]) -> int:
    issues = 0
    for s in stats_list:
        if not s.errors:
            continue
        issues += len(s.errors)
        print(f"[FAIL] {s.path.as_posix()}")
        for err in s.errors:
            print(f"  - {err}")
    if issues == 0:
        print("[OK] 计划格式检查通过。")
        return 0
    print(f"[ERROR] 计划格式检查失败，共 {issues} 项问题。")
    return 1


def run_gate(stats_list: list[PlanStats]) -> int:
    if run_lint(stats_list) != 0:
        return 1
    not_done = [s for s in stats_list if s.todo > 0]
    if not_done:
        print("[BLOCK] 存在未完成任务，禁止停止/收尾。")
        for s in not_done:
            print(f"  - {s.path.as_posix()}: 仍有 {s.todo} 项 `[ ]`")
        return 2
    print("[OK] 全部计划任务均已完成（全部为 `[x]`），允许停止/收尾。")
    return 0


def build_missing_section(group: tuple[str, ...]) -> str:
    primary = group[0]
    if primary == "执行状态总览":
        return (
            "## 执行状态总览（自动补齐）\n\n"
            "- [x] 已补齐章节结构\n"
            "- [x] 已补齐任务状态行\n"
        )
    if primary == "测试策略（TDD）":
        return (
            "## 测试策略（TDD，自动补齐）\n\n"
            "- [ ] 先写失败测试并确认红灯（验证命令：`go test ./path/to/pkg -run TestName`; 通过标准：新增测试先失败，且失败原因与待修问题直接对应）\n"
            "- [ ] 采用最小实现让测试转绿（验证命令：`go test ./path/to/pkg -run TestName`; 通过标准：目标测试转绿，且未引入兼容分支）\n"
            "- [ ] 完成重构并执行回归验证（验证命令：`go test ./path/to/pkg`; 通过标准：相关测试全部通过，且旧实现已删除）\n"
        )
    if primary == "执行计划":
        return (
            "## 执行计划（自动补齐）\n\n"
            "### Phase-A：文档结构补齐\n\n"
            "- [ ] 统一章节结构（验证命令：`python scripts/refactor_plan_guard.py lint --target <计划文件名>`; 通过标准：章节结构满足门禁要求）\n"
            "- [ ] 补齐任务状态行（验证命令：`python scripts/refactor_plan_guard.py report --target <计划文件名>`; 通过标准：报告可统计全部任务状态）\n"
        )
    return (
        "## 完成定义（DoD，自动补齐）\n\n"
        "- [ ] 已具备 DoD 章节（验证命令：`python scripts/refactor_plan_guard.py lint --target <计划文件名>`; 通过标准：DoD 章节存在且结构合法）\n"
        "- [ ] 已纳入 gate 校验范围（验证命令：`python scripts/refactor_plan_guard.py gate --target <计划文件名>`; 通过标准：所有任务完成后 gate 可通过）\n"
    )


def apply_autofix(
    path: Path, requirements: PlanRequirements | None = None
) -> tuple[bool, list[str]]:
    requirements = requirements or PlanRequirements()
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()
    headings = [line.strip() for line in lines if HEADING_RE.match(line)]

    missing_groups: list[tuple[str, ...]] = []
    required_heading_groups = list(REQUIRED_HEADING_GROUPS)
    if requirements.require_tdd:
        required_heading_groups.append(TDD_HEADING_GROUP)

    for group in required_heading_groups:
        if not any(any(token in h for token in group) for h in headings):
            missing_groups.append(group)

    # 没有任何任务状态时，补一条默认任务
    if not any(CHECKBOX_RE.match(line) for line in lines):
        text = text.rstrip() + "\n\n- [ ] 自动补齐：新增任务状态入口\n"
        lines = text.splitlines()

    if not missing_groups:
        if text != path.read_text(encoding="utf-8"):
            path.write_text(text, encoding="utf-8")
        return False, []

    additions: list[str] = []
    for group in missing_groups:
        additions.append(build_missing_section(group))

    patched = text.rstrip() + "\n\n---\n\n" + "\n\n".join(additions) + "\n"
    path.write_text(patched, encoding="utf-8")
    return True, ["/".join(g) for g in missing_groups]


def run_autofix(
    files: list[Path], requirements: PlanRequirements | None = None
) -> int:
    requirements = requirements or PlanRequirements()
    changed = 0
    for path in files:
        did_change, sections = apply_autofix(path, requirements=requirements)
        if did_change:
            changed += 1
            print(f"[FIXED] {path.as_posix()} -> 补齐章节: {', '.join(sections)}")
    if changed == 0:
        print("[OK] 无需自动补齐，所有文档已具备必需章节。")
    else:
        print(f"[OK] 已自动补齐 {changed} 个文档。")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description="Guard refactor plan markdown files.")
    parser.add_argument("cmd", choices=["lint", "report", "gate", "autofix"])
    parser.add_argument(
        "--root",
        default="docs/重构计划",
        help="重构计划目录（默认: docs/重构计划）",
    )
    parser.add_argument(
        "--target",
        default="all",
        help="目标文件名（默认 all）。例如: workflow层重构.md",
    )
    parser.add_argument(
        "--require-tdd",
        action="store_true",
        help="强制计划包含 TDD 章节，并写出失败测试/转绿/重构节奏。",
    )
    parser.add_argument(
        "--require-verifiable-completion",
        action="store_true",
        help="强制测试策略/执行计划/完成定义中的每条任务都包含验证命令与通过标准。",
    )
    args = parser.parse_args()
    requirements = PlanRequirements(
        require_tdd=args.require_tdd,
        require_verifiable_completion=args.require_verifiable_completion,
    )

    root = Path(args.root)
    if not root.exists():
        print(f"[ERROR] 目录不存在: {root}")
        return 1

    files = list(iter_plan_files(root, args.target))
    if not files:
        print("[ERROR] 未找到可检查的计划文档。")
        return 1

    stats_list = [collect_stats(p, requirements=requirements) for p in files]

    if args.cmd == "report":
        print_report(stats_list)
        return 0
    if args.cmd == "lint":
        return run_lint(stats_list)
    if args.cmd == "autofix":
        return run_autofix(files, requirements=requirements)
    return run_gate(stats_list)


if __name__ == "__main__":
    sys.exit(main())
