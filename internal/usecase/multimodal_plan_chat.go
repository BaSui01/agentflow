package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/structured"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

const (
	defaultMultimodalPlanShotCount     = 6
	maxMultimodalPlanShotCount         = 12
	defaultMultimodalPlanShotDuration  = 3
	defaultMultimodalPlanTimeout       = 90 * time.Second
	defaultMultimodalChatTimeout       = 90 * time.Second
	defaultMultimodalChatModelFallback = "gpt-4o-mini"
)

func (s *DefaultMultimodalService) GeneratePlan(ctx context.Context, req MultimodalPlanRequest) (*MultimodalPlanResult, error) {
	runtime := s.runtime()
	if runtime.Gateway == nil || !runtime.ChatEnabled {
		return nil, types.NewServiceUnavailableError("chat provider is not configured")
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, types.NewInvalidRequestError("prompt is required")
	}
	shotCount := req.ShotCount
	if shotCount <= 0 {
		shotCount = defaultMultimodalPlanShotCount
	}
	if shotCount > maxMultimodalPlanShotCount {
		return nil, types.NewInvalidRequestError("shot_count must be <= 12")
	}

	structuredOutput, err := structured.NewStructuredOutput[MultimodalVisualPlan](runtime.Gateway)
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultMultimodalPlanTimeout)
	defer cancel()

	plan, err := structuredOutput.Generate(timeoutCtx, buildMultimodalPlanPrompt(prompt, shotCount, req.Advanced))
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, types.NewError(types.ErrUpstreamError, "empty plan result").WithHTTPStatus(502)
	}
	normalizeMultimodalVisualPlan(plan)
	return &MultimodalPlanResult{Plan: plan}, nil
}

func (s *DefaultMultimodalService) Chat(ctx context.Context, req MultimodalChatRequest) (*MultimodalChatResult, error) {
	runtime := s.runtime()
	if runtime.Gateway == nil || !runtime.ChatEnabled {
		return nil, types.NewServiceUnavailableError("chat provider is not configured")
	}
	if len(req.Messages) == 0 {
		return nil, types.NewInvalidRequestError("messages is required")
	}

	messages := append([]types.Message(nil), req.Messages...)
	if req.Advanced {
		systemPrompt := strings.TrimSpace(req.SystemPrompt)
		if systemPrompt == "" {
			systemPrompt = "You are a multimodal agent framework assistant. Produce clear, executable and structured outputs."
		}
		messages = append([]types.Message{{Role: types.RoleSystem, Content: systemPrompt}}, messages...)
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = firstNonEmptyNonBlank(runtime.DefaultChatModel, defaultMultimodalChatModelFallback)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, defaultMultimodalChatTimeout)
	defer cancel()

	if !req.AgentMode {
		resp, err := invokeMultimodalChat(timeoutCtx, runtime.Gateway, &llm.ChatRequest{
			Model:       model,
			Messages:    messages,
			Temperature: req.Temperature,
		})
		if err != nil {
			return nil, err
		}
		return &MultimodalChatResult{Mode: "single", Response: resp}, nil
	}

	userText := latestMultimodalUserText(messages)
	planResp, err := invokeMultimodalChat(timeoutCtx, runtime.Gateway, &llm.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{Role: types.RoleSystem, Content: "You are an orchestration planner. Return 3-6 concise action steps."},
			{Role: types.RoleUser, Content: userText},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return nil, err
	}

	planText := firstMultimodalChoice(planResp)
	finalMessages := append([]types.Message{{Role: types.RoleSystem, Content: "You are an executor agent. Execute the provided plan and produce final answer."}}, messages...)
	finalMessages = append(finalMessages, types.Message{Role: types.RoleUser, Content: "Planner output:\n" + planText})

	finalResp, err := invokeMultimodalChat(timeoutCtx, runtime.Gateway, &llm.ChatRequest{
		Model:       model,
		Messages:    finalMessages,
		Temperature: req.Temperature,
	})
	if err != nil {
		return nil, err
	}

	return &MultimodalChatResult{
		Mode:          "agent",
		PlannerOutput: planText,
		FinalResponse: finalResp,
		FinalText:     firstMultimodalChoice(finalResp),
	}, nil
}

func invokeMultimodalChat(ctx context.Context, gateway llmcore.Gateway, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if gateway == nil {
		return nil, types.NewServiceUnavailableError("llm gateway is not configured")
	}
	resp, err := gateway.Invoke(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  req.Model,
		TraceID:    req.TraceID,
		Payload:    req,
	})
	if err != nil {
		return nil, err
	}
	chatResp, ok := resp.Output.(*llm.ChatResponse)
	if !ok || chatResp == nil {
		return nil, types.NewInternalError("invalid chat gateway response")
	}
	return chatResp, nil
}

func buildMultimodalPlanPrompt(userPrompt string, shotCount int, advanced bool) string {
	base := fmt.Sprintf(`Create a visual production plan with %d shots.
User intent: %s

Requirements:
1. Output concise, production-ready shots.
2. Keep character/style continuity across shots.
3. Each shot needs purpose, visual, action, camera and duration_sec.
4. duration_sec should be 1-8.`, shotCount, strings.TrimSpace(userPrompt))
	if !advanced {
		return base
	}
	return base + "\n5. Add cinematic specificity for lighting, composition, lens language, and continuity while staying concise."
}

func normalizeMultimodalVisualPlan(plan *MultimodalVisualPlan) {
	if plan == nil {
		return
	}
	for i := range plan.Shots {
		if plan.Shots[i].ID <= 0 {
			plan.Shots[i].ID = i + 1
		}
		if plan.Shots[i].DurationSec <= 0 {
			plan.Shots[i].DurationSec = defaultMultimodalPlanShotDuration
		}
	}
}

func latestMultimodalUserText(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleUser && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}

func firstMultimodalChoice(resp *llm.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content)
}

func firstNonEmptyNonBlank(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
