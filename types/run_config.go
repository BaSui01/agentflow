package types

import (
	"strings"
	"time"
)

// RunConfig provides provider-neutral runtime overrides for an execution.
// Pointer fields use nil to mean "no override"; non-nil zero values are
// intentional overrides and must be preserved across boundaries.
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

// Clone returns a detached copy of the runtime override contract.
func (rc *RunConfig) Clone() *RunConfig {
	if rc == nil {
		return nil
	}
	out := *rc
	out.Model = cloneExecutionStringPtr(rc.Model)
	out.Provider = cloneExecutionStringPtr(rc.Provider)
	out.RoutePolicy = cloneExecutionStringPtr(rc.RoutePolicy)
	out.Temperature = cloneExecutionFloat32Ptr(rc.Temperature)
	out.MaxTokens = cloneExecutionIntPtr(rc.MaxTokens)
	out.TopP = cloneExecutionFloat32Ptr(rc.TopP)
	out.Stop = append([]string(nil), rc.Stop...)
	out.ToolChoice = cloneExecutionStringPtr(rc.ToolChoice)
	out.ToolWhitelist = append([]string(nil), rc.ToolWhitelist...)
	out.Timeout = cloneRunConfigDurationPtr(rc.Timeout)
	out.MaxReActIterations = cloneExecutionIntPtr(rc.MaxReActIterations)
	out.MaxLoopIterations = cloneExecutionIntPtr(rc.MaxLoopIterations)
	out.Metadata = cloneRunConfigMetadata(rc.Metadata)
	out.Tags = append([]string(nil), rc.Tags...)
	return &out
}

// ApplyToExecutionOptions applies RunConfig overrides to the provider-neutral
// execution options consumed by agent, team, and workflow runtimes.
func (rc *RunConfig) ApplyToExecutionOptions(opts *ExecutionOptions) {
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
		opts.Tools.ToolChoice = ParseToolChoiceString(strings.TrimSpace(*rc.ToolChoice))
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

func cloneRunConfigDurationPtr(value *time.Duration) *time.Duration {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneRunConfigMetadata(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]string, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
