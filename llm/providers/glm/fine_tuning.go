package glm

import (
	"context"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// CreateFineTuningJob 创建 GLM 微调任务。
func (p *GLMProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return p.fineTuning.CreateFineTuningJob(ctx, req)
}

// ListFineTuningJobs 列出 GLM 微调任务。
func (p *GLMProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return p.fineTuning.ListFineTuningJobs(ctx)
}

// GetFineTuningJob 获取 GLM 微调任务。
func (p *GLMProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return p.fineTuning.GetFineTuningJob(ctx, jobID)
}

// CancelFineTuningJob 取消 GLM 微调任务。
func (p *GLMProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return p.fineTuning.CancelFineTuningJob(ctx, jobID)
}
