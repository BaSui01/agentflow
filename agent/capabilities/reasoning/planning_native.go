package reasoning

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"
)

const (
	submitToolPlanTool      = "submit_tool_plan"
	submitExecutionPlanTool = "submit_execution_plan"
	submitNextStepsTool     = "submit_next_steps"
)

type toolPlanSubmission struct {
	Steps []struct {
		ID        string `json:"id"`
		Tool      string `json:"tool"`
		Arguments string `json:"arguments"`
		Reasoning string `json:"reasoning,omitempty"`
	} `json:"steps"`
}

type executionPlanSubmission struct {
	Goal  string `json:"goal"`
	Steps []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Tool        string `json:"tool,omitempty"`
		Arguments   string `json:"arguments,omitempty"`
	} `json:"steps"`
}

type nextStepsSubmission struct {
	Steps []struct {
		Action       string  `json:"action"`
		Description  string  `json:"description"`
		Confidence   float64 `json:"confidence"`
		Alternatives []struct {
			Action      string  `json:"action"`
			Description string  `json:"description"`
			Confidence  float64 `json:"confidence"`
		} `json:"alternatives,omitempty"`
	} `json:"steps"`
}

func toolPlanToolSchema() types.ToolSchema {
	strict := true
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        submitToolPlanTool,
		Description: "Submit a tool-first plan with ordered tool calls and dependencies.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"steps": {
					"type": "array",
					"minItems": 1,
					"items": {
						"type": "object",
						"properties": {
							"id": {"type": "string"},
							"tool": {"type": "string"},
							"arguments": {"type": "string"},
							"reasoning": {"type": "string"}
						},
						"required": ["id", "tool", "arguments"],
						"additionalProperties": false
					}
				}
			},
			"required": ["steps"],
			"additionalProperties": false
		}`),
		Strict: &strict,
	}
}

func executionPlanToolSchema() types.ToolSchema {
	strict := true
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        submitExecutionPlanTool,
		Description: "Submit an execution plan with a goal and ordered executable steps.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"goal": {"type": "string"},
				"steps": {
					"type": "array",
					"minItems": 1,
					"items": {
						"type": "object",
						"properties": {
							"id": {"type": "string"},
							"description": {"type": "string"},
							"tool": {"type": "string"},
							"arguments": {"type": "string"}
						},
						"required": ["id", "description"],
						"additionalProperties": false
					}
				}
			},
			"required": ["goal", "steps"],
			"additionalProperties": false
		}`),
		Strict: &strict,
	}
}

func nextStepsToolSchema() types.ToolSchema {
	strict := true
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        submitNextStepsTool,
		Description: "Submit the next 1-3 steps and optional alternatives for dynamic planning.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"steps": {
					"type": "array",
					"minItems": 1,
					"maxItems": 3,
					"items": {
						"type": "object",
						"properties": {
							"action": {"type": "string"},
							"description": {"type": "string"},
							"confidence": {"type": "number"},
							"alternatives": {
								"type": "array",
								"items": {
									"type": "object",
									"properties": {
										"action": {"type": "string"},
										"description": {"type": "string"},
										"confidence": {"type": "number"}
									},
									"required": ["action", "description", "confidence"],
									"additionalProperties": false
								}
							}
						},
						"required": ["action", "description", "confidence"],
						"additionalProperties": false
					}
				}
			},
			"required": ["steps"],
			"additionalProperties": false
		}`),
		Strict: &strict,
	}
}

func parseToolPlanToolCall(message types.Message) ([]PlanStep, error) {
	call, err := findToolCall(message.ToolCalls, submitToolPlanTool)
	if err != nil {
		return nil, err
	}
	var submission toolPlanSubmission
	if err := json.Unmarshal(call.Arguments, &submission); err != nil {
		return nil, fmt.Errorf("decode tool plan tool call: %w", err)
	}
	if len(submission.Steps) == 0 {
		return nil, fmt.Errorf("tool plan tool call did not include steps")
	}
	plan := make([]PlanStep, 0, len(submission.Steps))
	for _, step := range submission.Steps {
		plan = append(plan, PlanStep{
			ID:        step.ID,
			Tool:      step.Tool,
			Arguments: step.Arguments,
			Reasoning: step.Reasoning,
		})
	}
	return plan, nil
}

func parseExecutionPlanToolCall(message types.Message) (ExecutionPlan, error) {
	call, err := findToolCall(message.ToolCalls, submitExecutionPlanTool)
	if err != nil {
		return ExecutionPlan{}, err
	}
	var submission executionPlanSubmission
	if err := json.Unmarshal(call.Arguments, &submission); err != nil {
		return ExecutionPlan{}, fmt.Errorf("decode execution plan tool call: %w", err)
	}
	if len(submission.Steps) == 0 {
		return ExecutionPlan{}, fmt.Errorf("execution plan tool call did not include steps")
	}
	plan := ExecutionPlan{
		Goal:   submission.Goal,
		Status: planStatusExecuting,
	}
	for _, step := range submission.Steps {
		plan.Steps = append(plan.Steps, ExecutionStep{
			ID:          step.ID,
			Description: step.Description,
			Tool:        step.Tool,
			Arguments:   step.Arguments,
		})
	}
	return plan, nil
}

func (d *DynamicPlanner) parseNextStepsToolCall(message types.Message) ([]*PlanNode, error) {
	call, err := findToolCall(message.ToolCalls, submitNextStepsTool)
	if err != nil {
		return nil, err
	}
	var submission nextStepsSubmission
	if err := json.Unmarshal(call.Arguments, &submission); err != nil {
		return nil, fmt.Errorf("decode next steps tool call: %w", err)
	}
	if len(submission.Steps) == 0 {
		return nil, fmt.Errorf("next steps tool call did not include steps")
	}
	nodes := make([]*PlanNode, 0, len(submission.Steps))
	for _, step := range submission.Steps {
		node := &PlanNode{
			ID:          d.nextNodeID(),
			Action:      step.Action,
			Description: step.Description,
			Status:      NodeStatusPending,
			Confidence:  step.Confidence,
			CreatedAt:   time.Now(),
		}
		for _, alt := range step.Alternatives {
			node.Alternatives = append(node.Alternatives, &PlanNode{
				ID:          d.nextNodeID(),
				Action:      alt.Action,
				Description: alt.Description,
				Status:      NodeStatusPending,
				Confidence:  alt.Confidence,
				CreatedAt:   time.Now(),
			})
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func findToolCall(calls []types.ToolCall, name string) (types.ToolCall, error) {
	for _, call := range calls {
		if call.Name == name {
			return call, nil
		}
	}
	return types.ToolCall{}, fmt.Errorf("tool call %q not found", name)
}
