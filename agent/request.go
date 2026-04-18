package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// runConfigKey is the unexported context key for RunConfig.
type runConfigKey struct{}

// RunConfig provides runtime overrides for Agent execution.
// All pointer fields use nil to indicate "no override" — only non-nil values
// are applied, leaving the base Config defaults intact.
type RunConfig struct {
	Model              *string           `json:"model,omitempty"`
	Provider           *string           `json:"provider,omitempty"`
	RoutePolicy        *string           `json:"route_policy,omitempty"`
	Temperature        *float32          `json:"temperature,omitempty"`
	MaxTokens          *int              `json:"max_tokens,omitempty"`
	TopP               *float32          `json:"top_p,omitempty"`
	Stop               []string          `json:"stop,omitempty"`
	ToolChoice         *string           `json:"tool_choice,omitempty"`
	ToolWhitelist      []string          `json:"tool_whitelist,omitempty"`
	DisableTools       bool              `json:"disable_tools,omitempty"`
	Timeout            *time.Duration    `json:"timeout,omitempty"`
	MaxReActIterations *int              `json:"max_react_iterations,omitempty"`
	MaxLoopIterations  *int              `json:"max_loop_iterations,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
}

// WithRunConfig stores a RunConfig in the context.
func WithRunConfig(ctx context.Context, rc *RunConfig) context.Context {
	return context.WithValue(ctx, runConfigKey{}, rc)
}

// GetRunConfig retrieves the RunConfig from the context.
// Returns nil if no RunConfig is present.
func GetRunConfig(ctx context.Context) *RunConfig {
	rc, _ := ctx.Value(runConfigKey{}).(*RunConfig)
	return rc
}

// ApplyToRequest applies RunConfig overrides to a ChatRequest.
// Fields in baseCfg are used as defaults; only non-nil RunConfig fields override them.
// If rc is nil, this is a no-op.
func (rc *RunConfig) ApplyToRequest(req *types.ChatRequest, baseCfg types.AgentConfig) {
	if rc == nil || req == nil {
		return
	}

	if rc.Model != nil {
		req.Model = *rc.Model
	}
	if rc.Provider != nil {
		provider := strings.TrimSpace(*rc.Provider)
		if provider != "" {
			if req.Metadata == nil {
				req.Metadata = make(map[string]string, 2)
			}
			req.Metadata[llmcore.MetadataKeyChatProvider] = provider
		}
	}
	if rc.RoutePolicy != nil {
		routePolicy := strings.TrimSpace(*rc.RoutePolicy)
		if routePolicy != "" {
			if req.Metadata == nil {
				req.Metadata = make(map[string]string, 2)
			}
			req.Metadata["route_policy"] = routePolicy
		}
	}
	if rc.Temperature != nil {
		req.Temperature = *rc.Temperature
	}
	if rc.MaxTokens != nil {
		req.MaxTokens = *rc.MaxTokens
	}
	if rc.TopP != nil {
		req.TopP = *rc.TopP
	}
	if len(rc.Stop) > 0 {
		req.Stop = rc.Stop
	}
	if rc.ToolChoice != nil {
		req.ToolChoice = *rc.ToolChoice
	}
	if rc.Timeout != nil {
		req.Timeout = *rc.Timeout
	}
	if len(rc.Metadata) > 0 {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, len(rc.Metadata))
		}
		for k, v := range rc.Metadata {
			req.Metadata[k] = v
		}
	}
	if rc.MaxLoopIterations != nil {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, 1)
		}
		req.Metadata["max_loop_iterations"] = strconv.Itoa(*rc.MaxLoopIterations)
	}
	if len(rc.Tags) > 0 {
		req.Tags = rc.Tags
	}
}

// ApplyToExecutionOptions applies RunConfig overrides to the provider-agnostic
// execution options consumed by the agent runtime.
func (rc *RunConfig) ApplyToExecutionOptions(opts *types.ExecutionOptions) {
	if rc == nil || opts == nil {
		return
	}

	if rc.Model != nil {
		opts.Model.Model = *rc.Model
	}
	if rc.Provider != nil {
		opts.Model.Provider = strings.TrimSpace(*rc.Provider)
	}
	if rc.RoutePolicy != nil {
		opts.Model.RoutePolicy = strings.TrimSpace(*rc.RoutePolicy)
	}
	if rc.Temperature != nil {
		opts.Model.Temperature = *rc.Temperature
	}
	if rc.MaxTokens != nil {
		opts.Model.MaxTokens = *rc.MaxTokens
	}
	if rc.TopP != nil {
		opts.Model.TopP = *rc.TopP
	}
	if len(rc.Stop) > 0 {
		opts.Model.Stop = append([]string(nil), rc.Stop...)
	}
	if rc.ToolChoice != nil {
		opts.Tools.ToolChoice = types.ParseToolChoiceString(strings.TrimSpace(*rc.ToolChoice))
	}
	if rc.DisableTools {
		opts.Tools.DisableTools = true
		opts.Tools.ToolWhitelist = nil
	}
	if len(rc.ToolWhitelist) > 0 {
		opts.Tools.ToolWhitelist = append([]string(nil), rc.ToolWhitelist...)
		opts.Tools.DisableTools = false
	}
	if rc.Timeout != nil {
		opts.Control.Timeout = *rc.Timeout
	}
	if rc.MaxReActIterations != nil {
		opts.Control.MaxReActIterations = *rc.MaxReActIterations
	}
	if rc.MaxLoopIterations != nil {
		opts.Control.MaxLoopIterations = *rc.MaxLoopIterations
	}
	if len(rc.Metadata) > 0 {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]string, len(rc.Metadata))
		}
		for key, value := range rc.Metadata {
			opts.Metadata[key] = value
		}
	}
	if len(rc.Tags) > 0 {
		opts.Tags = append([]string(nil), rc.Tags...)
	}
	if opts.Control.DisablePlanner {
		return
	}
	opts.Control.DisablePlanner = parseBoolString(opts.Metadata["disable_planner"])
}

// EffectiveMaxReActIterations returns the RunConfig override if set,
// otherwise falls back to defaultVal.
func (rc *RunConfig) EffectiveMaxReActIterations(defaultVal int) int {
	if rc != nil && rc.MaxReActIterations != nil {
		return *rc.MaxReActIterations
	}
	return defaultVal
}

// EffectiveMaxLoopIterations returns the RunConfig override if set,
// otherwise falls back to defaultVal.
func (rc *RunConfig) EffectiveMaxLoopIterations(defaultVal int) int {
	if rc != nil && rc.MaxLoopIterations != nil {
		return *rc.MaxLoopIterations
	}
	return defaultVal
}

// ResolveRunConfig merges context-level config, Input.Context-derived config,
// and explicit input overrides into a single effective RunConfig.
func ResolveRunConfig(ctx context.Context, input *Input) *RunConfig {
	rc := GetRunConfig(ctx)
	if input == nil {
		return rc
	}
	rc = MergeRunConfig(rc, RunConfigFromInputContext(input.Context))
	rc = MergeRunConfig(rc, input.Overrides)
	return rc
}

// DisablePlannerEnabled reports whether runtime planner phases should be skipped.
// This flag exists for specialist execution tasks that an upstream orchestrator has
// already decomposed, so the runtime does not wrap the subtask into another plan.
// Input.Context["disable_planner"] has precedence over RunConfig.Metadata.
func DisablePlannerEnabled(input *Input, rc *RunConfig) bool {
	if flag, ok := boolOverrideFromContext(inputContext(input), "disable_planner"); ok {
		return flag
	}
	if rc == nil || len(rc.Metadata) == 0 {
		return false
	}
	return parseBoolString(rc.Metadata["disable_planner"])
}

// MergeRunConfig merges two RunConfigs, preserving base values unless override
// explicitly provides a replacement. The returned config is always a deep copy.
func MergeRunConfig(base *RunConfig, override *RunConfig) *RunConfig {
	switch {
	case base == nil && override == nil:
		return nil
	case base == nil:
		return cloneRunConfig(override)
	case override == nil:
		return cloneRunConfig(base)
	}

	merged := cloneRunConfig(base)
	if override.Model != nil {
		merged.Model = cloneStringPtr(override.Model)
	}
	if override.Provider != nil {
		merged.Provider = cloneStringPtr(override.Provider)
	}
	if override.RoutePolicy != nil {
		merged.RoutePolicy = cloneStringPtr(override.RoutePolicy)
	}
	if override.Temperature != nil {
		merged.Temperature = cloneFloat32Ptr(override.Temperature)
	}
	if override.MaxTokens != nil {
		merged.MaxTokens = cloneIntPtr(override.MaxTokens)
	}
	if override.TopP != nil {
		merged.TopP = cloneFloat32Ptr(override.TopP)
	}
	if len(override.Stop) > 0 {
		merged.Stop = append([]string(nil), override.Stop...)
	}
	if override.ToolChoice != nil {
		merged.ToolChoice = cloneStringPtr(override.ToolChoice)
	}
	if override.DisableTools {
		merged.DisableTools = true
		merged.ToolWhitelist = nil
	}
	if len(override.ToolWhitelist) > 0 {
		merged.ToolWhitelist = append([]string(nil), override.ToolWhitelist...)
		merged.DisableTools = false
	}
	if override.Timeout != nil {
		merged.Timeout = cloneDurationPtr(override.Timeout)
	}
	if override.MaxReActIterations != nil {
		merged.MaxReActIterations = cloneIntPtr(override.MaxReActIterations)
	}
	if override.MaxLoopIterations != nil {
		merged.MaxLoopIterations = cloneIntPtr(override.MaxLoopIterations)
	}
	if len(override.Metadata) > 0 {
		if merged.Metadata == nil {
			merged.Metadata = make(map[string]string, len(override.Metadata))
		}
		for k, v := range override.Metadata {
			merged.Metadata[k] = v
		}
	}
	if len(override.Tags) > 0 {
		merged.Tags = append([]string(nil), override.Tags...)
	}
	return merged
}

func cloneRunConfig(rc *RunConfig) *RunConfig {
	if rc == nil {
		return nil
	}
	out := *rc
	out.Model = cloneStringPtr(rc.Model)
	out.Provider = cloneStringPtr(rc.Provider)
	out.RoutePolicy = cloneStringPtr(rc.RoutePolicy)
	out.Temperature = cloneFloat32Ptr(rc.Temperature)
	out.MaxTokens = cloneIntPtr(rc.MaxTokens)
	out.TopP = cloneFloat32Ptr(rc.TopP)
	out.Stop = append([]string(nil), rc.Stop...)
	out.ToolChoice = cloneStringPtr(rc.ToolChoice)
	out.ToolWhitelist = append([]string(nil), rc.ToolWhitelist...)
	out.Timeout = cloneDurationPtr(rc.Timeout)
	out.MaxReActIterations = cloneIntPtr(rc.MaxReActIterations)
	out.MaxLoopIterations = cloneIntPtr(rc.MaxLoopIterations)
	out.Metadata = cloneStringMap(rc.Metadata)
	out.Tags = append([]string(nil), rc.Tags...)
	return &out
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneFloat32Ptr(v *float32) *float32 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneDurationPtr(v *time.Duration) *time.Duration {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string { return &s }

// Float32Ptr returns a pointer to the given float32.
func Float32Ptr(f float32) *float32 { return &f }

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int { return &i }

// DurationPtr returns a pointer to the given time.Duration.
func DurationPtr(d time.Duration) *time.Duration { return &d }

// preparedRequest holds the fully-built ChatRequest together with provider
// references needed by the execution paths (streaming, ReAct, plain completion).
type preparedRequest struct {
	req          *types.ChatRequest
	chatProvider llm.Provider
	toolProvider llm.Provider // for ReAct loop (may equal chatProvider)
	hasTools     bool
	handoffTools map[string]RuntimeHandoffTarget
	toolRisks    map[string]string
	maxReActIter int
	maxLoopIter  int
	options      types.ExecutionOptions
}

// prepareChatRequest builds a ChatRequest from messages, applying context
// engineering, model selection, RunConfig overrides, route hints, and tool
// filtering. Both ChatCompletion and StreamCompletion delegate here so that
// the logic is maintained in a single place.
func (b *BaseAgent) prepareChatRequest(ctx context.Context, messages []types.Message) (*preparedRequest, error) {
	if !b.hasMainExecutionSurface() {
		return nil, ErrProviderNotSet
	}
	if messages == nil || len(messages) == 0 {
		return nil, NewError(types.ErrInputValidation, "messages cannot be nil or empty")
	}

	chatProv := b.gatewayProvider()
	options := b.executionOptionsResolver().Resolve(ctx, b.config, nil)
	req, err := b.chatRequestAdapter().Build(options, messages)
	if err != nil {
		return nil, err
	}

	// 1. Tool whitelist filtering
	if b.toolManager != nil {
		allowedTools := b.toolManager.GetAllowedTools(b.config.Core.ID)
		switch {
		case options.Tools.DisableTools:
			req.Tools = nil
		case len(options.Tools.ToolWhitelist) > 0:
			req.Tools = filterToolSchemasByWhitelist(allowedTools, options.Tools.ToolWhitelist)
		case len(options.Tools.AllowedTools) > 0:
			req.Tools = filterToolSchemasByWhitelist(allowedTools, options.Tools.AllowedTools)
		}
	}
	handoffMap := map[string]RuntimeHandoffTarget(nil)
	handoffTargets := runtimeHandoffTargetsFromContext(ctx, b.config.Core.ID)
	if len(handoffTargets) > 0 {
		if len(req.Tools) == 0 {
			req.Tools = make([]types.ToolSchema, 0, len(handoffTargets))
		}
		handoffMap = make(map[string]RuntimeHandoffTarget, len(handoffTargets))
		seen := make(map[string]struct{}, len(req.Tools))
		for _, schema := range req.Tools {
			seen[schema.Name] = struct{}{}
		}
		for _, target := range handoffTargets {
			schema := runtimeHandoffToolSchema(target)
			handoffMap[schema.Name] = target
			if _, exists := seen[schema.Name]; exists {
				continue
			}
			seen[schema.Name] = struct{}{}
			req.Tools = append(req.Tools, schema)
		}
		if len(handoffMap) > 0 {
			if req.Metadata == nil {
				req.Metadata = make(map[string]string, 1)
			}
			req.Metadata["handoff_enabled"] = "true"
		}
	}

	// 2. 选择执行 provider。工具协议差异（如 XML fallback）统一在 llm/gateway 内处理。
	toolProv := chatProv
	if b.hasDedicatedToolExecutionSurface() {
		toolProv = b.gatewayToolProvider()
	}

	// 3. Effective loop budgets
	effectiveIter := options.Control.MaxReActIterations
	if effectiveIter <= 0 {
		effectiveIter = b.maxReActIterations()
	}
	toolRisks := make(map[string]string, len(req.Tools))
	for _, tool := range req.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		toolRisks[name] = classifyToolRiskByName(name)
	}

	return &preparedRequest{
		req:          req,
		chatProvider: chatProv,
		toolProvider: toolProv,
		hasTools:     len(req.Tools) > 0 && (b.toolManager != nil || len(handoffTargets) > 0),
		handoffTools: handoffMap,
		toolRisks:    toolRisks,
		maxReActIter: effectiveIter,
		maxLoopIter:  options.Control.MaxLoopIterations,
		options:      options,
	}, nil
}

// RunConfigFromInputContext extracts supported runtime overrides from Input.Context-style maps.
// Unknown keys are ignored.
func RunConfigFromInputContext(inputCtx map[string]any) *RunConfig {
	if len(inputCtx) == 0 {
		return nil
	}
	var rc RunConfig
	var hasOverride bool

	if value, ok := intOverrideFromContext(inputCtx, "max_react_iterations"); ok {
		rc.MaxReActIterations = IntPtr(value)
		hasOverride = true
	}
	if value, ok := intOverrideFromContext(inputCtx, "max_loop_iterations"); ok {
		rc.MaxLoopIterations = IntPtr(value)
		hasOverride = true
	}

	if !hasOverride {
		return nil
	}
	return &rc
}

func intOverrideFromContext(values map[string]any, key string) (int, bool) {
	if len(values) == 0 {
		return 0, false
	}
	raw, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := raw.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case json.Number:
		if value, err := typed.Int64(); err == nil {
			return int(value), true
		}
		if value, err := typed.Float64(); err == nil {
			return int(value), true
		}
		return 0, false
	case string:
		value, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

func boolOverrideFromContext(values map[string]any, key string) (bool, bool) {
	if len(values) == 0 {
		return false, false
	}
	raw, ok := values[key]
	if !ok {
		return false, false
	}
	switch typed := raw.(type) {
	case bool:
		return typed, true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return false, false
		}
		parsed, err := strconv.ParseBool(text)
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
}

func parseBoolString(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

func inputContext(input *Input) map[string]any {
	if input == nil {
		return nil
	}
	return input.Context
}

// lastUserQuery extracts the content of the last user message.
func lastUserQuery(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// effectiveToolModel returns the tool-specific model if configured, otherwise
// falls back to the main model.
func effectiveToolModel(mainModel string, configuredToolModel string) string {
	if v := strings.TrimSpace(configuredToolModel); v != "" {
		return v
	}
	return mainModel
}
