package router

import "context"

// RouteCapability identifies the capability being routed.
type RouteCapability string

const (
	RouteCapabilityChat  RouteCapability = "chat"
	RouteCapabilityImage RouteCapability = "image"
	RouteCapabilityVideo RouteCapability = "video"
)

// RouteMode distinguishes sync and streaming entrypoints.
type RouteMode string

const (
	RouteModeCompletion RouteMode = "completion"
	RouteModeStream     RouteMode = "stream"
)

// ChannelRouteRequest is the capability-agnostic route planning input.
type ChannelRouteRequest struct {
	Capability         RouteCapability   `json:"capability"`
	Mode               RouteMode         `json:"mode"`
	Attempt            int               `json:"attempt,omitempty"`
	TraceID            string            `json:"trace_id,omitempty"`
	RequestedModel     string            `json:"requested_model,omitempty"`
	ProviderHint       string            `json:"provider_hint,omitempty"`
	RoutePolicy        string            `json:"route_policy,omitempty"`
	Region             string            `json:"region,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
	ExcludedChannelIDs []string          `json:"excluded_channel_ids,omitempty"`
	ExcludedKeyIDs     []string          `json:"excluded_key_ids,omitempty"`
}

// ModelResolution normalizes the requested public model before mapping lookup.
type ModelResolution struct {
	RequestedModel string            `json:"requested_model,omitempty"`
	ResolvedModel  string            `json:"resolved_model,omitempty"`
	ProviderHint   string            `json:"provider_hint,omitempty"`
	Region         string            `json:"region,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ChannelRouteRetryPolicy controls route retry behavior after a selected key/channel fails.
type ChannelRouteRetryPolicy struct {
	MaxAttempts          int
	ExcludeFailedChannel bool
	ShouldRetry          func(ctx context.Context, err error, selection *ChannelSelection) bool
}

// ChannelModelMapping describes a channel-specific mapping candidate.
type ChannelModelMapping struct {
	MappingID   string            `json:"mapping_id,omitempty"`
	ChannelID   string            `json:"channel_id,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	PublicModel string            `json:"public_model,omitempty"`
	RemoteModel string            `json:"remote_model,omitempty"`
	BaseURL     string            `json:"base_url,omitempty"`
	Region      string            `json:"region,omitempty"`
	Priority    int               `json:"priority,omitempty"`
	Weight      int               `json:"weight,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ChannelSelection is the concrete route selected for a request.
type ChannelSelection struct {
	MappingID   string            `json:"mapping_id,omitempty"`
	ChannelID   string            `json:"channel_id,omitempty"`
	KeyID       string            `json:"key_id,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	RemoteModel string            `json:"remote_model,omitempty"`
	BaseURL     string            `json:"base_url,omitempty"`
	Region      string            `json:"region,omitempty"`
	Priority    int               `json:"priority,omitempty"`
	Weight      int               `json:"weight,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ChannelSecret contains resolved secret material for a selected route.
type ChannelSecret struct {
	APIKey    string            `json:"api_key,omitempty"`
	SecretKey string            `json:"secret_key,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ChannelProviderConfig is the runtime provider configuration derived from a route selection.
type ChannelProviderConfig struct {
	Provider string            `json:"provider,omitempty"`
	BaseURL  string            `json:"base_url,omitempty"`
	Region   string            `json:"region,omitempty"`
	Model    string            `json:"model,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Extra    map[string]any    `json:"extra,omitempty"`
}

// ChannelUsageRecord captures the final outcome of one routed call.
type ChannelUsageRecord struct {
	Capability     RouteCapability   `json:"capability"`
	Mode           RouteMode         `json:"mode"`
	Attempt        int               `json:"attempt,omitempty"`
	TraceID        string            `json:"trace_id,omitempty"`
	ChannelID      string            `json:"channel_id,omitempty"`
	KeyID          string            `json:"key_id,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	RequestedModel string            `json:"requested_model,omitempty"`
	RemoteModel    string            `json:"remote_model,omitempty"`
	BaseURL        string            `json:"base_url,omitempty"`
	Success        bool              `json:"success"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	LatencyMS      int64             `json:"latency_ms,omitempty"`
	Usage          *ChatUsage        `json:"usage,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// ChannelSelector chooses the concrete channel/key route for a request.
type ChannelSelector interface {
	SelectChannel(ctx context.Context, request *ChannelRouteRequest, resolution *ModelResolution, mappings []ChannelModelMapping) (*ChannelSelection, error)
}

// ModelResolver normalizes the public model name before mapping lookup.
type ModelResolver interface {
	ResolveModel(ctx context.Context, request *ChannelRouteRequest) (*ModelResolution, error)
}

// ModelMappingResolver returns mapping candidates for the resolved model.
type ModelMappingResolver interface {
	ResolveMappings(ctx context.Context, request *ChannelRouteRequest, resolution *ModelResolution) ([]ChannelModelMapping, error)
}

// SecretResolver returns secret material for the selected route.
type SecretResolver interface {
	ResolveSecret(ctx context.Context, request *ChannelRouteRequest, selection *ChannelSelection) (*ChannelSecret, error)
}

// UsageRecorder persists final route usage outcomes.
type UsageRecorder interface {
	RecordUsage(ctx context.Context, usage *ChannelUsageRecord) error
}

// CooldownController filters or updates route cooldown state.
type CooldownController interface {
	Allow(ctx context.Context, request *ChannelRouteRequest, selection *ChannelSelection) error
	RecordResult(ctx context.Context, usage *ChannelUsageRecord) error
}

// QuotaPolicy enforces runtime quota constraints around a selected route.
type QuotaPolicy interface {
	Allow(ctx context.Context, request *ChannelRouteRequest, selection *ChannelSelection) error
	RecordUsage(ctx context.Context, usage *ChannelUsageRecord) error
}

// ProviderConfigSource resolves runtime provider config from the selected route.
type ProviderConfigSource interface {
	ResolveProviderConfig(ctx context.Context, request *ChannelRouteRequest, selection *ChannelSelection) (*ChannelProviderConfig, error)
}

// ModelRemapEvent is emitted when a public model is translated to an upstream model.
type ModelRemapEvent struct {
	RequestedModel string `json:"requested_model,omitempty"`
	ResolvedModel  string `json:"resolved_model,omitempty"`
	RemoteModel    string `json:"remote_model,omitempty"`
	Provider       string `json:"provider,omitempty"`
	ChannelID      string `json:"channel_id,omitempty"`
	KeyID          string `json:"key_id,omitempty"`
	BaseURL        string `json:"base_url,omitempty"`
}

// ChannelRouteCallbacks exposes non-persistent route events for embedding runtimes.
type ChannelRouteCallbacks struct {
	OnKeySelected   func(ctx context.Context, selection *ChannelSelection)
	OnModelRemapped func(ctx context.Context, event *ModelRemapEvent)
	OnUsageRecorded func(ctx context.Context, usage *ChannelUsageRecord, err error)
}

// PassthroughModelResolver keeps the requested model unchanged.
type PassthroughModelResolver struct{}

func (PassthroughModelResolver) ResolveModel(_ context.Context, request *ChannelRouteRequest) (*ModelResolution, error) {
	if request == nil {
		return &ModelResolution{}, nil
	}
	return &ModelResolution{
		RequestedModel: request.RequestedModel,
		ResolvedModel:  request.RequestedModel,
		ProviderHint:   request.ProviderHint,
		Region:         request.Region,
		Metadata:       cloneStringMap(request.Metadata),
	}, nil
}

// StaticProviderConfigSource mirrors provider/baseURL/model information from the selected route.
type StaticProviderConfigSource struct{}

func (StaticProviderConfigSource) ResolveProviderConfig(_ context.Context, _ *ChannelRouteRequest, selection *ChannelSelection) (*ChannelProviderConfig, error) {
	if selection == nil {
		return &ChannelProviderConfig{}, nil
	}
	return &ChannelProviderConfig{
		Provider: selection.Provider,
		BaseURL:  selection.BaseURL,
		Region:   selection.Region,
		Model:    selection.RemoteModel,
		Metadata: cloneStringMap(selection.Metadata),
	}, nil
}

// NoopUsageRecorder is the default no-op implementation.
type NoopUsageRecorder struct{}

func (NoopUsageRecorder) RecordUsage(context.Context, *ChannelUsageRecord) error { return nil }

// NoopCooldownController is the default no-op implementation.
type NoopCooldownController struct{}

func (NoopCooldownController) Allow(context.Context, *ChannelRouteRequest, *ChannelSelection) error {
	return nil
}

func (NoopCooldownController) RecordResult(context.Context, *ChannelUsageRecord) error { return nil }

// NoopQuotaPolicy is the default no-op implementation.
type NoopQuotaPolicy struct{}

func (NoopQuotaPolicy) Allow(context.Context, *ChannelRouteRequest, *ChannelSelection) error {
	return nil
}

func (NoopQuotaPolicy) RecordUsage(context.Context, *ChannelUsageRecord) error { return nil }

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
