package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
)

// LLMStep 通过 GatewayLike 抽象调用 LLM。
type LLMStep struct {
	id          string
	Model       string
	Prompt      string
	Temperature float64
	MaxTokens   int
	Gateway     core.GatewayLike
}

// NewLLMStep 创建 LLM 步骤。
func NewLLMStep(id string, gateway core.GatewayLike) *LLMStep {
	return &LLMStep{id: id, Gateway: gateway}
}

func (s *LLMStep) ID() string          { return s.id }
func (s *LLMStep) Type() core.StepType { return core.StepTypeLLM }

func (s *LLMStep) Validate() error {
	if s.Gateway == nil {
		return core.NewStepError(s.id, core.StepTypeLLM, core.ErrStepNotConfigured)
	}
	return nil
}

func (s *LLMStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Gateway == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeLLM, core.ErrStepNotConfigured)
	}

	start := time.Now()

	// 构建 prompt：合并配置 prompt 与输入数据
	prompt := s.Prompt
	if content, ok := input.Data["content"].(string); ok && content != "" {
		if prompt != "" {
			prompt = prompt + "\n\n" + content
		} else {
			prompt = content
		}
	}

	req := &core.LLMRequest{
		Model:       s.Model,
		Prompt:      prompt,
		Temperature: s.Temperature,
		MaxTokens:   s.MaxTokens,
		Metadata:    input.Metadata,
	}

	resp, err := s.Gateway.Invoke(ctx, req)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeLLM, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}

	output := core.StepOutput{
		Data:    map[string]any{"content": resp.Content, "model": resp.Model},
		Latency: time.Since(start),
	}

	if resp.Usage != nil {
		output.Usage = &types.TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return output, nil
}
