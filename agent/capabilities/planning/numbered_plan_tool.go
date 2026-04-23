package planning

import (
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

const SubmitNumberedPlanTool = "submit_numbered_plan"

type NumberedPlanSubmission struct {
	Steps []string `json:"steps"`
}

func NumberedPlanToolSchema() types.ToolSchema {
	strict := true
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        SubmitNumberedPlanTool,
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

func ParseNumberedPlanToolCall(message types.Message) ([]string, error) {
	for _, call := range message.ToolCalls {
		if call.Name != SubmitNumberedPlanTool {
			continue
		}
		var submission NumberedPlanSubmission
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
