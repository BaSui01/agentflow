package jsonschema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
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

// propertyDef represents a JSON Schema property definition.
type propertyDef struct {
	Type      string            `json:"type"`
	Enum      []json.RawMessage `json:"enum,omitempty"`
	Pattern   string            `json:"pattern,omitempty"`
	MinLength *int              `json:"minLength,omitempty"`
	MaxLength *int              `json:"maxLength,omitempty"`
	Minimum   *float64          `json:"minimum,omitempty"`
	Maximum   *float64          `json:"maximum,omitempty"`
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
		var prop propertyDef
		if err := json.Unmarshal(propSchema, &prop); err != nil {
			continue
		}
		if typeErr := checkType(name, val, prop.Type); typeErr != nil {
			errs = append(errs, *typeErr)
			continue
		}
		errs = append(errs, checkConstraints(name, val, &prop)...)
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

// checkConstraints validates enum, pattern, length, and range constraints.
func checkConstraints(field string, val json.RawMessage, prop *propertyDef) []ValidationError {
	var errs []ValidationError

	if len(prop.Enum) > 0 {
		if err := checkEnum(field, val, prop.Enum); err != nil {
			errs = append(errs, *err)
		}
	}

	if prop.Type == "string" && string(val)[0] == '"' {
		var s string
		if json.Unmarshal(val, &s) == nil {
			if prop.Pattern != "" {
				if err := checkPattern(field, s, prop.Pattern); err != nil {
					errs = append(errs, *err)
				}
			}
			if prop.MinLength != nil && len([]rune(s)) < *prop.MinLength {
				errs = append(errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("string length %d is less than minLength %d", len([]rune(s)), *prop.MinLength),
				})
			}
			if prop.MaxLength != nil && len([]rune(s)) > *prop.MaxLength {
				errs = append(errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("string length %d exceeds maxLength %d", len([]rune(s)), *prop.MaxLength),
				})
			}
		}
	}

	if (prop.Type == "number" || prop.Type == "integer") && (prop.Minimum != nil || prop.Maximum != nil) {
		if n, err := strconv.ParseFloat(strings.TrimSpace(string(val)), 64); err == nil {
			if prop.Minimum != nil && n < *prop.Minimum {
				errs = append(errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("value %g is less than minimum %g", n, *prop.Minimum),
				})
			}
			if prop.Maximum != nil && n > *prop.Maximum {
				errs = append(errs, ValidationError{
					Field:   field,
					Message: fmt.Sprintf("value %g exceeds maximum %g", n, *prop.Maximum),
				})
			}
		}
	}

	return errs
}

func checkEnum(field string, val json.RawMessage, allowed []json.RawMessage) *ValidationError {
	valStr := strings.TrimSpace(string(val))
	for _, a := range allowed {
		if strings.TrimSpace(string(a)) == valStr {
			return nil
		}
	}
	allowedStrs := make([]string, len(allowed))
	for i, a := range allowed {
		allowedStrs[i] = string(a)
	}
	return &ValidationError{
		Field:   field,
		Message: fmt.Sprintf("value %s not in enum [%s]", valStr, strings.Join(allowedStrs, ", ")),
	}
}

func checkPattern(field, value, pattern string) *ValidationError {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	if !re.MatchString(value) {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("value %q does not match pattern %q", value, pattern),
		}
	}
	return nil
}
