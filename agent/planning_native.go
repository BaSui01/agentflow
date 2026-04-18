package agent

import (
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

const submitNumberedPlanTool = "submit_numbered_plan"

type numberedPlanSubmission struct {
	Steps []string `json:"steps"`
}

func numberedPlanToolSchema() types.ToolSchema {
	strict := true
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        submitNumberedPlanTool,
		Description: "Submit the ordered execution steps for the task.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"steps": {
					"type": "array",
					"items": {"type": "string"},
					"minItems": 1,
					"description": "Ordered execution steps."
				}
			},
			"required": ["steps"],
			"additionalProperties": false
		}`),
		Strict: &strict,
	}
}

func parseNumberedPlanToolCall(message types.Message) ([]string, error) {
	for _, call := range message.ToolCalls {
		if call.Name != submitNumberedPlanTool {
			continue
		}
		var submission numberedPlanSubmission
		if err := json.Unmarshal(call.Arguments, &submission); err != nil {
			return nil, fmt.Errorf("decode numbered plan tool call: %w", err)
		}
		if len(submission.Steps) == 0 {
			return nil, fmt.Errorf("numbered plan tool call did not include steps")
		}
		return submission.Steps, nil
	}
	return nil, fmt.Errorf("numbered plan tool call not found")
}
