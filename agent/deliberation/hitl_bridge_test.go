package deliberation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test double (function callback pattern, §30) ---

type mockRequester struct {
	requestFn func(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error)
}

func (m *mockRequester) RequestApproval(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
	if m.requestFn != nil {
		return m.requestFn(ctx, opts)
	}
	return nil, fmt.Errorf("mockRequester: no requestFn configured")
}

// helper: build an engine that always returns the given confidence.
func newTestEngine(confidence float64) *Engine {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return fmt.Sprintf("reasoning\nCONFIDENCE: %.2f", confidence), confidence, nil
		},
	}
	cfg := DefaultDeliberationConfig()
	cfg.Mode = ModeDeliberate
	cfg.MinConfidence = 0.3 // low so engine always converges
	cfg.MaxIterations = 1
	return NewEngine(cfg, reasoner, nil)
}

// --- tests ---

func TestHITLBridge_HighConfidence_NoEscalation(t *testing.T) {
	engine := newTestEngine(0.95)
	called := false
	requester := &mockRequester{
		requestFn: func(_ context.Context, _ ApprovalRequest) (*ApprovalResponse, error) {
			called = true
			return nil, fmt.Errorf("should not be called")
		},
	}

	bridge := NewHITLBridge(engine, requester, HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Minute,
	}, nil)

	task := Task{ID: "t1", Description: "high confidence task"}
	result, err := bridge.DeliberateWithApproval(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.False(t, called, "requester should not be called for high-confidence results")
	assert.GreaterOrEqual(t, result.FinalConfidence, 0.7)
}

func TestHITLBridge_LowConfidence_Approved(t *testing.T) {
	engine := newTestEngine(0.4)
	requester := &mockRequester{
		requestFn: func(_ context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
			assert.Equal(t, "Low-confidence decision requires approval", opts.Title)
			assert.NotNil(t, opts.Data)
			return &ApprovalResponse{Action: "approve", Feedback: "looks fine"}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Minute,
	}, nil)

	task := Task{ID: "t2", Description: "low confidence task"}
	result, err := bridge.DeliberateWithApproval(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, result.Decision)
}

func TestHITLBridge_LowConfidence_Rejected(t *testing.T) {
	engine := newTestEngine(0.4)
	requester := &mockRequester{
		requestFn: func(_ context.Context, _ ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{Action: "reject", Feedback: "too risky"}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Minute,
	}, nil)

	task := Task{ID: "t3", Description: "will be rejected"}
	result, err := bridge.DeliberateWithApproval(context.Background(), task)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecisionRejected))
	require.NotNil(t, result) // result is still returned alongside the error
}

func TestHITLBridge_LowConfidence_Modified(t *testing.T) {
	engine := newTestEngine(0.4)
	modifiedDecision := &Decision{
		Action:     "execute",
		Tool:       "browse",
		Reasoning:  "human corrected the tool choice",
		Confidence: 0.99,
	}
	requester := &mockRequester{
		requestFn: func(_ context.Context, _ ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{
				Action: "modify",
				Data:   modifiedDecision,
			}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Minute,
	}, nil)

	task := Task{ID: "t4", Description: "will be modified"}
	result, err := bridge.DeliberateWithApproval(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.Equal(t, "browse", result.Decision.Tool)
	assert.Equal(t, 0.99, result.FinalConfidence)
}

func TestHITLBridge_Timeout(t *testing.T) {
	engine := newTestEngine(0.4)
	requester := &mockRequester{
		requestFn: func(_ context.Context, _ ApprovalRequest) (*ApprovalResponse, error) {
			return nil, fmt.Errorf("interrupt timeout: int_123")
		},
	}

	bridge := NewHITLBridge(engine, requester, HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Millisecond,
	}, nil)

	task := Task{ID: "t5", Description: "will timeout"}
	_, err := bridge.DeliberateWithApproval(context.Background(), task)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestHITLBridge_Disabled_ReturnsDirectly(t *testing.T) {
	engine := newTestEngine(0.4)
	called := false
	requester := &mockRequester{
		requestFn: func(_ context.Context, _ ApprovalRequest) (*ApprovalResponse, error) {
			called = true
			return nil, fmt.Errorf("should not be called")
		},
	}

	bridge := NewHITLBridge(engine, requester, HITLConfig{
		Enabled:             false, // disabled
		ConfidenceThreshold: 0.7,
	}, nil)

	task := Task{ID: "t6", Description: "hitl disabled"}
	result, err := bridge.DeliberateWithApproval(context.Background(), task)

	require.NoError(t, err)
	require.NotNil(t, result.Decision)
	assert.False(t, called, "requester should not be called when HITL is disabled")
}

func TestHITLBridge_ModifyWithInvalidData(t *testing.T) {
	engine := newTestEngine(0.4)
	requester := &mockRequester{
		requestFn: func(_ context.Context, _ ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{
				Action: "modify",
				Data:   "not a *Decision", // wrong type
			}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Minute,
	}, nil)

	task := Task{ID: "t7", Description: "bad modify data"}
	_, err := bridge.DeliberateWithApproval(context.Background(), task)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid decision data")
}

