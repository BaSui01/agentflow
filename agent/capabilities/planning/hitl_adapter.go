package planning

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
)

// InterruptRequester is a local interface for HITL integration.
// Matches the subset of hitl.InterruptManager needed by workflow runtime.
type InterruptRequester interface {
	RequestApproval(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error)
}

// ApprovalRequest describes what the human needs to review.
type ApprovalRequest struct {
	Title       string
	Description string
	Data        any
	Options     []ApprovalOption
	Timeout     time.Duration
}

// ApprovalOption is a single choice presented to the reviewer.
type ApprovalOption struct {
	ID    string
	Label string
}

// ApprovalResponse carries the human reviewer's decision.
type ApprovalResponse struct {
	Action   string // "approve", "reject", "modify"
	Feedback string
	Data     any
}

// HITLInterruptAdapter adapts hitl.InterruptManager to the InterruptRequester
// interface. This is the only file in the deliberation package that imports
// agent/hitl, keeping the rest of the package loosely coupled.
type HITLInterruptAdapter struct {
	manager *hitl.InterruptManager
}

// NewHITLInterruptAdapter wraps an InterruptManager for deliberation integration.
func NewHITLInterruptAdapter(manager *hitl.InterruptManager) *HITLInterruptAdapter {
	return &HITLInterruptAdapter{manager: manager}
}

// RequestApproval translates an ApprovalRequest into a hitl.InterruptOptions call
// and maps the hitl.Response back to an ApprovalResponse.
func (a *HITLInterruptAdapter) RequestApproval(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
	options := make([]hitl.Option, len(opts.Options))
	for i, o := range opts.Options {
		options[i] = hitl.Option{
			ID:    o.ID,
			Label: o.Label,
		}
	}

	resp, err := a.manager.CreateInterrupt(ctx, hitl.InterruptOptions{
		Type:        hitl.InterruptTypeApproval,
		Title:       opts.Title,
		Description: opts.Description,
		Data:        opts.Data,
		Options:     options,
		Timeout:     opts.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create interrupt failed: %w", err)
	}

	action := "reject"
	if resp.Approved {
		action = "approve"
	}
	if resp.OptionID == "modify" {
		action = "modify"
	}

	return &ApprovalResponse{
		Action:   action,
		Feedback: resp.Comment,
		Data:     resp.Input,
	}, nil
}
