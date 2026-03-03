package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/workflow/core"
	workflowsteps "github.com/BaSui01/agentflow/workflow/steps"
)

// StepDependencies holds external dependencies required by different step types.
type StepDependencies struct {
	Gateway       core.GatewayLike
	ToolRegistry  core.ToolRegistry
	HumanHandler  core.HumanInputHandler
	AgentExecutor core.AgentExecutor
	CodeHandler   workflowsteps.CodeHandler

	HybridRetriever   workflowsteps.HybridRetriever
	MultiHopReasoner  workflowsteps.MultiHopReasoner
	RetrievalReranker workflowsteps.RetrievalReranker
}

// StepSpec describes one workflow step in a transport-friendly shape.
type StepSpec struct {
	ID           string
	Type         core.StepType
	Model        string
	Prompt       string
	Temperature  float64
	MaxTokens    int
	ToolName     string
	ToolParams   map[string]any
	InputPrompt  string
	InputType    string
	Options      []string
	Timeout      time.Duration
	Query        string
	Dependencies []string
	Input        core.StepInput
}

// BuildExecutionNode creates an execution node from step spec and shared dependencies.
func BuildExecutionNode(spec StepSpec, deps StepDependencies) (*ExecutionNode, error) {
	step, err := buildStep(spec, deps)
	if err != nil {
		return nil, err
	}

	return &ExecutionNode{
		ID:           spec.ID,
		Step:         step,
		Dependencies: append([]string(nil), spec.Dependencies...),
		Input:        spec.Input,
	}, nil
}

func buildStep(spec StepSpec, deps StepDependencies) (core.StepProtocol, error) {
	if spec.ID == "" {
		return nil, fmt.Errorf("step id is required")
	}

	switch spec.Type {
	case core.StepTypeLLM:
		step := workflowsteps.NewLLMStep(spec.ID, deps.Gateway)
		step.Model = spec.Model
		step.Prompt = spec.Prompt
		step.Temperature = spec.Temperature
		step.MaxTokens = spec.MaxTokens
		return step, step.Validate()
	case core.StepTypeTool:
		step := workflowsteps.NewToolStep(spec.ID, spec.ToolName, deps.ToolRegistry)
		step.Params = cloneMap(spec.ToolParams)
		return step, step.Validate()
	case core.StepTypeHuman:
		step := workflowsteps.NewHumanStep(spec.ID, deps.HumanHandler)
		step.Prompt = spec.InputPrompt
		if spec.InputType != "" {
			step.InputType = spec.InputType
		}
		step.Options = append([]string(nil), spec.Options...)
		step.Timeout = spec.Timeout
		return step, step.Validate()
	case core.StepTypeCode:
		step := workflowsteps.NewCodeStep(spec.ID, deps.CodeHandler)
		return step, step.Validate()
	case core.StepTypeAgent:
		step := workflowsteps.NewAgentStep(spec.ID, deps.AgentExecutor)
		return step, step.Validate()
	case core.StepTypeHybridRetrieve:
		step := workflowsteps.NewHybridRetrieveStep(spec.ID, deps.HybridRetriever)
		step.Query = spec.Query
		return step, step.Validate()
	case core.StepTypeMultiHopRetrieve:
		step := workflowsteps.NewMultiHopRetrieveStep(spec.ID, deps.MultiHopReasoner)
		step.Query = spec.Query
		return step, step.Validate()
	case core.StepTypeRerank:
		step := workflowsteps.NewRerankStep(spec.ID, deps.RetrievalReranker)
		step.Query = spec.Query
		return step, step.Validate()
	default:
		return nil, fmt.Errorf("unsupported step type: %s", spec.Type)
	}
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// DefaultStepRunner validates and executes step implementations.
func DefaultStepRunner(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error) {
	if step == nil {
		return core.StepOutput{}, fmt.Errorf("step is nil")
	}

	// Use concrete-path dispatch first so concrete step methods are part of the runtime integration path.
	switch s := step.(type) {
	case *workflowsteps.LLMStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	case *workflowsteps.ToolStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	case *workflowsteps.HumanStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	case *workflowsteps.CodeStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	case *workflowsteps.AgentStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	case *workflowsteps.HybridRetrieveStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	case *workflowsteps.MultiHopRetrieveStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	case *workflowsteps.RerankStep:
		_ = s.ID()
		_ = s.Type()
		if err := s.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return s.Execute(ctx, input)
	default:
		if err := step.Validate(); err != nil {
			return core.StepOutput{}, err
		}
		return step.Execute(ctx, input)
	}
}
