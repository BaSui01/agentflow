package workflow

import (
	"context"
	"fmt"
)

// PassthroughStep passes input directly to output.
type PassthroughStep struct{}

func (s *PassthroughStep) Name() string { return "passthrough" }

func (s *PassthroughStep) Execute(ctx context.Context, input any) (any, error) {
	return input, nil
}

// LLMStep executes an LLM call.
type LLMStep struct {
	Model       string
	Prompt      string
	Temperature float64
	MaxTokens   int
}

func (s *LLMStep) Name() string { return "llm" }

func (s *LLMStep) Execute(ctx context.Context, input any) (any, error) {
	// Placeholder - actual implementation would call LLM provider
	return map[string]any{
		"model":  s.Model,
		"prompt": s.Prompt,
		"input":  input,
	}, nil
}

// ToolStep executes a tool call.
type ToolStep struct {
	ToolName string
	Params   map[string]any
}

func (s *ToolStep) Name() string { return s.ToolName }

func (s *ToolStep) Execute(ctx context.Context, input any) (any, error) {
	return map[string]any{
		"tool":   s.ToolName,
		"params": s.Params,
		"input":  input,
	}, nil
}

// HumanInputStep waits for human input.
type HumanInputStep struct {
	Prompt  string
	Type    string
	Options []string
	Timeout int
}

func (s *HumanInputStep) Name() string { return "human_input" }

func (s *HumanInputStep) Execute(ctx context.Context, input any) (any, error) {
	// Placeholder - actual implementation would integrate with HITL manager
	return map[string]any{
		"prompt":  s.Prompt,
		"type":    s.Type,
		"options": s.Options,
		"input":   input,
	}, nil
}

// CodeStep executes custom code.
type CodeStep struct {
	Code     string
	Language string
	Handler  func(ctx context.Context, input any) (any, error)
}

func (s *CodeStep) Name() string { return "code" }

func (s *CodeStep) Execute(ctx context.Context, input any) (any, error) {
	if s.Handler != nil {
		return s.Handler(ctx, input)
	}
	return nil, fmt.Errorf("code execution not implemented")
}
