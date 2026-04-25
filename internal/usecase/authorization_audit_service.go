package usecase

import (
	"context"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

const (
	AuthorizationAuditEventType = "authorization_decision"

	defaultAuthorizationAuditLimit = 100
	maxAuthorizationAuditLimit     = 500
	authorizationAuditScanLimit    = 1000
)

type AuthorizationAuditRuntime interface {
	ListHistory(ctx context.Context, limit int) ([]*ToolApprovalHistoryEntry, error)
}

type ListAuthorizationAuditInput struct {
	Limit           int
	PrincipalID     string
	UserID          string
	AgentID         string
	RunID           string
	TraceID         string
	ResourceKind    string
	ResourceID      string
	ToolName        string
	Action          string
	RiskTier        string
	Decision        string
	ApprovalID      string
	Fingerprint     string
	ArgsFingerprint string
}

type AuthorizationAuditService interface {
	List(ctx context.Context, input ListAuthorizationAuditInput) ([]*ToolApprovalHistoryEntry, *types.Error)
}

type DefaultAuthorizationAuditService struct {
	runtime AuthorizationAuditRuntime
}

func NewDefaultAuthorizationAuditService(runtime AuthorizationAuditRuntime) *DefaultAuthorizationAuditService {
	return &DefaultAuthorizationAuditService{runtime: runtime}
}

func (s *DefaultAuthorizationAuditService) List(
	ctx context.Context,
	input ListAuthorizationAuditInput,
) ([]*ToolApprovalHistoryEntry, *types.Error) {
	if s.runtime == nil {
		return nil, types.NewInternalError("authorization audit runtime is not configured")
	}
	limit, err := normalizeAuthorizationAuditLimit(input.Limit)
	if err != nil {
		return nil, err
	}
	rows, listErr := s.runtime.ListHistory(ctx, authorizationAuditScanLimit)
	if listErr != nil {
		return nil, types.NewInternalError("failed to list authorization audit history").WithCause(listErr)
	}

	out := make([]*ToolApprovalHistoryEntry, 0, limit)
	for _, row := range rows {
		if !matchesAuthorizationAuditEntry(row, input) {
			continue
		}
		cloned := *row
		out = append(out, &cloned)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func normalizeAuthorizationAuditLimit(limit int) (int, *types.Error) {
	if limit == 0 {
		return defaultAuthorizationAuditLimit, nil
	}
	if limit < 0 {
		return 0, types.NewInvalidRequestError("limit must be greater than 0")
	}
	if limit > maxAuthorizationAuditLimit {
		return 0, types.NewInvalidRequestError("limit must be less than or equal to 500")
	}
	return limit, nil
}

func matchesAuthorizationAuditEntry(row *ToolApprovalHistoryEntry, input ListAuthorizationAuditInput) bool {
	if row == nil {
		return false
	}
	if strings.TrimSpace(row.EventType) != AuthorizationAuditEventType {
		return false
	}
	if !matchesAuditFilter(row.PrincipalID, input.PrincipalID) {
		return false
	}
	if !matchesAuditFilter(row.UserID, input.UserID) {
		return false
	}
	if !matchesAuditFilter(row.AgentID, input.AgentID) {
		return false
	}
	if !matchesAuditFilter(row.RunID, input.RunID) {
		return false
	}
	if !matchesAuditFilter(row.TraceID, input.TraceID) {
		return false
	}
	if !matchesAuditFilter(row.ResourceKind, input.ResourceKind) {
		return false
	}
	if !matchesAuditFilter(row.ResourceID, input.ResourceID) {
		return false
	}
	if !matchesAuditFilter(row.ToolName, input.ToolName) {
		return false
	}
	if !matchesAuditFilter(row.Action, input.Action) {
		return false
	}
	if !matchesAuditFilter(row.RiskTier, input.RiskTier) {
		return false
	}
	if !matchesAuditFilter(row.Decision, input.Decision) {
		return false
	}
	if !matchesAuditFilter(row.ApprovalID, input.ApprovalID) {
		return false
	}
	if !matchesAuditFilter(row.Fingerprint, input.Fingerprint) {
		return false
	}
	return matchesAuditFilter(row.ArgsFingerprint, input.ArgsFingerprint)
}

func matchesAuditFilter(value, expected string) bool {
	normalized := strings.TrimSpace(expected)
	if normalized == "" {
		return true
	}
	return strings.TrimSpace(value) == normalized
}
