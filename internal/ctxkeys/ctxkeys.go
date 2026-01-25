package ctxkeys

import "context"

// contextKey 用于在 context 中存储值的键类型
type contextKey string

const (
	traceIDKey             contextKey = "trace_id"
	runIDKey               contextKey = "run_id"
	promptBundleVersionKey contextKey = "prompt_bundle_version"
	llmModelKey            contextKey = "llm_model"
)

// WithTraceID 设置 TraceID
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceID 获取 TraceID
func TraceID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(traceIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// WithRunID 设置 RunID
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKey, runID)
}

// RunID 获取 RunID
func RunID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(runIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// WithPromptBundleVersion 设置 PromptBundle 版本
func WithPromptBundleVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, promptBundleVersionKey, version)
}

// PromptBundleVersion 获取 PromptBundle 版本
func PromptBundleVersion(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(promptBundleVersionKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// WithLLMModel 设置 LLM 模型（用于覆盖默认模型）
func WithLLMModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, llmModelKey, model)
}

// LLMModel 获取 LLM 模型
func LLMModel(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(llmModelKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
