#!/usr/bin/env python3
"""
Git 分批提交自动化脚本（通用版）

自动分析 git 变更 → 按模块智能分组 → 创建临时分支 → 分批提交 → 合并 → 清理。
支持任意语言/框架的项目，自动检测依赖文件、测试文件、文档等。

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
    python3 batch_commit.py auto --target master --exclude "*.bin" --exclude "dist/"

    # 自定义分支名
    python3 batch_commit.py auto --target main --branch "feat/my-changes"
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


# ── 通用排除模式 ─────────────────────────────────────────

DEFAULT_EXCLUDES = [
    # 编译产物
    "*.exe", "*.dll", "*.so", "*.dylib", "*.o", "*.a",
    # 打包产物
    "*.tar.gz", "*.zip", "*.jar", "*.war",
    # 目录
    "vendor/", "node_modules/", "dist/", "build/", "__pycache__/",
    ".next/", ".nuxt/", "target/",
]


# ── 依赖文件识别（语言无关）──────────────────────────────

DEPENDENCY_FILES = {
    # Go
    "go.mod", "go.sum",
    # Node.js
    "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lockb",
    # Python
    "requirements.txt", "Pipfile", "Pipfile.lock", "pyproject.toml", "poetry.lock",
    "uv.lock", "setup.py", "setup.cfg",
    # Rust
    "Cargo.toml", "Cargo.lock",
    # Java/Kotlin
    "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
    # Ruby
    "Gemfile", "Gemfile.lock",
    # PHP
    "composer.json", "composer.lock",
    # .NET
    "*.csproj", "*.sln", "packages.config", "Directory.Build.props",
}


# ── 测试文件模式（语言无关）──────────────────────────────

TEST_PATTERNS = [
    # Go
    "_test.go",
    # JS/TS
    ".test.ts", ".test.tsx", ".test.js", ".test.jsx",
    ".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx",
    # Python
    "test_", "_test.py",
    # Java
    "Test.java", "Tests.java",
    # Rust
    # (Rust tests are inline, detected by directory)
]

TEST_DIRS = {"test/", "tests/", "__tests__/", "spec/", "testutil/", "testdata/"}


# ── 文档文件模式 ─────────────────────────────────────────

DOC_EXTENSIONS = {".md", ".rst", ".txt", ".adoc"}
DOC_DIRS = {"docs/", "doc/", "documentation/"}


# ── Git 操作 ─────────────────────────────────────────────

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


def detect_default_branch():
    """自动检测仓库的默认分支（main/master/develop 等）

    检测顺序：
    1. remote HEAD 指向（最可靠）
    2. 常见分支名存在性检查
    3. 当前分支作为 fallback
    """
    # 方法 1: 从 remote HEAD 获取
    try:
        ref = run_git("symbolic-ref", "refs/remotes/origin/HEAD", check=False)
        if ref:
            # refs/remotes/origin/main → main
            return ref.split("/")[-1]
    except Exception:
        pass

    # 方法 2: 检查常见默认分支名
    try:
        branches = run_git("branch", "--list", raw=True)
        branch_names = {b.strip().lstrip("* ") for b in branches.splitlines() if b.strip()}
        for candidate in ("main", "master", "develop"):
            if candidate in branch_names:
                return candidate
    except Exception:
        pass

    # 方法 3: 当前分支
    try:
        current = run_git("branch", "--show-current")
        if current:
            return current
    except Exception:
        pass

    return "master"


def get_changed_files():
    """解析 git status --porcelain，返回 [(status, filepath)] 列表"""
    output = run_git("status", "--porcelain", raw=True)
    if not output:
        return []
    files = []
    for line in output.splitlines():
        if len(line) < 4:
            continue
        status = line[:2].strip()
        path = line[3:]
        if " -> " in path:
            path = path.split(" -> ")[1]
        files.append((status, path))
    return files


def detect_binary_products():
    """自动检测项目根目录下可能的编译产物（无扩展名的可执行文件）"""
    products = []
    for entry in os.listdir("."):
        if os.path.isfile(entry) and "." not in entry:
            # 检查是否是可执行文件（非文本）
            try:
                with open(entry, "rb") as f:
                    chunk = f.read(512)
                    # ELF, Mach-O, PE 魔数检测
                    if chunk[:4] in (b"\x7fELF", b"\xfe\xed\xfa\xce", b"\xfe\xed\xfa\xcf",
                                     b"\xce\xfa\xed\xfe", b"\xcf\xfa\xed\xfe", b"MZ\x90\x00"):
                        products.append(entry)
            except (OSError, PermissionError):
                pass
    return products


# ── 排除逻辑 ─────────────────────────────────────────────

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


# ── 智能分组 ─────────────────────────────────────────────

def is_dependency_file(filepath):
    """判断是否为依赖管理文件"""
    basename = os.path.basename(filepath)
    if basename in DEPENDENCY_FILES:
        return True
    # 通配符匹配（如 *.csproj）
    for pattern in DEPENDENCY_FILES:
        if "*" in pattern and fnmatch(basename, pattern):
            return True
    return False


def is_test_file(filepath):
    """判断是否为测试文件"""
    basename = os.path.basename(filepath)
    for pattern in TEST_PATTERNS:
        if pattern.startswith("test_"):
            if basename.startswith("test_"):
                return True
        elif basename.endswith(pattern):
            return True
    # 测试目录
    for d in TEST_DIRS:
        if f"/{d}" in filepath or filepath.startswith(d):
            return True
    return False


def is_doc_file(filepath):
    """判断是否为文档文件"""
    _, ext = os.path.splitext(filepath)
    if ext.lower() in DOC_EXTENSIONS:
        return True
    for d in DOC_DIRS:
        if filepath.startswith(d):
            return True
    return False


def is_ci_config(filepath):
    """判断是否为 CI/CD 配置文件"""
    ci_patterns = [
        ".github/workflows/", ".gitlab-ci", "Jenkinsfile",
        ".circleci/", ".travis.yml", "azure-pipelines",
        "Dockerfile", "docker-compose", ".dockerignore",
        "Makefile", "Taskfile", "justfile",
    ]
    for p in ci_patterns:
        if filepath.startswith(p) or os.path.basename(filepath).startswith(p):
            return True
    return False


def infer_group(filepath):
    """根据文件路径推断分组名（通用逻辑，不依赖硬编码目录映射）"""
    basename = os.path.basename(filepath)

    # 1. 依赖文件 → deps 组
    if is_dependency_file(filepath):
        return "deps"

    # 2. CI/CD 配置 → ci 组
    if is_ci_config(filepath):
        return "ci"

    # 3. 按顶层目录分组
    parts = filepath.split("/")
    top_dir = parts[0] if parts else filepath

    # 隐藏目录保留原名（如 .trellis, .github）
    if top_dir.startswith("."):
        return top_dir.lstrip(".")  # .trellis → trellis, .github → github

    # 根目录文件
    if len(parts) == 1:
        return "root"

    return top_dir


# ── Commit Type 推断 ─────────────────────────────────────

def infer_commit_type(files_with_status):
    """根据文件内容和状态推断 commit type"""
    statuses = [s for s, _ in files_with_status]
    paths = [p for _, p in files_with_status]

    # 全是测试文件
    if all(is_test_file(p) for p in paths):
        return "test"

    # 全是文档
    if all(is_doc_file(p) for p in paths):
        return "docs"

    # 全是删除
    if all(s == "D" for s in statuses):
        return "chore"

    # 全是新文件
    if all(s in ("??", "A") for s in statuses):
        return "feat"

    # CI/CD 配置
    if all(is_ci_config(p) for p in paths):
        return "ci"

    # 依赖文件
    if all(is_dependency_file(p) for p in paths):
        return "chore"

    # 配置文件
    config_exts = {".yml", ".yaml", ".json", ".toml", ".env", ".gitignore", ".editorconfig"}
    if all(os.path.splitext(p)[1] in config_exts for p in paths):
        return "chore"

    # 混合：有新文件 → feat，纯修改 → refactor
    has_new = any(s in ("??", "A") for s in statuses)
    return "feat" if has_new else "refactor"


# ── Commit Scope 推断 ────────────────────────────────────

def infer_scope(group_name, files_with_status):
    """推断 commit scope，尝试找到更精确的子目录"""
    paths = [p for _, p in files_with_status]

    # 收集所有二级目录
    subdirs = set()
    for p in paths:
        parts = p.split("/")
        if len(parts) >= 2:
            subdirs.add(parts[1])

    # 如果所有文件都在同一个子目录下，使用更精确的 scope
    if len(subdirs) == 1:
        sub = subdirs.pop()
        return f"{group_name}/{sub}"

    return group_name


# ── Commit Message 生成 ──────────────────────────────────

def summarize_changes(files_with_status):
    """分析文件变更，生成语义化描述"""
    statuses = [s for s, _ in files_with_status]
    paths = [p for _, p in files_with_status]

    added = [p for s, p in files_with_status if s in ("??", "A")]
    modified = [p for s, p in files_with_status if s == "M"]
    deleted = [p for s, p in files_with_status if s == "D"]

    parts = []

    # 按文件类型分类描述
    test_files = [p for p in paths if is_test_file(p)]
    doc_files = [p for p in paths if is_doc_file(p)]
    source_files = [p for p in paths if not is_test_file(p) and not is_doc_file(p)]

    if source_files:
        names = _extract_meaningful_names(source_files)
        if added and not modified and not deleted:
            parts.append(f"新增 {', '.join(names[:4])}")
        elif deleted and not added and not modified:
            parts.append(f"移除 {', '.join(names[:4])}")
        elif modified and not added:
            parts.append(f"更新 {', '.join(names[:4])}")
        else:
            parts.append(', '.join(names[:4]))
        if len(names) > 4:
            parts[-1] += f" 等 {len(names)} 个文件"

    if test_files and not all(is_test_file(p) for p in paths):
        test_names = _extract_meaningful_names(test_files)
        parts.append(f"测试: {', '.join(test_names[:2])}")

    if doc_files and not all(is_doc_file(p) for p in paths):
        parts.append(f"文档更新 {len(doc_files)} 个")

    return " + ".join(parts) if parts else f"{len(paths)} 个文件"


def _extract_meaningful_names(paths):
    """从文件路径中提取有意义的名称（去扩展名、去重、保序）"""
    names = []
    seen = set()
    for p in paths:
        name = os.path.basename(p.rstrip("/"))
        # 去掉常见扩展名
        stem, ext = os.path.splitext(name)
        if ext in (".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java",
                    ".md", ".json", ".yaml", ".yml", ".toml", ".jsonl", ".sql"):
            name = stem
        if name and name not in seen:
            seen.add(name)
            names.append(name)
    return names


def generate_commit_message(group_name, files_with_status):
    """生成语义化的 commit message"""
    commit_type = infer_commit_type(files_with_status)
    scope = infer_scope(group_name, files_with_status)
    description = summarize_changes(files_with_status)

    return f"{commit_type}({scope}): {description}"


# ── 计划构建 ─────────────────────────────────────────────

def build_plan(excludes=None, target=None):
    """构建分组计划"""
    if excludes is None:
        excludes = list(DEFAULT_EXCLUDES)
    if target is None:
        target = detect_default_branch()

    # 自动检测编译产物并加入排除列表
    binary_products = detect_binary_products()
    excludes.extend(binary_products)

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
        "target": target,
        "groups": plan_groups,
        "excluded": excluded_files,
    }
    return plan


# ── 计划执行 ─────────────────────────────────────────────

def execute_plan(plan, dry_run=False):
    """执行分组计划：创建分支 → 分批提交 → 合并 → 清理"""
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


# ── CLI ──────────────────────────────────────────────────

def main():
    default_target = detect_default_branch()

    parser = argparse.ArgumentParser(
        description="Git 分批提交自动化 — 按模块智能分组、语义化 commit message",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=f"""
示例:
  %(prog)s auto                              # 自动分组并提交到 {default_target}
  %(prog)s auto --target main --dry-run      # 预览模式
  %(prog)s plan --output plan.json           # 导出计划供手动编辑
  %(prog)s execute --plan plan.json          # 从计划文件执行
        """,
    )
    sub = parser.add_subparsers(dest="command", required=True)

    # plan
    p_plan = sub.add_parser("plan", help="查看分组计划（不执行）")
    p_plan.add_argument("--output", "-o", help="保存计划到 JSON 文件")
    p_plan.add_argument("--exclude", action="append", default=[], help="额外排除模式（可多次使用）")
    p_plan.add_argument("--target", default=default_target, help=f"目标分支 (自动检测: {default_target})")

    # execute
    p_exec = sub.add_parser("execute", help="从计划文件执行提交")
    p_exec.add_argument("--plan", required=True, help="计划 JSON 文件路径")
    p_exec.add_argument("--dry-run", action="store_true", help="只打印命令，不实际执行")

    # auto
    p_auto = sub.add_parser("auto", help="自动分组并执行全部提交+合并")
    p_auto.add_argument("--target", default=default_target, help=f"目标分支 (自动检测: {default_target})")
    p_auto.add_argument("--exclude", action="append", default=[], help="额外排除模式（可多次使用）")
    p_auto.add_argument("--dry-run", action="store_true", help="只打印命令，不实际执行")
    p_auto.add_argument("--branch", help="自定义临时分支名 (默认: batch/YYYYMMDD-HHMM)")

    args = parser.parse_args()

    if args.command == "plan":
        excludes = list(DEFAULT_EXCLUDES) + args.exclude
        plan = build_plan(excludes, target=args.target)
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
        excludes = list(DEFAULT_EXCLUDES) + args.exclude
        plan = build_plan(excludes, target=args.target)
        if args.branch:
            plan["branch"] = args.branch
        print_plan(plan)
        execute_plan(plan, dry_run=args.dry_run)


if __name__ == "__main__":
    main()
