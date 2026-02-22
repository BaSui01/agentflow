#!/usr/bin/env python3
"""
Git 分批提交自动化脚本

用法:
    # 查看分组计划（不执行）
    python3 batch_commit.py plan

    # 查看计划并保存到文件
    python3 batch_commit.py plan --output plan.json

    # 自动分组并一次性执行全部提交+合并
    python3 batch_commit.py auto --target master

    # 预览模式（只打印命令，不执行）
    python3 batch_commit.py auto --target master --dry-run

    # 从计划文件执行
    python3 batch_commit.py execute --plan plan.json

    # 排除特定文件/模式
    python3 batch_commit.py auto --target master --exclude "agentflow" --exclude "*.bin"
"""

import argparse
import json
import os
import re
import subprocess
import sys
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from fnmatch import fnmatch


# ── 分组配置 ──────────────────────────────────────────────

# 顶层目录 → 组名映射
DIR_GROUP_MAP = {
    ".trellis": "trellis",
    ".claude": "claude",
    "api": "api",
    "agent": "agent",
    "cmd": "cmd",
    "internal": "internal",
    "llm": "llm",
    "workflow": "workflow",
    "rag": "rag",
    "mcp": "mcp",
    "types": "types",
    "config": "config",
    "docs": "docs",
    "examples": "examples",
}

# 默认排除模式（二进制/构建产物）
DEFAULT_EXCLUDES = [
    "agentflow",       # Go 编译产物
    "*.exe",
    "*.dll",
    "*.so",
    "*.dylib",
    "vendor/",
]

# commit type 推断规则


def run_git(*args, dry_run=False, check=True, raw=False):
    """执行 git 命令，返回 stdout"""
    cmd = ["git"] + list(args)
    if dry_run:
        print(f"  [dry-run] {' '.join(cmd)}")
        return ""
    result = subprocess.run(cmd, capture_output=True, text=True, check=check)
    if raw:
        return result.stdout.rstrip("\n")
    return result.stdout.strip()


def get_changed_files():
    """解析 git status --porcelain，返回 [(status, filepath)] 列表"""
    output = run_git("status", "--porcelain", raw=True)
    if not output:
        return []
    files = []
    for line in output.splitlines():
        if len(line) < 4:
            continue
        # porcelain 格式: XY filename  或  XY old -> new
        status = line[:2].strip()
        path = line[3:]
        # 处理重命名
        if " -> " in path:
            path = path.split(" -> ")[1]
        files.append((status, path))
    return files


def should_exclude(filepath, excludes):
    """检查文件是否应被排除"""
    for pattern in excludes:
        if pattern.endswith("/"):
            if filepath.startswith(pattern) or f"/{pattern}" in filepath:
                return True
        elif fnmatch(filepath, pattern) or fnmatch(os.path.basename(filepath), pattern):
            return True
        elif filepath == pattern:
            return True
    return False


def infer_group(filepath):
    """根据文件路径推断分组名"""
    # go.mod / go.sum 单独一组
    basename = os.path.basename(filepath)
    if basename in ("go.mod", "go.sum"):
        return "deps"

    # 按顶层目录匹配
    parts = filepath.split("/")
    top_dir = parts[0] if parts else filepath
    if top_dir in DIR_GROUP_MAP:
        return DIR_GROUP_MAP[top_dir]

    # 根目录文件
    if len(parts) == 1:
        return "root"

    # 未知目录 → 用目录名
    return top_dir


def infer_commit_type(files_with_status):
    """根据文件状态和路径推断 commit type"""
    statuses = [s for s, _ in files_with_status]
    paths = [p for _, p in files_with_status]

    # 全是测试文件
    if all("_test." in p or "/test" in p or "test_" in os.path.basename(p) for p in paths):
        return "test"

    # 全是文档
    if all(p.endswith(".md") or p.startswith("docs/") for p in paths):
        return "docs"

    # 全是删除
    if all(s == "D" for s in statuses):
        return "chore"

    # 全是新文件
    if all(s == "??" or s == "A" for s in statuses):
        return "feat"

    # 配置文件
    config_patterns = [".yml", ".yaml", ".json", ".toml", ".env", ".gitignore"]
    if all(any(p.endswith(ext) for ext in config_patterns) for p in paths):
        return "chore"

    # 混合 → 默认 feat（有新文件）或 refactor（纯修改）
    has_new = any(s in ("??", "A") for s in statuses)
    return "feat" if has_new else "refactor"


def infer_scope(group_name, files_with_status):
    """推断 commit scope"""
    paths = [p for _, p in files_with_status]

    # 尝试找更精确的子目录
    if group_name in ("agent", "api", "internal", "llm"):
        subdirs = set()
        for p in paths:
            parts = p.split("/")
            if len(parts) >= 2:
                subdirs.add(parts[1])
        if len(subdirs) == 1:
            return f"{group_name}/{subdirs.pop()}"

    return group_name


def generate_commit_message(group_name, files_with_status):
    """自动生成 commit message"""
    commit_type = infer_commit_type(files_with_status)
    scope = infer_scope(group_name, files_with_status)
    paths = [p for _, p in files_with_status]

    # 生成描述
    basenames = []
    for p in paths:
        name = os.path.basename(p.rstrip("/"))
        # 去掉常见扩展名
        for ext in (".go", ".md", ".json", ".yaml", ".yml", ".toml", ".jsonl"):
            if name.endswith(ext):
                name = name[:-len(ext)]
                break
        if name:
            basenames.append(name)
    # 去重保序
    seen = set()
    unique = []
    for b in basenames:
        if b not in seen:
            seen.add(b)
            unique.append(b)
    basenames = unique

    if len(basenames) <= 3:
        desc = "/".join(basenames)
    elif len(basenames) <= 6:
        desc = f"{', '.join(basenames[:3])} 等 {len(basenames)} 文件"
    else:
        desc = f"{len(basenames)} 文件"

    return f"{commit_type}({scope}): {desc}"


def build_plan(excludes=None):
    """构建分组计划"""
    excludes = excludes or DEFAULT_EXCLUDES
    changed = get_changed_files()
    if not changed:
        print("没有检测到任何变更。")
        sys.exit(0)

    groups = defaultdict(list)
    excluded_files = []

    for status, filepath in changed:
        if should_exclude(filepath, excludes):
            excluded_files.append(filepath)
            continue
        group = infer_group(filepath)
        groups[group].append((status, filepath))

    # 构建计划
    plan_groups = []
    for group_name in sorted(groups.keys()):
        files_with_status = groups[group_name]
        message = generate_commit_message(group_name, files_with_status)
        plan_groups.append({
            "name": group_name,
            "message": message,
            "files": [p for _, p in files_with_status],
            "statuses": [s for s, _ in files_with_status],
        })

    timestamp = datetime.now().strftime("%Y%m%d-%H%M")
    plan = {
        "branch": f"batch/{timestamp}",
        "target": "master",
        "groups": plan_groups,
        "excluded": excluded_files,
    }
    return plan


def execute_plan(plan, dry_run=False):
    """执行分组计划：创建分支→分批提交→合并→清理"""
    branch = plan["branch"]
    target = plan["target"]
    groups = plan["groups"]

    if not groups:
        print("计划中没有任何分组，跳过。")
        return

    print(f"\n{'='*50}")
    print(f"  分批提交: {len(groups)} 组 → {target}")
    print(f"  临时分支: {branch}")
    print(f"{'='*50}\n")

    # 1. 创建临时分支
    print(f"[1/4] 创建临时分支 {branch}")
    run_git("checkout", "-b", branch, dry_run=dry_run)

    # 2. 分批提交
    print(f"\n[2/4] 分批提交 ({len(groups)} 组)")
    for i, group in enumerate(groups, 1):
        name = group["name"]
        message = group["message"]
        files = group["files"]
        statuses = group.get("statuses", ["M"] * len(files))

        print(f"\n  ── 第 {i}/{len(groups)} 组: {name} ({len(files)} 文件)")
        print(f"     message: {message}")

        for f in files:
            print(f"     + {f}")

        # 处理删除的文件和其他文件
        for status, filepath in zip(statuses, files):
            if status == "D":
                run_git("rm", filepath, dry_run=dry_run, check=False)
            else:
                run_git("add", filepath, dry_run=dry_run)

        run_git("commit", "-m", message, dry_run=dry_run)

    # 3. 合并到目标分支
    print(f"\n[3/4] 合并到 {target}")
    run_git("checkout", target, dry_run=dry_run)

    merge_msg = f"merge: 分批提交 — {', '.join(g['name'] for g in groups)}"
    run_git("merge", "--no-ff", branch, "-m", merge_msg, dry_run=dry_run)

    # 4. 清理临时分支
    print(f"\n[4/4] 清理临时分支 {branch}")
    run_git("branch", "-d", branch, dry_run=dry_run)

    print(f"\n{'='*50}")
    print(f"  完成! {len(groups)} 组提交已合并到 {target}")
    print(f"{'='*50}\n")


def print_plan(plan):
    """打印计划摘要"""
    groups = plan["groups"]
    excluded = plan.get("excluded", [])

    print(f"\n分支: {plan['branch']}")
    print(f"目标: {plan['target']}")
    print(f"分组: {len(groups)} 组\n")

    total_files = 0
    for i, g in enumerate(groups, 1):
        files = g["files"]
        total_files += len(files)
        print(f"  [{i}] {g['name']} ({len(files)} 文件)")
        print(f"      message: {g['message']}")
        for f in files:
            print(f"        - {f}")
        print()

    if excluded:
        print(f"  排除: {', '.join(excluded)}")

    print(f"  总计: {total_files} 文件, {len(groups)} 次提交\n")


# ── CLI ───────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Git 分批提交自动化")
    sub = parser.add_subparsers(dest="command", required=True)

    # plan
    p_plan = sub.add_parser("plan", help="查看分组计划")
    p_plan.add_argument("--output", "-o", help="保存计划到 JSON 文件")
    p_plan.add_argument("--exclude", action="append", default=[], help="额外排除模式")
    p_plan.add_argument("--target", default="master", help="目标分支 (默认: master)")

    # execute
    p_exec = sub.add_parser("execute", help="从计划文件执行")
    p_exec.add_argument("--plan", required=True, help="计划 JSON 文件路径")
    p_exec.add_argument("--dry-run", action="store_true", help="只打印命令")

    # auto
    p_auto = sub.add_parser("auto", help="自动分组并执行全部")
    p_auto.add_argument("--target", default="master", help="目标分支 (默认: master)")
    p_auto.add_argument("--exclude", action="append", default=[], help="额外排除模式")
    p_auto.add_argument("--dry-run", action="store_true", help="只打印命令")
    p_auto.add_argument("--branch", help="自定义临时分支名")

    args = parser.parse_args()

    if args.command == "plan":
        excludes = DEFAULT_EXCLUDES + args.exclude
        plan = build_plan(excludes)
        plan["target"] = args.target
        print_plan(plan)
        if args.output:
            with open(args.output, "w", encoding="utf-8") as f:
                json.dump(plan, f, ensure_ascii=False, indent=2)
            print(f"计划已保存到 {args.output}")

    elif args.command == "execute":
        with open(args.plan, "r", encoding="utf-8") as f:
            plan = json.load(f)
        print_plan(plan)
        execute_plan(plan, dry_run=args.dry_run)

    elif args.command == "auto":
        excludes = DEFAULT_EXCLUDES + args.exclude
        plan = build_plan(excludes)
        plan["target"] = args.target
        if args.branch:
            plan["branch"] = args.branch
        print_plan(plan)
        execute_plan(plan, dry_run=args.dry_run)


if __name__ == "__main__":
    main()
