package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookRegistry_FourStandardPoints(t *testing.T) {
	t.Parallel()

	registry := NewHookRegistry()
	assert.Equal(t, HookPoint("before_model"), HookBeforeModel)
	assert.Equal(t, HookPoint("before_tool"), HookBeforeTool)
	assert.Equal(t, HookPoint("after_tool"), HookAfterTool)
	assert.Equal(t, HookPoint("after_output"), HookAfterOutput)

	result, err := registry.Execute(context.Background(), HookBeforeModel, nil)
	require.NoError(t, err)
	assert.Equal(t, HookActionPass, result.Action)
}

func TestHookRegistry_ExecutionOrder(t *testing.T) {
	t.Parallel()

	var order []string
	registry := NewHookRegistry()

	for _, point := range []HookPoint{HookBeforeModel, HookBeforeTool, HookAfterTool, HookAfterOutput} {
		p := point
		registry.Register(&stubHook{
			name:  "hook_" + string(p),
			point: p,
			exec: func(_ context.Context, _ any) (HookResult, error) {
				order = append(order, string(p))
				return HookResult{Action: HookActionPass}, nil
			},
		})
	}

	_, _ = registry.Execute(context.Background(), HookBeforeModel, nil)
	_, _ = registry.Execute(context.Background(), HookBeforeTool, nil)
	_, _ = registry.Execute(context.Background(), HookAfterTool, nil)
	_, _ = registry.Execute(context.Background(), HookAfterOutput, nil)

	assert.Equal(t, []string{"before_model", "before_tool", "after_tool", "after_output"}, order)
}

func TestHookRegistry_AbortStopsExecution(t *testing.T) {
	t.Parallel()

	var executed atomic.Int32
	registry := NewHookRegistry()

	registry.Register(&stubHook{
		name:  "abort_hook",
		point: HookBeforeTool,
		exec: func(_ context.Context, _ any) (HookResult, error) {
			return HookResult{Action: HookActionAbort, Reason: "blocked"}, nil
		},
	})
	registry.Register(&stubHook{
		name:  "should_not_run",
		point: HookBeforeTool,
		exec: func(_ context.Context, _ any) (HookResult, error) {
			executed.Add(1)
			return HookResult{Action: HookActionPass}, nil
		},
	})

	result, err := registry.Execute(context.Background(), HookBeforeTool, nil)
	require.NoError(t, err)
	assert.Equal(t, HookActionAbort, result.Action)
	assert.Equal(t, int32(0), executed.Load())
}

func TestHookRegistry_TimeoutProtection(t *testing.T) {
	registry := NewHookRegistry()

	registry.Register(&stubHook{
		name:  "slow_hook",
		point: HookBeforeTool,
		exec: func(ctx context.Context, _ any) (HookResult, error) {
			<-ctx.Done()
			return HookResult{Action: HookActionPass}, nil
		},
	})

	result, err := registry.Execute(context.Background(), HookBeforeTool, nil)
	assert.Error(t, err)
	assert.Equal(t, HookActionAbort, result.Action)
}

func TestAuthzMiddleware_DenyAndApproval(t *testing.T) {
	t.Parallel()

	authorizeDeny := func(_ context.Context, _ types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
		return &types.AuthorizationDecision{Decision: types.DecisionDeny, Reason: "forbidden"}, nil
	}

	m := NewAuthzMiddleware(authorizeDeny)
	toolCall := &types.ToolCall{Name: "shell.exec"}
	result, err := m.Execute(context.Background(), toolCall)
	require.NoError(t, err)
	assert.Equal(t, HookActionAbort, result.Action)
	assert.Contains(t, result.Reason, "denied")

	authorizeApproval := func(_ context.Context, _ types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
		return &types.AuthorizationDecision{Decision: types.DecisionRequireApproval, Reason: "needs review"}, nil
	}

	m2 := NewAuthzMiddleware(authorizeApproval)
	result2, err := m2.Execute(context.Background(), toolCall)
	require.NoError(t, err)
	assert.Equal(t, HookActionAbort, result2.Action)
	assert.Contains(t, result2.Reason, "requires approval")
}

func TestAuthzMiddleware_AllowPasses(t *testing.T) {
	t.Parallel()

	authorizeAllow := func(_ context.Context, _ types.AuthorizationRequest) (*types.AuthorizationDecision, error) {
		return &types.AuthorizationDecision{Decision: types.DecisionAllow, Reason: "ok"}, nil
	}

	m := NewAuthzMiddleware(authorizeAllow)
	toolCall := &types.ToolCall{Name: "safe.read"}
	result, err := m.Execute(context.Background(), toolCall)
	require.NoError(t, err)
	assert.Equal(t, HookActionPass, result.Action)
}

func TestInputGuardrailMiddleware(t *testing.T) {
	t.Parallel()

	m := NewInputGuardrailMiddleware(func(_ context.Context, input any) (HookResult, error) {
		if s, ok := input.(string); ok && len(s) > 100 {
			return HookResult{Action: HookActionAbort, Reason: "input too long"}, nil
		}
		return HookResult{Action: HookActionPass}, nil
	})

	assert.Equal(t, HookBeforeModel, m.Point())

	result, err := m.Execute(context.Background(), "short")
	require.NoError(t, err)
	assert.Equal(t, HookActionPass, result.Action)

	result2, err := m.Execute(context.Background(), "this is a very long input that exceeds the limit of one hundred characters for sure definitely absolutely")
	require.NoError(t, err)
	assert.Equal(t, HookActionAbort, result2.Action)
}

func TestOutputGuardrailMiddleware(t *testing.T) {
	t.Parallel()

	m := NewOutputGuardrailMiddleware(func(_ context.Context, output any) (HookResult, error) {
		return HookResult{Action: HookActionModify, Modified: "sanitized"}, nil
	})

	assert.Equal(t, HookAfterOutput, m.Point())

	result, err := m.Execute(context.Background(), "original")
	require.NoError(t, err)
	assert.Equal(t, HookActionModify, result.Action)
	assert.Equal(t, "sanitized", result.Modified)
}

func TestHookRegistry_ModifyChains(t *testing.T) {
	t.Parallel()

	registry := NewHookRegistry()
	registry.Register(&stubHook{
		name:  "add_prefix",
		point: HookBeforeModel,
		exec: func(_ context.Context, input any) (HookResult, error) {
			return HookResult{Action: HookActionModify, Modified: "prefix_" + input.(string)}, nil
		},
	})
	registry.Register(&stubHook{
		name:  "add_suffix",
		point: HookBeforeModel,
		exec: func(_ context.Context, input any) (HookResult, error) {
			return HookResult{Action: HookActionModify, Modified: input.(string) + "_suffix"}, nil
		},
	})

	result, err := registry.Execute(context.Background(), HookBeforeModel, "data")
	require.NoError(t, err)
	assert.Equal(t, HookActionPass, result.Action)
}

type stubHook struct {
	name  string
	point HookPoint
	exec  func(ctx context.Context, input any) (HookResult, error)
}

func (h *stubHook) Name() string    { return h.name }
func (h *stubHook) Point() HookPoint { return h.point }
func (h *stubHook) Execute(ctx context.Context, input any) (HookResult, error) {
	return h.exec(ctx, input)
}

func TestHookRegistry_DefaultTimeout(t *testing.T) {
	assert.Equal(t, 5*time.Second, defaultHookTimeout)
}
