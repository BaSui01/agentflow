package router

import (
	"context"
	"errors"
	"testing"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"go.uber.org/zap"
)

type channelCaptureProvider struct {
	name                string
	lastCompletionModel string
	lastStreamModel     string
}

func (p *channelCaptureProvider) Completion(_ context.Context, req *ChatRequest) (*ChatResponse, error) {
	p.lastCompletionModel = req.Model
	return &ChatResponse{
		Choices: []ChatChoice{{
			Index: 0,
			Message: Message{
				Role:    RoleAssistant,
				Content: "ok",
			},
		}},
		Usage: ChatUsage{
			PromptTokens:     11,
			CompletionTokens: 7,
			TotalTokens:      18,
		},
	}, nil
}

func (p *channelCaptureProvider) Stream(_ context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	p.lastStreamModel = req.Model
	ch := make(chan StreamChunk, 2)
	ch <- StreamChunk{
		Delta: Message{
			Role:    RoleAssistant,
			Content: "hello",
		},
	}
	ch <- StreamChunk{
		Delta: Message{
			Role:    RoleAssistant,
			Content: "",
		},
		FinishReason: "stop",
		Usage: &ChatUsage{
			PromptTokens:     5,
			CompletionTokens: 4,
			TotalTokens:      9,
		},
	}
	close(ch)
	return ch, nil
}

func (p *channelCaptureProvider) HealthCheck(context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (p *channelCaptureProvider) Name() string { return p.name }

func (p *channelCaptureProvider) SupportsNativeFunctionCalling() bool { return true }

func (p *channelCaptureProvider) ListModels(context.Context) ([]Model, error) { return nil, nil }

func (p *channelCaptureProvider) Endpoints() ProviderEndpoints { return ProviderEndpoints{} }

type captureChannelFactory struct {
	provider Provider
	configs  []ChannelProviderConfig
	secrets  []ChannelSecret
}

func (f *captureChannelFactory) CreateChatProvider(_ context.Context, config ChannelProviderConfig, secret ChannelSecret) (Provider, error) {
	f.configs = append(f.configs, config)
	f.secrets = append(f.secrets, secret)
	return f.provider, nil
}

type captureModelMappingResolver struct {
	mappings       []ChannelModelMapping
	lastRequest    *ChannelRouteRequest
	lastResolution *ModelResolution
}

func (r *captureModelMappingResolver) ResolveMappings(_ context.Context, request *ChannelRouteRequest, resolution *ModelResolution) ([]ChannelModelMapping, error) {
	r.lastRequest = request
	r.lastResolution = resolution
	return r.mappings, nil
}

type captureChannelSelector struct {
	selections     []ChannelSelection
	callCount      int
	lastRequest    *ChannelRouteRequest
	lastResolution *ModelResolution
	lastMappings   []ChannelModelMapping
	requests       []*ChannelRouteRequest
}

func (s *captureChannelSelector) SelectChannel(_ context.Context, request *ChannelRouteRequest, resolution *ModelResolution, mappings []ChannelModelMapping) (*ChannelSelection, error) {
	s.lastRequest = request
	s.lastResolution = resolution
	s.lastMappings = append([]ChannelModelMapping(nil), mappings...)
	s.requests = append(s.requests, cloneRouteRequest(request))
	var selection ChannelSelection
	if len(s.selections) == 0 {
		return nil, nil
	}
	if s.callCount >= len(s.selections) {
		selection = s.selections[len(s.selections)-1]
	} else {
		selection = s.selections[s.callCount]
	}
	s.callCount++
	return &selection, nil
}

type captureSecretResolver struct {
	secret        ChannelSecret
	lastRequest   *ChannelRouteRequest
	lastSelection *ChannelSelection
}

func (r *captureSecretResolver) ResolveSecret(_ context.Context, request *ChannelRouteRequest, selection *ChannelSelection) (*ChannelSecret, error) {
	r.lastRequest = request
	r.lastSelection = selection
	secret := r.secret
	return &secret, nil
}

type scriptedSecretResolver struct {
	secret     ChannelSecret
	errs       []error
	callCount  int
	requests   []*ChannelRouteRequest
	selections []*ChannelSelection
}

func (r *scriptedSecretResolver) ResolveSecret(_ context.Context, request *ChannelRouteRequest, selection *ChannelSelection) (*ChannelSecret, error) {
	r.requests = append(r.requests, cloneRouteRequest(request))
	r.selections = append(r.selections, cloneChannelSelection(selection))
	if r.callCount < len(r.errs) && r.errs[r.callCount] != nil {
		err := r.errs[r.callCount]
		r.callCount++
		return nil, err
	}
	r.callCount++
	secret := r.secret
	return &secret, nil
}

type captureProviderConfigSource struct {
	config        ChannelProviderConfig
	lastRequest   *ChannelRouteRequest
	lastSelection *ChannelSelection
}

func (s *captureProviderConfigSource) ResolveProviderConfig(_ context.Context, request *ChannelRouteRequest, selection *ChannelSelection) (*ChannelProviderConfig, error) {
	s.lastRequest = request
	s.lastSelection = selection
	config := s.config
	return &config, nil
}

type captureUsageRecorder struct {
	records []*ChannelUsageRecord
}

func (r *captureUsageRecorder) RecordUsage(_ context.Context, usage *ChannelUsageRecord) error {
	r.records = append(r.records, cloneChannelUsageRecord(usage))
	return nil
}

func TestChannelRoutedProvider_GatewayCompletionPropagatesResolvedCall(t *testing.T) {
	t.Parallel()

	inner := &channelCaptureProvider{name: "factory-provider"}
	factory := &captureChannelFactory{provider: inner}
	mappingResolver := &captureModelMappingResolver{
		mappings: []ChannelModelMapping{{
			MappingID:   "mapping-1",
			ChannelID:   "channel-1",
			Provider:    "mock-provider",
			PublicModel: "public-model",
			RemoteModel: "upstream-model",
			BaseURL:     "https://mapping.example",
			Region:      "us-east",
		}},
	}
	selector := &captureChannelSelector{
		selections: []ChannelSelection{{
			MappingID:   "mapping-1",
			ChannelID:   "channel-1",
			KeyID:       "key-1",
			Provider:    "mock-provider",
			RemoteModel: "upstream-model",
			BaseURL:     "https://selection.example",
			Region:      "us-east",
			Priority:    10,
			Weight:      50,
		}},
	}
	secretResolver := &captureSecretResolver{
		secret: ChannelSecret{APIKey: "sk-test"},
	}
	configSource := &captureProviderConfigSource{
		config: ChannelProviderConfig{
			Provider: "mock-provider",
			BaseURL:  "https://provider.example",
			Model:    "upstream-model",
		},
	}
	usageRecorder := &captureUsageRecorder{}

	var (
		keySelections []*ChannelSelection
		remaps        []*ModelRemapEvent
		usageEvents   []*ChannelUsageRecord
	)

	routed := NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: mappingResolver,
		ChannelSelector:      selector,
		SecretResolver:       secretResolver,
		UsageRecorder:        usageRecorder,
		ProviderConfigSource: configSource,
		Factory:              factory,
		Callbacks: ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *ChannelSelection) {
				keySelections = append(keySelections, cloneChannelSelection(selection))
			},
			OnModelRemapped: func(_ context.Context, event *ModelRemapEvent) {
				cloned := *event
				remaps = append(remaps, &cloned)
			},
			OnUsageRecorded: func(_ context.Context, usage *ChannelUsageRecord, _ error) {
				usageEvents = append(usageEvents, cloneChannelUsageRecord(usage))
			},
		},
		Logger: zap.NewNop(),
	})

	gateway := llmgateway.New(llmgateway.Config{ChatProvider: routed, Logger: zap.NewNop()})
	chatReq := &ChatRequest{
		TraceID: "trace-1",
		Model:   "public-model",
		Messages: []Message{{
			Role:    RoleUser,
			Content: "hello",
		}},
		Metadata: map[string]string{
			"chat_provider": "mock-provider",
			"route_policy":  "cost",
			"region":        "us-east",
		},
	}

	resp, err := gateway.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    chatReq,
		TraceID:    chatReq.TraceID,
	})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}

	if inner.lastCompletionModel != "upstream-model" {
		t.Fatalf("expected upstream-model, got %s", inner.lastCompletionModel)
	}
	if got := resp.ProviderDecision.Provider; got != "mock-provider" {
		t.Fatalf("expected provider mock-provider, got %s", got)
	}
	if got := resp.ProviderDecision.Model; got != "upstream-model" {
		t.Fatalf("expected resolved model upstream-model, got %s", got)
	}
	if got := resp.ProviderDecision.BaseURL; got != "https://provider.example" {
		t.Fatalf("expected base URL https://provider.example, got %s", got)
	}
	if selector.lastRequest == nil || selector.lastRequest.Mode != RouteModeCompletion {
		t.Fatalf("expected completion route request to reach selector, got %+v", selector.lastRequest)
	}
	if selector.lastRequest.RoutePolicy != "cost_first" {
		t.Fatalf("expected normalized cost_first route policy, got %s", selector.lastRequest.RoutePolicy)
	}
	if len(factory.configs) != 1 || factory.configs[0].BaseURL != "https://provider.example" {
		t.Fatalf("expected provider factory to receive resolved baseURL, got %+v", factory.configs)
	}
	if len(factory.secrets) != 1 || factory.secrets[0].APIKey != "sk-test" {
		t.Fatalf("expected provider factory to receive resolved secret, got %+v", factory.secrets)
	}
	if len(usageRecorder.records) != 1 {
		t.Fatalf("expected one usage record, got %d", len(usageRecorder.records))
	}
	record := usageRecorder.records[0]
	if !record.Success || record.RequestedModel != "public-model" || record.RemoteModel != "upstream-model" {
		t.Fatalf("unexpected usage record: %+v", record)
	}
	if record.Usage == nil || record.Usage.TotalTokens != 18 {
		t.Fatalf("expected usage total 18, got %+v", record.Usage)
	}
	if len(keySelections) != 1 || keySelections[0].KeyID != "key-1" {
		t.Fatalf("expected key selection callback for key-1, got %+v", keySelections)
	}
	if len(remaps) != 1 || remaps[0].RemoteModel != "upstream-model" {
		t.Fatalf("expected model remap callback, got %+v", remaps)
	}
	if len(usageEvents) != 1 || usageEvents[0].BaseURL != "https://provider.example" {
		var gotBaseURL string
		if len(usageEvents) > 0 && usageEvents[0] != nil {
			gotBaseURL = usageEvents[0].BaseURL
		}
		t.Fatalf("expected usage callback with baseURL https://provider.example, got len=%d baseURL=%q events=%+v", len(usageEvents), gotBaseURL, usageEvents)
	}
}

func TestChannelRoutedProvider_GatewayStreamPropagatesResolvedCall(t *testing.T) {
	t.Parallel()

	inner := &channelCaptureProvider{name: "factory-provider"}
	factory := &captureChannelFactory{provider: inner}
	usageRecorder := &captureUsageRecorder{}

	routed := NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: &captureModelMappingResolver{},
		ChannelSelector: &captureChannelSelector{selections: []ChannelSelection{{
			ChannelID:   "channel-stream",
			KeyID:       "key-stream",
			Provider:    "stream-provider",
			RemoteModel: "upstream-stream-model",
		}}},
		SecretResolver: &captureSecretResolver{secret: ChannelSecret{APIKey: "sk-stream"}},
		ProviderConfigSource: &captureProviderConfigSource{config: ChannelProviderConfig{
			Provider: "stream-provider",
			BaseURL:  "https://stream.example",
			Model:    "upstream-stream-model",
		}},
		UsageRecorder: usageRecorder,
		Factory:       factory,
		Logger:        zap.NewNop(),
	})

	gateway := llmgateway.New(llmgateway.Config{ChatProvider: routed, Logger: zap.NewNop()})
	chatReq := &ChatRequest{
		TraceID: "trace-stream",
		Model:   "public-stream-model",
		Messages: []Message{{
			Role:    RoleUser,
			Content: "stream",
		}},
	}

	stream, err := gateway.Stream(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    chatReq,
		TraceID:    chatReq.TraceID,
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var chunks []llmcore.UnifiedChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	if inner.lastStreamModel != "upstream-stream-model" {
		t.Fatalf("expected upstream-stream-model, got %s", inner.lastStreamModel)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if got := chunk.ProviderDecision.Provider; got != "stream-provider" {
			t.Fatalf("expected stream provider stream-provider, got %s", got)
		}
		if got := chunk.ProviderDecision.Model; got != "upstream-stream-model" {
			t.Fatalf("expected stream model upstream-stream-model, got %s", got)
		}
		if got := chunk.ProviderDecision.BaseURL; got != "https://stream.example" {
			t.Fatalf("expected stream baseURL https://stream.example, got %s", got)
		}
	}
	if len(usageRecorder.records) != 1 {
		t.Fatalf("expected one stream usage record, got %d", len(usageRecorder.records))
	}
	record := usageRecorder.records[0]
	if !record.Success || record.KeyID != "key-stream" || record.RemoteModel != "upstream-stream-model" {
		t.Fatalf("unexpected stream usage record: %+v", record)
	}
	if record.Usage == nil || record.Usage.TotalTokens != 9 {
		t.Fatalf("expected stream usage total 9, got %+v", record.Usage)
	}
}

type scriptedCompletionProvider struct {
	models []string
	errs   []error
	index  int
}

func (p *scriptedCompletionProvider) Completion(_ context.Context, req *ChatRequest) (*ChatResponse, error) {
	p.models = append(p.models, req.Model)
	if p.index < len(p.errs) && p.errs[p.index] != nil {
		err := p.errs[p.index]
		p.index++
		return nil, err
	}
	p.index++
	return &ChatResponse{
		Choices: []ChatChoice{{Index: 0, Message: Message{Role: RoleAssistant, Content: "ok"}}},
		Usage:   ChatUsage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
	}, nil
}

func (p *scriptedCompletionProvider) Stream(context.Context, *ChatRequest) (<-chan StreamChunk, error) {
	return nil, errors.New("unused")
}

func (p *scriptedCompletionProvider) HealthCheck(context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (p *scriptedCompletionProvider) Name() string                                { return "scripted-completion" }
func (p *scriptedCompletionProvider) SupportsNativeFunctionCalling() bool         { return true }
func (p *scriptedCompletionProvider) ListModels(context.Context) ([]Model, error) { return nil, nil }
func (p *scriptedCompletionProvider) Endpoints() ProviderEndpoints                { return ProviderEndpoints{} }

type scriptedStreamProvider struct {
	models []string
	errs   []error
	index  int
}

func (p *scriptedStreamProvider) Completion(context.Context, *ChatRequest) (*ChatResponse, error) {
	return nil, errors.New("unused")
}

func (p *scriptedStreamProvider) Stream(_ context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	p.models = append(p.models, req.Model)
	ch := make(chan StreamChunk, 1)
	if p.index < len(p.errs) && p.errs[p.index] != nil {
		ch <- StreamChunk{Err: toTypesError(p.errs[p.index])}
		close(ch)
		p.index++
		return ch, nil
	}
	ch <- StreamChunk{
		Delta: Message{Role: RoleAssistant, Content: "ok"},
		Usage: &ChatUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}
	close(ch)
	p.index++
	return ch, nil
}

func (p *scriptedStreamProvider) HealthCheck(context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (p *scriptedStreamProvider) Name() string                                { return "scripted-stream" }
func (p *scriptedStreamProvider) SupportsNativeFunctionCalling() bool         { return true }
func (p *scriptedStreamProvider) ListModels(context.Context) ([]Model, error) { return nil, nil }
func (p *scriptedStreamProvider) Endpoints() ProviderEndpoints                { return ProviderEndpoints{} }

type immediateFailStreamProvider struct {
	models []string
	errs   []error
	index  int
}

func (p *immediateFailStreamProvider) Completion(context.Context, *ChatRequest) (*ChatResponse, error) {
	return nil, errors.New("unused")
}

func (p *immediateFailStreamProvider) Stream(_ context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	p.models = append(p.models, req.Model)
	if p.index < len(p.errs) && p.errs[p.index] != nil {
		err := p.errs[p.index]
		p.index++
		return nil, err
	}
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{
		Delta: Message{Role: RoleAssistant, Content: "ok"},
		Usage: &ChatUsage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
	}
	close(ch)
	p.index++
	return ch, nil
}

func (p *immediateFailStreamProvider) HealthCheck(context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (p *immediateFailStreamProvider) Name() string                                { return "immediate-fail-stream" }
func (p *immediateFailStreamProvider) SupportsNativeFunctionCalling() bool         { return true }
func (p *immediateFailStreamProvider) ListModels(context.Context) ([]Model, error) { return nil, nil }
func (p *immediateFailStreamProvider) Endpoints() ProviderEndpoints                { return ProviderEndpoints{} }

func TestChannelRoutedProvider_CompletionRetriesWithExcludedSelection(t *testing.T) {
	t.Parallel()

	inner := &scriptedCompletionProvider{errs: []error{errors.New("first key failed"), nil}}
	selector := &captureChannelSelector{
		selections: []ChannelSelection{
			{ChannelID: "channel-a", KeyID: "key-a", Provider: "mock-provider", RemoteModel: "upstream-a"},
			{ChannelID: "channel-b", KeyID: "key-b", Provider: "mock-provider", RemoteModel: "upstream-b"},
		},
	}
	usageRecorder := &captureUsageRecorder{}
	var (
		keySelections []*ChannelSelection
		usageEvents   []*ChannelUsageRecord
	)
	routed := NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		RetryPolicy: ChannelRouteRetryPolicy{
			MaxAttempts:          2,
			ExcludeFailedChannel: true,
			ShouldRetry: func(context.Context, error, *ChannelSelection) bool {
				return true
			},
		},
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: &captureModelMappingResolver{},
		ChannelSelector:      selector,
		SecretResolver:       &captureSecretResolver{secret: ChannelSecret{APIKey: "sk"}},
		ProviderConfigSource: &captureProviderConfigSource{config: ChannelProviderConfig{Provider: "mock-provider", BaseURL: "https://retry.example"}},
		UsageRecorder:        usageRecorder,
		Factory:              &captureChannelFactory{provider: inner},
		Callbacks: ChannelRouteCallbacks{
			OnKeySelected: func(_ context.Context, selection *ChannelSelection) {
				keySelections = append(keySelections, cloneChannelSelection(selection))
			},
			OnUsageRecorded: func(_ context.Context, usage *ChannelUsageRecord, _ error) {
				usageEvents = append(usageEvents, cloneChannelUsageRecord(usage))
			},
		},
		Logger: zap.NewNop(),
	})

	resp, err := routed.Completion(context.Background(), &ChatRequest{
		Model:    "public-model",
		Messages: []Message{{Role: RoleUser, Content: "retry"}},
	})
	if err != nil {
		t.Fatalf("Completion error: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response")
	}
	if len(selector.requests) != 2 {
		t.Fatalf("expected 2 selector requests, got %d", len(selector.requests))
	}
	if len(selector.requests[0].ExcludedChannelIDs) != 0 || len(selector.requests[0].ExcludedKeyIDs) != 0 {
		t.Fatalf("expected first attempt without exclusions, got %+v", selector.requests[0])
	}
	if selector.requests[0].Attempt != 1 {
		t.Fatalf("expected first attempt number 1, got %+v", selector.requests[0])
	}
	if got := selector.requests[1].ExcludedChannelIDs; len(got) != 1 || got[0] != "channel-a" {
		t.Fatalf("expected channel-a excluded on retry, got %+v", got)
	}
	if got := selector.requests[1].ExcludedKeyIDs; len(got) != 1 || got[0] != "key-a" {
		t.Fatalf("expected key-a excluded on retry, got %+v", got)
	}
	if selector.requests[1].Attempt != 2 {
		t.Fatalf("expected second attempt number 2, got %+v", selector.requests[1])
	}
	if len(usageRecorder.records) != 2 {
		t.Fatalf("expected 2 usage records, got %d", len(usageRecorder.records))
	}
	if usageRecorder.records[0].Success {
		t.Fatalf("expected first usage record to be failure")
	}
	if usageRecorder.records[0].Attempt != 1 || usageRecorder.records[1].Attempt != 2 {
		t.Fatalf("expected usage attempts 1 and 2, got %+v %+v", usageRecorder.records[0], usageRecorder.records[1])
	}
	if !usageRecorder.records[1].Success || usageRecorder.records[1].ChannelID != "channel-b" || usageRecorder.records[1].KeyID != "key-b" {
		t.Fatalf("unexpected second usage record: %+v", usageRecorder.records[1])
	}
	if len(keySelections) != 2 || keySelections[0].KeyID != "key-a" || keySelections[1].KeyID != "key-b" {
		t.Fatalf("expected key selection callbacks for both attempts, got %+v", keySelections)
	}
	if len(usageEvents) != 2 || usageEvents[0].Attempt != 1 || usageEvents[1].Attempt != 2 {
		t.Fatalf("expected usage callbacks for both attempts, got %+v", usageEvents)
	}
	if len(inner.models) != 2 {
		t.Fatalf("expected 2 completion attempts, got %d", len(inner.models))
	}
}

func TestChannelRoutedProvider_CompletionRetriesWithExcludedKeyOnly(t *testing.T) {
	t.Parallel()

	inner := &scriptedCompletionProvider{errs: []error{errors.New("first key failed"), nil}}
	selector := &captureChannelSelector{
		selections: []ChannelSelection{
			{ChannelID: "channel-a", KeyID: "key-a", Provider: "mock-provider", RemoteModel: "upstream-a"},
			{ChannelID: "channel-a", KeyID: "key-b", Provider: "mock-provider", RemoteModel: "upstream-b"},
		},
	}
	routed := NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		RetryPolicy: ChannelRouteRetryPolicy{
			MaxAttempts:          2,
			ExcludeFailedChannel: false,
			ShouldRetry: func(context.Context, error, *ChannelSelection) bool {
				return true
			},
		},
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: &captureModelMappingResolver{},
		ChannelSelector:      selector,
		SecretResolver:       &captureSecretResolver{secret: ChannelSecret{APIKey: "sk"}},
		ProviderConfigSource: &captureProviderConfigSource{config: ChannelProviderConfig{Provider: "mock-provider", BaseURL: "https://retry.example"}},
		UsageRecorder:        &captureUsageRecorder{},
		Factory:              &captureChannelFactory{provider: inner},
		Logger:               zap.NewNop(),
	})

	resp, err := routed.Completion(context.Background(), &ChatRequest{
		Model:    "public-model",
		Messages: []Message{{Role: RoleUser, Content: "retry"}}},
	)
	if err != nil {
		t.Fatalf("Completion error: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response")
	}
	if len(selector.requests) != 2 {
		t.Fatalf("expected 2 selector requests, got %d", len(selector.requests))
	}
	if got := selector.requests[1].ExcludedChannelIDs; len(got) != 0 {
		t.Fatalf("expected no excluded channels when ExcludeFailedChannel=false, got %+v", got)
	}
	if got := selector.requests[1].ExcludedKeyIDs; len(got) != 1 || got[0] != "key-a" {
		t.Fatalf("expected only key-a excluded on retry, got %+v", got)
	}
}

func TestChannelRoutedProvider_CompletionRetriesAfterSecretResolverFailure(t *testing.T) {
	t.Parallel()

	inner := &scriptedCompletionProvider{}
	selector := &captureChannelSelector{
		selections: []ChannelSelection{
			{ChannelID: "channel-a", KeyID: "key-a", Provider: "mock-provider", RemoteModel: "upstream-a"},
			{ChannelID: "channel-b", KeyID: "key-b", Provider: "mock-provider", RemoteModel: "upstream-b"},
		},
	}
	secretResolver := &scriptedSecretResolver{
		secret: ChannelSecret{APIKey: "sk"},
		errs:   []error{errors.New("decrypt key failed"), nil},
	}
	usageRecorder := &captureUsageRecorder{}
	var usageEvents []*ChannelUsageRecord

	routed := NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		RetryPolicy: ChannelRouteRetryPolicy{
			MaxAttempts:          2,
			ExcludeFailedChannel: false,
			ShouldRetry: func(context.Context, error, *ChannelSelection) bool {
				return true
			},
		},
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: &captureModelMappingResolver{},
		ChannelSelector:      selector,
		SecretResolver:       secretResolver,
		ProviderConfigSource: &captureProviderConfigSource{config: ChannelProviderConfig{Provider: "mock-provider", BaseURL: "https://retry-secret.example"}},
		UsageRecorder:        usageRecorder,
		Factory:              &captureChannelFactory{provider: inner},
		Callbacks: ChannelRouteCallbacks{
			OnUsageRecorded: func(_ context.Context, usage *ChannelUsageRecord, _ error) {
				usageEvents = append(usageEvents, cloneChannelUsageRecord(usage))
			},
		},
		Logger: zap.NewNop(),
	})

	resp, err := routed.Completion(context.Background(), &ChatRequest{
		Model:    "public-model",
		Messages: []Message{{Role: RoleUser, Content: "retry secret"}},
	})
	if err != nil {
		t.Fatalf("Completion error: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response")
	}
	if len(selector.requests) != 2 {
		t.Fatalf("expected 2 selector requests, got %d", len(selector.requests))
	}
	if got := selector.requests[1].ExcludedChannelIDs; len(got) != 0 {
		t.Fatalf("expected no excluded channels after secret failure when ExcludeFailedChannel=false, got %+v", got)
	}
	if got := selector.requests[1].ExcludedKeyIDs; len(got) != 1 || got[0] != "key-a" {
		t.Fatalf("expected key-a excluded after secret failure, got %+v", got)
	}
	if len(secretResolver.requests) != 2 {
		t.Fatalf("expected secret resolver to run twice, got %d", len(secretResolver.requests))
	}
	if len(usageRecorder.records) != 2 {
		t.Fatalf("expected 2 usage records, got %d", len(usageRecorder.records))
	}
	if usageRecorder.records[0].Success {
		t.Fatalf("expected first usage record to be failure")
	}
	if usageRecorder.records[0].KeyID != "key-a" || usageRecorder.records[0].ErrorMessage == "" {
		t.Fatalf("expected first failure to record key-a and error, got %+v", usageRecorder.records[0])
	}
	if !usageRecorder.records[1].Success || usageRecorder.records[1].KeyID != "key-b" {
		t.Fatalf("unexpected second usage record: %+v", usageRecorder.records[1])
	}
	if len(usageEvents) != 2 || usageEvents[0].Attempt != 1 || usageEvents[1].Attempt != 2 {
		t.Fatalf("expected usage callbacks for both attempts, got %+v", usageEvents)
	}
	if len(inner.models) != 1 || inner.models[0] != "upstream-b" {
		t.Fatalf("expected provider completion to run only on the successful second attempt, got %+v", inner.models)
	}
}

func TestChannelRoutedProvider_StreamRetriesBeforeEmittingChunks(t *testing.T) {
	t.Parallel()

	inner := &scriptedStreamProvider{errs: []error{errors.New("stream bootstrap failed"), nil}}
	selector := &captureChannelSelector{
		selections: []ChannelSelection{
			{ChannelID: "channel-a", KeyID: "key-a", Provider: "mock-provider", RemoteModel: "upstream-a"},
			{ChannelID: "channel-b", KeyID: "key-b", Provider: "mock-provider", RemoteModel: "upstream-b"},
		},
	}
	usageRecorder := &captureUsageRecorder{}
	routed := NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		RetryPolicy: ChannelRouteRetryPolicy{
			MaxAttempts:          2,
			ExcludeFailedChannel: true,
			ShouldRetry: func(context.Context, error, *ChannelSelection) bool {
				return true
			},
		},
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: &captureModelMappingResolver{},
		ChannelSelector:      selector,
		SecretResolver:       &captureSecretResolver{secret: ChannelSecret{APIKey: "sk"}},
		ProviderConfigSource: &captureProviderConfigSource{config: ChannelProviderConfig{Provider: "mock-provider", BaseURL: "https://retry-stream.example"}},
		UsageRecorder:        usageRecorder,
		Factory:              &captureChannelFactory{provider: inner},
		Logger:               zap.NewNop(),
	})

	stream, err := routed.Stream(context.Background(), &ChatRequest{
		Model:    "public-model",
		Messages: []Message{{Role: RoleUser, Content: "retry stream"}},
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}
	var chunks []StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	if len(selector.requests) != 2 {
		t.Fatalf("expected 2 selector requests, got %d", len(selector.requests))
	}
	if selector.requests[0].Attempt != 1 || selector.requests[1].Attempt != 2 {
		t.Fatalf("expected stream attempts 1 and 2, got %+v %+v", selector.requests[0], selector.requests[1])
	}
	if got := selector.requests[1].ExcludedChannelIDs; len(got) != 1 || got[0] != "channel-a" {
		t.Fatalf("expected channel-a excluded on stream retry, got %+v", got)
	}
	if got := selector.requests[1].ExcludedKeyIDs; len(got) != 1 || got[0] != "key-a" {
		t.Fatalf("expected key-a excluded on stream retry, got %+v", got)
	}
	if len(chunks) != 1 || chunks[0].Err != nil {
		t.Fatalf("expected one successful chunk after retry, got %+v", chunks)
	}
	if len(usageRecorder.records) != 2 {
		t.Fatalf("expected 2 usage records, got %d", len(usageRecorder.records))
	}
	if usageRecorder.records[0].Success {
		t.Fatalf("expected first stream usage to be failure")
	}
	if usageRecorder.records[0].Attempt != 1 || usageRecorder.records[1].Attempt != 2 {
		t.Fatalf("expected usage attempts 1 and 2, got %+v %+v", usageRecorder.records[0], usageRecorder.records[1])
	}
	if !usageRecorder.records[1].Success || usageRecorder.records[1].KeyID != "key-b" {
		t.Fatalf("unexpected second stream usage record: %+v", usageRecorder.records[1])
	}
}

func TestChannelRoutedProvider_StreamRetriesOnOpenErrorWithExcludedSelection(t *testing.T) {
	t.Parallel()

	inner := &immediateFailStreamProvider{errs: []error{errors.New("dial upstream failed"), nil}}
	selector := &captureChannelSelector{
		selections: []ChannelSelection{
			{ChannelID: "channel-a", KeyID: "key-a", Provider: "mock-provider", RemoteModel: "upstream-a"},
			{ChannelID: "channel-b", KeyID: "key-b", Provider: "mock-provider", RemoteModel: "upstream-b"},
		},
	}
	usageRecorder := &captureUsageRecorder{}
	routed := NewChannelRoutedProvider(ChannelRoutedProviderOptions{
		MaxAttempts:          2,
		ModelResolver:        PassthroughModelResolver{},
		ModelMappingResolver: &captureModelMappingResolver{},
		ChannelSelector:      selector,
		SecretResolver:       &captureSecretResolver{secret: ChannelSecret{APIKey: "sk"}},
		ProviderConfigSource: &captureProviderConfigSource{config: ChannelProviderConfig{Provider: "mock-provider", BaseURL: "https://retry-open.example"}},
		UsageRecorder:        usageRecorder,
		Factory:              &captureChannelFactory{provider: inner},
		Logger:               zap.NewNop(),
	})

	stream, err := routed.Stream(context.Background(), &ChatRequest{
		Model:    "public-model",
		Messages: []Message{{Role: RoleUser, Content: "retry stream open"}},
	})
	if err != nil {
		t.Fatalf("Stream error: %v", err)
	}

	var chunks []StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}

	if len(selector.requests) != 2 {
		t.Fatalf("expected 2 selector requests, got %d", len(selector.requests))
	}
	if got := selector.requests[1].ExcludedChannelIDs; len(got) != 0 {
		t.Fatalf("expected no excluded channels when key is present, got %+v", got)
	}
	if got := selector.requests[1].ExcludedKeyIDs; len(got) != 1 || got[0] != "key-a" {
		t.Fatalf("expected key-a excluded on stream open retry, got %+v", got)
	}
	if len(chunks) != 1 || chunks[0].Err != nil {
		t.Fatalf("expected one successful chunk after stream open retry, got %+v", chunks)
	}
	if len(usageRecorder.records) != 2 {
		t.Fatalf("expected 2 usage records, got %d", len(usageRecorder.records))
	}
	if usageRecorder.records[0].Success {
		t.Fatalf("expected first stream open usage to be failure")
	}
	if usageRecorder.records[0].KeyID != "key-a" {
		t.Fatalf("expected first failure to record key-a, got %+v", usageRecorder.records[0])
	}
	if !usageRecorder.records[1].Success || usageRecorder.records[1].KeyID != "key-b" {
		t.Fatalf("unexpected second stream open usage record: %+v", usageRecorder.records[1])
	}
	if len(inner.models) != 2 {
		t.Fatalf("expected 2 stream open attempts, got %d", len(inner.models))
	}
}

func cloneRouteRequest(req *ChannelRouteRequest) *ChannelRouteRequest {
	if req == nil {
		return nil
	}
	cloned := *req
	cloned.Metadata = cloneStringMap(req.Metadata)
	cloned.Tags = cloneStrings(req.Tags)
	cloned.ExcludedChannelIDs = cloneStrings(req.ExcludedChannelIDs)
	cloned.ExcludedKeyIDs = cloneStrings(req.ExcludedKeyIDs)
	return &cloned
}

func TestRoutedChatProvider_GatewayCompletionIncludesResolvedBaseURL(t *testing.T) {
	t.Parallel()

	router, _ := setupRouterForRoutedProviderTest(t)
	routed := NewRoutedChatProvider(router, RoutedChatProviderOptions{
		DefaultStrategy: StrategyCostBased,
		Logger:          zap.NewNop(),
	})
	gateway := llmgateway.New(llmgateway.Config{ChatProvider: routed, Logger: zap.NewNop()})

	req := &ChatRequest{
		TraceID: "trace-existing",
		Model:   "gpt-4o",
		Messages: []Message{{
			Role:    RoleUser,
			Content: "hello",
		}},
	}
	resp, err := gateway.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		Payload:    req,
		TraceID:    req.TraceID,
	})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}

	if got := resp.ProviderDecision.Provider; got != "mockA" {
		t.Fatalf("expected provider mockA, got %s", got)
	}
	if got := resp.ProviderDecision.Model; got != "remote-a" {
		t.Fatalf("expected remote model remote-a, got %s", got)
	}
	if got := resp.ProviderDecision.BaseURL; got != "http://example-a" {
		t.Fatalf("expected baseURL http://example-a, got %s", got)
	}
}
