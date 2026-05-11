package runtime

import (
	"context"
	"fmt"
	"strings"
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
	HookActionPass   HookAction = "pass"
	HookActionAbort  HookAction = "abort"
	HookActionModify HookAction = "modify"
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

type toolAuthorizationInput struct {
	ToolCall  *types.ToolCall
	ToolRisks map[string]string
	AgentID   string
}

func NewAuthzMiddleware(authorize AuthorizeFunc) *AuthzMiddleware {
	return &AuthzMiddleware{authorize: authorize}
}

func authzRiskTierForTool(name string) types.RiskTier {
	return authzRiskTierForToolRisk(name, classifyToolRiskByName(name))
}

func authzRiskTierForToolRisk(name string, risk string) types.RiskTier {
	switch strings.TrimSpace(risk) {
	case toolRiskSafeRead:
		return types.RiskSafeRead
	case toolRiskRequiresApproval:
		return types.RiskExecution
	case string(types.RiskMutating):
		return types.RiskMutating
	case string(types.RiskExecution):
		return types.RiskExecution
	case string(types.RiskNetworkExecution):
		return types.RiskNetworkExecution
	case string(types.RiskAdmin):
		return types.RiskAdmin
	case "":
		return authzRiskTierForToolRisk(name, classifyToolRiskByName(name))
	default:
		return types.RiskExecution
	}
}

func authzToolCallInput(input any) (*types.ToolCall, map[string]string, string, bool) {
	switch v := input.(type) {
	case *types.ToolCall:
		return v, nil, "", v != nil
	case types.ToolCall:
		call := v
		return &call, nil, "", true
	case *toolAuthorizationInput:
		if v == nil || v.ToolCall == nil {
			return nil, nil, "", false
		}
		return v.ToolCall, v.ToolRisks, strings.TrimSpace(v.AgentID), true
	case toolAuthorizationInput:
		if v.ToolCall == nil {
			return nil, nil, "", false
		}
		return v.ToolCall, v.ToolRisks, strings.TrimSpace(v.AgentID), true
	default:
		return nil, nil, "", false
	}
}

func authzToolRiskFromMap(name string, risks map[string]string) string {
	if len(risks) == 0 {
		return ""
	}
	risk, ok := risks[strings.TrimSpace(name)]
	if !ok {
		return ""
	}
	return strings.TrimSpace(risk)
}

func (m *AuthzMiddleware) Name() string     { return "authz_middleware" }
func (m *AuthzMiddleware) Point() HookPoint { return HookBeforeTool }

func (m *AuthzMiddleware) Execute(ctx context.Context, input any) (HookResult, error) {
	toolCall, toolRisks, agentID, ok := authzToolCallInput(input)
	if !ok {
		return HookResult{Action: HookActionPass}, nil
	}

	toolRisk := authzToolRiskFromMap(toolCall.Name, toolRisks)
	reqContext := map[string]any{
		"tool_call_id": toolCall.ID,
		"metadata": map[string]string{
			"runtime":          "agent_runtime",
			"hosted_tool_risk": firstNonEmpty(toolRisk, classifyToolRiskByName(toolCall.Name)),
		},
	}
	if agentID != "" {
		reqContext["agent_id"] = agentID
		reqContext["metadata"].(map[string]string)["agent_id"] = agentID
	}

	req := types.AuthorizationRequest{
		ResourceKind: types.ResourceTool,
		ResourceID:   toolCall.Name,
		Action:       types.ActionExecute,
		RiskTier:     authzRiskTierForToolRisk(toolCall.Name, toolRisk),
		Context:      reqContext,
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

func (m *InputGuardrailMiddleware) Name() string     { return "input_guardrail" }
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

func (m *OutputGuardrailMiddleware) Name() string     { return "output_guardrail" }
func (m *OutputGuardrailMiddleware) Point() HookPoint { return HookAfterOutput }
func (m *OutputGuardrailMiddleware) Execute(ctx context.Context, input any) (HookResult, error) {
	return m.checkFunc(ctx, input)
}
