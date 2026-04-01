package handlers

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/types"
)

type ToolApprovalRuntime interface {
	GetInterrupt(ctx context.Context, interruptID string) (*hitl.Interrupt, error)
	ListInterrupts(ctx context.Context, workflowID string, status hitl.InterruptStatus) ([]*hitl.Interrupt, error)
	ResolveInterrupt(ctx context.Context, interruptID string, response *hitl.Response) error
	GrantStats(ctx context.Context) (*ToolApprovalStats, error)
	CleanupExpiredGrants(ctx context.Context) (int, error)
	ListGrants(ctx context.Context) ([]*ToolApprovalGrantView, error)
	RevokeGrant(ctx context.Context, fingerprint string) error
	ListHistory(ctx context.Context, limit int) ([]*ToolApprovalHistoryEntry, error)
}

type ToolApprovalStats struct {
	Backend              string `json:"backend"`
	Scope                string `json:"scope"`
	GrantTTL             string `json:"grant_ttl"`
	ActiveGrantCount     int    `json:"active_grant_count"`
	PendingApprovalCount int    `json:"pending_approval_count"`
	ResolvedCount        int    `json:"resolved_count"`
	RejectedCount        int    `json:"rejected_count"`
	TimeoutCount         int    `json:"timeout_count"`
	CanceledCount        int    `json:"canceled_count"`
}

type ToolApprovalGrantView struct {
	Fingerprint string `json:"fingerprint"`
	ApprovalID  string `json:"approval_id"`
	Scope       string `json:"scope"`
	ToolName    string `json:"tool_name"`
	AgentID     string `json:"agent_id,omitempty"`
	ExpiresAt   string `json:"expires_at"`
}

type ToolApprovalHistoryEntry struct {
	EventType   string `json:"event_type"`
	ApprovalID  string `json:"approval_id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	Status      string `json:"status,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Timestamp   string `json:"timestamp"`
}

type ToolApprovalService interface {
	List(ctx context.Context, status string) ([]*hitl.Interrupt, *types.Error)
	Get(ctx context.Context, interruptID string) (*hitl.Interrupt, *types.Error)
	Resolve(ctx context.Context, interruptID string, approved bool, optionID string, comment string, userID string) *types.Error
	Stats(ctx context.Context) (*ToolApprovalStats, *types.Error)
	Cleanup(ctx context.Context) (int, *types.Error)
	ListGrants(ctx context.Context) ([]*ToolApprovalGrantView, *types.Error)
	RevokeGrant(ctx context.Context, fingerprint string) *types.Error
	ListHistory(ctx context.Context, limit int) ([]*ToolApprovalHistoryEntry, *types.Error)
}

type DefaultToolApprovalService struct {
	runtime    ToolApprovalRuntime
	workflowID string
}

func NewDefaultToolApprovalService(runtime ToolApprovalRuntime, workflowID string) *DefaultToolApprovalService {
	return &DefaultToolApprovalService{
		runtime:    runtime,
		workflowID: strings.TrimSpace(workflowID),
	}
}

func (s *DefaultToolApprovalService) List(ctx context.Context, status string) ([]*hitl.Interrupt, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool approval runtime is not configured")
	}
	normalizedStatus, err := parseInterruptStatus(status)
	if err != nil {
		return nil, err
	}
	rows, listErr := s.runtime.ListInterrupts(ctx, s.workflowID, normalizedStatus)
	if listErr != nil {
		return nil, types.NewInternalError("failed to list tool approvals").WithCause(listErr)
	}
	return rows, nil
}

func (s *DefaultToolApprovalService) Get(ctx context.Context, interruptID string) (*hitl.Interrupt, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool approval runtime is not configured")
	}
	id := strings.TrimSpace(interruptID)
	if id == "" {
		return nil, types.NewInvalidRequestError("approval ID is required")
	}
	interrupt, err := s.runtime.GetInterrupt(ctx, id)
	if err != nil {
		return nil, types.NewNotFoundError("tool approval not found")
	}
	if interrupt == nil || strings.TrimSpace(interrupt.WorkflowID) != s.workflowID {
		return nil, types.NewNotFoundError("tool approval not found")
	}
	return interrupt, nil
}

func (s *DefaultToolApprovalService) Resolve(
	ctx context.Context,
	interruptID string,
	approved bool,
	optionID string,
	comment string,
	userID string,
) *types.Error {
	interrupt, err := s.Get(ctx, interruptID)
	if err != nil {
		return err
	}

	selectedOption := strings.TrimSpace(optionID)
	if selectedOption == "" {
		if approved {
			selectedOption = "approve"
		} else {
			selectedOption = "reject"
		}
	}

	if interrupt.Status != hitl.InterruptStatusPending {
		return types.NewInvalidRequestError("tool approval is no longer pending")
	}

	resolveErr := s.runtime.ResolveInterrupt(ctx, interrupt.ID, &hitl.Response{
		OptionID: selectedOption,
		Comment:  strings.TrimSpace(comment),
		Approved: approved,
		UserID:   strings.TrimSpace(userID),
	})
	if resolveErr != nil {
		return types.NewInternalError("failed to resolve tool approval").WithCause(resolveErr)
	}
	return nil
}

func (s *DefaultToolApprovalService) Stats(ctx context.Context) (*ToolApprovalStats, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool approval runtime is not configured")
	}
	stats, err := s.runtime.GrantStats(ctx)
	if err != nil {
		return nil, types.NewInternalError("failed to inspect tool approval grants").WithCause(err)
	}
	if stats == nil {
		stats = &ToolApprovalStats{}
	}
	pending, err := s.runtime.ListInterrupts(ctx, s.workflowID, hitl.InterruptStatusPending)
	if err != nil {
		return nil, types.NewInternalError("failed to list pending tool approvals").WithCause(err)
	}
	resolved, err := s.runtime.ListInterrupts(ctx, s.workflowID, hitl.InterruptStatusResolved)
	if err != nil {
		return nil, types.NewInternalError("failed to list resolved tool approvals").WithCause(err)
	}
	rejected, err := s.runtime.ListInterrupts(ctx, s.workflowID, hitl.InterruptStatusRejected)
	if err != nil {
		return nil, types.NewInternalError("failed to list rejected tool approvals").WithCause(err)
	}
	timeout, err := s.runtime.ListInterrupts(ctx, s.workflowID, hitl.InterruptStatusTimeout)
	if err != nil {
		return nil, types.NewInternalError("failed to list timed out tool approvals").WithCause(err)
	}
	canceled, err := s.runtime.ListInterrupts(ctx, s.workflowID, hitl.InterruptStatusCanceled)
	if err != nil {
		return nil, types.NewInternalError("failed to list canceled tool approvals").WithCause(err)
	}
	stats.PendingApprovalCount = len(pending)
	stats.ResolvedCount = len(resolved)
	stats.RejectedCount = len(rejected)
	stats.TimeoutCount = len(timeout)
	stats.CanceledCount = len(canceled)
	return stats, nil
}

func (s *DefaultToolApprovalService) Cleanup(ctx context.Context) (int, *types.Error) {
	if s.runtime == nil {
		return 0, types.NewInternalError("tool approval runtime is not configured")
	}
	removed, err := s.runtime.CleanupExpiredGrants(ctx)
	if err != nil {
		return 0, types.NewInternalError("failed to cleanup expired tool approval grants").WithCause(err)
	}
	return removed, nil
}

func (s *DefaultToolApprovalService) ListGrants(ctx context.Context) ([]*ToolApprovalGrantView, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool approval runtime is not configured")
	}
	rows, err := s.runtime.ListGrants(ctx)
	if err != nil {
		return nil, types.NewInternalError("failed to list tool approval grants").WithCause(err)
	}
	return rows, nil
}

func (s *DefaultToolApprovalService) RevokeGrant(ctx context.Context, fingerprint string) *types.Error {
	if s.runtime == nil {
		return types.NewInternalError("tool approval runtime is not configured")
	}
	key := strings.TrimSpace(fingerprint)
	if key == "" {
		return types.NewInvalidRequestError("grant fingerprint is required")
	}
	if err := s.runtime.RevokeGrant(ctx, key); err != nil {
		return types.NewInternalError("failed to revoke tool approval grant").WithCause(err)
	}
	return nil
}

func (s *DefaultToolApprovalService) ListHistory(ctx context.Context, limit int) ([]*ToolApprovalHistoryEntry, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("tool approval runtime is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.runtime.ListHistory(ctx, limit)
	if err != nil {
		return nil, types.NewInternalError("failed to list tool approval history").WithCause(err)
	}
	return rows, nil
}

func parseInterruptStatus(raw string) (hitl.InterruptStatus, *types.Error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	if status == "" {
		return hitl.InterruptStatusPending, nil
	}
	switch hitl.InterruptStatus(status) {
	case hitl.InterruptStatusPending,
		hitl.InterruptStatusResolved,
		hitl.InterruptStatusRejected,
		hitl.InterruptStatusTimeout,
		hitl.InterruptStatusCanceled:
		return hitl.InterruptStatus(status), nil
	default:
		return "", types.NewInvalidRequestError("status must be one of pending,resolved,rejected,timeout,canceled")
	}
}
