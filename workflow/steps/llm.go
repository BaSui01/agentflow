package steps

import (
	"context"
	"fmt"
	"strings"
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
	req := s.buildRequest(input)

	if _, ok := core.WorkflowStreamEmitterFromContext(ctx); ok {
		if output, err := s.executeStreaming(ctx, req, start); err == nil {
			return output, nil
		}
	}

	resp, err := core.InvokeGatewayLike(ctx, s.Gateway, req)
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

func (s *LLMStep) buildRequest(input core.StepInput) *core.LLMRequest {
	prompt := s.Prompt
	if content, ok := input.Data["content"].(string); ok && content != "" {
		if prompt != "" {
			prompt = prompt + "\n\n" + content
		} else {
			prompt = content
		}
	}

	return &core.LLMRequest{
		Model:       s.Model,
		Prompt:      prompt,
		Temperature: s.Temperature,
		MaxTokens:   s.MaxTokens,
		Metadata:    input.Metadata,
	}
}

func (s *LLMStep) executeStreaming(ctx context.Context, req *core.LLMRequest, start time.Time) (core.StepOutput, error) {
	stream, err := core.StreamGatewayLike(ctx, s.Gateway, req)
	if err != nil {
		return core.StepOutput{}, err
	}

	emitter, _ := core.WorkflowStreamEmitterFromContext(ctx)
	var (
		contentBuilder   strings.Builder
		reasoningBuilder strings.Builder
		model            = req.Model
		usage            *core.LLMUsage
	)

	for chunk := range stream {
		if chunk.Err != nil {
			return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeLLM, fmt.Errorf("%w: %w", core.ErrStepExecution, chunk.Err))
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.Delta != "" {
			contentBuilder.WriteString(chunk.Delta)
		}
		if chunk.ReasoningContent != nil && *chunk.ReasoningContent != "" {
			reasoningBuilder.WriteString(*chunk.ReasoningContent)
		}
		if emitter != nil && (chunk.Delta != "" || (chunk.ReasoningContent != nil && *chunk.ReasoningContent != "")) {
			payload := map[string]any{
				"delta": chunk.Delta,
				"model": model,
			}
			if chunk.ReasoningContent != nil && *chunk.ReasoningContent != "" {
				payload["reasoning_content"] = *chunk.ReasoningContent
			}
			emitter(core.WorkflowStreamEvent{
				Type:     core.WorkflowEventToken,
				NodeID:   s.id,
				NodeName: s.id,
				Data:     payload,
			})
		}
	}

	output := core.StepOutput{
		Data: map[string]any{
			"content": contentBuilder.String(),
			"model":   model,
		},
		Latency: time.Since(start),
	}
	if reasoning := reasoningBuilder.String(); reasoning != "" {
		output.Data["reasoning_content"] = reasoning
	}
	if usage != nil {
		output.Usage = &types.TokenUsage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
	}
	return output, nil
}
