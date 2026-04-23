package steps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/workflow/core"
)

// SubgraphExecutor executes a subgraph workflow DAG and returns the result.
type SubgraphExecutor interface {
	ExecuteSubgraph(ctx context.Context, input any) (any, error)
}

// SubgraphTool wraps a subgraph DAG as a tool that agents can invoke.
type SubgraphTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	executor    SubgraphExecutor
}

// NewSubgraphTool creates a tool backed by a subgraph executor.
func NewSubgraphTool(name, description string, executor SubgraphExecutor) *SubgraphTool {
	return &SubgraphTool{
		Name:        name,
		Description: description,
		executor:    executor,
	}
}

// WithInputSchema sets a custom JSON Schema for the tool's input parameters.
func (t *SubgraphTool) WithInputSchema(schema map[string]any) *SubgraphTool {
	t.InputSchema = schema
	return t
}

// Execute runs the subgraph with the provided JSON arguments and returns the result as JSON.
func (t *SubgraphTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t.executor == nil {
		return nil, core.NewStepError(t.Name, core.StepTypeTool, fmt.Errorf("no executor configured"))
	}

	var input any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, core.NewStepError(t.Name, core.StepTypeTool, fmt.Errorf("unmarshal args: %w", err))
		}
	}

	result, err := t.executor.ExecuteSubgraph(ctx, input)
	if err != nil {
		return nil, core.NewStepError(t.Name, core.StepTypeTool, fmt.Errorf("execution failed: %w", err))
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, core.NewStepError(t.Name, core.StepTypeTool, fmt.Errorf("marshal result: %w", err))
	}
	return out, nil
}

// ToolSchema returns the tool schema for agent registration.
func (t *SubgraphTool) ToolSchema() map[string]any {
	schema := t.InputSchema
	if schema == nil {
		schema = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return map[string]any{
		"name":        t.Name,
		"description": t.Description,
		"inputSchema": schema,
	}
}
