package gateway

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ═══ Invoke 缺失分支测试 ═══

func TestService_Invoke_InvalidCapability(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	_, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: "nonexistent",
		Payload:    "whatever",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nonexistent")
}

func TestService_Invoke_Chat(t *testing.T) {
	mockProv := &gatewayMockChatProvider{}
	svc := New(Config{ChatProvider: mockProv, Logger: zap.NewNop()})
	resp, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	chatResp, ok := resp.Output.(*llm.ChatResponse)
	require.True(t, ok)
	require.Equal(t, "mock reply", chatResp.Choices[0].Message.Content)
}

func TestService_Invoke_Chat_NilProvider(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()}) // no ChatProvider
	_, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test"},
	})
	require.Error(t, err)
}

func TestService_Invoke_Image_Generate(t *testing.T) {
	svc := newImageServiceForTest()
	resp, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityImage,
		Payload: &ImageInput{
			Generate: &image.GenerateRequest{Prompt: "a cat"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &image.GenerateResponse{}, resp.Output)
}

func TestService_Invoke_Image_NilPayload(t *testing.T) {
	svc := newImageServiceForTest()
	_, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityImage,
		Payload:    &ImageInput{}, // both Generate and Edit are nil
	})
	require.Error(t, err)
}

func TestService_Invoke_Video_Generate(t *testing.T) {
	svc := newVideoServiceForTest()
	resp, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityVideo,
		Payload: &VideoInput{
			Generate: &video.GenerateRequest{Prompt: "sunset"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &video.GenerateResponse{}, resp.Output)
}

func TestService_Invoke_Video_NilPayload(t *testing.T) {
	svc := newVideoServiceForTest()
	_, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityVideo,
		Payload:    &VideoInput{}, // Generate is nil
	})
	require.Error(t, err)
}

func TestService_Invoke_Audio_NilPayload(t *testing.T) {
	svc := newCapabilityServiceForTest()
	_, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityAudio,
		Payload:    &AudioInput{}, // both nil
	})
	require.Error(t, err)
}

// ═══ Stream 缺失分支测试 ═══

func TestService_Stream_NilChatProvider(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	_, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test"},
	})
	require.Error(t, err)
}

func TestService_Stream_NonChatCapability(t *testing.T) {
	svc := New(Config{Logger: zap.NewNop()})
	_, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityImage,
		Payload:    &ImageInput{},
	})
	require.Error(t, err)
}

func TestService_Stream_Success(t *testing.T) {
	mockProv := &gatewayMockChatProvider{}
	svc := New(Config{ChatProvider: mockProv, Logger: zap.NewNop()})
	ch, err := svc.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    &llm.ChatRequest{Model: "test", Messages: []types.Message{{Role: "user", Content: "hi"}}},
	})
	require.NoError(t, err)
	require.NotNil(t, ch)
	// drain
	for range ch {
	}
}

// ═══ Mock Providers ═══

type gatewayMockChatProvider struct{}

func (p *gatewayMockChatProvider) Name() string { return "mock-chat" }
func (p *gatewayMockChatProvider) Completion(_ context.Context, _ *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		ID: "resp1", Model: "test",
		Choices: []llm.ChatChoice{{Message: types.Message{Content: "mock reply"}}},
	}, nil
}
func (p *gatewayMockChatProvider) Stream(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk, 2)
	ch <- llm.StreamChunk{Delta: types.Message{Content: "hi"}}
	close(ch)
	return ch, nil
}
func (p *gatewayMockChatProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (p *gatewayMockChatProvider) SupportsNativeFunctionCalling() bool { return true }
func (p *gatewayMockChatProvider) ListModels(_ context.Context) ([]llm.Model, error) { return nil, nil }
func (p *gatewayMockChatProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

type gatewayMockImageProvider struct{}

func (p *gatewayMockImageProvider) Generate(_ context.Context, _ *image.GenerateRequest) (*image.GenerateResponse, error) {
	return &image.GenerateResponse{Provider: "mock-image"}, nil
}
func (p *gatewayMockImageProvider) Edit(_ context.Context, _ *image.EditRequest) (*image.GenerateResponse, error) {
	return &image.GenerateResponse{Provider: "mock-image-edit"}, nil
}
func (p *gatewayMockImageProvider) CreateVariation(_ context.Context, _ *image.VariationRequest) (*image.GenerateResponse, error) {
	return &image.GenerateResponse{}, nil
}
func (p *gatewayMockImageProvider) Name() string          { return "mock-image" }
func (p *gatewayMockImageProvider) SupportedSizes() []string { return []string{"1024x1024"} }

type gatewayMockVideoProvider struct{}

func (p *gatewayMockVideoProvider) Analyze(_ context.Context, _ *video.AnalyzeRequest) (*video.AnalyzeResponse, error) {
	return &video.AnalyzeResponse{}, nil
}
func (p *gatewayMockVideoProvider) Generate(_ context.Context, _ *video.GenerateRequest) (*video.GenerateResponse, error) {
	return &video.GenerateResponse{Provider: "mock-video"}, nil
}
func (p *gatewayMockVideoProvider) Name() string                    { return "mock-video" }
func (p *gatewayMockVideoProvider) SupportedFormats() []video.VideoFormat { return nil }
func (p *gatewayMockVideoProvider) SupportsGeneration() bool        { return true }

// ═══ Service 构造辅助 ═══

func newImageServiceForTest() *Service {
	svc := newCapabilityServiceForTest()
	svc.capabilities.Router().RegisterImage("img", &gatewayMockImageProvider{}, true)
	return svc
}

func newVideoServiceForTest() *Service {
	svc := newCapabilityServiceForTest()
	svc.capabilities.Router().RegisterVideo("vid", &gatewayMockVideoProvider{}, true)
	return svc
}
