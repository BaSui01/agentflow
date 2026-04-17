package router

import (
	"context"
	"strings"
	"testing"
	"time"

	llmroot "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

type builderMappingResolver struct {
	mappings []ChannelModelMapping
}

func (r builderMappingResolver) ResolveMappings(_ context.Context, _ *ChannelRouteRequest, _ *ModelResolution) ([]ChannelModelMapping, error) {
	return r.mappings, nil
}

type builderSelector struct {
	selection ChannelSelection
}

func (s builderSelector) SelectChannel(_ context.Context, _ *ChannelRouteRequest, _ *ModelResolution, _ []ChannelModelMapping) (*ChannelSelection, error) {
	selection := s.selection
	return &selection, nil
}

type builderLegacyFactory struct {
	lastProvider string
	lastAPIKey   string
	lastBaseURL  string
	callCount    int
	provider     Provider
}

func (f *builderLegacyFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error) {
	f.lastProvider = providerCode
	f.lastAPIKey = apiKey
	f.lastBaseURL = baseURL
	f.callCount++
	return f.provider, nil
}

type builderProvider struct {
	lastCompletionModel string
	lastCountModel      string
}

func (p *builderProvider) Completion(_ context.Context, req *ChatRequest) (*ChatResponse, error) {
	p.lastCompletionModel = req.Model
	return &ChatResponse{
		Provider: "legacy-provider",
		Model:    req.Model,
		Usage: ChatUsage{
			PromptTokens:     1,
			CompletionTokens: 1,
			TotalTokens:      2,
		},
	}, nil
}

func (*builderProvider) Stream(_ context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Provider: "legacy-provider", Model: req.Model}
	close(ch)
	return ch, nil
}

func (*builderProvider) HealthCheck(context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (*builderProvider) Name() string { return "legacy-provider" }

func (*builderProvider) SupportsNativeFunctionCalling() bool { return true }

func (*builderProvider) ListModels(context.Context) ([]Model, error) { return nil, nil }

func (*builderProvider) Endpoints() ProviderEndpoints { return ProviderEndpoints{} }

func (p *builderProvider) CountTokens(_ context.Context, req *ChatRequest) (*llmroot.TokenCountResponse, error) {
	p.lastCountModel = req.Model
	return &llmroot.TokenCountResponse{InputTokens: len(req.Messages) + 1}, nil
}

type builderSecretResolver struct{}

func (builderSecretResolver) ResolveSecret(_ context.Context, _ *ChannelRouteRequest, _ *ChannelSelection) (*ChannelSecret, error) {
	return &ChannelSecret{APIKey: "sk-builder"}, nil
}

type builderChatFactory struct {
	callCount int
	provider  Provider
}

func (f *builderChatFactory) CreateChatProvider(_ context.Context, _ ChannelProviderConfig, _ ChannelSecret) (Provider, error) {
	f.callCount++
	return f.provider, nil
}

func TestBuildChannelRoutedProvider_RequiresMappingResolver(t *testing.T) {
	t.Parallel()

	_, err := BuildChannelRoutedProvider(ChannelRoutedProviderConfig{
		ChannelSelector: builderSelector{},
	})
	if err == nil || !strings.Contains(err.Error(), "model mapping resolver") {
		t.Fatalf("expected missing model mapping resolver error, got %v", err)
	}
}

func TestBuildChannelRoutedProvider_RequiresChannelSelector(t *testing.T) {
	t.Parallel()

	_, err := BuildChannelRoutedProvider(ChannelRoutedProviderConfig{
		ModelMappingResolver: builderMappingResolver{},
	})
	if err == nil || !strings.Contains(err.Error(), "channel selector") {
		t.Fatalf("expected missing channel selector error, got %v", err)
	}
}

func TestBuildChannelRoutedProvider_UsesLegacyFactoryAdapter(t *testing.T) {
	t.Parallel()

	inner := &builderProvider{}
	legacyFactory := &builderLegacyFactory{provider: inner}
	provider, err := BuildChannelRoutedProvider(ChannelRoutedProviderConfig{
		Name:                 "channel-chain",
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: builderMappingResolver{},
		ChannelSelector: builderSelector{selection: ChannelSelection{
			Provider:    "openai",
			BaseURL:     "https://example.com/v1",
			RemoteModel: "gpt-upstream",
		}},
		SecretResolver:        builderSecretResolver{},
		ProviderConfigSource:  StaticProviderConfigSource{},
		LegacyProviderFactory: legacyFactory,
		Logger:                zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	resp, err := provider.Completion(context.Background(), &ChatRequest{
		Model: "gpt-public",
	})
	if err != nil {
		t.Fatalf("Completion error: %v", err)
	}

	if resp.Provider != "legacy-provider" {
		t.Fatalf("expected legacy provider response, got %s", resp.Provider)
	}
	if inner.lastCompletionModel != "gpt-upstream" {
		t.Fatalf("expected routed upstream model gpt-upstream, got %s", inner.lastCompletionModel)
	}
	if legacyFactory.lastProvider != "openai" {
		t.Fatalf("expected legacy factory provider openai, got %s", legacyFactory.lastProvider)
	}
	if legacyFactory.lastAPIKey != "sk-builder" {
		t.Fatalf("expected legacy factory api key sk-builder, got %s", legacyFactory.lastAPIKey)
	}
	if legacyFactory.lastBaseURL != "https://example.com/v1" {
		t.Fatalf("expected legacy factory baseURL https://example.com/v1, got %s", legacyFactory.lastBaseURL)
	}
}

func TestBuildChannelRoutedProvider_DefaultsToVendorFactory(t *testing.T) {
	t.Parallel()

	provider, err := BuildChannelRoutedProvider(ChannelRoutedProviderConfig{
		ModelMappingResolver: builderMappingResolver{},
		ChannelSelector:      builderSelector{},
		ProviderTimeout:      3 * time.Second,
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	factory, ok := provider.factory.(VendorChatProviderFactory)
	if !ok {
		t.Fatalf("expected VendorChatProviderFactory, got %T", provider.factory)
	}
	if factory.Timeout != 3*time.Second {
		t.Fatalf("expected vendor factory timeout 3s, got %s", factory.Timeout)
	}
}

func TestBuildChannelRoutedProvider_ExplicitChatFactoryOverridesLegacyFactory(t *testing.T) {
	t.Parallel()

	chatFactoryProvider := &builderProvider{}
	chatFactory := &builderChatFactory{provider: chatFactoryProvider}
	legacyFactory := &builderLegacyFactory{provider: &builderProvider{}}

	provider, err := BuildChannelRoutedProvider(ChannelRoutedProviderConfig{
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: builderMappingResolver{},
		ChannelSelector: builderSelector{selection: ChannelSelection{
			Provider:    "openai",
			BaseURL:     "https://example.com/v1",
			RemoteModel: "gpt-upstream",
		}},
		SecretResolver:        builderSecretResolver{},
		ProviderConfigSource:  StaticProviderConfigSource{},
		ChatProviderFactory:   chatFactory,
		LegacyProviderFactory: legacyFactory,
		Logger:                zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if _, err := provider.Completion(context.Background(), &ChatRequest{Model: "gpt-public"}); err != nil {
		t.Fatalf("Completion error: %v", err)
	}

	if chatFactory.callCount != 1 {
		t.Fatalf("expected explicit chat factory to be called once, got %d", chatFactory.callCount)
	}
	if legacyFactory.callCount != 0 {
		t.Fatalf("expected legacy factory to be bypassed, got %d calls", legacyFactory.callCount)
	}
	if chatFactoryProvider.lastCompletionModel != "gpt-upstream" {
		t.Fatalf("expected explicit chat factory provider to receive upstream model, got %s", chatFactoryProvider.lastCompletionModel)
	}
}

func TestBuildChannelRoutedProvider_PropagatesRetryPolicy(t *testing.T) {
	t.Parallel()

	provider, err := BuildChannelRoutedProvider(ChannelRoutedProviderConfig{
		RetryPolicy:          ChannelRouteRetryPolicy{MaxAttempts: 2, ExcludeFailedChannel: true},
		ModelMappingResolver: builderMappingResolver{},
		ChannelSelector:      builderSelector{},
	})
	if err != nil {
		t.Fatalf("BuildChannelRoutedProvider error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.retryPolicy.MaxAttempts != 2 {
		t.Fatalf("expected retry maxAttempts=2, got %d", provider.retryPolicy.MaxAttempts)
	}
	if !provider.retryPolicy.ExcludeFailedChannel {
		t.Fatalf("expected retry policy to exclude failed channel")
	}
}

func TestBuildChannelRoutedProvider_CountTokensRoutesUpstreamModel(t *testing.T) {
	t.Parallel()

	inner := &builderProvider{}
	provider, err := BuildChannelRoutedProvider(ChannelRoutedProviderConfig{
		Name:                 "channel-chain",
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: builderMappingResolver{},
		ChannelSelector: builderSelector{selection: ChannelSelection{
			Provider:    "openai",
			BaseURL:     "https://example.com/v1",
			RemoteModel: "gpt-upstream",
		}},
		SecretResolver:       builderSecretResolver{},
		ProviderConfigSource: StaticProviderConfigSource{},
		ChatProviderFactory:  &builderChatFactory{provider: inner},
		Logger:               zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	resp, err := provider.CountTokens(context.Background(), &ChatRequest{
		Model: "gpt-public",
	})
	if err != nil {
		t.Fatalf("CountTokens error: %v", err)
	}
	if resp == nil || resp.InputTokens != 1 {
		t.Fatalf("expected token count response, got %+v", resp)
	}
	if inner.lastCountModel != "gpt-upstream" {
		t.Fatalf("expected routed upstream model gpt-upstream, got %s", inner.lastCountModel)
	}
}
