package deliberation

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_SetMode(t *testing.T) {
	engine := NewEngine(DefaultDeliberationConfig(), &mockReasoner{}, nil)
	assert.Equal(t, ModeDeliberate, engine.GetMode())

	engine.SetMode(ModeImmediate)
	assert.Equal(t, ModeImmediate, engine.GetMode())

	engine.SetMode(ModeAdaptive)
	assert.Equal(t, ModeAdaptive, engine.GetMode())
}

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

func TestLLMReasoner_Think(t *testing.T) {
	provider := &testLLMProvider{
		completionFn: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Choices: []llm.ChatChoice{
					{Message: types.Message{Content: "analysis\nCONFIDENCE: 0.85"}},
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

func (p *testLLMProvider) Name() string                        { return "test" }
func (p *testLLMProvider) SupportsNativeFunctionCalling() bool { return false }
func (p *testLLMProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}
func (p *testLLMProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
