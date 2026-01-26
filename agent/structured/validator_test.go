package structured

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	assert.NotNil(t, v)
	assert.NotNil(t, v.formatValidators)
}

func TestValidator_ValidateString(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		schema  *JSONSchema
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid string",
			data:    `"hello"`,
			schema:  NewStringSchema(),
			wantErr: false,
		},
		{
			name:    "invalid type - number instead of string",
			data:    `123`,
			schema:  NewStringSchema(),
			wantErr: true,
			errMsg:  "expected string",
		},
		{
			name:    "valid minLength",
			data:    `"hello"`,
			schema:  NewStringSchema().WithMinLength(3),
			wantErr: false,
		},
		{
			name:    "invalid minLength",
			data:    `"hi"`,
			schema:  NewStringSchema().WithMinLength(3),
			wantErr: true,
			errMsg:  "less than minimum",
		},
		{
			name:    "valid maxLength",
			data:    `"hi"`,
			schema:  NewStringSchema().WithMaxLength(5),
			wantErr: false,
		},
		{
			name:    "invalid maxLength",
			data:    `"hello world"`,
			schema:  NewStringSchema().WithMaxLength(5),
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name:    "valid pattern",
			data:    `"abc123"`,
			schema:  NewStringSchema().WithPattern(`^[a-z]+[0-9]+$`),
			wantErr: false,
		},
		{
			name:    "invalid pattern",
			data:    `"123abc"`,
			schema:  NewStringSchema().WithPattern(`^[a-z]+[0-9]+$`),
			wantErr: true,
			errMsg:  "does not match pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateStringFormat(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		format  StringFormat
		wantErr bool
	}{
		{
			name:    "valid email",
			data:    `"test@example.com"`,
			format:  FormatEmail,
			wantErr: false,
		},
		{
			name:    "invalid email",
			data:    `"not-an-email"`,
			format:  FormatEmail,
			wantErr: true,
		},
		{
			name:    "valid uri",
			data:    `"https://example.com/path"`,
			format:  FormatURI,
			wantErr: false,
		},
		{
			name:    "invalid uri",
			data:    `"not-a-uri"`,
			format:  FormatURI,
			wantErr: true,
		},
		{
			name:    "valid uuid",
			data:    `"550e8400-e29b-41d4-a716-446655440000"`,
			format:  FormatUUID,
			wantErr: false,
		},
		{
			name:    "invalid uuid",
			data:    `"not-a-uuid"`,
			format:  FormatUUID,
			wantErr: true,
		},
		{
			name:    "valid date-time",
			data:    `"2024-01-15T10:30:00Z"`,
			format:  FormatDateTime,
			wantErr: false,
		},
		{
			name:    "valid date",
			data:    `"2024-01-15"`,
			format:  FormatDate,
			wantErr: false,
		},
		{
			name:    "valid ipv4",
			data:    `"192.168.1.1"`,
			format:  FormatIPv4,
			wantErr: false,
		},
		{
			name:    "invalid ipv4 - out of range",
			data:    `"256.168.1.1"`,
			format:  FormatIPv4,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := NewStringSchema().WithFormat(tt.format)
			err := v.Validate([]byte(tt.data), schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateNumber(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		schema  *JSONSchema
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid number",
			data:    `3.14`,
			schema:  NewNumberSchema(),
			wantErr: false,
		},
		{
			name:    "invalid type - string instead of number",
			data:    `"hello"`,
			schema:  NewNumberSchema(),
			wantErr: true,
			errMsg:  "expected number",
		},
		{
			name:    "valid minimum",
			data:    `10`,
			schema:  NewNumberSchema().WithMinimum(5),
			wantErr: false,
		},
		{
			name:    "invalid minimum",
			data:    `3`,
			schema:  NewNumberSchema().WithMinimum(5),
			wantErr: true,
			errMsg:  "less than minimum",
		},
		{
			name:    "valid maximum",
			data:    `10`,
			schema:  NewNumberSchema().WithMaximum(15),
			wantErr: false,
		},
		{
			name:    "invalid maximum",
			data:    `20`,
			schema:  NewNumberSchema().WithMaximum(15),
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name:    "valid exclusiveMinimum",
			data:    `6`,
			schema:  NewNumberSchema().WithExclusiveMinimum(5),
			wantErr: false,
		},
		{
			name:    "invalid exclusiveMinimum - equal",
			data:    `5`,
			schema:  NewNumberSchema().WithExclusiveMinimum(5),
			wantErr: true,
			errMsg:  "must be greater than",
		},
		{
			name:    "valid exclusiveMaximum",
			data:    `4`,
			schema:  NewNumberSchema().WithExclusiveMaximum(5),
			wantErr: false,
		},
		{
			name:    "invalid exclusiveMaximum - equal",
			data:    `5`,
			schema:  NewNumberSchema().WithExclusiveMaximum(5),
			wantErr: true,
			errMsg:  "must be less than",
		},
		{
			name:    "valid multipleOf",
			data:    `10`,
			schema:  NewNumberSchema().WithMultipleOf(5),
			wantErr: false,
		},
		{
			name:    "invalid multipleOf",
			data:    `7`,
			schema:  NewNumberSchema().WithMultipleOf(5),
			wantErr: true,
			errMsg:  "not a multiple of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateInteger(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		schema  *JSONSchema
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid integer",
			data:    `42`,
			schema:  NewIntegerSchema(),
			wantErr: false,
		},
		{
			name:    "invalid - float instead of integer",
			data:    `3.14`,
			schema:  NewIntegerSchema(),
			wantErr: true,
			errMsg:  "expected integer",
		},
		{
			name:    "valid integer with constraints",
			data:    `10`,
			schema:  NewIntegerSchema().WithMinimum(5).WithMaximum(15),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateBoolean(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid true",
			data:    `true`,
			wantErr: false,
		},
		{
			name:    "valid false",
			data:    `false`,
			wantErr: false,
		},
		{
			name:    "invalid - string instead of boolean",
			data:    `"true"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := NewBooleanSchema()
			err := v.Validate([]byte(tt.data), schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateNull(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name:    "valid null",
			data:    `null`,
			wantErr: false,
		},
		{
			name:    "invalid - string instead of null",
			data:    `"null"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := NewSchema(TypeNull)
			err := v.Validate([]byte(tt.data), schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateObject(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		schema  *JSONSchema
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid object",
			data: `{"name": "John", "age": 30}`,
			schema: NewObjectSchema().
				AddProperty("name", NewStringSchema()).
				AddProperty("age", NewIntegerSchema()),
			wantErr: false,
		},
		{
			name:    "invalid type - array instead of object",
			data:    `[1, 2, 3]`,
			schema:  NewObjectSchema(),
			wantErr: true,
			errMsg:  "expected object",
		},
		{
			name: "missing required field",
			data: `{"name": "John"}`,
			schema: NewObjectSchema().
				AddProperty("name", NewStringSchema()).
				AddProperty("age", NewIntegerSchema()).
				AddRequired("name", "age"),
			wantErr: true,
			errMsg:  "required field is missing",
		},
		{
			name: "required field is null",
			data: `{"name": "John", "age": null}`,
			schema: NewObjectSchema().
				AddProperty("name", NewStringSchema()).
				AddProperty("age", NewIntegerSchema()).
				AddRequired("name", "age"),
			wantErr: true,
			errMsg:  "required field must not be null",
		},
		{
			name: "valid minProperties",
			data: `{"a": 1, "b": 2}`,
			schema: NewObjectSchema().
				WithMinProperties(2),
			wantErr: false,
		},
		{
			name: "invalid minProperties",
			data: `{"a": 1}`,
			schema: NewObjectSchema().
				WithMinProperties(2),
			wantErr: true,
			errMsg:  "minimum is 2",
		},
		{
			name: "valid maxProperties",
			data: `{"a": 1, "b": 2}`,
			schema: NewObjectSchema().
				WithMaxProperties(3),
			wantErr: false,
		},
		{
			name: "invalid maxProperties",
			data: `{"a": 1, "b": 2, "c": 3, "d": 4}`,
			schema: NewObjectSchema().
				WithMaxProperties(3),
			wantErr: true,
			errMsg:  "maximum is 3",
		},
		{
			name: "additionalProperties false - valid",
			data: `{"name": "John"}`,
			schema: NewObjectSchema().
				AddProperty("name", NewStringSchema()).
				WithAdditionalProperties(false),
			wantErr: false,
		},
		{
			name: "additionalProperties false - invalid",
			data: `{"name": "John", "extra": "field"}`,
			schema: NewObjectSchema().
				AddProperty("name", NewStringSchema()).
				WithAdditionalProperties(false),
			wantErr: true,
			errMsg:  "additional property not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateArray(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		schema  *JSONSchema
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid array",
			data:    `[1, 2, 3]`,
			schema:  NewArraySchema(NewIntegerSchema()),
			wantErr: false,
		},
		{
			name:    "invalid type - object instead of array",
			data:    `{"a": 1}`,
			schema:  NewArraySchema(NewIntegerSchema()),
			wantErr: true,
			errMsg:  "expected array",
		},
		{
			name:    "invalid item type",
			data:    `[1, "two", 3]`,
			schema:  NewArraySchema(NewIntegerSchema()),
			wantErr: true,
			errMsg:  "expected integer",
		},
		{
			name:    "valid minItems",
			data:    `[1, 2, 3]`,
			schema:  NewArraySchema(NewIntegerSchema()).WithMinItems(2),
			wantErr: false,
		},
		{
			name:    "invalid minItems",
			data:    `[1]`,
			schema:  NewArraySchema(NewIntegerSchema()).WithMinItems(2),
			wantErr: true,
			errMsg:  "minimum is 2",
		},
		{
			name:    "valid maxItems",
			data:    `[1, 2]`,
			schema:  NewArraySchema(NewIntegerSchema()).WithMaxItems(3),
			wantErr: false,
		},
		{
			name:    "invalid maxItems",
			data:    `[1, 2, 3, 4]`,
			schema:  NewArraySchema(NewIntegerSchema()).WithMaxItems(3),
			wantErr: true,
			errMsg:  "maximum is 3",
		},
		{
			name:    "valid uniqueItems",
			data:    `[1, 2, 3]`,
			schema:  NewArraySchema(NewIntegerSchema()).WithUniqueItems(true),
			wantErr: false,
		},
		{
			name:    "invalid uniqueItems - duplicates",
			data:    `[1, 2, 2, 3]`,
			schema:  NewArraySchema(NewIntegerSchema()).WithUniqueItems(true),
			wantErr: true,
			errMsg:  "duplicate item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateEnum(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		schema  *JSONSchema
		wantErr bool
	}{
		{
			name:    "valid enum value",
			data:    `"success"`,
			schema:  NewEnumSchema("success", "failure", "pending"),
			wantErr: false,
		},
		{
			name:    "invalid enum value",
			data:    `"unknown"`,
			schema:  NewEnumSchema("success", "failure", "pending"),
			wantErr: true,
		},
		{
			name:    "valid numeric enum",
			data:    `2`,
			schema:  NewEnumSchema(1, 2, 3),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), tt.schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateConst(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name    string
		data    string
		schema  *JSONSchema
		wantErr bool
	}{
		{
			name:    "valid const string",
			data:    `"fixed"`,
			schema:  NewStringSchema().WithConst("fixed"),
			wantErr: false,
		},
		{
			name:    "invalid const string",
			data:    `"other"`,
			schema:  NewStringSchema().WithConst("fixed"),
			wantErr: true,
		},
		{
			name:    "valid const number",
			data:    `42`,
			schema:  NewNumberSchema().WithConst(42),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), tt.schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateNestedObject(t *testing.T) {
	v := NewValidator()

	schema := NewObjectSchema().
		AddProperty("user", NewObjectSchema().
			AddProperty("name", NewStringSchema().WithMinLength(1)).
			AddProperty("email", NewStringSchema().WithFormat(FormatEmail)).
			AddRequired("name", "email")).
		AddProperty("scores", NewArraySchema(NewIntegerSchema().WithMinimum(0).WithMaximum(100))).
		AddRequired("user")

	tests := []struct {
		name    string
		data    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid nested object",
			data:    `{"user": {"name": "John", "email": "john@example.com"}, "scores": [85, 90, 95]}`,
			wantErr: false,
		},
		{
			name:    "missing nested required field",
			data:    `{"user": {"name": "John"}}`,
			wantErr: true,
			errMsg:  "user.email",
		},
		{
			name:    "invalid nested field type",
			data:    `{"user": {"name": "John", "email": "invalid-email"}}`,
			wantErr: true,
			errMsg:  "does not match format",
		},
		{
			name:    "invalid array item",
			data:    `{"user": {"name": "John", "email": "john@example.com"}, "scores": [85, 150]}`,
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), schema)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_InvalidJSON(t *testing.T) {
	v := NewValidator()
	schema := NewStringSchema()

	err := v.Validate([]byte(`{invalid json`), schema)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestValidator_NilSchema(t *testing.T) {
	v := NewValidator()
	err := v.Validate([]byte(`"anything"`), nil)
	assert.NoError(t, err)
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name   string
		errors []ParseError
		want   string
	}{
		{
			name:   "no errors",
			errors: []ParseError{},
			want:   "validation failed",
		},
		{
			name:   "single error",
			errors: []ParseError{{Path: "name", Message: "required field is missing"}},
			want:   "name: required field is missing",
		},
		{
			name: "multiple errors",
			errors: []ParseError{
				{Path: "name", Message: "required"},
				{Path: "age", Message: "invalid type"},
			},
			want: "validation failed with 2 errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ValidationErrors{Errors: tt.errors}
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParseError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  ParseError
		want string
	}{
		{
			name: "with path",
			err:  ParseError{Path: "user.name", Message: "required"},
			want: "user.name: required",
		},
		{
			name: "without path",
			err:  ParseError{Path: "", Message: "invalid JSON"},
			want: "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestValidator_RegisterFormat(t *testing.T) {
	v := NewValidator()

	// Register custom format
	v.RegisterFormat("custom", func(s string) bool {
		return s == "valid"
	})

	schema := NewStringSchema().WithFormat("custom")

	err := v.Validate([]byte(`"valid"`), schema)
	assert.NoError(t, err)

	err = v.Validate([]byte(`"invalid"`), schema)
	require.Error(t, err)
}

// TestValidator_RequiredFieldValidation validates requirement 3.6
// WHEN Schema 定义了必需字段 THEN Structured_Output 系统 SHALL 验证所有必需字段存在且非空
func TestValidator_RequiredFieldValidation(t *testing.T) {
	v := NewValidator()

	schema := NewObjectSchema().
		AddProperty("name", NewStringSchema()).
		AddProperty("email", NewStringSchema()).
		AddProperty("age", NewIntegerSchema()).
		AddRequired("name", "email")

	tests := []struct {
		name    string
		data    string
		wantErr bool
		errPath string
	}{
		{
			name:    "all required fields present",
			data:    `{"name": "John", "email": "john@example.com", "age": 30}`,
			wantErr: false,
		},
		{
			name:    "optional field missing is ok",
			data:    `{"name": "John", "email": "john@example.com"}`,
			wantErr: false,
		},
		{
			name:    "required field missing",
			data:    `{"name": "John"}`,
			wantErr: true,
			errPath: "email",
		},
		{
			name:    "required field is null",
			data:    `{"name": "John", "email": null}`,
			wantErr: true,
			errPath: "email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), schema)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errPath)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidator_FieldLevelErrors validates requirement 3.2
// WHEN Agent 输出不符合 Schema THEN Structured_Output 系统 SHALL 返回验证错误并包含具体违规字段
func TestValidator_FieldLevelErrors(t *testing.T) {
	v := NewValidator()

	schema := NewObjectSchema().
		AddProperty("name", NewStringSchema().WithMinLength(2)).
		AddProperty("age", NewIntegerSchema().WithMinimum(0).WithMaximum(150)).
		AddProperty("email", NewStringSchema().WithFormat(FormatEmail)).
		AddRequired("name", "age")

	data := `{"name": "J", "age": 200, "email": "invalid"}`

	err := v.Validate([]byte(data), schema)
	require.Error(t, err)

	validationErr, ok := err.(*ValidationErrors)
	require.True(t, ok)

	// Should have errors for name (minLength), age (maximum), and email (format)
	assert.GreaterOrEqual(t, len(validationErr.Errors), 3)

	// Check that each error has a path
	for _, e := range validationErr.Errors {
		assert.NotEmpty(t, e.Path, "error should have a path")
		assert.NotEmpty(t, e.Message, "error should have a message")
	}
}

// TestValidator_ComplexSchemaTypes validates requirement 3.5
// THE Structured_Output 系统 SHALL 支持嵌套对象、数组、枚举等复杂 Schema 类型
func TestValidator_ComplexSchemaTypes(t *testing.T) {
	v := NewValidator()

	// Complex schema with nested objects, arrays, and enums
	schema := NewObjectSchema().
		AddProperty("status", NewEnumSchema("active", "inactive", "pending")).
		AddProperty("user", NewObjectSchema().
			AddProperty("name", NewStringSchema()).
			AddProperty("roles", NewArraySchema(NewStringSchema())).
			AddRequired("name")).
		AddProperty("tags", NewArraySchema(NewStringSchema()).WithMinItems(1).WithUniqueItems(true)).
		AddProperty("metadata", NewObjectSchema().
			WithAdditionalPropertiesSchema(NewStringSchema())).
		AddRequired("status", "user")

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{
			name: "valid complex object",
			data: `{
				"status": "active",
				"user": {"name": "John", "roles": ["admin", "user"]},
				"tags": ["important", "urgent"],
				"metadata": {"key1": "value1", "key2": "value2"}
			}`,
			wantErr: false,
		},
		{
			name: "invalid enum value",
			data: `{
				"status": "unknown",
				"user": {"name": "John"}
			}`,
			wantErr: true,
		},
		{
			name: "invalid nested required",
			data: `{
				"status": "active",
				"user": {"roles": ["admin"]}
			}`,
			wantErr: true,
		},
		{
			name: "duplicate tags",
			data: `{
				"status": "active",
				"user": {"name": "John"},
				"tags": ["tag1", "tag1"]
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate([]byte(tt.data), schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
