package runtimepolicy

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
	"github.com/BaSui01/agentflow/types"
)

type integrationFactory struct {
	provider router.Provider
}

func (f integrationFactory) CreateChatProvider(context.Context, router.ChannelProviderConfig, router.ChannelSecret) (router.Provider, error) {
	return f.provider, nil
}

type integrationProvider struct {
	content         string
	completionErrs  []error
	streamUsage     *router.ChatUsage
	completionUsage *router.ChatUsage
	completionCalls int
	streamCalls     int
}

func (p *integrationProvider) Completion(_ context.Context, req *router.ChatRequest) (*router.ChatResponse, error) {
	p.completionCalls++
	if p.completionCalls <= len(p.completionErrs) && p.completionErrs[p.completionCalls-1] != nil {
		return nil, p.completionErrs[p.completionCalls-1]
	}
	resp := &router.ChatResponse{
		Provider: "integration-provider",
		Model:    req.Model,
		Choices: []router.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: p.content,
			},
		}},
	}
	if p.completionUsage != nil {
		resp.Usage = *p.completionUsage
	}
	return resp, nil
}

func (p *integrationProvider) Stream(_ context.Context, req *router.ChatRequest) (<-chan router.StreamChunk, error) {
	p.streamCalls++
	out := make(chan router.StreamChunk, 1)
	var usage *router.ChatUsage
	if p.streamUsage != nil {
		copied := *p.streamUsage
		usage = &copied
	}
	out <- router.StreamChunk{
		Provider: "integration-provider",
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
		Usage: usage,
	}
	close(out)
	return out, nil
}

func (*integrationProvider) HealthCheck(context.Context) (*router.HealthStatus, error) {
	return &router.HealthStatus{Healthy: true}, nil
}

func (*integrationProvider) Name() string { return "integration-provider" }

func (*integrationProvider) SupportsNativeFunctionCalling() bool { return true }

func (*integrationProvider) ListModels(context.Context) ([]router.Model, error) { return nil, nil }

func (*integrationProvider) Endpoints() router.ProviderEndpoints { return router.ProviderEndpoints{} }

func TestRuntimePolicyHelpers_StreamFlowUpdatesUsageAndQuota(t *testing.T) {
	t.Parallel()

	store := channelstore.NewStaticStore(channelstore.StaticStoreConfig{
		Channels: []channelstore.Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
		}},
		Keys: []channelstore.Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
		}},
		Mappings: []channelstore.ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
		}},
		Secrets: map[string]channelstore.Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	usageRecorder := &InMemoryUsageRecorder{}
	policy := NewInMemoryQuotaPolicy(InMemoryQuotaPolicyConfig{
		KeyLimits: QuotaLimits{DailyRequests: 1, DailyTokens: 20, Concurrency: 1},
		Now: func() time.Time {
			return time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
		},
	})
	provider := &integrationProvider{
		content:     "hello-stream",
		streamUsage: &router.ChatUsage{PromptTokens: 7, CompletionTokens: 5, TotalTokens: 12},
	}

	routed := router.NewChannelRoutedProvider(router.ChannelRoutedProviderOptions{
		ModelResolver:        router.PassthroughModelResolver{},
		ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
		ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{Random: rand.New(rand.NewSource(1))}),
		SecretResolver:       channelstore.StoreSecretResolver{Source: store},
		UsageRecorder:        usageRecorder,
		QuotaPolicy:          policy,
		ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
		Factory:              integrationFactory{provider: provider},
		Logger:               zap.NewNop(),
	})

	stream, err := routed.Stream(context.Background(), &router.ChatRequest{
		Model: "gpt-4o",
		Messages: []router.Message{{
			Role:    router.RoleUser,
			Content: "ping",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.NoError(t, err)

	var chunks []router.StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	require.Len(t, chunks, 1)
	require.Nil(t, chunks[0].Err)
	require.Equal(t, "hello-stream", chunks[0].Delta.Content)

	records := usageRecorder.Snapshot()
	require.Len(t, records, 1)
	require.True(t, records[0].Success)
	require.Equal(t, "key-a", records[0].KeyID)
	require.NotNil(t, records[0].Usage)
	require.Equal(t, 12, records[0].Usage.TotalTokens)

	snapshot := policy.Snapshot()
	require.Equal(t, int64(1), snapshot.KeyCounters["key-a"].Requests)
	require.Equal(t, int64(12), snapshot.KeyCounters["key-a"].Tokens)
	require.Empty(t, snapshot.KeyInflight)

	_, err = routed.Stream(context.Background(), &router.ChatRequest{
		Model: "gpt-4o",
		Messages: []router.Message{{
			Role:    router.RoleUser,
			Content: "again",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "daily request limit")
}

func TestRuntimePolicyHelpers_CompletionFailureActivatesCooldown(t *testing.T) {
	t.Parallel()

	store := channelstore.NewStaticStore(channelstore.StaticStoreConfig{
		Channels: []channelstore.Channel{{
			ID:       "channel-a",
			Provider: "openai",
			BaseURL:  "https://channel.example/v1",
			Region:   "us",
		}},
		Keys: []channelstore.Key{{
			ID:        "key-a",
			ChannelID: "channel-a",
			BaseURL:   "https://key.example/v1",
			Region:    "us",
		}},
		Mappings: []channelstore.ModelMapping{{
			ID:          "mapping-a",
			ChannelID:   "channel-a",
			PublicModel: "gpt-4o",
			RemoteModel: "gpt-4o-upstream",
			Provider:    "openai",
		}},
		Secrets: map[string]channelstore.Secret{
			"key-a": {APIKey: "sk-demo"},
		},
	})

	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	usageRecorder := &InMemoryUsageRecorder{}
	cooldown := &InMemoryCooldownController{
		Decider: FailureCooldownDecider{KeyTTL: time.Hour, ChannelTTL: time.Hour},
		Now: func() time.Time {
			return now
		},
	}
	provider := &integrationProvider{
		completionErrs: []error{errors.New("upstream failed")},
	}

	routed := router.NewChannelRoutedProvider(router.ChannelRoutedProviderOptions{
		ModelResolver:        router.PassthroughModelResolver{},
		ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
		ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{Random: rand.New(rand.NewSource(1))}),
		SecretResolver:       channelstore.StoreSecretResolver{Source: store},
		UsageRecorder:        usageRecorder,
		CooldownController:   cooldown,
		ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
		Factory:              integrationFactory{provider: provider},
		Logger:               zap.NewNop(),
	})

	_, err := routed.Completion(context.Background(), &router.ChatRequest{
		Model: "gpt-4o",
		Messages: []router.Message{{
			Role:    router.RoleUser,
			Content: "ping",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "upstream failed")
	require.Equal(t, 1, provider.completionCalls)

	records := usageRecorder.Snapshot()
	require.Len(t, records, 1)
	require.False(t, records[0].Success)
	require.Equal(t, "key-a", records[0].KeyID)

	snapshot := cooldown.Snapshot()
	require.Contains(t, snapshot.KeyCooldowns, "key-a")
	require.Contains(t, snapshot.ChannelCooldowns, "channel-a")

	_, err = routed.Completion(context.Background(), &router.ChatRequest{
		Model: "gpt-4o",
		Messages: []router.Message{{
			Role:    router.RoleUser,
			Content: "retry",
		}},
		Metadata: map[string]string{"region": "us"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "cooling down")
	require.Equal(t, 1, provider.completionCalls)
}
