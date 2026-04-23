package core

import "context"

// ProviderPromptUsageReport captures prompt token accounting computed from a
// provider-specific request body immediately before the HTTP request is sent.
type ProviderPromptUsageReport struct {
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	API          string `json:"api,omitempty"`
	PromptTokens int    `json:"prompt_tokens,omitempty"`
}

type providerPromptUsageReporterKey struct{}

// ProviderPromptUsageReporter receives provider-level prompt usage reports.
type ProviderPromptUsageReporter func(report ProviderPromptUsageReport)

// WithProviderPromptUsageReporter attaches a reporter callback to the context.
func WithProviderPromptUsageReporter(ctx context.Context, reporter ProviderPromptUsageReporter) context.Context {
	if reporter == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, providerPromptUsageReporterKey{}, reporter)
}

// ReportProviderPromptUsage reports prompt usage to the callback stored in ctx.
func ReportProviderPromptUsage(ctx context.Context, report ProviderPromptUsageReport) {
	if ctx == nil || report.PromptTokens <= 0 {
		return
	}
	reporter, ok := ctx.Value(providerPromptUsageReporterKey{}).(ProviderPromptUsageReporter)
	if !ok || reporter == nil {
		return
	}
	reporter(report)
}
