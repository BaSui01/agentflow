package agent

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/llm"
)

// runConfigKey is the unexported context key for RunConfig.
type runConfigKey struct{}

// RunConfig provides runtime overrides for Agent execution.
// All pointer fields use nil to indicate "no override" — only non-nil values
// are applied, leaving the base Config defaults intact.
type RunConfig struct {
	Model              *string           `json:"model,omitempty"`
	Temperature        *float32          `json:"temperature,omitempty"`
	MaxTokens          *int              `json:"max_tokens,omitempty"`
	TopP               *float32          `json:"top_p,omitempty"`
	Stop               []string          `json:"stop,omitempty"`
	ToolChoice         *string           `json:"tool_choice,omitempty"`
	Timeout            *time.Duration    `json:"timeout,omitempty"`
	MaxReActIterations *int              `json:"max_react_iterations,omitempty"`
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
func (rc *RunConfig) ApplyToRequest(req *llm.ChatRequest, baseCfg Config) {
	if rc == nil || req == nil {
		return
	}

	if rc.Model != nil {
		req.Model = *rc.Model
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
	if len(rc.Tags) > 0 {
		req.Tags = rc.Tags
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

// --- Pointer helper functions ---

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string { return &s }

// Float32Ptr returns a pointer to the given float32.
func Float32Ptr(f float32) *float32 { return &f }

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int { return &i }

// DurationPtr returns a pointer to the given time.Duration.
func DurationPtr(d time.Duration) *time.Duration { return &d }
