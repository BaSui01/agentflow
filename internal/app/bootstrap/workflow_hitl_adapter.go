package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/planning"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
)

type hitlHumanInputHandler struct {
	requester     planning.InterruptRequester
	authorization usecase.AuthorizationService
}

func (h hitlHumanInputHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (*core.HumanInputResult, error) {
	if h.requester == nil {
		return nil, fmt.Errorf("workflow hitl requester is not configured")
	}
	if err := authorizeWorkflowStep(ctx, h.authorization, workflowAuthorizationRequest(
		ctx,
		types.ResourceWorkflow,
		"human_input",
		types.ActionApprove,
		types.RiskMutating,
		map[string]any{
			"arguments": map[string]any{
				"input_type":         inputType,
				"options_count":      len(options),
				"prompt_bytes":       len(prompt),
				"prompt_fingerprint": workflowStringFingerprint(prompt),
			},
			"metadata": map[string]string{
				"runtime":       "workflow",
				"workflow_step": "human_input",
			},
		},
	)); err != nil {
		return nil, err
	}

	hitlOptions := make([]planning.ApprovalOption, 0, len(options))
	for idx, opt := range options {
		id := opt
		if id == "" {
			id = fmt.Sprintf("option_%d", idx+1)
		}
		hitlOptions = append(hitlOptions, planning.ApprovalOption{ID: id, Label: opt})
	}

	resp, err := h.requester.RequestApproval(ctx, planning.ApprovalRequest{
		Title:       "Workflow human input required",
		Description: prompt,
		Options:     hitlOptions,
		Timeout:     30 * time.Second,
		Data: map[string]any{
			"input_type": inputType,
		},
	})
	if err != nil {
		return nil, err
	}

	optionID := resp.Action
	if data, ok := resp.Data.(map[string]any); ok {
		if oid, ok := data["option_id"].(string); ok && oid != "" {
			optionID = oid
		}
	}
	return &core.HumanInputResult{Value: resp.Feedback, OptionID: optionID}, nil
}
