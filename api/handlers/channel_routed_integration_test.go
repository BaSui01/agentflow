package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	"github.com/BaSui01/agentflow/internal/usecase"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newChatHandlerFromProvider(provider llm.Provider, logger *zap.Logger) *handlers.ChatHandler {
	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider: provider,
		Logger:       logger,
	})
	chatProvider := llmgateway.NewChatProviderAdapter(gateway, provider)
	service, err := usecase.NewDefaultChatService(
		usecase.ChatRuntime{
			Gateway:      gateway,
			ChatProvider: chatProvider,
		},
		handlers.NewUsecaseChatConverter(handlers.NewDefaultChatConverter(30*time.Second)),
		logger,
	)
	if err != nil {
		panic(err)
	}
	handler, err := handlers.NewChatHandler(service, logger)
	if err != nil {
		panic(err)
	}
	return handler
}

func TestChatEndpoints_ChannelRoutedProviderCompletion(t *testing.T) {
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

	recorder := &channelRouteUsageRecorder{}
	factory := &channelRouteFactory{
		provider: &channelRouteProvider{
			content:         "hello-channel-http",
			completionUsage: &llmrouter.ChatUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20},
		},
	}

	var (
		selected []*llmrouter.ChannelSelection
		remapped []*llmrouter.ModelRemapEvent
		used     []*llmrouter.ChannelUsageRecord
	)
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
		ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{}),
		SecretResolver:       channelstore.StoreSecretResolver{Source: store},
		UsageRecorder:        recorder,
		ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
		Callbacks: llmrouter.ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *llmrouter.ChannelSelection) {
				selected = append(selected, cloneRouteSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *llmrouter.ModelRemapEvent) {
				remapped = append(remapped, cloneRouteRemapEvent(event))
			},
			OnUsageRecorded: func(_ context.Context, usage *llmrouter.ChannelUsageRecord, _ error) {
				used = append(used, cloneRouteUsageRecord(usage))
			},
		},
	})

	mux := http.NewServeMux()
	routes.RegisterChat(mux, newChatHandlerFromProvider(provider, zap.NewNop()), zap.NewNop())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"metadata": {"region": "us"}
	}`
	resp, err := http.Post(srv.URL+"/api/v1/chat/completions", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var envelope api.Response
	mustDecodeJSON(t, readBody(t, resp), &envelope)
	require.True(t, envelope.Success)

	dataBytes, err := json.Marshal(envelope.Data)
	require.NoError(t, err)

	var chatResp api.ChatResponse
	mustDecodeJSON(t, dataBytes, &chatResp)
	require.Equal(t, "openai", chatResp.Provider)
	require.Equal(t, "gpt-4o-upstream", chatResp.Model)
	require.Equal(t, "hello-channel-http", chatResp.Choices[0].Message.Content)

	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Equal(t, "sk-demo", factory.lastSecret.APIKey)
	require.Len(t, selected, 1)
	require.Equal(t, "key-a", selected[0].KeyID)
	require.Len(t, remapped, 1)
	require.Equal(t, "gpt-4o-upstream", remapped[0].RemoteModel)
	require.Len(t, used, 1)
	require.True(t, used[0].Success)
	require.Equal(t, 20, used[0].Usage.TotalTokens)
	require.Len(t, recorder.snapshot(), 1)
	require.Equal(t, "https://key.example/v1", recorder.snapshot()[0].BaseURL)
}

func TestChatEndpoints_ChannelRoutedProviderStream(t *testing.T) {
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

	recorder := &channelRouteUsageRecorder{}
	factory := &channelRouteFactory{
		provider: &channelRouteProvider{
			content:     "hello-channel-stream",
			streamUsage: &llmrouter.ChatUsage{PromptTokens: 7, CompletionTokens: 5, TotalTokens: 12},
		},
	}

	var (
		selected []*llmrouter.ChannelSelection
		remapped []*llmrouter.ModelRemapEvent
		used     []*llmrouter.ChannelUsageRecord
	)
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
		ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{}),
		SecretResolver:       channelstore.StoreSecretResolver{Source: store},
		UsageRecorder:        recorder,
		ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
		Callbacks: llmrouter.ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *llmrouter.ChannelSelection) {
				selected = append(selected, cloneRouteSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *llmrouter.ModelRemapEvent) {
				remapped = append(remapped, cloneRouteRemapEvent(event))
			},
			OnUsageRecorded: func(_ context.Context, usage *llmrouter.ChannelUsageRecord, _ error) {
				used = append(used, cloneRouteUsageRecord(usage))
			},
		},
	})

	mux := http.NewServeMux()
	routes.RegisterChat(mux, newChatHandlerFromProvider(provider, zap.NewNop()), zap.NewNop())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"metadata": {"region": "us"}
	}`
	resp, err := http.Post(srv.URL+"/api/v1/chat/completions/stream", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"))

	bodyBytes := readBody(t, resp)
	require.Contains(t, string(bodyBytes), "hello-channel-stream")
	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Len(t, selected, 1)
	require.Equal(t, "key-a", selected[0].KeyID)
	require.Len(t, remapped, 1)
	require.Equal(t, "gpt-4o-upstream", remapped[0].RemoteModel)
	require.Len(t, used, 1)
	require.True(t, used[0].Success)
	require.Equal(t, 12, used[0].Usage.TotalTokens)
	require.Len(t, recorder.snapshot(), 1)
	require.Equal(t, 12, recorder.snapshot()[0].Usage.TotalTokens)
}

func TestChatEndpoints_ChannelRoutedProviderOpenAICompatCompletion(t *testing.T) {
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

	recorder := &channelRouteUsageRecorder{}
	factory := &channelRouteFactory{
		provider: &channelRouteProvider{
			content:         "hello-channel-openai-compat",
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
		ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
		ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{}),
		SecretResolver:       channelstore.StoreSecretResolver{Source: store},
		UsageRecorder:        recorder,
		ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
		Callbacks: llmrouter.ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *llmrouter.ChannelSelection) {
				selected = append(selected, cloneRouteSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *llmrouter.ModelRemapEvent) {
				remapped = append(remapped, cloneRouteRemapEvent(event))
			},
			OnUsageRecorded: func(_ context.Context, usage *llmrouter.ChannelUsageRecord, _ error) {
				used = append(used, cloneRouteUsageRecord(usage))
			},
		},
	})

	mux := http.NewServeMux()
	routes.RegisterChat(mux, newChatHandlerFromProvider(provider, zap.NewNop()), zap.NewNop())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"metadata": {"region": "us"}
	}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload map[string]any
	mustDecodeJSON(t, readBody(t, resp), &payload)
	require.Equal(t, "gpt-4o-upstream", payload["model"])
	require.Equal(t, "chat.completion", payload["object"])

	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Len(t, selected, 1)
	require.Equal(t, "key-a", selected[0].KeyID)
	require.Len(t, remapped, 1)
	require.Equal(t, "gpt-4o-upstream", remapped[0].RemoteModel)
	require.Len(t, used, 1)
	require.True(t, used[0].Success)
	require.Equal(t, 16, used[0].Usage.TotalTokens)
}

func TestChatEndpoints_ChannelRoutedProviderOpenAICompatResponsesStream(t *testing.T) {
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

	recorder := &channelRouteUsageRecorder{}
	factory := &channelRouteFactory{
		provider: &channelRouteProvider{
			content:     "hello-channel-responses-stream",
			streamUsage: &llmrouter.ChatUsage{PromptTokens: 9, CompletionTokens: 4, TotalTokens: 13},
		},
	}

	var (
		selected []*llmrouter.ChannelSelection
		remapped []*llmrouter.ModelRemapEvent
		used     []*llmrouter.ChannelUsageRecord
	)
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
		ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{}),
		SecretResolver:       channelstore.StoreSecretResolver{Source: store},
		UsageRecorder:        recorder,
		ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
		Callbacks: llmrouter.ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *llmrouter.ChannelSelection) {
				selected = append(selected, cloneRouteSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *llmrouter.ModelRemapEvent) {
				remapped = append(remapped, cloneRouteRemapEvent(event))
			},
			OnUsageRecorded: func(_ context.Context, usage *llmrouter.ChannelUsageRecord, _ error) {
				used = append(used, cloneRouteUsageRecord(usage))
			},
		},
	})

	mux := http.NewServeMux()
	routes.RegisterChat(mux, newChatHandlerFromProvider(provider, zap.NewNop()), zap.NewNop())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{
		"model": "gpt-4o",
		"input": "Hi",
		"stream": true,
		"metadata": {"region": "us"}
	}`
	resp, err := http.Post(srv.URL+"/v1/responses", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"))

	bodyBytes := readBody(t, resp)
	require.Contains(t, string(bodyBytes), "response.output_text.delta")
	require.Contains(t, string(bodyBytes), "hello-channel-responses-stream")
	require.Contains(t, string(bodyBytes), "data: [DONE]")
	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Len(t, selected, 1)
	require.Equal(t, "key-a", selected[0].KeyID)
	require.Len(t, remapped, 1)
	require.Equal(t, "gpt-4o-upstream", remapped[0].RemoteModel)
	require.Len(t, used, 1)
	require.True(t, used[0].Success)
	require.Equal(t, 13, used[0].Usage.TotalTokens)
}

func TestChatEndpoints_ChannelRoutedProviderCompletionFailureStillRecordsRouteUsage(t *testing.T) {
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

	recorder := &channelRouteUsageRecorder{}
	factory := &channelRouteFactory{
		provider: &channelRouteProvider{
			completionErr: errors.New("upstream completion failed"),
		},
	}

	var (
		selected []*llmrouter.ChannelSelection
		remapped []*llmrouter.ModelRemapEvent
		used     []*llmrouter.ChannelUsageRecord
	)
	provider := llmrouter.NewChannelRoutedProvider(llmrouter.ChannelRoutedProviderOptions{
		ModelResolver:        llmrouter.PassthroughModelResolver{},
		ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
		ChannelSelector:      channelstore.NewPriorityWeightedSelector(store, channelstore.SelectorOptions{}),
		SecretResolver:       channelstore.StoreSecretResolver{Source: store},
		UsageRecorder:        recorder,
		ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
		Factory:              factory,
		Logger:               zap.NewNop(),
		Callbacks: llmrouter.ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *llmrouter.ChannelSelection) {
				selected = append(selected, cloneRouteSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *llmrouter.ModelRemapEvent) {
				remapped = append(remapped, cloneRouteRemapEvent(event))
			},
			OnUsageRecorded: func(_ context.Context, usage *llmrouter.ChannelUsageRecord, _ error) {
				used = append(used, cloneRouteUsageRecord(usage))
			},
		},
	})

	mux := http.NewServeMux()
	routes.RegisterChat(mux, newChatHandlerFromProvider(provider, zap.NewNop()), zap.NewNop())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "Hi"}],
		"metadata": {"region": "us"}
	}`
	resp, err := http.Post(srv.URL+"/api/v1/chat/completions", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	require.NotNil(t, factory.lastConfig)
	require.Equal(t, "https://key.example/v1", factory.lastConfig.BaseURL)
	require.Equal(t, "gpt-4o-upstream", factory.lastConfig.Model)
	require.Len(t, selected, 1)
	require.Equal(t, "key-a", selected[0].KeyID)
	require.Len(t, remapped, 1)
	require.Equal(t, "gpt-4o-upstream", remapped[0].RemoteModel)
	require.Len(t, used, 1)
	require.False(t, used[0].Success)
	require.Equal(t, "gpt-4o", used[0].RequestedModel)
	require.Equal(t, "gpt-4o-upstream", used[0].RemoteModel)
	require.Equal(t, "https://key.example/v1", used[0].BaseURL)
	require.Contains(t, used[0].ErrorMessage, "upstream completion failed")
	require.Nil(t, used[0].Usage)
	require.Len(t, recorder.snapshot(), 1)
	require.Contains(t, recorder.snapshot()[0].ErrorMessage, "upstream completion failed")
}

type channelRouteFactory struct {
	lastConfig *llmrouter.ChannelProviderConfig
	lastSecret *llmrouter.ChannelSecret
	provider   llmrouter.Provider
}

func (f *channelRouteFactory) CreateChatProvider(_ context.Context, config llmrouter.ChannelProviderConfig, secret llmrouter.ChannelSecret) (llmrouter.Provider, error) {
	configCopy := config
	secretCopy := secret
	f.lastConfig = &configCopy
	f.lastSecret = &secretCopy
	return f.provider, nil
}

type channelRouteProvider struct {
	content         string
	completionUsage *llmrouter.ChatUsage
	streamUsage     *llmrouter.ChatUsage
	completionErr   error
	streamErr       error
}

func (p *channelRouteProvider) Completion(_ context.Context, req *llmrouter.ChatRequest) (*llmrouter.ChatResponse, error) {
	if p.completionErr != nil {
		return nil, p.completionErr
	}

	usage := llmrouter.ChatUsage{}
	if p.completionUsage != nil {
		usage = *p.completionUsage
	}
	return &llmrouter.ChatResponse{
		Provider: "openai",
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

func (p *channelRouteProvider) Stream(_ context.Context, req *llmrouter.ChatRequest) (<-chan llmrouter.StreamChunk, error) {
	if p.streamErr != nil {
		return nil, p.streamErr
	}

	out := make(chan llmrouter.StreamChunk, 1)
	var usage *llmrouter.ChatUsage
	if p.streamUsage != nil {
		copied := *p.streamUsage
		usage = &copied
	}
	out <- llmrouter.StreamChunk{
		Provider: "openai",
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

func (*channelRouteProvider) HealthCheck(context.Context) (*llmrouter.HealthStatus, error) {
	return &llmrouter.HealthStatus{Healthy: true}, nil
}

func (*channelRouteProvider) Name() string { return "channel-route-provider" }

func (*channelRouteProvider) SupportsNativeFunctionCalling() bool { return true }

func (*channelRouteProvider) ListModels(context.Context) ([]llmrouter.Model, error) { return nil, nil }

func (*channelRouteProvider) Endpoints() llmrouter.ProviderEndpoints {
	return llmrouter.ProviderEndpoints{}
}

type channelRouteUsageRecorder struct {
	mu      sync.Mutex
	records []*llmrouter.ChannelUsageRecord
}

func (r *channelRouteUsageRecorder) RecordUsage(_ context.Context, usage *llmrouter.ChannelUsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, cloneRouteUsageRecord(usage))
	return nil
}

func (r *channelRouteUsageRecorder) snapshot() []*llmrouter.ChannelUsageRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*llmrouter.ChannelUsageRecord, 0, len(r.records))
	for _, record := range r.records {
		out = append(out, cloneRouteUsageRecord(record))
	}
	return out
}

func cloneRouteSelection(selection *llmrouter.ChannelSelection) *llmrouter.ChannelSelection {
	if selection == nil {
		return nil
	}
	cloned := *selection
	return &cloned
}

func cloneRouteRemapEvent(event *llmrouter.ModelRemapEvent) *llmrouter.ModelRemapEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	return &cloned
}

func cloneRouteUsageRecord(record *llmrouter.ChannelUsageRecord) *llmrouter.ChannelUsageRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	if record.Usage != nil {
		usage := *record.Usage
		cloned.Usage = &usage
	}
	return &cloned
}
