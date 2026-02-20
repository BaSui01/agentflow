package workflow

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/llm"
)

// ============================================================
// Workflow-local interfaces (avoid circular dependency on agent/)
// ============================================================

// ToolRegistry abstracts tool lookup and execution for workflow steps.
// Implement this interface to bridge workflow with your tool management layer.
type ToolRegistry interface {
	// GetTool returns a Tool by name. Returns nil, false if not found.
	GetTool(name string) (Tool, bool)
	// ExecuteTool looks up and executes a tool in one call.
	ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error)
}

// Tool represents an executable tool within a workflow.
type Tool interface {
	Name() string
	Execute(ctx context.Context, params map[string]any) (any, error)
}

// HumanInputHandler abstracts human-in-the-loop interaction for workflow steps.
// Implement this interface to bridge workflow with your HITL management layer.
type HumanInputHandler interface {
	// RequestInput sends a prompt to a human and waits for a response.
	// inputType hints at the expected response format (e.g. "text", "choice").
	// options provides selectable choices when inputType is "choice".
	RequestInput(ctx context.Context, prompt string, inputType string, options []string) (any, error)
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
// When Provider is set, it performs a real LLM completion request.
// When Provider is nil, it returns a placeholder map (backward compatible).
type LLMStep struct {
	Model       string
	Prompt      string
	Temperature float64
	MaxTokens   int
	Provider    llm.Provider // Optional: inject to enable real LLM calls
}

func (s *LLMStep) Name() string { return "llm" }

func (s *LLMStep) Execute(ctx context.Context, input any) (any, error) {
	if s.Provider == nil {
		// Backward-compatible placeholder
		return map[string]any{
			"model":  s.Model,
			"prompt": s.Prompt,
			"input":  input,
		}, nil
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

	req := &llm.ChatRequest{
		Model: s.Model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: userContent},
		},
		Temperature: float32(s.Temperature),
		MaxTokens:   s.MaxTokens,
	}

	resp, err := s.Provider.Completion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLMStep: completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLMStep: empty response from provider")
	}

	return resp.Choices[0].Message.Content, nil
}

// ============================================================
// ToolStep — executes a tool call
// ============================================================

// ToolStep executes a tool call.
// When Registry is set, it performs a real tool execution.
// When Registry is nil, it returns a placeholder map (backward compatible).
type ToolStep struct {
	ToolName string
	Params   map[string]any
	Registry ToolRegistry // Optional: inject to enable real tool execution
}

func (s *ToolStep) Name() string { return s.ToolName }

func (s *ToolStep) Execute(ctx context.Context, input any) (any, error) {
	if s.Registry == nil {
		// Backward-compatible placeholder
		return map[string]any{
			"tool":   s.ToolName,
			"params": s.Params,
			"input":  input,
		}, nil
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
		return nil, fmt.Errorf("ToolStep %q: execution failed: %w", s.ToolName, err)
	}

	return result, nil
}

// ============================================================
// HumanInputStep — waits for human input
// ============================================================

// HumanInputStep waits for human input.
// When Handler is set, it sends a request to the HITL handler and waits for a response.
// When Handler is nil, it returns a placeholder map (backward compatible).
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
		// Backward-compatible placeholder
		return map[string]any{
			"prompt":  s.Prompt,
			"type":    s.Type,
			"options": s.Options,
			"input":   input,
		}, nil
	}

	result, err := s.Handler.RequestInput(ctx, s.Prompt, s.Type, s.Options)
	if err != nil {
		return nil, fmt.Errorf("HumanInputStep: request failed: %w", err)
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
	return nil, fmt.Errorf("CodeStep: handler not configured")
}
