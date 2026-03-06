package steps

import (
	"context"
	"encoding/json"
	"fmt"
)

// SubgraphExecutor executes a subgraph workflow DAG and returns the result.
type SubgraphExecutor interface {
	ExecuteSubgraph(ctx context.Context, input any) (any, error)
}

// SubgraphTool wraps a subgraph DAG as a tool that agents can invoke.
type SubgraphTool struct {
	Name        string
	Description string
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

// Execute runs the subgraph with the provided JSON arguments and returns the result as JSON.
func (t *SubgraphTool) Execute(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t.executor == nil {
		return nil, fmt.Errorf("subgraph tool %q: no executor configured", t.Name)
	}

	var input any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("subgraph tool %q: unmarshal args: %w", t.Name, err)
		}
	}

	result, err := t.executor.ExecuteSubgraph(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("subgraph tool %q: execution failed: %w", t.Name, err)
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("subgraph tool %q: marshal result: %w", t.Name, err)
	}
	return out, nil
}

// ToolSchema returns the tool schema for agent registration.
func (t *SubgraphTool) ToolSchema() map[string]any {
	return map[string]any{
		"name":        t.Name,
		"description": t.Description,
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

