# Closed-Loop Agent Regression Guards

This document preserves the long-lived regression constraints for the default
single-agent closed-loop execution path after the one-off refactor plans under
`docs/重构计划/` are cleaned up.

## Required Guardrails

- `BaseAgent.Execute(...)` 不得直接回退为单步 ReAct 主链
- `reasoningRegistry` 已注入时，默认主链必须消费模式选择结果
- `Plan/Observe` 必须出现在默认闭环阶段流转中
- `team.ExecutionModeLoop` 不得成为单 Agent 默认执行旁路
- 增加默认闭环执行链测试：简单任务、工具任务、失败重规划、反思修正、人工升级
- SSE 新增统一 `status` 事件并稳定输出 `reasoning_mode_selected/completion_judge_decision/loop_stopped`

## Validation Stage

The default closed-loop stages must include a validation stage / validation
acceptance gate before a task can be marked solved.

## Acceptance Criteria

Completion cannot rely on non-empty output alone. The default judge must require
acceptance criteria evidence and refuse to mark a task solved when verification
is still pending.

## Tool Verification

Tool-backed tasks must record tool verification status explicitly and keep
`tool_verification` as part of the completion evidence when external effects or
lookups are required.

## Task-Level Loop Budget

The task-level loop budget remains independent from reflection budgets.
`max_loop_iterations` and `top-level loop budget` must continue to constrain the
top-level closed loop directly.

## Non-Empty Output Is Not Enough

“非空输出” 不能直接 solved. The completion judge must reject a plain text
response without validation or acceptance evidence when the goal requires it.
