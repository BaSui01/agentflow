package deliberation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ErrDecisionRejected is returned when a human reviewer rejects a low-confidence decision.
var ErrDecisionRejected = errors.New("decision rejected by human reviewer")

// InterruptRequester is a local interface for HITL integration.
// Matches the subset of hitl.InterruptManager needed here.
// Using a local interface (§15) to keep deliberation loosely coupled.
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

// HITLConfig configures human-in-the-loop escalation.
type HITLConfig struct {
	Enabled             bool
	ConfidenceThreshold float64       // below this, escalate to human
	InterruptTimeout    time.Duration // how long to wait for human response
}

// DefaultHITLConfig returns sensible defaults for HITL escalation.
func DefaultHITLConfig() HITLConfig {
	return HITLConfig{
		Enabled:             false,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    5 * time.Minute,
	}
}

// HITLBridge connects deliberation to human approval.
type HITLBridge struct {
	engine    *Engine
	requester InterruptRequester
	config    HITLConfig
	logger    *zap.Logger
}

// NewHITLBridge creates a bridge between the deliberation engine and human review.
func NewHITLBridge(engine *Engine, requester InterruptRequester, config HITLConfig, logger *zap.Logger) *HITLBridge {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HITLBridge{
		engine:    engine,
		requester: requester,
		config:    config,
		logger:    logger.With(zap.String("component", "hitl_bridge")),
	}
}

// DeliberateWithApproval runs deliberation and escalates to a human reviewer
// when the final confidence falls below the configured threshold.
func (b *HITLBridge) DeliberateWithApproval(ctx context.Context, task Task) (*DeliberationResult, error) {
	result, err := b.engine.Deliberate(ctx, task)
	if err != nil {
		return result, err
	}

	// If HITL is disabled or confidence is acceptable, return directly.
	if !b.config.Enabled || result.FinalConfidence >= b.config.ConfidenceThreshold {
		return result, nil
	}

	b.logger.Info("low confidence, escalating to human review",
		zap.String("task_id", task.ID),
		zap.Float64("confidence", result.FinalConfidence),
		zap.Float64("threshold", b.config.ConfidenceThreshold),
	)

	reasoning := ""
	if result.Decision != nil {
		reasoning = result.Decision.Reasoning
	}

	resp, err := b.requester.RequestApproval(ctx, ApprovalRequest{
		Title:       "Low-confidence decision requires approval",
		Description: reasoning,
		Data:        result,
		Options: []ApprovalOption{
			{ID: "approve", Label: "Approve"},
			{ID: "reject", Label: "Reject"},
			{ID: "modify", Label: "Modify"},
		},
		Timeout: b.config.InterruptTimeout,
	})
	if err != nil {
		return result, fmt.Errorf("hitl approval request failed: %w", err)
	}

	switch resp.Action {
	case "approve":
		b.logger.Info("decision approved by human", zap.String("task_id", task.ID))
		return result, nil

	case "reject":
		b.logger.Info("decision rejected by human",
			zap.String("task_id", task.ID),
			zap.String("feedback", resp.Feedback),
		)
		return result, ErrDecisionRejected

	case "modify":
		modified, ok := resp.Data.(*Decision)
		if !ok {
			return result, fmt.Errorf("hitl modify response contains invalid decision data")
		}
		b.logger.Info("decision modified by human", zap.String("task_id", task.ID))
		result.Decision = modified
		result.FinalConfidence = modified.Confidence
		return result, nil

	default:
		return result, fmt.Errorf("unknown hitl response action: %s", resp.Action)
	}
}
