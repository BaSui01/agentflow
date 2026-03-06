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

// --- Advanced constraint tests ---

func TestValidateArgs_Enum(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"color": {"type": "string", "enum": ["red", "green", "blue"]}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"color": "red"}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid enum, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"color": "yellow"}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid enum, got %d: %v", len(errs), errs)
	}
	if errs[0].Field != "color" {
		t.Errorf("expected field 'color', got %q", errs[0].Field)
	}
}

func TestValidateArgs_EnumInteger(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"priority": {"type": "integer", "enum": [1, 2, 3]}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"priority": 2}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"priority": 5}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateArgs_Pattern(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"email": {"type": "string", "pattern": "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"email": "test@example.com"}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid email, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"email": "not-an-email"}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid email, got %d: %v", len(errs), errs)
	}
}

func TestValidateArgs_MinMaxLength(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "minLength": 2, "maxLength": 10}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"name": "Bob"}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"name": "A"}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for too short, got %d: %v", len(errs), errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"name": "VeryLongNameExceeds"}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for too long, got %d: %v", len(errs), errs)
	}
}

func TestValidateArgs_MinMaxLengthUnicode(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string", "minLength": 2, "maxLength": 5}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"text": "你好世界"}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for 4 runes, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"text": "你"}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for 1 rune, got %d: %v", len(errs), errs)
	}
}

func TestValidateArgs_MinimumMaximum(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"age": {"type": "integer", "minimum": 0, "maximum": 150},
			"score": {"type": "number", "minimum": 0.0, "maximum": 100.0}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"age": 25, "score": 95.5}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"age": -1, "score": 105}`), schema)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateArgs_MinimumOnly(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"count": {"type": "integer", "minimum": 1}
		}
	}`)

	errs := ValidateArgs(json.RawMessage(`{"count": 0}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"count": 1}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateArgs_CombinedConstraints(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"code": {"type": "string", "minLength": 3, "maxLength": 3, "pattern": "^[A-Z]+$", "enum": ["USD", "EUR", "GBP"]}
		},
		"required": ["code"]
	}`)

	errs := ValidateArgs(json.RawMessage(`{"code": "USD"}`), schema)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"code": "JPY"}`), schema)
	if len(errs) != 1 {
		t.Fatalf("expected 1 enum error, got %d: %v", len(errs), errs)
	}

	errs = ValidateArgs(json.RawMessage(`{"code": "us"}`), schema)
	if len(errs) < 2 {
		t.Fatalf("expected >=2 errors (minLength + pattern or enum), got %d: %v", len(errs), errs)
	}
}
