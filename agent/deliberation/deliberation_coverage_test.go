package deliberation

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- SetMode ---

func TestEngine_SetMode(t *testing.T) {
	engine := NewEngine(DefaultDeliberationConfig(), &mockReasoner{}, nil)
	assert.Equal(t, ModeDeliberate, engine.GetMode())

	engine.SetMode(ModeImmediate)
	assert.Equal(t, ModeImmediate, engine.GetMode())

	engine.SetMode(ModeAdaptive)
	assert.Equal(t, ModeAdaptive, engine.GetMode())
}

// --- parseConfidence ---

func TestParseConfidence(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected float64
	}{
		{"found", "some text\nCONFIDENCE: 0.85\nmore text", 0.85},
		{"not found", "no confidence here", 0.5},
		{"below zero", "CONFIDENCE: -0.5", 0.0},
		{"above one", "CONFIDENCE: 1.5", 1.0},
		{"invalid number", "CONFIDENCE: abc", 0.5},
		{"case insensitive", "confidence: 0.9", 0.9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseConfidence(tt.content)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

// --- LLMReasoner ---

func TestLLMReasoner_Think(t *testing.T) {
	provider := &testLLMProvider{
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{
					{Message: llm.Message{Content: "analysis\nCONFIDENCE: 0.85"}},
				},
			}, nil
		},
	}
	reasoner := NewLLMReasoner(provider, "test-model", nil)

	content, confidence, err := reasoner.Think(context.Background(), "test prompt")
	require.NoError(t, err)
	assert.Contains(t, content, "analysis")
	assert.InDelta(t, 0.85, confidence, 0.001)
}

func TestLLMReasoner_Think_NoChoices(t *testing.T) {
	provider := &testLLMProvider{
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{Choices: nil}, nil
		},
	}
	reasoner := NewLLMReasoner(provider, "test-model", nil)

	_, _, err := reasoner.Think(context.Background(), "test prompt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

// --- DefaultHITLConfig ---

func TestDefaultHITLConfig(t *testing.T) {
	config := DefaultHITLConfig()
	assert.False(t, config.Enabled)
	assert.Equal(t, 0.7, config.ConfidenceThreshold)
	assert.Equal(t, 5*time.Minute, config.InterruptTimeout)
}

// --- HITLBridge ---

func TestHITLBridge_DeliberateWithApproval_Disabled(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "analysis\nCONFIDENCE: 0.9", 0.9, nil
		},
	}
	engine := NewEngine(DefaultDeliberationConfig(), reasoner, nil)

	config := DefaultHITLConfig()
	config.Enabled = false
	bridge := NewHITLBridge(engine, nil, config, nil)

	result, err := bridge.DeliberateWithApproval(context.Background(), Task{
		ID:          "task1",
		Description: "test task",
		Goal:        "test",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestHITLBridge_DeliberateWithApproval_HighConfidence(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "analysis\nCONFIDENCE: 0.95", 0.95, nil
		},
	}
	engine := NewEngine(DefaultDeliberationConfig(), reasoner, nil)

	config := DefaultHITLConfig()
	config.Enabled = true
	config.ConfidenceThreshold = 0.7
	bridge := NewHITLBridge(engine, nil, config, nil)

	result, err := bridge.DeliberateWithApproval(context.Background(), Task{
		ID:          "task1",
		Description: "test task",
		Goal:        "test",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}
func TestHITLBridge_DeliberateWithApproval_LowConfidence_Approve(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "analysis\nCONFIDENCE: 0.3", 0.3, nil
		},
	}
	config := DefaultDeliberationConfig()
	config.EnableSelfCritique = false
	engine := NewEngine(config, reasoner, nil)

	hitlConfig := HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Second,
	}

	requester := &mockInterruptRequester{
		fn: func(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{Action: "approve"}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, hitlConfig, zap.NewNop())
	result, err := bridge.DeliberateWithApproval(context.Background(), Task{
		ID:          "task1",
		Description: "test",
		Goal:        "test",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestHITLBridge_DeliberateWithApproval_LowConfidence_Reject(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "analysis\nCONFIDENCE: 0.3", 0.3, nil
		},
	}
	config := DefaultDeliberationConfig()
	config.EnableSelfCritique = false
	engine := NewEngine(config, reasoner, nil)

	hitlConfig := HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Second,
	}

	requester := &mockInterruptRequester{
		fn: func(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{Action: "reject", Feedback: "not good"}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, hitlConfig, zap.NewNop())
	_, err := bridge.DeliberateWithApproval(context.Background(), Task{
		ID:          "task1",
		Description: "test",
		Goal:        "test",
	})
	assert.ErrorIs(t, err, ErrDecisionRejected)
}

func TestHITLBridge_DeliberateWithApproval_LowConfidence_Modify(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "analysis\nCONFIDENCE: 0.3", 0.3, nil
		},
	}
	config := DefaultDeliberationConfig()
	config.EnableSelfCritique = false
	engine := NewEngine(config, reasoner, nil)

	hitlConfig := HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Second,
	}

	modifiedDecision := &Decision{
		Action:     "execute",
		Tool:       "modified_tool",
		Confidence: 0.95,
	}

	requester := &mockInterruptRequester{
		fn: func(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{Action: "modify", Data: modifiedDecision}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, hitlConfig, zap.NewNop())
	result, err := bridge.DeliberateWithApproval(context.Background(), Task{
		ID:          "task1",
		Description: "test",
		Goal:        "test",
	})
	require.NoError(t, err)
	assert.Equal(t, "modified_tool", result.Decision.Tool)
	assert.Equal(t, 0.95, result.FinalConfidence)
}

func TestHITLBridge_DeliberateWithApproval_LowConfidence_UnknownAction(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "analysis\nCONFIDENCE: 0.3", 0.3, nil
		},
	}
	config := DefaultDeliberationConfig()
	config.EnableSelfCritique = false
	engine := NewEngine(config, reasoner, nil)

	hitlConfig := HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Second,
	}

	requester := &mockInterruptRequester{
		fn: func(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{Action: "unknown"}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, hitlConfig, zap.NewNop())
	_, err := bridge.DeliberateWithApproval(context.Background(), Task{
		ID:          "task1",
		Description: "test",
		Goal:        "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown hitl response action")
}

func TestHITLBridge_DeliberateWithApproval_ModifyInvalidData(t *testing.T) {
	reasoner := &mockReasoner{
		thinkFn: func(ctx context.Context, prompt string) (string, float64, error) {
			return "analysis\nCONFIDENCE: 0.3", 0.3, nil
		},
	}
	config := DefaultDeliberationConfig()
	config.EnableSelfCritique = false
	engine := NewEngine(config, reasoner, nil)

	hitlConfig := HITLConfig{
		Enabled:             true,
		ConfidenceThreshold: 0.7,
		InterruptTimeout:    time.Second,
	}

	requester := &mockInterruptRequester{
		fn: func(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
			return &ApprovalResponse{Action: "modify", Data: "not a decision"}, nil
		},
	}

	bridge := NewHITLBridge(engine, requester, hitlConfig, zap.NewNop())
	_, err := bridge.DeliberateWithApproval(context.Background(), Task{
		ID:          "task1",
		Description: "test",
		Goal:        "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid decision data")
}

// --- mock types ---

type mockInterruptRequester struct {
	fn func(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error)
}

func (m *mockInterruptRequester) RequestApproval(ctx context.Context, opts ApprovalRequest) (*ApprovalResponse, error) {
	return m.fn(ctx, opts)
}

// testLLMProvider implements llm.Provider for testing
type testLLMProvider struct {
	completionFn func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
}

func (p *testLLMProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return p.completionFn(ctx, req)
}
func (p *testLLMProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, nil
}
func (p *testLLMProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *testLLMProvider) Name() string                                    { return "test" }
func (p *testLLMProvider) SupportsNativeFunctionCalling() bool             { return false }
func (p *testLLMProvider) ListModels(ctx context.Context) ([]llm.Model, error) { return nil, nil }
func (p *testLLMProvider) Endpoints() llm.ProviderEndpoints                { return llm.ProviderEndpoints{} }
