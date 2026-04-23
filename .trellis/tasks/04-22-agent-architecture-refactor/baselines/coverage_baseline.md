# Agent 模块覆盖率基线（2026-04-22）

## 执行命令

```bash
go test ./agent/... -count=1 -cover -timeout=300s
```

## 总览

- 有覆盖率数据的包数: 40
- 平均覆盖率: 76.9%

## 已知问题

⚠️ **Flaky Test**: `agent/streaming/TestBidirectionalStream_Send_BufferFull` 在覆盖率模式下失败（插桩影响时序），但在普通测试模式下通过。这是 pre-existing 问题，不影响基线有效性。

## 各包覆盖率

| 包 | 覆盖率 |
|---|--------|
| agent | 81.8% |
| agent/artifacts | 88.6% |
| agent/collaboration | 84.2% |
| agent/context | 69.6% |
| agent/conversation | 88.3% |
| coverage: | statements |
| agent/crews | 88.1% |
| agent/declarative | 100.0% |
| agent/deliberation | 84.5% |
| agent/deployment | 86.4% |
| agent/discovery | 79.3% |
| agent/evaluation | 88.7% |
| agent/execution | 89.8% |
| agent/federation | 82.6% |
| coverage: | statements |
| agent/guardrails | 86.2% |
| agent/handoff | 87.4% |
| agent/hierarchical | 97.7% |
| agent/hitl | 86.2% |
| agent/hosted | 61.4% |
| agent/k8s | 87.5% |
| agent/longrunning | 85.4% |
| agent/lsp | 82.2% |
| agent/memory | 78.9% |
| agent/memorycore | 94.2% |
| agent/multiagent | 79.2% |
| agent/observability | 72.0% |
| agent/orchestration | 77.4% |
| agent/persistence | 80.4% |
| coverage: | statements |
| agent/planner | 89.8% |
| agent/protocol/a2a | 84.5% |
| agent/protocol/mcp | 71.0% |
| agent/reasoning | 84.9% |
| agent/runtime | 77.3% |
| agent/skills | 84.2% |
| agent/structured | 81.3% |
| agent/team | 85.4% |
| agent/teamadapter | 58.1% |
| agent/voice | 91.0% |

## 状态

✅ **基线已固化** — 重构后覆盖率不得低于此基线（除非有合理解释）。
