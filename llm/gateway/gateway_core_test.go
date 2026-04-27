package gateway

import (
	"context"
	"testing"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_ChatProvider_NilService(t *testing.T) {
	var s *Service
	assert.Nil(t, s.ChatProvider())
}

func TestService_ChatProvider_Set(t *testing.T) {
	provider := &stubProvider{}
	s := New(Config{ChatProvider: provider, Logger: zap.NewNop()})
	assert.Equal(t, provider, s.ChatProvider())
}

func TestService_New_NilLogger(t *testing.T) {
	provider := &stubProvider{}
	s := New(Config{ChatProvider: provider})
	require.NotNil(t, s)
	assert.Equal(t, provider, s.ChatProvider())
}

func TestService_Invoke_ChatCapability(t *testing.T) {
	provider := &stubProvider{}
	s := New(Config{ChatProvider: provider, Logger: zap.NewNop()})

	resp, err := s.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llmcore.ChatRequest{
			Model:    "test-model",
			Messages: []types.Message{{Role: types.RoleUser, Content: "hello"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestService_Invoke_UnknownCapability(t *testing.T) {
	provider := &stubProvider{}
	s := New(Config{ChatProvider: provider, Logger: zap.NewNop()})

	_, err := s.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.Capability("unknown"),
	})
	require.Error(t, err)
}

func TestService_Stream_NilPayload_2(t *testing.T) {
	provider := &stubProvider{}
	s := New(Config{ChatProvider: provider, Logger: zap.NewNop()})

	_, err := s.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    nil,
	})
	require.Error(t, err)
}

func TestService_Stream_WrongCapability(t *testing.T) {
	provider := &stubProvider{}
	s := New(Config{ChatProvider: provider, Logger: zap.NewNop()})

	_, err := s.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityEmbedding,
	})
	require.Error(t, err)
}

func TestService_Stream_ChatOK(t *testing.T) {
	provider := &stubProvider{}
	s := New(Config{ChatProvider: provider, Logger: zap.NewNop()})

	ch, err := s.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload: &llmcore.ChatRequest{
			Model:    "test-model",
			Messages: []types.Message{{Role: types.RoleUser, Content: "hello"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, ch)

	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
	}
}

func TestChatProviderAdapter_Gateway_Nil(t *testing.T) {
	var a *ChatProviderAdapter
	assert.Nil(t, a.Gateway())
}

func TestChatProviderAdapter_Gateway_Set(t *testing.T) {
	provider := &stubProvider{}
	gw := New(Config{ChatProvider: provider, Logger: zap.NewNop()})
	a := NewChatProviderAdapter(gw, nil)
	assert.Equal(t, gw, a.Gateway())
}

type stubProvider struct{}

func (p *stubProvider) Completion(_ context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	return &llmcore.ChatResponse{
		Model: req.Model,
		Choices: []llmcore.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: "stub response",
			},
		}},
	}, nil
}

func (p *stubProvider) Stream(_ context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	ch := make(chan llmcore.StreamChunk, 1)
	ch <- llmcore.StreamChunk{
		Model: req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: "stub stream",
		},
	}
	close(ch)
	return ch, nil
}

func (p *stubProvider) HealthCheck(_ context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true, Latency: time.Millisecond}, nil
}

func (p *stubProvider) Name() string                        { return "stub" }
func (p *stubProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *stubProvider) ListModels(_ context.Context) ([]llmcore.Model, error) {
	return []llmcore.Model{{ID: "stub-model"}}, nil
}
func (p *stubProvider) Endpoints() llmcore.ProviderEndpoints {
	return llmcore.ProviderEndpoints{}
}
