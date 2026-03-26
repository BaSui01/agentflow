package channelstore

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/llm/runtime/router/extensions/runtimepolicy"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestChannelStoreAdapters_CompletionFlowThroughChannelRoutedProvider(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
			Metadata: map[string]string{"channel_scope": "team-a"},
			Extra:    map[string]any{"transport": "openai-compatible"},
		}},
		Keys: []Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
			Weight:    10,
		}},
		Mappings: []ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
			Priority:    10,
			Weight:      5,
			Metadata:    map[string]string{"mapping_scope": "premium"},
		}},
		Secrets: map[string]Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	factory := &captureChatProviderFactory{
		provider: &captureChatProvider{content: "hello"},
	}
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: StoreModelMappingResolver{Source: store},
		ChannelSelector:      NewPriorityWeightedSelector(store, SelectorOptions{Random: rand.New(rand.NewSource(1))}),
		SecretResolver:       StoreSecretResolver{Source: store},
		ProviderConfigSource: StoreProviderConfigSource{Channels: store, Extra: map[string]any{"adapter": "channelstore"}},
		Factory:              factory,
		Logger:               zap.NewNop(),
	})

	resp, err := provider.Completion(context.Background(), &llmrouter.ChatRequest{
		Model: "gpt-4o",
		Messages: []llmrouter.Message{{
			Role:    llmrouter.RoleUser,
			Content: "ping",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "hello", resp.Choices[0].Message.Content)
	require.NotNil(t, factory.lastConfig)
	require.NotNil(t, factory.lastSecret)
	require.Equal(t, "openai", factory.lastConfig.Provider)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Equal(t, "channelstore", factory.lastConfig.Extra["adapter"])
	require.Equal(t, "openai-compatible", factory.lastConfig.Extra["transport"])
	require.Equal(t, "sk-demo", factory.lastSecret.APIKey)
	require.Equal(t, "gpt-4o-upstream", factory.captureProvider().lastCompletionModel)
}

func TestChannelStoreAdapters_StreamFlowThroughChannelRoutedProvider(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
		}},
		Keys: []Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
		}},
		Mappings: []ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
		}},
		Secrets: map[string]Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	captureProvider := &captureChatProvider{content: "hello-stream"}
	factory := &captureChatProviderFactory{provider: captureProvider}
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: StoreModelMappingResolver{Source: store},
		ChannelSelector:      NewPriorityWeightedSelector(store, SelectorOptions{Random: rand.New(rand.NewSource(1))}),
		SecretResolver:       StoreSecretResolver{Source: store},
		ProviderConfigSource: StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
	})

	stream, err := provider.Stream(context.Background(), &llmrouter.ChatRequest{
		Model: "gpt-4o",
		Messages: []llmrouter.Message{{
			Role:    llmrouter.RoleUser,
			Content: "ping",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.NoError(t, err)

	chunk, ok := <-stream
	require.True(t, ok)
	require.Nil(t, chunk.Err)
	require.Equal(t, "hello-stream", chunk.Delta.Content)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "sk-demo", factory.lastSecret.APIKey)
	require.Equal(t, "gpt-4o-upstream", captureProvider.lastStreamModel)
}

func TestChannelStoreAdapters_GatewayCompletionPropagatesCallbacksAndUsage(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
		}},
		Keys: []Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
		}},
		Mappings: []ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
		}},
		Secrets: map[string]Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	recorder := &captureUsageRecorder{}
	factory := &captureChatProviderFactory{
		provider: &captureChatProvider{
			content:         "hello-gateway",
			completionUsage: &llmrouter.ChatUsage{PromptTokens: 10, CompletionTokens: 6, TotalTokens: 16},
		},
	}

	var (
		selected []*llmrouter.ChannelSelection
		remapped []*llmrouter.ModelRemapEvent
		used     []*llmrouter.ChannelUsageRecord
	)
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: StoreModelMappingResolver{Source: store},
		ChannelSelector:      NewPriorityWeightedSelector(store, SelectorOptions{Random: rand.New(rand.NewSource(1))}),
		SecretResolver:       StoreSecretResolver{Source: store},
		UsageRecorder:        recorder,
		ProviderConfigSource: StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
		Callbacks: llmrouter.ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *llmrouter.ChannelSelection) {
				selected = append(selected, cloneSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *llmrouter.ModelRemapEvent) {
				remapped = append(remapped, cloneRemapEvent(event))
			},
			OnUsageRecorded: func(_ context.Context, usage *llmrouter.ChannelUsageRecord, _ error) {
				used = append(used, cloneUsageRecord(usage))
			},
		},
	})

	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	})
	resp, err := gateway.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability:  llmcore.CapabilityChat,
		ModelHint:   "gpt-4o",
		RoutePolicy: llmcore.RoutePolicyBalanced,
		Metadata:    map[string]string{"region": "us"},
		Payload: &llmrouter.ChatRequest{
			Model: "gpt-4o",
			Messages: []llmrouter.Message{{
				Role:    llmrouter.RoleUser,
				Content: "ping",
			}},
			Metadata: map[string]string{"region": "us"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "openai", resp.ProviderDecision.Provider)
	require.Equal(t, "gpt-4o-upstream", resp.ProviderDecision.Model)
	require.Equal(t, "https://key.example/v1", resp.ProviderDecision.BaseURL)
	require.Len(t, selected, 1)
	require.Equal(t, "channel-a", selected[0].ChannelID)
	require.Equal(t, "key-a", selected[0].KeyID)
	require.Len(t, remapped, 1)
	require.Equal(t, "gpt-4o", remapped[0].RequestedModel)
	require.Equal(t, "gpt-4o-upstream", remapped[0].RemoteModel)
	require.Len(t, used, 1)
	require.True(t, used[0].Success)
	require.Equal(t, "https://key.example/v1", used[0].BaseURL)
	require.NotNil(t, used[0].Usage)
	require.Equal(t, 16, used[0].Usage.TotalTokens)
	require.Len(t, recorder.recordsSnapshot(), 1)
	require.Equal(t, "key-a", recorder.recordsSnapshot()[0].KeyID)
}

func TestChannelStoreAdapters_GatewayStreamPropagatesCallbacksAndUsage(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
		}},
		Keys: []Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
		}},
		Mappings: []ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
		}},
		Secrets: map[string]Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	recorder := &captureUsageRecorder{}
	factory := &captureChatProviderFactory{
		provider: &captureChatProvider{
			content:     "hello-stream-gateway",
			streamUsage: &llmrouter.ChatUsage{PromptTokens: 8, CompletionTokens: 4, TotalTokens: 12},
		},
	}

	usageCh := make(chan *llmrouter.ChannelUsageRecord, 1)
	var (
		selected []*llmrouter.ChannelSelection
		remapped []*llmrouter.ModelRemapEvent
	)
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: StoreModelMappingResolver{Source: store},
		ChannelSelector:      NewPriorityWeightedSelector(store, SelectorOptions{Random: rand.New(rand.NewSource(1))}),
		SecretResolver:       StoreSecretResolver{Source: store},
		UsageRecorder:        recorder,
		ProviderConfigSource: StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
		Callbacks: llmrouter.ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *llmrouter.ChannelSelection) {
				selected = append(selected, cloneSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *llmrouter.ModelRemapEvent) {
				remapped = append(remapped, cloneRemapEvent(event))
			},
			OnUsageRecorded: func(_ context.Context, usage *llmrouter.ChannelUsageRecord, _ error) {
				usageCh <- cloneUsageRecord(usage)
			},
		},
	})

	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       zap.NewNop(),
	})
	stream, err := gateway.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability:  llmcore.CapabilityChat,
		ModelHint:   "gpt-4o",
		RoutePolicy: llmcore.RoutePolicyBalanced,
		Metadata:    map[string]string{"region": "us"},
		Payload: &llmrouter.ChatRequest{
			Model: "gpt-4o",
			Messages: []llmrouter.Message{{
				Role:    llmrouter.RoleUser,
				Content: "ping",
			}},
			Metadata: map[string]string{"region": "us"},
		},
	})
	require.NoError(t, err)

	var chunks []llmcore.UnifiedChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	require.NotEmpty(t, chunks)
	require.Equal(t, "openai", chunks[0].ProviderDecision.Provider)
	require.Equal(t, "gpt-4o-upstream", chunks[0].ProviderDecision.Model)
	require.Equal(t, "https://key.example/v1", chunks[0].ProviderDecision.BaseURL)
	require.Len(t, selected, 1)
	require.Equal(t, "key-a", selected[0].KeyID)
	require.Len(t, remapped, 1)
	require.Equal(t, "gpt-4o-upstream", remapped[0].RemoteModel)

	usage := <-usageCh
	require.NotNil(t, usage)
	require.True(t, usage.Success)
	require.Equal(t, "key-a", usage.KeyID)
	require.NotNil(t, usage.Usage)
	require.Equal(t, 12, usage.Usage.TotalTokens)
	require.Len(t, recorder.recordsSnapshot(), 1)
	require.Equal(t, 12, recorder.recordsSnapshot()[0].Usage.TotalTokens)
}

func TestComposeChannelRoutedProviderConfig_IntegratesRuntimePolicies(t *testing.T) {
	t.Parallel()

	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
		}},
		Keys: []Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
		}},
		Mappings: []ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
		}},
		Secrets: map[string]Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	usageRecorder := &runtimepolicy.InMemoryUsageRecorder{}
	quotaPolicy := runtimepolicy.NewInMemoryQuotaPolicy(runtimepolicy.InMemoryQuotaPolicyConfig{
		KeyLimits: runtimepolicy.QuotaLimits{DailyRequests: 1, DailyTokens: 20, Concurrency: 1},
		Now: func() time.Time {
			return time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		},
	})
	factory := &captureChatProviderFactory{
		provider: &captureChatProvider{
			content:         "hello-composed",
			completionUsage: &llmrouter.ChatUsage{PromptTokens: 4, CompletionTokens: 3, TotalTokens: 7},
		},
	}

	routedConfig, err := ComposeChannelRoutedProviderConfig(RoutedProviderOptions{
		Name:                "channelstore-composed",
		Store:               store,
		UsageRecorder:       usageRecorder,
		QuotaPolicy:         quotaPolicy,
		ChatProviderFactory: factory,
		Logger:              zap.NewNop(),
	})
	require.NoError(t, err)

	provider, err := llmrouter.BuildChannelRoutedProvider(routedConfig)
	require.NoError(t, err)
	require.Equal(t, "channelstore-composed", provider.Name())

	resp, err := provider.Completion(context.Background(), &llmrouter.ChatRequest{
		Model: "gpt-4o",
		Messages: []llmrouter.Message{{
			Role:    llmrouter.RoleUser,
			Content: "ping",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "hello-composed", resp.Choices[0].Message.Content)
	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "openai", factory.lastConfig.Provider)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)

	records := usageRecorder.Snapshot()
	require.Len(t, records, 1)
	require.True(t, records[0].Success)
	require.Equal(t, "key-a", records[0].KeyID)
	require.Equal(t, "gpt-4o", records[0].RequestedModel)
	require.Equal(t, "gpt-4o-upstream", records[0].RemoteModel)
	require.Equal(t, "https://key.example/v1", records[0].BaseURL)
	require.NotNil(t, records[0].Usage)
	require.Equal(t, 7, records[0].Usage.TotalTokens)

	quotaSnapshot := quotaPolicy.Snapshot()
	require.Equal(t, int64(1), quotaSnapshot.KeyCounters["key-a"].Requests)
	require.Equal(t, int64(7), quotaSnapshot.KeyCounters["key-a"].Tokens)

	_, err = provider.Completion(context.Background(), &llmrouter.ChatRequest{
		Model: "gpt-4o",
		Messages: []llmrouter.Message{{
			Role:    llmrouter.RoleUser,
			Content: "again",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "daily request limit")
}

type captureChatProviderFactory struct {
	lastConfig *llmrouter.ChannelProviderConfig
	lastSecret *llmrouter.ChannelSecret
	provider   llmrouter.Provider
}

func (f *captureChatProviderFactory) CreateChatProvider(_ context.Context, config llmrouter.ChannelProviderConfig, secret llmrouter.ChannelSecret) (llmrouter.Provider, error) {
	configCopy := config
	secretCopy := secret
	f.lastConfig = &configCopy
	f.lastSecret = &secretCopy
	return f.provider, nil
}

func (f *captureChatProviderFactory) captureProvider() *captureChatProvider {
	provider, _ := f.provider.(*captureChatProvider)
	return provider
}

type captureChatProvider struct {
	content             string
	lastCompletionModel string
	lastStreamModel     string
	completionUsage     *llmrouter.ChatUsage
	streamUsage         *llmrouter.ChatUsage
}

func (p *captureChatProvider) Completion(_ context.Context, req *llmrouter.ChatRequest) (*llmrouter.ChatResponse, error) {
	p.lastCompletionModel = req.Model
	var usage llmrouter.ChatUsage
	if p.completionUsage != nil {
		usage = *p.completionUsage
	}
	return &llmrouter.ChatResponse{
		Provider: "capture-chat-provider",
		Model:    req.Model,
		Usage:    usage,
		Choices: []llmrouter.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: p.content,
			},
		}},
	}, nil
}

func (p *captureChatProvider) Stream(_ context.Context, req *llmrouter.ChatRequest) (<-chan llmrouter.StreamChunk, error) {
	p.lastStreamModel = req.Model
	out := make(chan llmrouter.StreamChunk, 1)
	var usage *llmrouter.ChatUsage
	if p.streamUsage != nil {
		copied := *p.streamUsage
		usage = &copied
	}
	out <- llmrouter.StreamChunk{
		Provider: "capture-chat-provider",
		Model:    req.Model,
		Usage:    usage,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
	}
	close(out)
	return out, nil
}

func (*captureChatProvider) HealthCheck(context.Context) (*llmrouter.HealthStatus, error) {
	return &llmrouter.HealthStatus{Healthy: true}, nil
}

func (*captureChatProvider) Name() string { return "capture-chat-provider" }

func (*captureChatProvider) SupportsNativeFunctionCalling() bool { return true }

func (*captureChatProvider) ListModels(context.Context) ([]llmrouter.Model, error) { return nil, nil }

func (*captureChatProvider) Endpoints() llmrouter.ProviderEndpoints {
	return llmrouter.ProviderEndpoints{}
}

type captureUsageRecorder struct {
	mu      sync.Mutex
	records []*llmrouter.ChannelUsageRecord
}

func (r *captureUsageRecorder) RecordUsage(_ context.Context, usage *llmrouter.ChannelUsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, cloneUsageRecord(usage))
	return nil
}

func (r *captureUsageRecorder) recordsSnapshot() []*llmrouter.ChannelUsageRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*llmrouter.ChannelUsageRecord, 0, len(r.records))
	for _, record := range r.records {
		out = append(out, cloneUsageRecord(record))
	}
	return out
}

func cloneSelection(selection *llmrouter.ChannelSelection) *llmrouter.ChannelSelection {
	if selection == nil {
		return nil
	}
	cloned := *selection
	if len(selection.Metadata) != 0 {
		cloned.Metadata = make(map[string]string, len(selection.Metadata))
		for key, value := range selection.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return &cloned
}

func cloneRemapEvent(event *llmrouter.ModelRemapEvent) *llmrouter.ModelRemapEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	return &cloned
}

func cloneUsageRecord(record *llmrouter.ChannelUsageRecord) *llmrouter.ChannelUsageRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	if record.Usage != nil {
		usage := *record.Usage
		cloned.Usage = &usage
	}
	if len(record.Metadata) != 0 {
		cloned.Metadata = make(map[string]string, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return &cloned
}
