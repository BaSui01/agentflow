package jsonschema

import (
	"encoding/json"
	"fmt"
)

// ValidationError describes a schema validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidateArgs validates JSON arguments against a JSON Schema definition.
// schema is a json.RawMessage containing a JSON Schema object with "type", "properties", "required".
// args is the actual arguments to validate.
// Returns nil if valid, or a list of ValidationErrors.
func ValidateArgs(args json.RawMessage, schema json.RawMessage) []ValidationError {
	if len(schema) == 0 {
		return nil
	}

	var schemaDef struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(schema, &schemaDef); err != nil {
		return nil
	}

	if schemaDef.Type != "object" && schemaDef.Type != "" {
		return nil
	}

	var argsMap map[string]json.RawMessage
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return []ValidationError{{Message: fmt.Sprintf("arguments must be a JSON object: %v", err)}}
	}

	var errs []ValidationError

	for _, req := range schemaDef.Required {
		if _, ok := argsMap[req]; !ok {
			errs = append(errs, ValidationError{Field: req, Message: "required field missing"})
		}
	}

	for name, propSchema := range schemaDef.Properties {
		val, exists := argsMap[name]
		if !exists {
			continue
		}
		var propDef struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(propSchema, &propDef); err != nil {
			continue
		}
		if typeErr := checkType(name, val, propDef.Type); typeErr != nil {
			errs = append(errs, *typeErr)
		}
	}

	return errs
}

func checkType(field string, val json.RawMessage, expectedType string) *ValidationError {
	if expectedType == "" {
		return nil
	}
	trimmed := string(val)
	if len(trimmed) == 0 {
		return nil
	}

	switch expectedType {
	case "string":
		if trimmed[0] != '"' {
			return &ValidationError{Field: field, Message: fmt.Sprintf("expected string, got %c...", trimmed[0])}
		}
	case "number", "integer":
		if trimmed[0] != '-' && (trimmed[0] < '0' || trimmed[0] > '9') {
			return &ValidationError{Field: field, Message: fmt.Sprintf("expected %s, got %c...", expectedType, trimmed[0])}
		}
	case "boolean":
		if trimmed != "true" && trimmed != "false" {
			return &ValidationError{Field: field, Message: "expected boolean"}
		}
	case "array":
		if trimmed[0] != '[' {
			return &ValidationError{Field: field, Message: "expected array"}
		}
	case "object":
		if trimmed[0] != '{' {
			return &ValidationError{Field: field, Message: "expected object"}
		}
	}
	return nil
}
