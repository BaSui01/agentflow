package core

import (
	"context"
	"errors"
	"fmt"
)

// ErrNotConfigured is returned when a step's required dependency (Gateway, Registry, Handler)
// has not been injected. Callers can check for this with errors.Is(err, ErrNotConfigured).
var ErrNotConfigured = errors.New("step dependency not configured")

// Tool represents an executable tool within a workflow.
type Tool interface {
	Name() string
	Execute(ctx context.Context, params map[string]any) (any, error)
}

// ============================================================
// Step implementations
// ============================================================

// PassthroughStep passes input directly to output.
type PassthroughStep struct{}

func (s *PassthroughStep) Name() string { return "passthrough" }

func (s *PassthroughStep) Execute(ctx context.Context, input any) (any, error) {
	return input, nil
}

// ============================================================
// LLMStep — executes an LLM completion call
// ============================================================

// LLMStep executes an LLM call.
// When Gateway is set, it performs a real LLM invoke request.
type LLMStep struct {
	Model       string
	Prompt      string
	Temperature float64
	MaxTokens   int
	Gateway     GatewayLike // Optional: inject to enable real LLM calls
}

func (s *LLMStep) Name() string { return "llm" }

func (s *LLMStep) Execute(ctx context.Context, input any) (any, error) {
	if s.Gateway == nil {
		return nil, NewStepError("llm", StepTypeLLM, ErrNotConfigured)
	}

	// Build the user message from prompt + input
	userContent := s.Prompt
	if input != nil {
		if str, ok := input.(string); ok && str != "" {
			if userContent != "" {
				userContent = userContent + "\n\n" + str
			} else {
				userContent = str
			}
		}
	}

	req := &LLMRequest{
		Model:       s.Model,
		Prompt:      userContent,
		Temperature: s.Temperature,
		MaxTokens:   s.MaxTokens,
	}

	resp, err := s.Gateway.Invoke(ctx, req)
	if err != nil {
		return nil, NewStepError("llm", StepTypeLLM, fmt.Errorf("%w: %w", ErrStepExecution, err))
	}

	if resp == nil || resp.Content == "" {
		return nil, NewStepError("llm", StepTypeLLM, fmt.Errorf("%w: empty response", ErrStepExecution))
	}

	return resp.Content, nil
}

// ============================================================
// ToolStep — executes a tool call
// ============================================================

// ToolStep executes a tool call.
// When Registry is set, it performs a real tool execution.
type ToolStep struct {
	ToolName string
	Params   map[string]any
	Registry ToolRegistry // Optional: inject to enable real tool execution
}

func (s *ToolStep) Name() string { return s.ToolName }

func (s *ToolStep) Execute(ctx context.Context, input any) (any, error) {
	if s.Registry == nil {
		return nil, NewStepError(s.ToolName, StepTypeTool, ErrNotConfigured)
	}

	// Merge static params with dynamic input if input is a map
	params := make(map[string]any, len(s.Params)+1)
	for k, v := range s.Params {
		params[k] = v
	}
	if inputMap, ok := input.(map[string]any); ok {
		for k, v := range inputMap {
			if _, exists := params[k]; !exists {
				params[k] = v
			}
		}
	} else if input != nil {
		params["input"] = input
	}

	result, err := s.Registry.ExecuteTool(ctx, s.ToolName, params)
	if err != nil {
		return nil, NewStepError(s.ToolName, StepTypeTool, fmt.Errorf("%w: %w", ErrStepExecution, err))
	}

	return result, nil
}

// ============================================================
// HumanInputStep — waits for human input
// ============================================================

// HumanInputStep waits for human input.
// When Handler is set, it sends a request to the HITL handler and waits for a response.
type HumanInputStep struct {
	Prompt  string
	Type    string
	Options []string
	Timeout int
	Handler HumanInputHandler // Optional: inject to enable real HITL
}

func (s *HumanInputStep) Name() string { return "human_input" }

func (s *HumanInputStep) Execute(ctx context.Context, input any) (any, error) {
	if s.Handler == nil {
		return nil, NewStepError("human_input", StepTypeHumanInput, ErrNotConfigured)
	}

	result, err := s.Handler.RequestInput(ctx, s.Prompt, s.Type, s.Options)
	if err != nil {
		return nil, NewStepError("human_input", StepTypeHumanInput, fmt.Errorf("%w: %w", ErrStepExecution, err))
	}

	return result, nil
}

// ============================================================
// CodeStep — executes a Go handler function
// ============================================================

// CodeStep executes custom code via an injected Go handler function.
type CodeStep struct {
	Handler func(ctx context.Context, input any) (any, error)
}

func (s *CodeStep) Name() string { return "code" }

func (s *CodeStep) Execute(ctx context.Context, input any) (any, error) {
	if s.Handler != nil {
		return s.Handler(ctx, input)
	}
	return nil, NewStepError("code", StepTypeCode, ErrNotConfigured)
}
