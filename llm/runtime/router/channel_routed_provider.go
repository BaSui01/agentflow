package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	llmroot "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ChannelRoutedProviderOptions configures the generic channel-routed provider.
type ChannelRoutedProviderOptions struct {
	Name                 string
	MaxAttempts          int
	ModelResolver        ModelResolver
	ModelMappingResolver ModelMappingResolver
	ChannelSelector      ChannelSelector
	SecretResolver       SecretResolver
	UsageRecorder        UsageRecorder
	CooldownController   CooldownController
	QuotaPolicy          QuotaPolicy
	ProviderConfigSource ProviderConfigSource
	Factory              ChatProviderFactory
	RetryPolicy          ChannelRouteRetryPolicy
	Callbacks            ChannelRouteCallbacks
	Logger               *zap.Logger
}

// ChannelRoutedProvider is a generic routed chat provider that delegates route semantics to injected interfaces.
type ChannelRoutedProvider struct {
	name                 string
	maxAttempts          int
	modelResolver        ModelResolver
	modelMappingResolver ModelMappingResolver
	channelSelector      ChannelSelector
	secretResolver       SecretResolver
	usageRecorder        UsageRecorder
	cooldownController   CooldownController
	quotaPolicy          QuotaPolicy
	providerConfigSource ProviderConfigSource
	factory              ChatProviderFactory
	retryPolicy          ChannelRouteRetryPolicy
	callbacks            ChannelRouteCallbacks
	logger               *zap.Logger
}

type resolvedChannelInvocation struct {
	request     *ChannelRouteRequest
	resolution  *ModelResolution
	selection   *ChannelSelection
	config      *ChannelProviderConfig
	provider    Provider
	remoteModel string
}

// NewChannelRoutedProvider creates a generic channel-routed chat provider entrypoint.
func NewChannelRoutedProvider(opts ChannelRoutedProviderOptions) *ChannelRoutedProvider {
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "channel-routed-provider"
	}

	modelResolver := opts.ModelResolver
	if modelResolver == nil {
		modelResolver = PassthroughModelResolver{}
	}

	usageRecorder := opts.UsageRecorder
	if usageRecorder == nil {
		usageRecorder = NoopUsageRecorder{}
	}

	cooldownController := opts.CooldownController
	if cooldownController == nil {
		cooldownController = NoopCooldownController{}
	}

	quotaPolicy := opts.QuotaPolicy
	if quotaPolicy == nil {
		quotaPolicy = NoopQuotaPolicy{}
	}

	providerConfigSource := opts.ProviderConfigSource
	if providerConfigSource == nil {
		providerConfigSource = StaticProviderConfigSource{}
	}

	maxAttempts, retryPolicy := normalizeChannelRouteRetryConfig(opts.MaxAttempts, opts.RetryPolicy)

	return &ChannelRoutedProvider{
		name:                 name,
		maxAttempts:          maxAttempts,
		modelResolver:        modelResolver,
		modelMappingResolver: opts.ModelMappingResolver,
		channelSelector:      opts.ChannelSelector,
		secretResolver:       opts.SecretResolver,
		usageRecorder:        usageRecorder,
		cooldownController:   cooldownController,
		quotaPolicy:          quotaPolicy,
		providerConfigSource: providerConfigSource,
		factory:              opts.Factory,
		retryPolicy:          retryPolicy,
		callbacks:            opts.Callbacks,
		logger:               logger,
	}
}

func (p *ChannelRoutedProvider) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, types.NewInvalidRequestError("chat request is required")
	}

	state := retryExclusionState{}
	var lastErr error

	for attempt := 1; attempt <= p.maxAttempts; attempt++ {
		attemptStart := time.Now()
		routeRequest := buildChannelRouteRequest(req, RouteModeCompletion, attempt, state)
		invocation, err := p.prepareInvocation(ctx, routeRequest)
		if err != nil {
			lastErr = err
			if invocation.hasSelection() {
				p.recordUsage(ctx, invocation, false, err.Error(), nil, time.Since(attemptStart))
			}
			if !invocation.hasSelection() || !p.shouldRetry(ctx, err, invocation.selection, attempt) {
				return nil, err
			}
			state = state.exclude(invocation.selection, p.retryPolicy.ExcludeFailedChannel)
			continue
		}

		callStart := time.Now()
		routedReq := cloneChatRequest(req, invocation.remoteModelName())
		resp, callErr := invocation.provider.Completion(ctx, routedReq)
		if callErr == nil {
			if resp != nil {
				if strings.TrimSpace(resp.Provider) == "" {
					resp.Provider = invocation.providerName()
				}
				if strings.TrimSpace(resp.Model) == "" {
					resp.Model = invocation.remoteModelName()
				}
			}
			p.recordUsage(ctx, invocation, true, "", usageFromResponse(resp), time.Since(callStart))
			return resp, nil
		}

		p.recordUsage(ctx, invocation, false, callErr.Error(), nil, time.Since(callStart))
		lastErr = callErr
		if !p.shouldRetry(ctx, callErr, invocation.selection, attempt) {
			return nil, callErr
		}
		state = state.exclude(invocation.selection, p.retryPolicy.ExcludeFailedChannel)
	}

	return nil, firstNonNilError(lastErr, types.NewServiceUnavailableError("channel routed provider exhausted retry attempts"))
}

func (p *ChannelRoutedProvider) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	if req == nil {
		return nil, types.NewInvalidRequestError("chat request is required")
	}

	state := retryExclusionState{}
	var lastErr error

	for attempt := 1; attempt <= p.maxAttempts; attempt++ {
		attemptStart := time.Now()
		routeRequest := buildChannelRouteRequest(req, RouteModeStream, attempt, state)
		invocation, err := p.prepareInvocation(ctx, routeRequest)
		if err != nil {
			lastErr = err
			if invocation.hasSelection() {
				p.recordUsage(ctx, invocation, false, err.Error(), nil, time.Since(attemptStart))
			}
			if !invocation.hasSelection() || !p.shouldRetry(ctx, err, invocation.selection, attempt) {
				return nil, err
			}
			state = state.exclude(invocation.selection, p.retryPolicy.ExcludeFailedChannel)
			continue
		}

		callStart := time.Now()
		routedReq := cloneChatRequest(req, invocation.remoteModelName())
		source, streamErr := invocation.provider.Stream(ctx, routedReq)
		if streamErr == nil {
			out := make(chan StreamChunk)
			go p.relayStreamWithRetry(ctx, out, req, state, invocation, source)
			return out, nil
		}

		p.recordUsage(ctx, invocation, false, streamErr.Error(), nil, time.Since(callStart))
		lastErr = streamErr
		if !p.shouldRetry(ctx, streamErr, invocation.selection, attempt) {
			return nil, streamErr
		}
		state = state.exclude(invocation.selection, p.retryPolicy.ExcludeFailedChannel)
	}

	return nil, firstNonNilError(lastErr, types.NewServiceUnavailableError("channel routed provider exhausted retry attempts"))
}

func (p *ChannelRoutedProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{Healthy: true}, nil
}

func (p *ChannelRoutedProvider) Name() string {
	return p.name
}

func (p *ChannelRoutedProvider) SupportsNativeFunctionCalling() bool {
	return true
}

func (p *ChannelRoutedProvider) ListModels(ctx context.Context) ([]Model, error) {
	return nil, nil
}

func (p *ChannelRoutedProvider) Endpoints() ProviderEndpoints {
	return ProviderEndpoints{}
}

func (p *ChannelRoutedProvider) CountTokens(ctx context.Context, req *ChatRequest) (*llmroot.TokenCountResponse, error) {
	if req == nil {
		return nil, types.NewInvalidRequestError("chat request is required")
	}

	invocation, err := p.prepareInvocation(ctx, buildChannelRouteRequest(req, RouteModeCompletion, 1, retryExclusionState{}))
	if err != nil {
		return nil, err
	}

	counter, ok := invocation.provider.(llmroot.TokenCountProvider)
	if !ok {
		return nil, types.NewServiceUnavailableError("selected channel provider does not implement native token counting")
	}

	routedReq := cloneChatRequest(req, invocation.remoteModelName())
	return counter.CountTokens(ctx, routedReq)
}

func (p *ChannelRoutedProvider) prepareInvocation(ctx context.Context, routeRequest *ChannelRouteRequest) (*resolvedChannelInvocation, error) {
	if routeRequest == nil {
		return nil, types.NewInvalidRequestError("channel route request is required")
	}
	if strings.TrimSpace(routeRequest.RequestedModel) == "" {
		return nil, types.NewInvalidRequestError("model is required")
	}
	if p.modelMappingResolver == nil {
		return nil, types.NewServiceUnavailableError("channel routed provider requires a model mapping resolver")
	}
	if p.channelSelector == nil {
		return nil, types.NewServiceUnavailableError("channel routed provider requires a channel selector")
	}
	if p.providerConfigSource == nil {
		return nil, types.NewServiceUnavailableError("channel routed provider requires a provider config source")
	}
	if p.factory == nil {
		return nil, types.NewServiceUnavailableError("channel routed provider requires a chat provider factory")
	}

	invocation := &resolvedChannelInvocation{request: routeRequest}

	resolution, err := p.modelResolver.ResolveModel(ctx, routeRequest)
	if err != nil {
		return invocation, err
	}
	if resolution == nil {
		resolution = &ModelResolution{}
	}
	resolution.RequestedModel = firstNonEmpty(resolution.RequestedModel, routeRequest.RequestedModel)
	resolution.ResolvedModel = firstNonEmpty(resolution.ResolvedModel, routeRequest.RequestedModel)
	resolution.ProviderHint = firstNonEmpty(resolution.ProviderHint, routeRequest.ProviderHint)
	resolution.Region = firstNonEmpty(resolution.Region, routeRequest.Region)
	if len(resolution.Metadata) == 0 {
		resolution.Metadata = cloneStringMap(routeRequest.Metadata)
	}
	invocation.resolution = resolution

	mappings, err := p.modelMappingResolver.ResolveMappings(ctx, routeRequest, resolution)
	if err != nil {
		return invocation, err
	}

	selection, err := p.channelSelector.SelectChannel(ctx, routeRequest, resolution, mappings)
	if err != nil {
		return invocation, err
	}
	if selection == nil {
		return invocation, types.NewServiceUnavailableError("channel selector returned no selection")
	}
	invocation.selection = selection

	if p.callbacks.OnKeySelected != nil {
		p.callbacks.OnKeySelected(ctx, cloneChannelSelection(selection))
	}
	if err := p.cooldownController.Allow(ctx, routeRequest, selection); err != nil {
		return invocation, err
	}
	if err := p.quotaPolicy.Allow(ctx, routeRequest, selection); err != nil {
		return invocation, err
	}

	secret := &ChannelSecret{}
	if p.secretResolver != nil {
		secret, err = p.secretResolver.ResolveSecret(ctx, routeRequest, selection)
		if err != nil {
			return invocation, err
		}
		if secret == nil {
			secret = &ChannelSecret{}
		}
	}

	config, err := p.providerConfigSource.ResolveProviderConfig(ctx, routeRequest, selection)
	if err != nil {
		return invocation, err
	}
	if config == nil {
		config = &ChannelProviderConfig{}
	}
	config.Provider = firstNonEmpty(config.Provider, selection.Provider)
	config.BaseURL = firstNonEmpty(config.BaseURL, selection.BaseURL)
	config.Region = firstNonEmpty(config.Region, selection.Region)
	config.Model = firstNonEmpty(config.Model, selection.RemoteModel, resolution.ResolvedModel, routeRequest.RequestedModel)
	if len(config.Metadata) == 0 {
		config.Metadata = cloneStringMap(selection.Metadata)
	}
	if strings.TrimSpace(config.Provider) == "" {
		return invocation, types.NewServiceUnavailableError("provider config source returned empty provider")
	}
	invocation.config = config
	invocation.remoteModel = strings.TrimSpace(config.Model)

	provider, err := p.factory.CreateChatProvider(ctx, *config, *secret)
	if err != nil {
		return invocation, err
	}
	invocation.provider = provider

	llmroot.RecordResolvedProviderCall(ctx, llmroot.ResolvedProviderCall{
		Provider: config.Provider,
		Model:    invocation.remoteModelName(),
		BaseURL:  config.BaseURL,
	})

	if p.callbacks.OnModelRemapped != nil {
		p.callbacks.OnModelRemapped(ctx, &ModelRemapEvent{
			RequestedModel: routeRequest.RequestedModel,
			ResolvedModel:  resolution.ResolvedModel,
			RemoteModel:    invocation.remoteModelName(),
			Provider:       config.Provider,
			ChannelID:      selection.ChannelID,
			KeyID:          selection.KeyID,
			BaseURL:        config.BaseURL,
		})
	}

	return invocation, nil
}

func (p *ChannelRoutedProvider) recordUsage(
	ctx context.Context,
	invocation *resolvedChannelInvocation,
	success bool,
	errMsg string,
	usage *ChatUsage,
	latency time.Duration,
) {
	if invocation == nil {
		return
	}

	record := &ChannelUsageRecord{
		Capability:     invocation.capability(),
		Mode:           invocation.mode(),
		Attempt:        invocation.attempt(),
		TraceID:        invocation.traceID(),
		ChannelID:      invocation.channelID(),
		KeyID:          invocation.keyID(),
		Provider:       invocation.providerName(),
		RequestedModel: invocation.requestedModel(),
		RemoteModel:    invocation.remoteModelName(),
		BaseURL:        invocation.baseURL(),
		Success:        success,
		ErrorMessage:   strings.TrimSpace(errMsg),
		LatencyMS:      latency.Milliseconds(),
		Usage:          cloneChatUsage(usage),
		Metadata:       invocation.metadata(),
	}

	var errs []error
	if err := p.usageRecorder.RecordUsage(ctx, record); err != nil {
		errs = append(errs, fmt.Errorf("usage recorder: %w", err))
		p.logger.Warn("failed to record channel route usage", zap.Error(err))
	}
	if err := p.cooldownController.RecordResult(ctx, record); err != nil {
		errs = append(errs, fmt.Errorf("cooldown controller: %w", err))
		p.logger.Warn("failed to record cooldown result", zap.Error(err))
	}
	if err := p.quotaPolicy.RecordUsage(ctx, record); err != nil {
		errs = append(errs, fmt.Errorf("quota policy: %w", err))
		p.logger.Warn("failed to record quota usage", zap.Error(err))
	}
	if p.callbacks.OnUsageRecorded != nil {
		p.callbacks.OnUsageRecorded(ctx, cloneChannelUsageRecord(record), errors.Join(errs...))
	}
}

func (p *ChannelRoutedProvider) shouldRetry(ctx context.Context, err error, selection *ChannelSelection, attempt int) bool {
	if attempt >= p.maxAttempts || err == nil {
		return false
	}
	return p.retryPolicy.ShouldRetry(ctx, err, selection)
}

func (p *ChannelRoutedProvider) relayStreamWithRetry(
	ctx context.Context,
	out chan<- StreamChunk,
	req *ChatRequest,
	state retryExclusionState,
	invocation *resolvedChannelInvocation,
	source <-chan StreamChunk,
) {
	defer close(out)

	currentState := state
	currentInvocation := invocation
	currentSource := source

	for {
		start := time.Now()
		success, retryableFailure, errMsg, usage := p.relayStreamAttempt(ctx, out, req, currentInvocation, currentSource)
		p.recordUsage(ctx, currentInvocation, success, errMsg, usage, time.Since(start))
		if !retryableFailure {
			return
		}

		currentState = currentState.exclude(currentInvocation.selection, p.retryPolicy.ExcludeFailedChannel)
		nextInvocation, nextSource, nextState, err := p.openStreamRetry(ctx, req, currentInvocation.attempt()+1, currentState)
		if err != nil {
			out <- StreamChunk{Err: toTypesError(err)}
			return
		}
		currentState = nextState
		currentInvocation = nextInvocation
		currentSource = nextSource
	}
}

func (p *ChannelRoutedProvider) relayStreamAttempt(
	ctx context.Context,
	out chan<- StreamChunk,
	req *ChatRequest,
	invocation *resolvedChannelInvocation,
	source <-chan StreamChunk,
) (success bool, retryableFailure bool, errMsg string, usage *ChatUsage) {
	emitted := false
	for chunk := range source {
		if chunk.Usage != nil {
			usageCopy := *chunk.Usage
			usage = &usageCopy
		}
		if chunk.Err != nil {
			errMsg = chunk.Err.Error()
			if !emitted && p.shouldRetry(ctx, chunk.Err, invocation.selection, invocation.attempt()) {
				return false, true, errMsg, usage
			}
			if strings.TrimSpace(chunk.Provider) == "" {
				chunk.Provider = invocation.providerName()
			}
			if strings.TrimSpace(chunk.Model) == "" {
				chunk.Model = invocation.remoteModelName()
			}
			out <- chunk
			return false, false, errMsg, usage
		}
		if strings.TrimSpace(chunk.Provider) == "" {
			chunk.Provider = invocation.providerName()
		}
		if strings.TrimSpace(chunk.Model) == "" {
			chunk.Model = invocation.remoteModelName()
		}
		out <- chunk
		emitted = true
	}
	return true, false, "", usage
}

func (p *ChannelRoutedProvider) openStreamRetry(
	ctx context.Context,
	req *ChatRequest,
	startAttempt int,
	state retryExclusionState,
) (*resolvedChannelInvocation, <-chan StreamChunk, retryExclusionState, error) {
	currentState := state
	var lastErr error

	for attempt := startAttempt; attempt <= p.maxAttempts; attempt++ {
		attemptStart := time.Now()
		routeRequest := buildChannelRouteRequest(req, RouteModeStream, attempt, currentState)
		invocation, err := p.prepareInvocation(ctx, routeRequest)
		if err != nil {
			lastErr = err
			if invocation.hasSelection() {
				p.recordUsage(ctx, invocation, false, err.Error(), nil, time.Since(attemptStart))
			}
			if !invocation.hasSelection() || !p.shouldRetry(ctx, err, invocation.selection, attempt) {
				return nil, nil, currentState, err
			}
			currentState = currentState.exclude(invocation.selection, p.retryPolicy.ExcludeFailedChannel)
			continue
		}

		callStart := time.Now()
		routedReq := cloneChatRequest(req, invocation.remoteModelName())
		source, streamErr := invocation.provider.Stream(ctx, routedReq)
		if streamErr == nil {
			return invocation, source, currentState, nil
		}

		p.recordUsage(ctx, invocation, false, streamErr.Error(), nil, time.Since(callStart))
		lastErr = streamErr
		if !p.shouldRetry(ctx, streamErr, invocation.selection, attempt) {
			return nil, nil, currentState, streamErr
		}
		currentState = currentState.exclude(invocation.selection, p.retryPolicy.ExcludeFailedChannel)
	}

	return nil, nil, currentState, firstNonNilError(lastErr, types.NewServiceUnavailableError("channel routed provider exhausted retry attempts"))
}

func buildChannelRouteRequest(req *ChatRequest, mode RouteMode, attempt int, state retryExclusionState) *ChannelRouteRequest {
	return &ChannelRouteRequest{
		Capability:         RouteCapabilityChat,
		Mode:               mode,
		Attempt:            attempt,
		TraceID:            strings.TrimSpace(req.TraceID),
		RequestedModel:     strings.TrimSpace(req.Model),
		ProviderHint:       extractProviderHint(req),
		RoutePolicy:        normalizeChannelRoutePolicy(req),
		Region:             extractRegionHint(req),
		Metadata:           cloneStringMap(req.Metadata),
		Tags:               cloneStrings(req.Tags),
		ExcludedChannelIDs: state.channelIDs(),
		ExcludedKeyIDs:     state.keyIDs(),
	}
}

func normalizeChannelRoutePolicy(req *ChatRequest) string {
	if req == nil || len(req.Metadata) == 0 {
		return "balanced"
	}
	raw := strings.ToLower(strings.TrimSpace(req.RoutePolicy))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(req.Metadata["route_policy"]))
	}
	switch raw {
	case "", "balanced":
		return "balanced"
	case "cost", "cost_first":
		return "cost_first"
	case "health", "health_first":
		return "health_first"
	case "latency", "latency_first":
		return "latency_first"
	default:
		return raw
	}
}

func extractRegionHint(req *ChatRequest) string {
	if req == nil || len(req.Metadata) == 0 {
		return ""
	}
	return firstNonEmpty(
		req.Metadata["region"],
		req.Metadata["provider_region"],
		req.Metadata["llm_region"],
	)
}

func usageFromResponse(resp *ChatResponse) *ChatUsage {
	if resp == nil {
		return nil
	}
	usage := resp.Usage
	return &usage
}

func cloneChatUsage(usage *ChatUsage) *ChatUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	if usage.PromptTokensDetails != nil {
		details := *usage.PromptTokensDetails
		cloned.PromptTokensDetails = &details
	}
	if usage.CompletionTokensDetails != nil {
		details := *usage.CompletionTokensDetails
		cloned.CompletionTokensDetails = &details
	}
	return &cloned
}

func cloneChannelSelection(selection *ChannelSelection) *ChannelSelection {
	if selection == nil {
		return nil
	}
	cloned := *selection
	cloned.Metadata = cloneStringMap(selection.Metadata)
	return &cloned
}

func cloneChannelUsageRecord(record *ChannelUsageRecord) *ChannelUsageRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	cloned.Metadata = cloneStringMap(record.Metadata)
	cloned.Usage = cloneChatUsage(record.Usage)
	return &cloned
}

func normalizeChannelRouteRetryConfig(maxAttempts int, policy ChannelRouteRetryPolicy) (int, ChannelRouteRetryPolicy) {
	if policy.MaxAttempts > 0 {
		maxAttempts = policy.MaxAttempts
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if policy.ShouldRetry == nil {
		policy.ShouldRetry = func(_ context.Context, err error, _ *ChannelSelection) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		}
	}
	policy.MaxAttempts = maxAttempts
	return maxAttempts, policy
}

type retryExclusionState struct {
	excludedChannelIDs []string
	excludedKeyIDs     []string
}

func (s retryExclusionState) exclude(selection *ChannelSelection, excludeChannel bool) retryExclusionState {
	next := retryExclusionState{
		excludedChannelIDs: cloneStrings(s.excludedChannelIDs),
		excludedKeyIDs:     cloneStrings(s.excludedKeyIDs),
	}
	if selection == nil {
		return next
	}
	if keyID := strings.TrimSpace(selection.KeyID); keyID != "" {
		next.excludedKeyIDs = appendUniqueString(next.excludedKeyIDs, keyID)
	}
	if excludeChannel || strings.TrimSpace(selection.KeyID) == "" {
		if channelID := strings.TrimSpace(selection.ChannelID); channelID != "" {
			next.excludedChannelIDs = appendUniqueString(next.excludedChannelIDs, channelID)
		}
	}
	return next
}

func (s retryExclusionState) channelIDs() []string {
	return cloneStrings(s.excludedChannelIDs)
}

func (s retryExclusionState) keyIDs() []string {
	return cloneStrings(s.excludedKeyIDs)
}

func appendUniqueString(values []string, raw string) []string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func firstNonNilError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func toTypesError(err error) *types.Error {
	if err == nil {
		return nil
	}
	if typed, ok := err.(*types.Error); ok {
		return typed
	}
	return types.WrapError(err, types.ErrUpstreamError, err.Error())
}

func (i *resolvedChannelInvocation) hasSelection() bool {
	return i != nil && i.selection != nil
}

func (i *resolvedChannelInvocation) capability() RouteCapability {
	if i == nil || i.request == nil {
		return ""
	}
	return i.request.Capability
}

func (i *resolvedChannelInvocation) mode() RouteMode {
	if i == nil || i.request == nil {
		return ""
	}
	return i.request.Mode
}

func (i *resolvedChannelInvocation) attempt() int {
	if i == nil || i.request == nil {
		return 0
	}
	return i.request.Attempt
}

func (i *resolvedChannelInvocation) traceID() string {
	if i == nil || i.request == nil {
		return ""
	}
	return i.request.TraceID
}

func (i *resolvedChannelInvocation) channelID() string {
	if i == nil || i.selection == nil {
		return ""
	}
	return i.selection.ChannelID
}

func (i *resolvedChannelInvocation) keyID() string {
	if i == nil || i.selection == nil {
		return ""
	}
	return i.selection.KeyID
}

func (i *resolvedChannelInvocation) providerName() string {
	if i == nil {
		return ""
	}
	if i.config != nil {
		return firstNonEmpty(i.config.Provider, selectionProvider(i.selection))
	}
	return selectionProvider(i.selection)
}

func (i *resolvedChannelInvocation) requestedModel() string {
	if i == nil || i.request == nil {
		return ""
	}
	return i.request.RequestedModel
}

func (i *resolvedChannelInvocation) remoteModelName() string {
	if i == nil {
		return ""
	}
	return firstNonEmpty(
		i.remoteModel,
		configModel(i.config),
		selectionRemoteModel(i.selection),
		resolutionModel(i.resolution),
		i.requestedModel(),
	)
}

func (i *resolvedChannelInvocation) baseURL() string {
	if i == nil {
		return ""
	}
	return firstNonEmpty(configBaseURL(i.config), selectionBaseURL(i.selection))
}

func (i *resolvedChannelInvocation) metadata() map[string]string {
	if i == nil {
		return nil
	}
	if i.selection != nil && len(i.selection.Metadata) != 0 {
		return cloneStringMap(i.selection.Metadata)
	}
	if i.config != nil && len(i.config.Metadata) != 0 {
		return cloneStringMap(i.config.Metadata)
	}
	if i.request != nil && len(i.request.Metadata) != 0 {
		return cloneStringMap(i.request.Metadata)
	}
	return nil
}

func selectionProvider(selection *ChannelSelection) string {
	if selection == nil {
		return ""
	}
	return selection.Provider
}

func selectionRemoteModel(selection *ChannelSelection) string {
	if selection == nil {
		return ""
	}
	return selection.RemoteModel
}

func selectionBaseURL(selection *ChannelSelection) string {
	if selection == nil {
		return ""
	}
	return selection.BaseURL
}

func configModel(config *ChannelProviderConfig) string {
	if config == nil {
		return ""
	}
	return config.Model
}

func configBaseURL(config *ChannelProviderConfig) string {
	if config == nil {
		return ""
	}
	return config.BaseURL
}

func resolutionModel(resolution *ModelResolution) string {
	if resolution == nil {
		return ""
	}
	return resolution.ResolvedModel
}
