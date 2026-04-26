package bootstrap

import (
	"context"
	"fmt"

	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
)

// workflowGatewayAdapter adapts the LLM gateway to the workflow engine's GatewayLike interface.
type workflowGatewayAdapter struct {
	gateway      llmcore.Gateway
	defaultModel string
}

func newWorkflowGatewayAdapter(gateway llmcore.Gateway, defaultModel string) core.GatewayLike {
	return &workflowGatewayAdapter{
		gateway:      gateway,
		defaultModel: defaultModel,
	}
}

func (g *workflowGatewayAdapter) Invoke(ctx context.Context, req *core.LLMRequest) (*core.LLMResponse, error) {
	if g.gateway == nil {
		return nil, fmt.Errorf("workflow LLM gateway is not configured")
	}

	model := req.Model
	if model == "" {
		model = g.defaultModel
	}

	completionReq := &llmcore.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{
				Role:    types.RoleUser,
				Content: req.Prompt,
			},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: float32(req.Temperature),
		Metadata:    req.Metadata,
	}

	resp, err := g.gateway.Invoke(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  model,
		Payload:    completionReq,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	chatResp, ok := resp.Output.(*llmcore.ChatResponse)
	if !ok || chatResp == nil {
		return nil, fmt.Errorf("workflow gateway returned invalid chat output type %T", resp.Output)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("llm response has no choices")
	}

	out := &core.LLMResponse{
		Content: chatResp.Choices[0].Message.Content,
		Model:   chatResp.Model,
	}
	out.Usage = &core.LLMUsage{
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
	}
	return out, nil
}

func (g *workflowGatewayAdapter) Stream(ctx context.Context, req *core.LLMRequest) (<-chan core.LLMStreamChunk, error) {
	if g.gateway == nil {
		return nil, fmt.Errorf("workflow LLM gateway is not configured")
	}

	model := req.Model
	if model == "" {
		model = g.defaultModel
	}

	streamReq := &llmcore.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{
				Role:    types.RoleUser,
				Content: req.Prompt,
			},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: float32(req.Temperature),
		Metadata:    req.Metadata,
		StreamOptions: &llmcore.StreamOptions{
			IncludeUsage:      true,
			ChunkIncludeUsage: true,
		},
	}

	source, err := g.gateway.Stream(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  model,
		Payload:    streamReq,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return nil, err
	}

	out := make(chan core.LLMStreamChunk)
	go func() {
		defer close(out)

		var finalUsage *core.LLMUsage
		finalModel := model

		for chunk := range source {
			if chunk.Err != nil {
				out <- core.LLMStreamChunk{Err: chunk.Err}
				continue
			}
			if chunk.Usage != nil {
				usage := &core.LLMUsage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				}
				finalUsage = usage
			}

			streamChunk := core.LLMStreamChunk{
				Model: finalModel,
				Usage: finalUsage,
				Done:  chunk.Done,
			}

			if typed, ok := chunk.Output.(*llmcore.StreamChunk); ok && typed != nil {
				streamChunk.Delta = typed.Delta.Content
				streamChunk.ReasoningContent = typed.Delta.ReasoningContent
				if typed.Model != "" {
					streamChunk.Model = typed.Model
					finalModel = typed.Model
				}
				if typed.Usage != nil {
					usage := &core.LLMUsage{
						PromptTokens:     typed.Usage.PromptTokens,
						CompletionTokens: typed.Usage.CompletionTokens,
						TotalTokens:      typed.Usage.TotalTokens,
					}
					streamChunk.Usage = usage
					finalUsage = usage
				}
				if typed.Err != nil {
					streamChunk.Err = typed.Err
				}
			}

			out <- streamChunk
		}

		if finalUsage != nil {
			out <- core.LLMStreamChunk{
				Model: finalModel,
				Usage: finalUsage,
				Done:  true,
			}
		}
	}()

	return out, nil
}
