package providerbase

import (
	"context"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// FineTuningAdapterConfig configures shared provider fine-tuning endpoints.
type FineTuningAdapterConfig struct {
	Endpoint string
}

// FineTuningAdapter delegates OpenAI-compatible fine-tuning operations to the
// embedded provider transport. Providers embed it and override only when their
// fine-tuning protocol differs.
type FineTuningAdapter struct {
	provider interface {
		BaseParams(ctx context.Context) OpenAICompatParams
	}
	endpoint string
}

// NewFineTuningAdapter creates a shared fine-tuning adapter.
func NewFineTuningAdapter(config FineTuningAdapterConfig) *FineTuningAdapter {
	return &FineTuningAdapter{endpoint: config.Endpoint}
}

// BindProvider attaches the provider transport used by fine-tuning methods.
func (a *FineTuningAdapter) BindProvider(provider interface {
	BaseParams(ctx context.Context) OpenAICompatParams
}) {
	if a != nil {
		a.provider = provider
	}
}

// CreateFineTuningJob creates a fine-tuning job.
func (a *FineTuningAdapter) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return CreateFineTuningJobOpenAICompat(ctx, a.params(ctx), req)
}

// ListFineTuningJobs lists fine-tuning jobs.
func (a *FineTuningAdapter) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return ListFineTuningJobsOpenAICompat(ctx, a.params(ctx))
}

// GetFineTuningJob gets a fine-tuning job by ID.
func (a *FineTuningAdapter) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return GetFineTuningJobOpenAICompat(ctx, a.params(ctx), jobID)
}

// CancelFineTuningJob cancels a fine-tuning job by ID.
func (a *FineTuningAdapter) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return CancelFineTuningJobOpenAICompat(ctx, a.params(ctx), jobID)
}

func (a *FineTuningAdapter) params(ctx context.Context) OpenAICompatParams {
	params := a.provider.BaseParams(ctx)
	params.Endpoint = a.endpoint
	return params
}
