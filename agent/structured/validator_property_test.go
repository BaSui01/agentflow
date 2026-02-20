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
// ** 参数:要求3.2**
// 对于任何不符合Schema的JSON输出,验证错误应当包含
// 特定的违反字段路径(JSON路径格式)和违反的原因。

// 验证错误的测试Property SchemaValidation ErrorPath Llocalization
// 包括 JSON 路径格式中的具体字段路径和违反原因。
func TestProperty_SchemaValidation_ErrorPathLocalization(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		// 生成一个包含所需字段的计划
		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		schema := NewObjectSchema().
			AddProperty(fieldName, NewStringSchema()).
			AddRequired(fieldName)

		// 测试缺少所需的字段 - 应该有出错的路径
		emptyObj := `{}`
		err := validator.Validate([]byte(emptyObj), schema)
		require.Error(t, err, "Missing required field should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok, "Error should be ValidationErrors type")
		require.NotEmpty(t, validationErr.Errors, "Should have at least one error")

		// 校验错误包含字段路径
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

// 测试Property SchemaValidation TypeMismatchErrorPath测试类型不匹配错误包括路径.
func TestProperty_SchemaValidation_TypeMismatchErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		schema := NewObjectSchema().
			AddProperty(fieldName, NewIntegerSchema())

		// 提供字符串而不是整数
		data := map[string]any{fieldName: "not_an_integer"}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Type mismatch should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.NotEmpty(t, validationErr.Errors)

		// 校验到字段的错误路径
		assert.Equal(t, fieldName, validationErr.Errors[0].Path, "Error path should be the field name")
		assert.Contains(t, validationErr.Errors[0].Message, "expected integer", "Message should explain type mismatch")
	})
}

// 测试Property SchemaValidation Nested FieldErrorPath测试嵌入了战地出错路径.
func TestProperty_SchemaValidation_NestedFieldErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		parentField := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "parentField")
		childField := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "childField")

		schema := NewObjectSchema().
			AddProperty(parentField, NewObjectSchema().
				AddProperty(childField, NewStringSchema().WithMinLength(5)).
				AddRequired(childField))

		// 为嵌入地提供太短的字符串
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

		// 校验嵌入式路径格式: sparent. child
		expectedPath := parentField + "." + childField
		assert.Equal(t, expectedPath, validationErr.Errors[0].Path, "Error path should be nested: %s", expectedPath)
		assert.Contains(t, validationErr.Errors[0].Message, "minimum", "Message should explain the constraint violation")
	})
}

// TestProperty SchemaValidation Array Project ErrorPath 测试阵列项目出错路径 。
func TestProperty_SchemaValidation_ArrayItemErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		arrayField := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "arrayField")
		invalidIndex := rapid.IntRange(0, 5).Draw(rt, "invalidIndex")

		schema := NewObjectSchema().
			AddProperty(arrayField, NewArraySchema(NewIntegerSchema()))

		// 用一个无效的项目创建数组( 字符串而不是整数)
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

		// 校验路径中的数组索引: 字段[index]
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

// 测试Property SchemaValidation NumericControlErrorPath测试 数值约束出错路径.
func TestProperty_SchemaValidation_NumericConstraintErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		minValue := float64(rapid.IntRange(10, 50).Draw(rt, "minValue"))
		maxValue := float64(rapid.IntRange(51, 100).Draw(rt, "maxValue"))

		schema := NewObjectSchema().
			AddProperty(fieldName, NewNumberSchema().WithMinimum(minValue).WithMaximum(maxValue))

		// 测试值低于最小值
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

// 测试Property SchemaValidation EnumControlErrorPath测试 enum约束出错路径.
func TestProperty_SchemaValidation_EnumConstraintErrorPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		validator := NewValidator()

		fieldName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "fieldName")
		enumValues := []any{"active", "inactive", "pending"}

		schema := NewObjectSchema().
			AddProperty(fieldName, NewEnumSchema(enumValues...))

		// 提供无效的enum值
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

// TestProperty SchemaValidation MultipleErrors HavePaths 测试多个出错都具有路径.
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

		// 提供多起侵权行为的数据
		data := map[string]any{
			field1: "short", // Too short
			field2: 50,      // Below minimum
			// 字段 3 缺失 - 需要
		}
		jsonData, _ := json.Marshal(data)

		err := validator.Validate(jsonData, schema)
		require.Error(t, err, "Multiple violations should cause error")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok)
		require.GreaterOrEqual(t, len(validationErr.Errors), 2, "Should have multiple errors")

		// 校验所有错误有路径和信件
		for _, e := range validationErr.Errors {
			assert.NotEmpty(t, e.Path, "Every error should have a path")
			assert.NotEmpty(t, e.Message, "Every error should have a message")
		}
	})
}

// 测试Property SchemaValidation 深层ErrorPath测试深层嵌入场出错路径.
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

		// 在最深处提供错误类型
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

		// 验证完整嵌入路径
		expectedPath := level1 + "." + level2 + "." + level3
		assert.Equal(t, expectedPath, validationErr.Errors[0].Path, "Error path should show full nesting")
		assert.Contains(t, validationErr.Errors[0].Message, "boolean", "Message should mention expected type")
	})
}
