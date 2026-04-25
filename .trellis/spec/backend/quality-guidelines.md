# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

<!--
Document your project's quality standards here.

Questions to answer:
- What patterns are forbidden?
- What linting rules do you enforce?
- What are your testing requirements?
- What code review standards apply?
-->

(To be filled by the team)

---

## Forbidden Patterns

<!-- Patterns that should never be used and why -->

(To be filled by the team)

---

## Required Patterns

<!-- Patterns that must always be used -->

(To be filled by the team)

---

## Testing Requirements

<!-- What level of testing is expected -->

(To be filled by the team)

---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)

---

## Scenario: Agent ModelOptions request-field contract

### 1. Scope / Trigger

- Trigger: adding provider-neutral model request fields that must be usable through the official Agent surface.
- Affected chain: `types.AgentConfig.Model -> types.ExecutionOptions.Model -> agent/adapters.DefaultChatRequestAdapter -> types.ChatRequest -> provider boundary`.
- Constraint: do not expose provider SDK request structs above `llm/providers/*`.

### 2. Signatures

- Public config surface: `types.ModelOptions` in `types/execution_options.go`.
- Runtime normalized view: `func (c AgentConfig) ExecutionOptions() ExecutionOptions`.
- Boundary adapter: `func (DefaultChatRequestAdapter) Build(options types.ExecutionOptions, messages []types.Message) (*types.ChatRequest, error)`.
- Catalog facts: `types.ModelDescriptor`, `types.ModelCatalog`, `types.NewModelCatalog`.

### 3. Contracts

- If a new provider-neutral request field is added to `types.ModelOptions`, update all of these in the same change:
	- `ModelOptions.clone()` for deep-copy behavior.
	- `AgentConfig.hasFormalMainFace()` so setting only the new field activates the formal main surface.
	- `mergeModelOptions(...)` so formal `AgentConfig.Model` overrides legacy-derived defaults.
	- `agent/adapters.DefaultChatRequestAdapter.Build(...)` so the field reaches `types.ChatRequest`.
	- tests in `types` and `agent/adapters` packages.
- Slice, map, and pointer fields must be deep-cloned at each boundary.
- Provider-specific validation and request rewriting stay in provider or compat profile code, not in handlers or runtime business code.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| `messages` empty in adapter | Return `types.ErrInputValidation` |
| only a new `ModelOptions` field is set | `hasFormalMainFace()` returns true and the field is merged |
| pointer field contains zero/false value | preserve it instead of dropping it |
| slice/map/pointer mutated after normalization | normalized request remains unchanged |

### 5. Good/Base/Bad Cases

#### Correct

```go
cfg := types.AgentConfig{
		Model: types.ModelOptions{
				Model:              "gpt-5.4",
				PreviousResponseID: "resp_prev_123",
				Include:            []string{"reasoning.encrypted_content"},
		},
}
req, err := adapters.NewDefaultChatRequestAdapter().Build(cfg.ExecutionOptions(), messages)
```

#### Wrong

```go
// Do not bypass the formal Agent surface from runtime or handlers.
req := &openai.ResponsesRequest{PreviousResponseID: "resp_prev_123"}
```

### 6. Tests Required

- `go test ./types -run "TestAgentConfigExecutionOptions|TestModelCatalog" -count=1`
- `go test ./agent/adapters -run TestDefaultChatRequestAdapter -count=1`
- Relevant architecture guard when the chain changes: `go test . -run "TestDependencyDirectionGuards|TestAgentExecutionOptionsArchitectureGuards" -count=1`

### 7. Wrong vs Correct

- Wrong: add a field only to `types.ChatRequest` or `api.ChatRequest` and expect Agent runtime users to access it.
- Correct: add provider-neutral fields to `types.ModelOptions`, normalize through `ExecutionOptions`, and lower once in `ChatRequestAdapter`.

---

## Scenario: Pending state timeout/cancel contract

### 1. Scope / Trigger

- Trigger: adding or changing async pending-state flows that can resolve, time out, or be cancelled.
- Affected examples: `agent/observability/hitl.InterruptManager`, approval managers, hosted-tool approval waits, long-running run checkpoints.
- Constraint: timeout and cancellation are different terminal states; cancellation must never be persisted as timeout.

### 2. Signatures

- Pending creation: `func (m *InterruptManager) CreateInterrupt(ctx context.Context, opts InterruptOptions) (*Response, error)`.
- Non-blocking pending creation: `func (m *InterruptManager) CreatePendingInterrupt(ctx context.Context, opts InterruptOptions) (*Interrupt, error)`.
- Cancellation: `func (m *InterruptManager) CancelInterrupt(ctx context.Context, interruptID string) error`.
- Status inspection: `func (m *InterruptManager) GetPendingInterrupts(workflowID string) []*Interrupt`.
- Persistence boundary: `InterruptStore.Update(ctx context.Context, interrupt *Interrupt) error`.

### 3. Contracts

- Only `context.DeadlineExceeded` may transition a pending item to timeout.
- `context.Canceled` and explicit cancel operations must transition to canceled, remove the item from the in-memory pending map, and persist `ResolvedAt`.
- If resolve response and cancellation context become ready together, the already-written response wins over a cancellation branch.
- Blocking wait APIs must not leave pending entries behind when parent context is cancelled.
- Tests for async pending flows must assert both in-memory cleanup and persisted terminal status.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| explicit cancel while pending | pending entry removed; store status `canceled`; `ResolvedAt` set |
| parent context cancelled | pending entry removed; store status `canceled`; caller receives `context.Canceled` |
| timeout expires | pending entry removed; store status `timeout`; caller receives timeout error |
| resolve wins before cancellation branch | caller receives response; store status `resolved` or `rejected` |

### 5. Good/Base/Bad Cases

#### Correct

```go
case <-pending.timeoutCtx.Done():
		if pending.timeoutCtx.Err() != context.DeadlineExceeded {
				_ = m.CancelInterrupt(context.Background(), pending.interrupt.ID)
				return nil, pending.timeoutCtx.Err()
		}
		m.handleTimeout(context.Background(), pending.interrupt)
```

#### Wrong

```go
case <-pending.timeoutCtx.Done():
		m.handleTimeout(context.Background(), pending.interrupt)
```

### 6. Tests Required

- `go test ./agent/observability/hitl -run 'TestCreatePendingInterrupt(Timeout|Cancel)CleansPendingAndPersistsStatus|TestCreateInterruptContextCanceled' -count=1`
- Race-sensitive path when resolve and cancel may compete: `go test -race ./agent/observability/hitl -run TestConcurrentResolveAndCancel -count=1`

---

## Scenario: Tool approval fingerprint coalescing contract

### 1. Scope / Trigger

- Trigger: changing hosted-tool approval, approval grants, or HITL interrupt creation behind `toolApprovalHandler.RequestApproval(...)`.
- Affected chain: `PermissionManager.CheckPermission(...) -> toolApprovalHandler.RequestApproval(...) -> hitl.InterruptManager.CreatePendingInterrupt(...) -> grant store/history store`.
- Constraint: one logical approval fingerprint may have only one active pending interrupt; concurrent duplicate requests must coalesce instead of creating competing approvals.

### 2. Signatures

- Approval request: `func (h *toolApprovalHandler) RequestApproval(ctx context.Context, permCtx *llmtools.PermissionContext, rule *llmtools.PermissionRule) (string, error)`.
- Approval status: `func (h *toolApprovalHandler) CheckApprovalStatus(ctx context.Context, approvalID string) (bool, error)`.
- Fingerprint: `func approvalFingerprint(permCtx *llmtools.PermissionContext, rule *llmtools.PermissionRule, scope string) string`.
- Grant persistence: `ToolApprovalGrantStore.Get/Put/Delete/List/CleanupExpired`.

### 3. Contracts

- `RequestApproval` must perform lookup, create, and remember for the same fingerprint under one critical section.
- A second request for a still-pending fingerprint must return the existing interrupt ID.
- If the stored interrupt is approved and a grant is still valid, the request must return `grant:<fingerprint>` instead of creating a new interrupt.
- If the stored interrupt is rejected, canceled, or timed out, the handler must forget that pending fingerprint and create a fresh interrupt.
- Approved grants expire from the grant store according to `ToolApprovalConfig.GrantTTL`.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| duplicate pending request | same interrupt ID returned; only one pending interrupt exists |
| concurrent duplicate requests | all callers receive the same interrupt ID |
| rejected approval | `CheckApprovalStatus` returns false; no grant is stored |
| timed-out prior approval | next request creates a fresh interrupt |
| approved grant TTL elapsed | grant store returns no active grant |

### 5. Good/Base/Bad Cases

#### Correct

```go
h.mu.Lock()
if existingID := h.lookupExistingApprovalLocked(ctx, key); existingID != "" {
		h.mu.Unlock()
		return existingID, nil
}
interrupt, err := h.manager.CreatePendingInterrupt(ctx, opts)
h.pending[key] = interrupt.ID
h.mu.Unlock()
```

#### Wrong

```go
if existingID := h.lookupExistingApproval(ctx, key); existingID != "" {
		return existingID, nil
}
interrupt, err := h.manager.CreatePendingInterrupt(ctx, opts)
h.rememberPending(key, interrupt.ID)
```

### 6. Tests Required

- `go test ./internal/app/bootstrap -run 'TestToolApprovalHandler_(ApprovalGrantExpiresByTTL|RejectedApprovalDoesNotCreateGrant|DuplicatePendingApprovalReusesInterrupt|TimedOutApprovalCreatesFreshInterrupt|ConcurrentDuplicateApprovalCoalesces)' -count=1`
- Race-sensitive duplicate approval path: `go test -race ./internal/app/bootstrap -run TestToolApprovalHandler_ConcurrentDuplicateApprovalCoalesces -count=1`

### 7. Wrong vs Correct

- Wrong: check `pending[key]` without holding the same lock across create and remember.
- Correct: serialize same-fingerprint lookup/create/remember so approval grants, HITL pending state, and history stay one-to-one.

---

## Scenario: Agent hosted tool AuthorizationService entry contract

### 1. Scope / Trigger

- Trigger: changing `AgentToolingRuntime`, hosted tool execution, chat tool execution, agent tool execution, MCP hosted tools, or retrieval hosted tools.
- Affected chain: `BuildAgentToolingRuntime(...) -> AgentToolingRuntime.ToolManager -> hostedToolManager.ExecuteForAgent(...) -> AuthorizationService.Authorize(...) -> hosted.ToolRegistry.Execute(...)`.
- Constraint: official chat and agent tool execution must authorize through `AuthorizationService` before invoking hosted tools; `agent/` packages must not import `internal/usecase`.

### 2. Signatures

- Runtime build: `func BuildAgentToolingRuntime(opts AgentToolingOptions, logger *zap.Logger) (*AgentToolingRuntime, error)`.
- Runtime service field: `AgentToolingRuntime.AuthorizationService usecase.AuthorizationService`.
- Tool execution: `func (m *hostedToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult`.
- Authorization request helper: `func toolAuthorizationRequest(ctx context.Context, agentID string, resourceKind types.ResourceKind, resourceID string, action types.ActionKind, riskTier types.RiskTier, values map[string]any) types.AuthorizationRequest`.

### 3. Contracts

- `BuildAgentToolingRuntime` must create one default `AuthorizationService` from the runtime `PermissionManager` when no explicit service is injected.
- `AgentToolingRuntime.AuthorizationService` is the reusable authorization service for server workflow wiring and hot reload wiring.
- `hostedToolManager.ExecuteForAgent` must call `AuthorizationService.Authorize` before `hosted.ToolRegistry.Execute`.
- Authorization context must include `agent_id`, `tool_call_id`, `args_fingerprint`, `trace_id`, `run_id`, and metadata containing `runtime=agent_tooling`, `hosted_tool_type`, and `hosted_tool_risk`.
- If `types.UserID` is present on context, the authorization principal is the user; otherwise the agent is the fallback principal.
- `DecisionDeny` and `DecisionRequireApproval` must return tool errors and must not execute the hosted tool.
- The hosted registry may retain `PermissionManager` as a lower-level direct-registry fallback, but official chat/agent execution goes through `ToolManager -> AuthorizationService`.

### 4. Validation & Error Matrix

| Case | Expected behavior |
|---|---|
| retrieval through runtime ToolManager | `AuthorizationService` sees `ResourceTool`, `RiskSafeRead`, and executes when allowed |
| MCP hosted tool through runtime ToolManager | `AuthorizationService` sees `ResourceMCPTool`; deny stops execution before registry call |
| context has user and agent | principal is user; `agent_id` remains in authorization context |
| authorization returns nil decision | tool result contains an authorization error |
| no explicit authorization service | runtime builds one from the shared `PermissionManager` |

### 5. Good/Base/Bad Cases

#### Correct

```go
authorizationService := opts.AuthorizationService
if authorizationService == nil {
		authorizationService = BuildAuthorizationRuntime(permissionManager, logger).Service
}
manager := newHostedToolManager(registry, permissionManager, authorizationService, logger)
```

#### Wrong

```go
manager := newHostedToolManager(registry, permissionManager, logger)
raw, err := registry.Execute(ctx, call.Name, call.Arguments)
```

### 6. Tests Required

- `go test ./internal/app/bootstrap -run 'TestBuildAgentToolingRuntime_(ToolManagerUsesAuthorizationService|AuthorizationServiceDeniesBeforeHostedExecution)' -count=1`
- For server wiring changes: `go test ./cmd/agentflow -run 'Test.*HotReload|Test.*Startup|TestLoadAndValidateConfig' -count=1`

### 7. Wrong vs Correct

- Wrong: add a new hosted-tool execution path that directly invokes `hosted.ToolRegistry.Execute` from chat or agent runtime.
- Correct: use the runtime `ToolManager`, which performs `AuthorizationService.Authorize` before hosted execution and carries audit metadata.
