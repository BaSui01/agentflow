# ADR-004: Multi-Agent Team Abstraction

## Status

Accepted

## Context

AgentFlow provides three independent multi-agent orchestration mechanisms:

1. **Collaboration** (`agent/collaboration/multiagent/`): Debate, Consensus, Pipeline, Broadcast, Network patterns
2. **Hierarchical** (`agent/collaboration/hierarchical/`): Supervisor decomposes tasks, workers execute in parallel
3. **Crew** (`agent/collaboration/team/crew.go`): Role-based task assignment with Sequential/Hierarchical/Consensus processes

Each mechanism has its own entry point, configuration, and execution model. This makes it difficult to:

- Switch between modes without rewriting calling code
- Compose modes within workflow DAG nodes
- Build higher-level orchestration (e.g., nested teams)

Additionally, the `ModeDeliberation` and `ModeFederation` entries in the mode registry were placeholder implementations that only executed the first agent.

## Decision

### 1. Unified Team Interface

Introduce a `Team` interface in `agent/execution/runtime/interfaces_runtime.go`:

```go
type Team interface {
    ID() string
    Members() []TeamMember
    Execute(ctx context.Context, task string, opts ...TeamOption) (*TeamResult, error)
}
```

Adapters in `agent/adapters/teamadapter/` wrap each existing mechanism to satisfy this interface.

### 2. Deliberation Mode

Replace the `primaryModeStrategy` placeholder for `ModeDeliberation` with a real implementation:

- Round 1: All agents produce independent drafts
- Round 2+: Each agent sees all outputs and self-reflects
- Convergence detection: early termination when outputs stabilize
- Final synthesis by first agent

### 3. SharedState

Provide a `SharedState` interface in `agent/collaboration/multiagent/shared_state.go` for agents to share intermediate results via a key-value store with `Watch` capability.

### 4. Workflow Bridge

`OrchestrationStep` in `workflow/steps/orchestration.go` implements `core.StepProtocol`, allowing multi-agent collaboration to be used as a DAG node via DSL `type: orchestration`.

## Consequences

- Calling code can use `Team` interface without knowing the underlying mechanism
- Workflow DAG can orchestrate multi-agent collaboration as a first-class node type
- Deliberation mode is now functional with convergence detection
- SharedState enables richer inter-agent communication beyond MessageHub
- The `agent/adapters/teamadapter` sub-package avoids circular imports between runtime-facing `Team` contracts and collaboration implementations
