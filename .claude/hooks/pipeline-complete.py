#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Pipeline Completion Hook — SubagentStop for dispatch/team-lead

Triggers when the top-level orchestrator (dispatch or team-lead) stops.
Checks if all pipeline phases are complete, then outputs instructions
for the parent agent to execute the finalization chain:
  1. finish-work check
  2. git batch commit
  3. task archive

Trigger: SubagentStop (matcher: dispatch or team-lead)

Decision logic:
  - Read .current-task → task.json
  - If current_phase >= max phase AND status == "in_progress":
    → Output finalization instructions as "block" with reason
  - Otherwise: allow stop normally
"""

# IMPORTANT: Suppress all warnings FIRST
import warnings
warnings.filterwarnings("ignore")

import json
import os
import sys
from pathlib import Path

# IMPORTANT: Force stdout to use UTF-8 on Windows
if sys.platform == "win32":
    import io as _io
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    elif hasattr(sys.stdout, "detach"):
        sys.stdout = _io.TextIOWrapper(
            sys.stdout.detach(), encoding="utf-8", errors="replace"
        )

# =============================================================================
# Configuration
# =============================================================================

DIR_WORKFLOW = ".trellis"
FILE_CURRENT_TASK = ".current-task"
FILE_TASK_JSON = "task.json"

# Agents that trigger this hook
TARGET_AGENTS = {"dispatch", "team-lead"}

# State file to prevent double-triggering
STATE_FILE = ".trellis/.pipeline-complete-state.json"


def find_repo_root(start_path: str) -> str | None:
    current = Path(start_path).resolve()
    while current != current.parent:
        if (current / ".git").exists():
            return str(current)
        current = current.parent
    return None


def get_current_task(repo_root: str) -> str | None:
    path = os.path.join(repo_root, DIR_WORKFLOW, FILE_CURRENT_TASK)
    if not os.path.exists(path):
        return None
    try:
        with open(path, "r", encoding="utf-8") as f:
            content = f.read().strip()
            return content if content else None
    except Exception:
        return None


def read_task_json(repo_root: str, task_dir: str) -> dict | None:
    path = os.path.join(repo_root, task_dir, FILE_TASK_JSON)
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return None


def is_already_finalized(repo_root: str, task_dir: str) -> bool:
    """Check if we already triggered finalization for this task."""
    state_path = os.path.join(repo_root, STATE_FILE)
    if not os.path.exists(state_path):
        return False
    try:
        with open(state_path, "r", encoding="utf-8") as f:
            state = json.load(f)
        return state.get("task") == task_dir and state.get("finalized", False)
    except Exception:
        return False


def mark_finalized(repo_root: str, task_dir: str) -> None:
    """Mark task as finalized to prevent double-triggering."""
    state_path = os.path.join(repo_root, STATE_FILE)
    try:
        os.makedirs(os.path.dirname(state_path), exist_ok=True)
        with open(state_path, "w", encoding="utf-8") as f:
            json.dump(
                {"task": task_dir, "finalized": True},
                f,
                ensure_ascii=False,
            )
    except Exception:
        pass


def is_pipeline_complete(task_data: dict) -> bool:
    """
    Check if all pipeline phases have been executed.

    Logic: current_phase >= max phase number in next_action,
    AND the last action is NOT "create-pr" (that's handled separately).
    """
    next_actions = task_data.get("next_action", [])
    if not next_actions:
        return False

    current_phase = task_data.get("current_phase", 0)

    # Find the max phase that is NOT create-pr
    # (create-pr is a script action, not a subagent phase)
    max_code_phase = 0
    for action in next_actions:
        action_name = action.get("action", "")
        phase_num = action.get("phase", 0)
        if action_name != "create-pr" and phase_num > max_code_phase:
            max_code_phase = phase_num

    return current_phase >= max_code_phase and max_code_phase > 0


def get_task_name(task_dir: str) -> str:
    """Extract task name from task directory path."""
    return Path(task_dir).name


def main():
    try:
        input_data = json.load(sys.stdin)
    except json.JSONDecodeError:
        sys.exit(0)

    hook_event = input_data.get("hook_event_name", "")
    if hook_event != "SubagentStop":
        sys.exit(0)

    subagent_type = input_data.get("subagent_type", "")
    if subagent_type not in TARGET_AGENTS:
        sys.exit(0)

    cwd = input_data.get("cwd", os.getcwd())
    repo_root = find_repo_root(cwd)
    if not repo_root:
        sys.exit(0)

    task_dir = get_current_task(repo_root)
    if not task_dir:
        sys.exit(0)

    # Don't double-trigger
    if is_already_finalized(repo_root, task_dir):
        sys.exit(0)

    task_data = read_task_json(repo_root, task_dir)
    if not task_data:
        sys.exit(0)

    # Only trigger if pipeline is complete
    if not is_pipeline_complete(task_data):
        sys.exit(0)

    # Mark as finalized before outputting instructions
    mark_finalized(repo_root, task_dir)

    task_name = get_task_name(task_dir)

    # Block the stop and inject finalization instructions
    output = {
        "decision": "block",
        "reason": f"""Pipeline phases complete. Execute the finalization chain:

## Step 1: Git Batch Commit

Run the batch commit script to commit all changes:

```bash
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target master --dry-run
```

Review the plan output. If it looks correct, run without --dry-run:

```bash
python3 .claude/skills/git-batch-commit/scripts/batch_commit.py auto --target master
```

## Step 2: Archive Task

```bash
python3 ./.trellis/scripts/task.py archive {task_name}
```

## Step 3: Report Completion

Report to the user:
- What was implemented (from prd.md)
- Files changed
- Commit(s) created
- Task archived

After completing all steps, you may stop.""",
    }

    print(json.dumps(output, ensure_ascii=False))
    sys.exit(0)


if __name__ == "__main__":
    main()
