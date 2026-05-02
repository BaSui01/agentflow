package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	"github.com/BaSui01/agentflow/internal/usecase"
)

type toolApprovalRuntime struct {
	manager *hitl.InterruptManager
	store   ToolApprovalGrantStore
	history ToolApprovalHistoryStore
	config  ToolApprovalConfig
}

type authorizationAuditHistoryRuntime struct {
	history ToolApprovalHistoryStore
}

func (r *toolApprovalRuntime) GetInterrupt(ctx context.Context, interruptID string) (*hitl.Interrupt, error) {
	return r.manager.GetInterrupt(ctx, interruptID)
}

func (r *toolApprovalRuntime) ListInterrupts(ctx context.Context, workflowID string, status hitl.InterruptStatus) ([]*hitl.Interrupt, error) {
	return r.manager.ListInterrupts(ctx, workflowID, status)
}

func (r *toolApprovalRuntime) ResolveInterrupt(ctx context.Context, interruptID string, response *hitl.Response) error {
	interrupt, err := r.manager.GetInterrupt(ctx, interruptID)
	if err != nil {
		return err
	}
	if err := r.manager.ResolveInterrupt(ctx, interruptID, response); err != nil {
		return err
	}
	if interrupt != nil && r.history != nil {
		status := string(hitl.InterruptStatusRejected)
		if response != nil && response.Approved {
			status = string(hitl.InterruptStatusResolved)
		}
		_ = r.history.Append(context.Background(), &ToolApprovalHistoryEntry{
			EventType:   "approval_resolved",
			ApprovalID:  interruptID,
			Fingerprint: metadataStringAny(interrupt.Metadata, "approval_fingerprint"),
			ToolName:    metadataStringAny(interrupt.Metadata, "tool_name"),
			AgentID:     metadataStringAny(interrupt.Metadata, "agent_id"),
			Status:      status,
			Scope:       metadataStringAny(interrupt.Metadata, "approval_scope"),
			Comment:     strings.TrimSpace(response.Comment),
			Timestamp:   time.Now().UTC(),
		})
	}
	return nil
}

func (r *toolApprovalRuntime) GrantStats(ctx context.Context) (*usecase.ToolApprovalStats, error) {
	count := 0
	if r.store != nil {
		grants, err := r.store.List(ctx)
		if err != nil {
			return nil, err
		}
		count = len(grants)
	}
	return &usecase.ToolApprovalStats{
		Backend:          strings.ToLower(strings.TrimSpace(r.config.Backend)),
		Scope:            normalizeToolApprovalScope(r.config.Scope),
		GrantTTL:         r.config.GrantTTL.String(),
		ActiveGrantCount: count,
	}, nil
}

func (r *toolApprovalRuntime) CleanupExpiredGrants(ctx context.Context) (int, error) {
	if r.store == nil {
		return 0, nil
	}
	removed, err := r.store.CleanupExpired(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	if removed > 0 && r.history != nil {
		_ = r.history.Append(context.Background(), &ToolApprovalHistoryEntry{
			EventType: "grant_cleanup",
			Comment:   fmt.Sprintf("removed %d expired grants", removed),
			Timestamp: time.Now().UTC(),
		})
	}
	return removed, nil
}

func (r *toolApprovalRuntime) ListGrants(ctx context.Context) ([]*usecase.ToolApprovalGrantView, error) {
	if r.store == nil {
		return []*usecase.ToolApprovalGrantView{}, nil
	}
	grants, err := r.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*usecase.ToolApprovalGrantView, 0, len(grants))
	for _, grant := range grants {
		if grant == nil {
			continue
		}
		out = append(out, &usecase.ToolApprovalGrantView{
			Fingerprint: grant.Fingerprint,
			ApprovalID:  grant.ApprovalID,
			Scope:       grant.Scope,
			ToolName:    grant.ToolName,
			AgentID:     grant.AgentID,
			ExpiresAt:   grant.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (r *toolApprovalRuntime) RevokeGrant(ctx context.Context, fingerprint string) error {
	if r.store == nil {
		return nil
	}
	key := strings.TrimSpace(fingerprint)
	if err := r.store.Delete(ctx, key); err != nil {
		return err
	}
	if r.history != nil {
		_ = r.history.Append(context.Background(), &ToolApprovalHistoryEntry{
			EventType:   "grant_revoked",
			Fingerprint: key,
			Timestamp:   time.Now().UTC(),
		})
	}
	return nil
}

func (r *toolApprovalRuntime) ListHistory(ctx context.Context, limit int) ([]*usecase.ToolApprovalHistoryEntry, error) {
	return listToolApprovalHistory(ctx, r.history, limit)
}

func (r *authorizationAuditHistoryRuntime) ListHistory(ctx context.Context, limit int) ([]*usecase.ToolApprovalHistoryEntry, error) {
	return listToolApprovalHistory(ctx, r.history, limit)
}

func listToolApprovalHistory(
	ctx context.Context,
	history ToolApprovalHistoryStore,
	limit int,
) ([]*usecase.ToolApprovalHistoryEntry, error) {
	if history == nil {
		return []*usecase.ToolApprovalHistoryEntry{}, nil
	}
	rows, err := history.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*usecase.ToolApprovalHistoryEntry, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		cloned := *row
		out = append(out, &usecase.ToolApprovalHistoryEntry{
			EventType:       cloned.EventType,
			ApprovalID:      cloned.ApprovalID,
			Fingerprint:     cloned.Fingerprint,
			ToolName:        cloned.ToolName,
			AgentID:         cloned.AgentID,
			PrincipalID:     cloned.PrincipalID,
			UserID:          cloned.UserID,
			RunID:           cloned.RunID,
			TraceID:         cloned.TraceID,
			ResourceKind:    cloned.ResourceKind,
			ResourceID:      cloned.ResourceID,
			Action:          cloned.Action,
			RiskTier:        cloned.RiskTier,
			Decision:        cloned.Decision,
			Status:          cloned.Status,
			Scope:           cloned.Scope,
			Comment:         cloned.Comment,
			ArgsFingerprint: cloned.ArgsFingerprint,
			Timestamp:       cloned.Timestamp.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}
