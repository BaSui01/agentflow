package gateway

import (
	"context"
	"testing"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_Invoke_Chat_XMLFallbackHandledInGateway(t *testing.T) {
	var capturedReq *llmcore.ChatRequest
	provider := &gatewayCaptureProvider{
		supportsNative: false,
		completionFn: func(_ context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
			capturedReq = req
			return &llmcore.ChatResponse{
				Model: "compat-model",
				Choices: []llmcore.ChatChoice{{
					Message:      types.Message{Content: "done"},
					FinishReason: "stop",
				}},
			}, nil
		},
	}
	svc := New(Config{ChatProvider: provider, Logger: zap.NewNop()})
	resp, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llmcore.ChatRequest{
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
	assert.Equal(t, llmcore.ToolCallModeXML, capturedReq.ToolCallMode)
	assert.Nil(t, capturedReq.Tools)
	assert.Contains(t, capturedReq.Messages[0].Content, "<tool_calls>")
}

func TestService_Stream_Chat_XMLFallbackHandledInGateway(t *testing.T) {
	var capturedReq *llmcore.ChatRequest
	provider := &gatewayCaptureProvider{
		supportsNative: false,
		streamFn: func(_ context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
			capturedReq = req
			ch := make(chan llmcore.StreamChunk, 1)
			ch <- llmcore.StreamChunk{Delta: types.Message{Content: "done"}, FinishReason: "stop"}
			close(ch)
			return ch, nil
		},
	}
	svc := New(Config{ChatProvider: provider, Logger: zap.NewNop()})
	ch, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llmcore.ChatRequest{
			Model:    "compat-model",
			Messages: []types.Message{{Role: types.RoleUser, Content: "search docs"}},
			Tools:    []types.ToolSchema{{Name: "search", Description: "search docs"}},
		},
	})
	require.NoError(t, err)
	for range ch {
	}
	require.NotNil(t, capturedReq)
	assert.Equal(t, llmcore.ToolCallModeXML, capturedReq.ToolCallMode)
	assert.Nil(t, capturedReq.Tools)
}

type gatewayCaptureProvider struct {
	supportsNative bool
	completionFn   func(context.Context, *llmcore.ChatRequest) (*llmcore.ChatResponse, error)
	streamFn       func(context.Context, *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error)
}

func (p *gatewayCaptureProvider) Name() string { return "captured-provider" }
func (p *gatewayCaptureProvider) Completion(ctx context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	if p.completionFn != nil {
		return p.completionFn(ctx, req)
	}
	return &llmcore.ChatResponse{}, nil
}
func (p *gatewayCaptureProvider) Stream(ctx context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	ch := make(chan llmcore.StreamChunk)
	close(ch)
	return ch, nil
}
func (p *gatewayCaptureProvider) HealthCheck(context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true}, nil
}
func (p *gatewayCaptureProvider) SupportsNativeFunctionCalling() bool { return p.supportsNative }
func (p *gatewayCaptureProvider) ListModels(context.Context) ([]llmcore.Model, error) {
	return nil, nil
}
func (p *gatewayCaptureProvider) Endpoints() llmcore.ProviderEndpoints {
	return llmcore.ProviderEndpoints{}
}
