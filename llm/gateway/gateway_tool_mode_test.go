package gateway

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_Invoke_Chat_XMLFallbackHandledInGateway(t *testing.T) {
	var capturedReq *llm.ChatRequest
	provider := &gatewayCaptureProvider{
		supportsNative: false,
		completionFn: func(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedReq = req
			return &llm.ChatResponse{
				Model: "compat-model",
				Choices: []llm.ChatChoice{{
					Message:      types.Message{Content: "done"},
					FinishReason: "stop",
				}},
			}, nil
		},
	}
	svc := New(Config{ChatProvider: provider, Logger: zap.NewNop()})
	resp, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llm.ChatRequest{
			Model:    "compat-model",
			Messages: []types.Message{{Role: types.RoleUser, Content: "search docs"}},
			Tools: []types.ToolSchema{{
				Name:        "search",
				Description: "search docs",
			}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, capturedReq)
	assert.Equal(t, llm.ToolCallModeXML, capturedReq.ToolCallMode)
	assert.Nil(t, capturedReq.Tools)
	assert.Contains(t, capturedReq.Messages[0].Content, "<tool_calls>")
}

func TestService_Stream_Chat_XMLFallbackHandledInGateway(t *testing.T) {
	var capturedReq *llm.ChatRequest
	provider := &gatewayCaptureProvider{
		supportsNative: false,
		streamFn: func(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			capturedReq = req
			ch := make(chan llm.StreamChunk, 1)
			ch <- llm.StreamChunk{Delta: types.Message{Content: "done"}, FinishReason: "stop"}
			close(ch)
			return ch, nil
		},
	}
	svc := New(Config{ChatProvider: provider, Logger: zap.NewNop()})
	ch, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llm.ChatRequest{
			Model:    "compat-model",
			Messages: []types.Message{{Role: types.RoleUser, Content: "search docs"}},
			Tools:    []types.ToolSchema{{Name: "search", Description: "search docs"}},
		},
	})
	require.NoError(t, err)
	for range ch {
	}
	require.NotNil(t, capturedReq)
	assert.Equal(t, llm.ToolCallModeXML, capturedReq.ToolCallMode)
	assert.Nil(t, capturedReq.Tools)
}

type gatewayCaptureProvider struct {
	supportsNative bool
	completionFn   func(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn       func(context.Context, *llm.ChatRequest) (<-chan llm.StreamChunk, error)
}

func (p *gatewayCaptureProvider) Name() string { return "captured-provider" }
func (p *gatewayCaptureProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return &llm.ChatResponse{}, nil
}
func (p *gatewayCaptureProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}
func (p *gatewayCaptureProvider) HealthCheck(context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *gatewayCaptureProvider) SupportsNativeFunctionCalling() bool { return p.supportsNative }
func (p *gatewayCaptureProvider) ListModels(context.Context) ([]llm.Model, error) {
	return nil, nil
}
func (p *gatewayCaptureProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }
