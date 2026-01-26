package types

import "context"

// contextKey is used for storing values in context.Context.
type contextKey string

const (
	keyTraceID             contextKey = "trace_id"
	keyTenantID            contextKey = "tenant_id"
	keyUserID              contextKey = "user_id"
	keyRunID               contextKey = "run_id"
	keyLLMModel            contextKey = "llm_model"
	keyPromptBundleVersion contextKey = "prompt_bundle_version"
)

// WithTraceID adds trace ID to context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, keyTraceID, traceID)
}

// TraceID extracts trace ID from context.
func TraceID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyTraceID).(string)
	return v, ok && v != ""
}

// WithTenantID adds tenant ID to context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, keyTenantID, tenantID)
}

// TenantID extracts tenant ID from context.
func TenantID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyTenantID).(string)
	return v, ok && v != ""
}

// WithUserID adds user ID to context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, keyUserID, userID)
}

// UserID extracts user ID from context.
func UserID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyUserID).(string)
	return v, ok && v != ""
}

// WithRunID adds run ID to context.
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, keyRunID, runID)
}

// RunID extracts run ID from context.
func RunID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyRunID).(string)
	return v, ok && v != ""
}

// WithLLMModel adds LLM model to context.
func WithLLMModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, keyLLMModel, model)
}

// LLMModel extracts LLM model from context.
func LLMModel(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyLLMModel).(string)
	return v, ok && v != ""
}

// WithPromptBundleVersion adds prompt bundle version to context.
func WithPromptBundleVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, keyPromptBundleVersion, version)
}

// PromptBundleVersion extracts prompt bundle version from context.
func PromptBundleVersion(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(keyPromptBundleVersion).(string)
	return v, ok && v != ""
}
