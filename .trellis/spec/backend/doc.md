# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

This directory contains guidelines for backend development. Fill in each file with your project's specific conventions.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Module organization and file layout | To fill |
| [Database Guidelines](./database-guidelines.md) | ORM patterns, queries, migrations | To fill |
| [Error Handling](./error-handling.md) | Error types, handling strategies | To fill |
| [Quality Guidelines](./quality-guidelines.md) | Code standards, forbidden patterns | To fill |
| [Logging Guidelines](./logging-guidelines.md) | Structured logging, log levels | To fill |

---

## How to Fill These Guidelines

For each guideline file:

1. Document your project's **actual conventions** (not ideals)
2. Include **code examples** from your codebase
3. List **forbidden patterns** and why
4. Add **common mistakes** your team has made

The goal is to help AI assistants and new team members understand how YOUR project works.

---

@@@section:skill-cn-refactor-plan-governance
## Refactor Plan Governance Integration Guide

### Overview
This skill maintains and enforces Chinese refactor plans (`docs/重构计划/*.md`) with strict governance rules. It ensures plans follow TDD methodology, include verifiable completion criteria, and enforce single-track replacement (no compatibility code).

**Core capabilities:**
- Plan creation with mandatory task status (`- [ ]` / `- [x]`)
- TDD test strategy enforcement (Red-Green-Refactor)
- Progress tracking with verifiable evidence
- Gate checks before plan closure
- Architecture refactor support (package merge, unique entry point, directory governance)

### Project Adaptation

**When to use this skill:**
- Creating or reviewing refactor plans in `docs/重构计划/`
- Tracking refactor progress with strict completion criteria
- Architecture refactoring (package consolidation, entry point unification, root directory governance)
- Ensuring no compatibility code or dual implementations remain

**Key constraints from CLAUDE.md:**
- No compatibility code during development phase (强制单轨替换)
- Respect layer dependencies: `types/` → `llm/` → `agent/` → `workflow/` → `api/` → `cmd/`
- Maintain single entry point: `cmd/agentflow/main.go → internal/app/bootstrap → server_* → routes → handlers → domain`
- Synchronize architecture docs (README/ADR) and guards (`architecture_guard_test.go`, `scripts/arch_guard.ps1`)

### Usage Steps

#### 1. Create New Refactor Plan

```bash
# Use the appropriate template
# For general refactors:
cp .agents/skills/cn-refactor-plan-governance/references/重构计划模板.md docs/重构计划/<plan-name>-<YYYY-MM-DD>.md

# For architecture refactors (package merge, unique entry, directory governance):
cp .agents/skills/cn-refactor-plan-governance/references/架构重构计划模板.md docs/重构计划/<plan-name>-<YYYY-MM-DD>.md
```

#### 2. Validate Plan Format

```bash
# Strict format check (requires TDD and verifiable completion)
python scripts/refactor_plan_guard.py lint --target "<plan-file-name>" --require-tdd --require-verifiable-completion

# Or use skill wrapper
python .agents/skills/cn-refactor-plan-governance/scripts/run_guard.py lint --target "<plan-file-name>"
```

#### 3. Track Progress

```bash
# Generate progress report
python scripts/refactor_plan_guard.py report --target "<plan-file-name>" --require-tdd --require-verifiable-completion
```

#### 4. Gate Check Before Closure

```bash
# Run gate check (must pass before declaring "done")
python scripts/refactor_plan_guard.py gate --target "<plan-file-name>" --require-tdd --require-verifiable-completion

# Plan can only be closed when:
# - All tasks are [x] (no [ ] remaining)
# - All verification commands executed
# - All pass criteria met
```

### Hard Rules

1. **Task Status Required**: Every plan must contain `- [ ]` or `- [x]` task items
2. **Four Mandatory Sections**:
   - 执行状态总览 (Execution Status Overview)
   - 测试策略（TDD）or 测试计划（TDD）(Test Strategy/Plan with TDD)
   - 执行计划 or Phase (Execution Plan)
   - 完成定义 or DoD (Definition of Done)
3. **TDD Explicit Steps**:
   - 先写失败测试 (Write failing test first)
   - 最小实现让测试转绿 (Minimal implementation to pass)
   - 重构并回归验证 (Refactor and regression)
4. **Verifiable Evidence**: Each phase must include:
   - 验证命令 (Verification command)
   - 通过标准 (Pass criteria)
5. **No Premature Closure**: Cannot output "完成/停止/归档" if any `[ ]` remains or pass criteria unmet
6. **Single-Track Replacement**: No compatibility code or dual implementations allowed
7. **Architecture Refactor Requirements**: Must explicitly define:
   - Which responsibilities to keep/merge/sink/delete
   - Which directories/files to migrate
   - Which old implementations to remove
   - Where the unique entry point is
8. **No Abstract Statements**: Must specify concrete files, directories, entry points, guards, or docs (not just "optimize/unify/converge")

### Architecture Refactor Specifics

For "architecture unification", "too many packages", "duplicate features to merge", "root directory cleanup":

1. **Responsibility Matrix**: List current responsibilities and mark each as "keep/merge/sink/delete"
2. **Unique Entry Declaration**: Specify the single official entry point (e.g., `Builder`/`Factory`/`Registry`/runtime entry)
3. **Duplicate Semantics Inventory**: Identify duplicate semantics (construction, registration, orchestration, runtime bridge, protocol adaptation) and assign unique ownership
4. **Directory Governance**: For root/top-level directory governance, define classification, budget, or allowlist
5. **Concrete Deletion List**: List old entry names, old files, old paths, old guards (not just "delete old logic")
6. **Doc/Guard Sync**: Specify which README, ADR, architecture docs, `architecture_guard_test.go`, `scripts/arch_guard.ps1` need updates

### Plan Maintenance Actions

1. **Creating Plans**: Copy from templates in `./references/`
2. **Reviewing Plans**:
   - Flag conflicts (status vs code mismatch, wrong paths, incorrect guard declarations)
   - Check TDD implementation (failing test first, green condition defined, regression steps)
   - Check architecture refactor clarity (keep/merge/sink/delete/unique entry)
   - Provide specific revision items (file and line)
3. **Advancing Plans**:
   - Only change `[ ]` to `[x]` for completed items
   - Never delete incomplete items
   - Every `[x]` must have actual evidence (test/script/code path/command output)
4. **Judging Completion**:
   - Check all verification commands executed
   - Check all pass criteria met line by line
   - Only complete when all pass

### Caveats

- **Project-specific**: This skill is tailored for AgentFlow's architecture rules (see CLAUDE.md)
- **Chinese plans only**: Designed for `docs/重构计划/` Chinese documentation
- **Strict gates**: Cannot bypass gate checks or declare completion prematurely
- **No compatibility code**: Enforces single-track replacement per project rules
- **Evidence required**: Every completion claim must have verifiable evidence

### Multi-Agent Parallel Execution

When a refactor plan contains multiple independent tasks, you can use multi-agent parallel execution to improve efficiency.

**Two approaches:**

1. **Trellis Multi-Agent Pipeline (Recommended)**:
   - Use `.trellis/scripts/multi_agent/plan.py` to create tasks
   - Use `.trellis/scripts/multi_agent/start.py` to launch parallel agents
   - Use `.trellis/scripts/multi_agent/status.py` to monitor progress
   - Each agent works in an isolated worktree branch

2. **Manual Parallel Invocation**:
   - Identify independent tasks from the refactor plan
   - Create separate prompts for each task
   - Launch multiple Agent tool calls in a single message
   - Verify results and update plan status

**Key principles:**
- ✅ Tasks must be truly independent (no shared files or dependencies)
- ✅ Use file-level isolation to avoid conflicts
- ✅ Track progress in the refactor plan
- ✅ Verify all results before marking tasks complete

See `examples/skills/cn-refactor-plan-governance/multi-agent-parallel-execution.md` for detailed guide.

### Reference Examples

See `examples/skills/cn-refactor-plan-governance/` for:
- Template usage examples
- Guard script integration
- TDD test strategy patterns
- Architecture refactor responsibility matrices
- Multi-agent parallel execution guide

@@@/section:skill-cn-refactor-plan-governance

---

**Language**: All documentation should be written in **English**.
