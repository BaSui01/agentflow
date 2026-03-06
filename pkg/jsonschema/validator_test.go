package jsonschema

import (
	"encoding/json"
	"testing"
)

func TestValidateArgs_ValidArgs(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age":  {"type": "integer"}
		},
		"required": ["name"]
	}`)
	args := json.RawMessage(`{"name": "Alice", "age": 30}`)

	errs := ValidateArgs(args, schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateArgs_MissingRequired(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age":  {"type": "integer"}
		},
		"required": ["name", "age"]
	}`)
	args := json.RawMessage(`{"name": "Alice"}`)

	errs := ValidateArgs(args, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "age" {
		t.Errorf("expected field 'age', got %q", errs[0].Field)
	}
}

func TestValidateArgs_WrongType(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age":  {"type": "integer"}
		},
		"required": ["name"]
	}`)
	args := json.RawMessage(`{"name": 123, "age": "not a number"}`)

	errs := ValidateArgs(args, schema)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateArgs_EmptySchema(t *testing.T) {
	args := json.RawMessage(`{"anything": "goes"}`)

	errs := ValidateArgs(args, nil)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for nil schema, got %v", errs)
	}

	errs = ValidateArgs(args, json.RawMessage(``))
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty schema, got %v", errs)
	}
}

func TestValidateArgs_EmptyArgsWithRequired(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`)
	args := json.RawMessage(`{}`)

	errs := ValidateArgs(args, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "name" || errs[0].Message != "required field missing" {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

func TestValidateArgs_NonObjectArgs(t *testing.T) {
	schema := json.RawMessage(`{"type": "object", "properties": {}}`)
	args := json.RawMessage(`"just a string"`)

	errs := ValidateArgs(args, schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "" {
		t.Errorf("expected empty field, got %q", errs[0].Field)
	}
}

func TestValidateArgs_BooleanType(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"flag": {"type": "boolean"}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"flag": true}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid boolean, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"flag": "yes"}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid boolean, got %d: %v", len(errs), errs)
	}
}

func TestValidateArgs_ArrayAndObjectTypes(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"items": {"type": "array"},
			"config": {"type": "object"}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"items": [1,2], "config": {"k":"v"}}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"items": "not array", "config": 42}`), schema)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidationError_Error(t *testing.T) {
	ve := ValidationError{Field: "name", Message: "required field missing"}
	if ve.Error() != "name: required field missing" {
		t.Errorf("unexpected: %s", ve.Error())
	}

	ve2 := ValidationError{Message: "bad input"}
	if ve2.Error() != "bad input" {
		t.Errorf("unexpected: %s", ve2.Error())
	}
}
