#!/usr/bin/env python3
"""Skill-local wrapper for scripts/refactor_plan_guard.py."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Run the repo refactor plan guard from the skill wrapper."
    )
    parser.add_argument("cmd", choices=["lint", "report", "gate", "autofix"])
    parser.add_argument("--root", default="docs/重构计划")
    parser.add_argument("--target", default="all")
    args = parser.parse_args()

    skill_script = Path(__file__).resolve()
    repo_root = skill_script.parents[4]
    guard_script = repo_root / "scripts" / "refactor_plan_guard.py"

    cmd = [
        sys.executable,
        str(guard_script),
        args.cmd,
        "--root",
        args.root,
        "--target",
        args.target,
    ]
    completed = subprocess.run(cmd, cwd=repo_root)
    return completed.returncode


if __name__ == "__main__":
    raise SystemExit(main())
