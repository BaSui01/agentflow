package structured

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 8: Schema 验证错误定位
// **Validates: Requirements 3.2**
// For any JSON output that doesn't conform to Schema, validation errors should contain
// the specific violating field path (JSON Path format) and the reason for violation.

// TestProperty_SchemaValidation_ErrorPathLocalization tests that validation errors
// include specific field paths in JSON Path format and violation reasons.
func TestProperty_SchemaValidation_ErrorPathLocalization(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		// Generate a schema with required fields
		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		schema := NewObjectSchema().
			AddProperty(fieldName, NewStringSchema()).
			AddRequired(fieldName)

		// Test missing required field - should have path in error
		emptyObj := `{}`
		err := validator.Validate([]byte(emptyObj), schema)
		require.Error(t, err, "Missing required field should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok, "Error should be ValidationErrors type")
		require.NotEmpty(t, validationErr.Errors, "Should have at least one error")

		// Verify error contains field path
		foundFieldPath := false
		for _, e := range validationErr.Errors {
			if strings.Contains(e.Path, fieldName) {
				foundFieldPath = true
				assert.NotEmpty(t, e.Message, "Error should have a message explaining the violation")
			}
		}
		assert.True(t, foundFieldPath, "Error should contain the violating field path: %s", fieldName)
	})
}

// TestProperty_SchemaValidation_TypeMismatchErrorPath tests type mismatch errors include path.
func TestProperty_SchemaValidation_TypeMismatchErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		schema := NewObjectSchema().
			AddProperty(fieldName, NewIntegerSchema())

		// Provide string instead of integer
		data := map[string]any{fieldName: "not_an_integer"}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Type mismatch should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.NotEmpty(t, validationErr.Errors)

		// Verify error path points to the field
		assert.Equal(t, fieldName, validationErr.Errors[0].Path, "Error path should be the field name")
		assert.Contains(t, validationErr.Errors[0].Message, "expected integer", "Message should explain type mismatch")
	})
}

// TestProperty_SchemaValidation_NestedFieldErrorPath tests nested field error paths.
func TestProperty_SchemaValidation_NestedFieldErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		parentField := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "parentField")
		childField := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "childField")

		schema := NewObjectSchema().
			AddProperty(parentField, NewObjectSchema().
				AddProperty(childField, NewStringSchema().WithMinLength(5)).
				AddRequired(childField))

		// Provide too short string for nested field
		data := map[string]any{
			parentField: map[string]any{
				childField: "ab", // Too short
			},
		}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Constraint violation should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.NotEmpty(t, validationErr.Errors)

		// Verify nested path format: parent.child
		expectedPath := parentField + "." + childField
		assert.Equal(t, expectedPath, validationErr.Errors[0].Path, "Error path should be nested: %s", expectedPath)
		assert.Contains(t, validationErr.Errors[0].Message, "minimum", "Message should explain the constraint violation")
	})
}

// TestProperty_SchemaValidation_ArrayItemErrorPath tests array item error paths.
func TestProperty_SchemaValidation_ArrayItemErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		arrayField := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "arrayField")
		invalidIndex := rapid.IntRange(0, 5).Draw(rt, "invalidIndex")

		schema := NewObjectSchema().
			AddProperty(arrayField, NewArraySchema(NewIntegerSchema()))

		// Create array with one invalid item (string instead of integer)
		items := make([]any, invalidIndex+1)
		for i := 0; i < invalidIndex; i++ {
			items[i] = i * 10
		}
		items[invalidIndex] = "not_an_integer"

		data := map[string]any{arrayField: items}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Invalid array item should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.NotEmpty(t, validationErr.Errors)

		// Verify array index in path: field[index]
		expectedPathPrefix := arrayField + "["
		foundArrayPath := false
		for _, e := range validationErr.Errors {
			if strings.HasPrefix(e.Path, expectedPathPrefix) {
				foundArrayPath = true
				assert.NotEmpty(t, e.Message, "Error should have violation reason")
			}
		}
		assert.True(t, foundArrayPath, "Error path should contain array index notation")
	})
}

// TestProperty_SchemaValidation_NumericConstraintErrorPath tests numeric constraint error paths.
func TestProperty_SchemaValidation_NumericConstraintErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		minValue := float64(rapid.IntRange(10, 50).Draw(rt, "minValue"))
		maxValue := float64(rapid.IntRange(51, 100).Draw(rt, "maxValue"))

		schema := NewObjectSchema().
			AddProperty(fieldName, NewNumberSchema().WithMinimum(minValue).WithMaximum(maxValue))

		// Test value below minimum
		belowMin := minValue - float64(rapid.IntRange(1, 10).Draw(rt, "belowMin"))
		data := map[string]any{fieldName: belowMin}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Value below minimum should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.NotEmpty(t, validationErr.Errors)

		assert.Equal(t, fieldName, validationErr.Errors[0].Path, "Error path should be the field name")
		assert.Contains(t, validationErr.Errors[0].Message, "minimum", "Message should mention minimum constraint")
	})
}

// TestProperty_SchemaValidation_EnumConstraintErrorPath tests enum constraint error paths.
func TestProperty_SchemaValidation_EnumConstraintErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		enumValues := []any{"active", "inactive", "pending"}

		schema := NewObjectSchema().
			AddProperty(fieldName, NewEnumSchema(enumValues...))

		// Provide invalid enum value
		invalidValue := rapid.StringMatching(`[a-z]{10,15}`).Draw(rt, "invalidValue")
		data := map[string]any{fieldName: invalidValue}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Invalid enum value should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.NotEmpty(t, validationErr.Errors)

		assert.Equal(t, fieldName, validationErr.Errors[0].Path, "Error path should be the field name")
		assert.Contains(t, validationErr.Errors[0].Message, "one of", "Message should list valid enum values")
	})
}

// TestProperty_SchemaValidation_MultipleErrorsHavePaths tests multiple errors all have paths.
func TestProperty_SchemaValidation_MultipleErrorsHavePaths(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		field1 := rapid.StringMatching(`[a-z]{3,6}`).Draw(rt, "field1")
		field2 := rapid.StringMatching(`[a-z]{3,6}`).Draw(rt, "field2")
		field3 := rapid.StringMatching(`[a-z]{3,6}`).Draw(rt, "field3")

		schema := NewObjectSchema().
			AddProperty(field1, NewStringSchema().WithMinLength(10)).
			AddProperty(field2, NewIntegerSchema().WithMinimum(100)).
			AddProperty(field3, NewStringSchema()).
			AddRequired(field1, field2, field3)

		// Provide data with multiple violations
		data := map[string]any{
			field1: "short", // Too short
			field2: 50,      // Below minimum
			// field3 missing - required
		}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Multiple violations should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.GreaterOrEqual(t, len(validationErr.Errors), 2, "Should have multiple errors")

		// Verify all errors have paths and messages
		for _, e := range validationErr.Errors {
			assert.NotEmpty(t, e.Path, "Every error should have a path")
			assert.NotEmpty(t, e.Message, "Every error should have a message")
		}
	})
}

// TestProperty_SchemaValidation_DeeplyNestedErrorPath tests deeply nested field error paths.
func TestProperty_SchemaValidation_DeeplyNestedErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		level1 := rapid.StringMatching(`[a-z]{3,6}`).Draw(rt, "level1")
		level2 := rapid.StringMatching(`[a-z]{3,6}`).Draw(rt, "level2")
		level3 := rapid.StringMatching(`[a-z]{3,6}`).Draw(rt, "level3")

		schema := NewObjectSchema().
			AddProperty(level1, NewObjectSchema().
				AddProperty(level2, NewObjectSchema().
					AddProperty(level3, NewBooleanSchema()).
					AddRequired(level3)).
				AddRequired(level2)).
			AddRequired(level1)

		// Provide wrong type at deepest level
		data := map[string]any{
			level1: map[string]any{
				level2: map[string]any{
					level3: "not_a_boolean",
				},
			},
		}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Type mismatch at deep level should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.NotEmpty(t, validationErr.Errors)

		// Verify full nested path
		expectedPath := level1 + "." + level2 + "." + level3
		assert.Equal(t, expectedPath, validationErr.Errors[0].Path, "Error path should show full nesting")
		assert.Contains(t, validationErr.Errors[0].Message, "boolean", "Message should mention expected type")
	})
}
