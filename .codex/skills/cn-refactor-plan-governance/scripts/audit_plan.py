#!/usr/bin/env python3
"""Audit refactor plans for archive readiness, references, and CI gating."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from dataclasses import asdict, dataclass
from pathlib import Path


CHECKBOX_RE = re.compile(r"^\s*[-*]\s+\[( |x|X)\]\s+")
TEXT_EXTENSIONS = {
    ".md",
    ".txt",
    ".go",
    ".py",
    ".ps1",
    ".sh",
    ".yaml",
    ".yml",
    ".json",
    ".toml",
    ".ini",
    ".cfg",
    ".conf",
}
SKIP_DIRS = {
    ".git",
    ".idea",
    ".vscode",
    "node_modules",
    "vendor",
    "bin",
    "dist",
}
FAIL_ON_CHOICES = {
    "open-tasks",
    "direct-references",
    "filename-references",
    "repo-gate-block",
    "repo-gate-error",
    "not-ready",
    "ready-to-archive",
    "already-archived",
}


@dataclass
class Hit:
    path: str
    line: int
    kind: str
    text: str


@dataclass
class SearchFile:
    path: Path
    rel_path: str
    lines: list[str]


def repo_root_from_script() -> Path:
    return Path(__file__).resolve().parents[4]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Audit refactor plans for direct references and archive readiness."
    )
    parser.add_argument("plan", nargs="?", help="Plan filename or path")
    parser.add_argument("--target", help="Plan filename/path, or use 'all' for batch audit")
    parser.add_argument("--root", default="docs/重构计划")
    parser.add_argument("--json", action="store_true", dest="as_json")
    parser.add_argument(
        "--fail-on",
        action="append",
        default=[],
        metavar="RULE",
        help=(
            "Fail the process when a rule matches. Supported: "
            + ", ".join(sorted(FAIL_ON_CHOICES))
            + ", or verdict:<value>. Can be repeated or comma-separated."
        ),
    )
    args = parser.parse_args()
    if args.plan and args.target:
        parser.error("请只传位置参数或 --target，不要同时传。")
    args.target = args.target or args.plan
    if not args.target:
        parser.error("必须提供目标计划，或使用 --target all。")
    args.fail_on = normalize_fail_rules(args.fail_on, parser)
    return args


def normalize_fail_rules(raw_rules: list[str], parser: argparse.ArgumentParser) -> list[str]:
    rules: list[str] = []
    for raw in raw_rules:
        for part in raw.split(","):
            rule = part.strip()
            if not rule:
                continue
            if rule.startswith("verdict:"):
                verdict = rule.split(":", 1)[1].strip()
                if not verdict:
                    parser.error("--fail-on verdict:<value> 需要具体 verdict 名称。")
                rules.append(rule)
                continue
            if rule not in FAIL_ON_CHOICES:
                parser.error(f"不支持的 --fail-on 条件: {rule}")
            rules.append(rule)
    return rules


def resolve_roots(repo_root: Path, root_arg: str) -> tuple[Path, Path]:
    root = (repo_root / root_arg).resolve()
    archive_root = (root / "归档").resolve()
    if not root.exists():
        raise FileNotFoundError(f"目录不存在: {root}")
    return root, archive_root


def resolve_plan_candidates(
    repo_root: Path,
    root: Path,
    archive_root: Path,
    target: str,
) -> list[Path]:
    candidates: list[Path] = []
    raw_target = Path(target)
    if raw_target.is_absolute() and raw_target.exists():
        candidates.append(raw_target.resolve())
    else:
        for candidate in (
            repo_root / target,
            root / target,
            archive_root / target,
        ):
            if candidate.exists():
                candidates.append(candidate.resolve())

    if not candidates:
        name = raw_target.name
        for search_root in (root, archive_root):
            candidates.extend(p.resolve() for p in search_root.rglob(name) if p.is_file())

    unique: list[Path] = []
    seen: set[Path] = set()
    for candidate in candidates:
        if candidate not in seen:
            unique.append(candidate)
            seen.add(candidate)
    return unique


def resolve_single_plan(
    repo_root: Path,
    root: Path,
    archive_root: Path,
    target: str,
) -> Path:
    candidates = resolve_plan_candidates(repo_root, root, archive_root, target)
    if not candidates:
        raise FileNotFoundError(f"未找到计划文件: {target}")
    if len(candidates) > 1:
        joined = "\n".join(str(p) for p in candidates)
        raise RuntimeError(f"目标不唯一，请改用完整路径:\n{joined}")
    return candidates[0]


def collect_all_plans(root: Path, archive_root: Path) -> list[Path]:
    plans: list[Path] = []
    seen: set[Path] = set()
    for search_root in (root, archive_root):
        for path in sorted(search_root.rglob("*.md")):
            if not path.is_file():
                continue
            if any(part in SKIP_DIRS for part in path.parts):
                continue
            if "evidence" in path.parts:
                continue
            if path.name == "README.md":
                continue
            resolved = path.resolve()
            if resolved in seen:
                continue
            seen.add(resolved)
            plans.append(resolved)
    return plans


def plan_stats(path: Path) -> dict[str, int]:
    lines = path.read_text(encoding="utf-8").splitlines()
    checkbox_lines = [line for line in lines if CHECKBOX_RE.match(line)]
    total = len(checkbox_lines)
    done = sum("[x]" in line.lower() for line in checkbox_lines)
    todo = total - done
    return {"total": total, "done": done, "todo": todo}


def is_text_file(path: Path) -> bool:
    return path.suffix.lower() in TEXT_EXTENSIONS


def load_search_files(repo_root: Path) -> list[SearchFile]:
    corpus: list[SearchFile] = []
    for path in repo_root.rglob("*"):
        if not path.is_file():
            continue
        if any(part in SKIP_DIRS for part in path.parts):
            continue
        if not is_text_file(path):
            continue
        try:
            lines = path.read_text(encoding="utf-8").splitlines()
        except UnicodeDecodeError:
            continue
        corpus.append(
            SearchFile(
                path=path.resolve(),
                rel_path=str(path.relative_to(repo_root)).replace("\\", "/"),
                lines=lines,
            )
        )
    return corpus


def scan_references(
    repo_root: Path,
    plan_path: Path,
    corpus: list[SearchFile],
) -> tuple[list[Hit], list[Hit]]:
    rel_plan = plan_path.relative_to(repo_root).as_posix()
    name = plan_path.name
    exact_patterns = {
        rel_plan,
        rel_plan.replace("/", "\\"),
    }
    exact_hits: list[Hit] = []
    name_hits: list[Hit] = []

    for item in corpus:
        if item.path == plan_path:
            continue
        for idx, line in enumerate(item.lines, start=1):
            normalized = line.replace("\\", "/")
            if any(pattern.replace("\\", "/") in normalized for pattern in exact_patterns):
                exact_hits.append(
                    Hit(
                        path=item.rel_path,
                        line=idx,
                        kind="exact-path",
                        text=line.strip(),
                    )
                )
            elif name in line:
                name_hits.append(
                    Hit(
                        path=item.rel_path,
                        line=idx,
                        kind="filename",
                        text=line.strip(),
                    )
                )

    return exact_hits, name_hits


def run_gate(repo_root: Path, root_arg: str) -> dict[str, object]:
    guard = (
        repo_root
        / ".codex"
        / "skills"
        / "cn-refactor-plan-governance"
        / "scripts"
        / "run_guard.py"
    )
    cmd = [sys.executable, str(guard), "gate", "--root", root_arg]
    completed = subprocess.run(
        cmd,
        cwd=repo_root,
        capture_output=True,
        text=True,
        encoding="utf-8",
    )
    status = "pass" if completed.returncode == 0 else "block"
    if completed.returncode == 1:
        status = "error"
    output = (completed.stdout + completed.stderr).strip()
    return {
        "exit_code": completed.returncode,
        "status": status,
        "output": output,
    }


def build_verdict(
    plan_path: Path,
    root: Path,
    archive_root: Path,
    stats: dict[str, int],
    exact_hits: list[Hit],
) -> str:
    if plan_path.is_relative_to(archive_root):
        return "already_archived"
    if stats["todo"] > 0:
        return "not_ready_open_tasks"
    if exact_hits:
        return "not_ready_direct_references"
    if plan_path.parent == root:
        return "ready_to_archive_plan_only"
    return "needs_manual_review"


def build_result(
    repo_root: Path,
    plan_path: Path,
    root: Path,
    archive_root: Path,
    gate: dict[str, object],
    corpus: list[SearchFile],
) -> dict[str, object]:
    stats = plan_stats(plan_path)
    exact_hits, name_hits = scan_references(repo_root, plan_path, corpus)
    verdict = build_verdict(plan_path, root, archive_root, stats, exact_hits)
    return {
        "plan": str(plan_path.relative_to(repo_root)).replace("\\", "/"),
        "location": (
            "archive-root"
            if plan_path.is_relative_to(archive_root)
            else "active-root"
            if plan_path.parent == root
            else "other"
        ),
        "done": stats["done"],
        "todo": stats["todo"],
        "total": stats["total"],
        "verdict": verdict,
        "gate_status": gate["status"],
        "gate_exit_code": gate["exit_code"],
        "gate_output": gate["output"],
        "exact_reference_hits": [asdict(hit) for hit in exact_hits],
        "filename_hits": [asdict(hit) for hit in name_hits],
    }


def summarize(results: list[dict[str, object]]) -> dict[str, object]:
    verdict_counts: dict[str, int] = {}
    for result in results:
        verdict = str(result["verdict"])
        verdict_counts[verdict] = verdict_counts.get(verdict, 0) + 1
    return {
        "plans": len(results),
        "todos": sum(int(result["todo"]) for result in results),
        "with_exact_reference_hits": sum(
            1 for result in results if result["exact_reference_hits"]
        ),
        "with_filename_hits": sum(1 for result in results if result["filename_hits"]),
        "verdict_counts": verdict_counts,
    }


def rule_matches(result: dict[str, object], rule: str) -> bool:
    if rule == "open-tasks":
        return int(result["todo"]) > 0
    if rule == "direct-references":
        return len(result["exact_reference_hits"]) > 0
    if rule == "filename-references":
        return len(result["filename_hits"]) > 0
    if rule == "repo-gate-block":
        return result["gate_status"] == "block"
    if rule == "repo-gate-error":
        return result["gate_status"] == "error"
    if rule == "not-ready":
        return result["verdict"] in {
            "not_ready_open_tasks",
            "not_ready_direct_references",
            "needs_manual_review",
        }
    if rule == "ready-to-archive":
        return result["verdict"] == "ready_to_archive_plan_only"
    if rule == "already-archived":
        return result["verdict"] == "already_archived"
    if rule.startswith("verdict:"):
        return result["verdict"] == rule.split(":", 1)[1]
    return False


def evaluate_fail_on(
    results: list[dict[str, object]],
    rules: list[str],
) -> tuple[bool, list[dict[str, str]]]:
    matches: list[dict[str, str]] = []
    if not rules:
        return False, matches
    for result in results:
        for rule in rules:
            if rule_matches(result, rule):
                matches.append({"plan": str(result["plan"]), "rule": rule})
    return bool(matches), matches


def print_text_report(result: dict[str, object]) -> None:
    print("Refactor Plan Audit")
    print("=" * 80)
    print(f"Plan: {result['plan']}")
    print(f"Location: {result['location']}")
    print(f"Done/Todo/Total: {result['done']}/{result['todo']}/{result['total']}")
    print(f"Verdict: {result['verdict']}")
    print(f"Repo gate: {result['gate_status']} (exit={result['gate_exit_code']})")
    print("-" * 80)

    exact_hits: list[dict[str, object]] = result["exact_reference_hits"]  # type: ignore[assignment]
    name_hits: list[dict[str, object]] = result["filename_hits"]  # type: ignore[assignment]

    print(f"Exact path references: {len(exact_hits)}")
    for hit in exact_hits[:20]:
        print(f"  - {hit['path']}:{hit['line']} [{hit['kind']}] {hit['text']}")
    if len(exact_hits) > 20:
        print(f"  ... 其余 {len(exact_hits) - 20} 条省略")

    print(f"Bare filename references: {len(name_hits)}")
    for hit in name_hits[:20]:
        print(f"  - {hit['path']}:{hit['line']} [{hit['kind']}] {hit['text']}")
    if len(name_hits) > 20:
        print(f"  ... 其余 {len(name_hits) - 20} 条省略")

    print("-" * 80)
    print("Interpretation:")
    if result["verdict"] == "already_archived":
        print("  - 目标计划已在归档目录。")
    elif result["verdict"] == "not_ready_open_tasks":
        print("  - 目标计划仍有未完成 [ ]，不能归档。")
    elif result["verdict"] == "not_ready_direct_references":
        print("  - 目标计划已完成，但仍有直接路径引用未修正。")
    elif result["verdict"] == "ready_to_archive_plan_only":
        print("  - 目标计划可作为单计划归档候选；若 repo gate 未通过，仍不能宣布全仓收尾。")
    else:
        print("  - 需要人工复核计划位置与引用情况。")


def print_batch_report(
    root_arg: str,
    summary: dict[str, object],
    results: list[dict[str, object]],
    fail_rules: list[str],
    fail_matches: list[dict[str, str]],
) -> None:
    print("Refactor Plan Batch Audit")
    print("=" * 80)
    print(f"Root: {root_arg}")
    print(f"Plans: {summary['plans']}")
    print(f"Total todo: {summary['todos']}")
    print(f"Plans with exact references: {summary['with_exact_reference_hits']}")
    print(f"Plans with filename references: {summary['with_filename_hits']}")
    print(f"Verdict counts: {summary['verdict_counts']}")
    print("-" * 80)
    for result in results:
        print(
            f"- {result['plan']}: verdict={result['verdict']}, "
            f"todo={result['todo']}, exact={len(result['exact_reference_hits'])}, "
            f"filename={len(result['filename_hits'])}, gate={result['gate_status']}"
        )
    if fail_rules:
        print("-" * 80)
        print(f"Fail rules: {fail_rules}")
        print(f"Matched: {len(fail_matches)}")
        for match in fail_matches[:40]:
            print(f"  - {match['plan']} -> {match['rule']}")
        if len(fail_matches) > 40:
            print(f"  ... 其余 {len(fail_matches) - 40} 条省略")


def main() -> int:
    args = parse_args()
    repo_root = repo_root_from_script()

    try:
        root, archive_root = resolve_roots(repo_root, args.root)
    except FileNotFoundError as exc:
        print(f"[ERROR] {exc}", file=sys.stderr)
        return 1

    gate = run_gate(repo_root, args.root)
    corpus = load_search_files(repo_root)

    try:
        if args.target == "all":
            plan_paths = collect_all_plans(root, archive_root)
        else:
            plan_paths = [resolve_single_plan(repo_root, root, archive_root, args.target)]
    except (FileNotFoundError, RuntimeError) as exc:
        print(f"[ERROR] {exc}", file=sys.stderr)
        return 1

    results = [
        build_result(repo_root, plan_path, root, archive_root, gate, corpus)
        for plan_path in plan_paths
    ]
    fail_matched, fail_matches = evaluate_fail_on(results, args.fail_on)

    if args.target == "all":
        payload = {
            "mode": "batch",
            "root": args.root,
            "summary": summarize(results),
            "fail_on": args.fail_on,
            "fail_matched": fail_matched,
            "fail_matches": fail_matches,
            "results": results,
        }
        if args.as_json:
            print(json.dumps(payload, ensure_ascii=False, indent=2))
        else:
            print_batch_report(
                args.root,
                payload["summary"],
                results,
                args.fail_on,
                fail_matches,
            )
        return 2 if fail_matched else 0

    result = results[0]
    payload = {
        "mode": "single",
        "fail_on": args.fail_on,
        "fail_matched": fail_matched,
        "fail_matches": fail_matches,
        **result,
    }
    if args.as_json:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
    else:
        print_text_report(result)
        if args.fail_on:
            print("-" * 80)
            print(f"Fail rules: {args.fail_on}")
            print(f"Matched: {len(fail_matches)}")
            for match in fail_matches:
                print(f"  - {match['plan']} -> {match['rule']}")
    return 2 if fail_matched else 0


if __name__ == "__main__":
    raise SystemExit(main())
