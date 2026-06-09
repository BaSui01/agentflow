package mistral

import (
	"context"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// CreateFineTuningJob 使用 Mistral 创建微调任务。
func (p *MistralProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return p.fineTuning.CreateFineTuningJob(ctx, req)
}

// ListFineTuningJobs 列出 Mistral 微调任务。
func (p *MistralProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return p.fineTuning.ListFineTuningJobs(ctx)
}

// GetFineTuningJob 获取 Mistral 微调任务。
func (p *MistralProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return p.fineTuning.GetFineTuningJob(ctx, jobID)
}

// CancelFineTuningJob 取消 Mistral 微调任务。
func (p *MistralProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return p.fineTuning.CancelFineTuningJob(ctx, jobID)
}
