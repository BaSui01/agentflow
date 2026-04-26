package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/types"
)

type HookPoint string

const (
	HookBeforeModel HookPoint = "before_model"
	HookBeforeTool  HookPoint = "before_tool"
	HookAfterTool   HookPoint = "after_tool"
	HookAfterOutput HookPoint = "after_output"
)

type HookAction string

const (
	HookActionPass    HookAction = "pass"
	HookActionAbort   HookAction = "abort"
	HookActionModify  HookAction = "modify"
)

type HookResult struct {
	Action   HookAction `json:"action"`
	Modified any        `json:"modified,omitempty"`
	Reason   string     `json:"reason,omitempty"`
}

type HookMiddleware interface {
	Name() string
	Point() HookPoint
	Execute(ctx context.Context, input any) (HookResult, error)
}

type HookRegistry struct {
	hooks map[HookPoint][]HookMiddleware
	mu    sync.RWMutex
}

func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks: make(map[HookPoint][]HookMiddleware),
	}
}

func (r *HookRegistry) Register(hook HookMiddleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[hook.Point()] = append(r.hooks[hook.Point()], hook)
}

var defaultHookTimeout = 5 * time.Second

func (r *HookRegistry) Execute(ctx context.Context, point HookPoint, input any) (HookResult, error) {
	r.mu.RLock()
	hooks := make([]HookMiddleware, len(r.hooks[point]))
	copy(hooks, r.hooks[point])
	r.mu.RUnlock()

	if len(hooks) == 0 {
		return HookResult{Action: HookActionPass}, nil
	}

	for _, hook := range hooks {
		result, err := r.executeWithTimeout(ctx, hook, input)
		if err != nil {
			return HookResult{Action: HookActionAbort, Reason: err.Error()}, err
		}
		switch result.Action {
		case HookActionAbort:
			return result, nil
		case HookActionModify:
			input = result.Modified
		}
	}
	return HookResult{Action: HookActionPass}, nil
}

func (r *HookRegistry) executeWithTimeout(ctx context.Context, hook HookMiddleware, input any) (HookResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, defaultHookTimeout)
	defer cancel()

	resultCh := make(chan hookExecResult, 1)
	go func() {
		result, err := hook.Execute(timeoutCtx, input)
		resultCh <- hookExecResult{result: result, err: err}
	}()

	select {
	case res := <-resultCh:
		return res.result, res.err
	case <-timeoutCtx.Done():
		return HookResult{}, types.NewRuntimeMiddlewareTimeoutError(
			fmt.Sprintf("hook %s timed out after %s", hook.Name(), defaultHookTimeout),
		)
	}
}

type hookExecResult struct {
	result HookResult
	err    error
}

type AuthorizeFunc func(ctx context.Context, req types.AuthorizationRequest) (*types.AuthorizationDecision, error)

type AuthzMiddleware struct {
	authorize AuthorizeFunc
}

func NewAuthzMiddleware(authorize AuthorizeFunc) *AuthzMiddleware {
	return &AuthzMiddleware{authorize: authorize}
}

func (m *AuthzMiddleware) Name() string    { return "authz_middleware" }
func (m *AuthzMiddleware) Point() HookPoint { return HookBeforeTool }

func (m *AuthzMiddleware) Execute(ctx context.Context, input any) (HookResult, error) {
	toolCall, ok := input.(*types.ToolCall)
	if !ok || toolCall == nil {
		return HookResult{Action: HookActionPass}, nil
	}

	req := types.AuthorizationRequest{
		ResourceKind: types.ResourceTool,
		ResourceID:   toolCall.Name,
		Action:       types.ActionExecute,
		RiskTier:     types.RiskExecution,
	}

	if principal, ok := types.PrincipalFromContext(ctx); ok {
		req.Principal = principal
	}

	decision, err := m.authorize(ctx, req)
	if err != nil {
		return HookResult{Action: HookActionAbort, Reason: err.Error()}, err
	}

	switch decision.Decision {
	case types.DecisionDeny:
		return HookResult{
			Action: HookActionAbort,
			Reason: fmt.Sprintf("tool %s denied: %s", toolCall.Name, decision.Reason),
		}, nil
	case types.DecisionRequireApproval:
		return HookResult{
			Action: HookActionAbort,
			Reason: fmt.Sprintf("tool %s requires approval: %s", toolCall.Name, decision.Reason),
		}, nil
	default:
		return HookResult{Action: HookActionPass}, nil
	}
}

type InputGuardrailMiddleware struct {
	checkFunc func(ctx context.Context, input any) (HookResult, error)
}

func NewInputGuardrailMiddleware(checkFunc func(ctx context.Context, input any) (HookResult, error)) *InputGuardrailMiddleware {
	return &InputGuardrailMiddleware{checkFunc: checkFunc}
}

func (m *InputGuardrailMiddleware) Name() string    { return "input_guardrail" }
func (m *InputGuardrailMiddleware) Point() HookPoint { return HookBeforeModel }
func (m *InputGuardrailMiddleware) Execute(ctx context.Context, input any) (HookResult, error) {
	return m.checkFunc(ctx, input)
}

type OutputGuardrailMiddleware struct {
	checkFunc func(ctx context.Context, output any) (HookResult, error)
}

func NewOutputGuardrailMiddleware(checkFunc func(ctx context.Context, output any) (HookResult, error)) *OutputGuardrailMiddleware {
	return &OutputGuardrailMiddleware{checkFunc: checkFunc}
}

func (m *OutputGuardrailMiddleware) Name() string    { return "output_guardrail" }
func (m *OutputGuardrailMiddleware) Point() HookPoint { return HookAfterOutput }
func (m *OutputGuardrailMiddleware) Execute(ctx context.Context, input any) (HookResult, error) {
	return m.checkFunc(ctx, input)
}
