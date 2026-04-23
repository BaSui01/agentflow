# Agent 模块测试基线（2026-04-22）

## 执行命令

```bash
go test ./agent/... -count=1 -timeout=180s
```

## 总览

- 测试包总数: 41
- 通过包数（ok）: 38
- 失败包数（FAIL）: 0
- 无测试文件包数（?）: 3

## 状态

✅ **全部通过** — 作为重构基线，重构后必须保持同级或更好。

## 测试结果详情

```
ok  	github.com/BaSui01/agentflow/agent	19.916s
ok  	github.com/BaSui01/agentflow/agent/artifacts	2.213s
ok  	github.com/BaSui01/agentflow/agent/collaboration	0.680s
ok  	github.com/BaSui01/agentflow/agent/context	1.233s
ok  	github.com/BaSui01/agentflow/agent/conversation	1.113s
?   	github.com/BaSui01/agentflow/agent/core	[no test files]
ok  	github.com/BaSui01/agentflow/agent/crews	1.257s
ok  	github.com/BaSui01/agentflow/agent/declarative	1.340s
ok  	github.com/BaSui01/agentflow/agent/deliberation	1.122s
ok  	github.com/BaSui01/agentflow/agent/deployment	2.969s
ok  	github.com/BaSui01/agentflow/agent/discovery	0.522s
ok  	github.com/BaSui01/agentflow/agent/evaluation	13.479s
ok  	github.com/BaSui01/agentflow/agent/execution	7.298s
ok  	github.com/BaSui01/agentflow/agent/federation	0.926s
?   	github.com/BaSui01/agentflow/agent/guardcore	[no test files]
ok  	github.com/BaSui01/agentflow/agent/guardrails	2.211s
ok  	github.com/BaSui01/agentflow/agent/handoff	1.304s
ok  	github.com/BaSui01/agentflow/agent/hierarchical	6.410s
ok  	github.com/BaSui01/agentflow/agent/hitl	1.159s
ok  	github.com/BaSui01/agentflow/agent/hosted	0.577s
ok  	github.com/BaSui01/agentflow/agent/k8s	2.030s
ok  	github.com/BaSui01/agentflow/agent/longrunning	5.318s
ok  	github.com/BaSui01/agentflow/agent/lsp	1.007s
ok  	github.com/BaSui01/agentflow/agent/memory	1.431s
ok  	github.com/BaSui01/agentflow/agent/memorycore	1.049s
ok  	github.com/BaSui01/agentflow/agent/multiagent	0.250s
ok  	github.com/BaSui01/agentflow/agent/observability	0.327s
ok  	github.com/BaSui01/agentflow/agent/orchestration	0.330s
ok  	github.com/BaSui01/agentflow/agent/persistence	1.095s
?   	github.com/BaSui01/agentflow/agent/persistence/mongodb	[no test files]
ok  	github.com/BaSui01/agentflow/agent/planner	1.226s
ok  	github.com/BaSui01/agentflow/agent/protocol/a2a	9.769s
ok  	github.com/BaSui01/agentflow/agent/protocol/mcp	0.385s
ok  	github.com/BaSui01/agentflow/agent/reasoning	0.292s
ok  	github.com/BaSui01/agentflow/agent/runtime	0.258s
ok  	github.com/BaSui01/agentflow/agent/skills	1.090s
ok  	github.com/BaSui01/agentflow/agent/streaming	2.273s
ok  	github.com/BaSui01/agentflow/agent/structured	1.525s
ok  	github.com/BaSui01/agentflow/agent/team	0.238s
ok  	github.com/BaSui01/agentflow/agent/teamadapter	0.247s
ok  	github.com/BaSui01/agentflow/agent/voice	0.926s
```
