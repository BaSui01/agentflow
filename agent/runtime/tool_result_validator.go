package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

type ToolResultValidator interface {
	Validate(result json.RawMessage, schema json.RawMessage) error
}

type JSONSchemaValidator struct{}

func (v *JSONSchemaValidator) Validate(result json.RawMessage, schema json.RawMessage) error {
	if len(schema) == 0 {
		return nil
	}
	if len(result) == 0 {
		return types.NewToolValidationError("tool result is empty but schema requires output")
	}

	var resultVal any
	if err := json.Unmarshal(result, &resultVal); err != nil {
		return types.NewToolValidationError(fmt.Sprintf("tool result is not valid JSON: %v", err))
	}

	var schemaVal any
	if err := json.Unmarshal(schema, &schemaVal); err != nil {
		return types.NewToolValidationError(fmt.Sprintf("tool result schema is not valid JSON: %v", err))
	}

	schemaMap, ok := schemaVal.(map[string]any)
	if !ok {
		return nil
	}

	schemaType, _ := schemaMap["type"].(string)
	switch schemaType {
	case "object":
		if _, isObj := resultVal.(map[string]any); !isObj {
			return types.NewToolValidationError(fmt.Sprintf("expected object result, got %T", resultVal))
		}
	case "array":
		if _, isArr := resultVal.([]any); !isArr {
			return types.NewToolValidationError(fmt.Sprintf("expected array result, got %T", resultVal))
		}
	case "string":
		if _, isStr := resultVal.(string); !isStr {
			return types.NewToolValidationError(fmt.Sprintf("expected string result, got %T", resultVal))
		}
	case "number":
		if _, isNum := resultVal.(float64); !isNum {
			return types.NewToolValidationError(fmt.Sprintf("expected number result, got %T", resultVal))
		}
	case "boolean":
		if _, isBool := resultVal.(bool); !isBool {
			return types.NewToolValidationError(fmt.Sprintf("expected boolean result, got %T", resultVal))
		}
	}

	return nil
}
