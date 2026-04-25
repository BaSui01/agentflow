package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// ExecutionOptionsResolver resolves a provider-agnostic runtime view from the
// current agent configuration and request-scoped overrides.
type ExecutionOptionsResolver interface {
	Resolve(ctx context.Context, cfg types.AgentConfig, input *Input) types.ExecutionOptions
}

// DefaultExecutionOptionsResolver is the runtime's canonical resolver.
type DefaultExecutionOptionsResolver struct{}

func NewDefaultExecutionOptionsResolver() ExecutionOptionsResolver {
	return DefaultExecutionOptionsResolver{}
}

// Resolve builds execution options from AgentConfig plus context-scoped hints
// and RunConfig overrides. The returned value is detached from cfg.
func (DefaultExecutionOptionsResolver) Resolve(ctx context.Context, cfg types.AgentConfig, input *Input) types.ExecutionOptions {
	options := cfg.ExecutionOptions()
	if model, ok := types.LLMModel(ctx); ok && strings.TrimSpace(model) != "" {
		options.Model.Model = strings.TrimSpace(model)
	}
	if provider, ok := types.LLMProvider(ctx); ok && strings.TrimSpace(provider) != "" {
		options.Model.Provider = strings.TrimSpace(provider)
	}
	if routePolicy, ok := types.LLMRoutePolicy(ctx); ok && strings.TrimSpace(routePolicy) != "" {
		options.Model.RoutePolicy = strings.TrimSpace(routePolicy)
	}
	if rc := ResolveRunConfig(ctx, input); rc != nil {
		rc.ApplyToExecutionOptions(&options)
	}
	if flag, ok := boolOverrideFromContext(inputContext(input), "disable_planner"); ok {
		options.Control.DisablePlanner = flag
	}
	if value, ok := intOverrideFromContext(inputContext(input), "top_level_loop_budget"); ok && value > 0 {
		options.Control.MaxLoopIterations = value
	}
	return options
}

func contextBool(input *Input, key string) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func contextString(input *Input, key string) string {
	if input == nil || len(input.Context) == 0 {
		return ""
	}
	value, ok := input.Context[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func intContextAtLeast(input *Input, key string, min int) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case int:
		return typed >= min
	case int32:
		return int(typed) >= min
	case int64:
		return int(typed) >= min
	case float64:
		return int(typed) >= min
	default:
		return false
	}
}

func contentContainsAny(input *Input, terms ...string) bool {
	if input == nil {
		return false
	}
	content := strings.ToLower(input.Content)
	for _, term := range terms {
		if strings.Contains(content, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

// runConfigKey is the unexported context key for RunConfig.
type runConfigKey struct{}

// RunConfig is the official provider-neutral runtime override contract.
type RunConfig = types.RunConfig

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
	return rc.Clone()
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
