package structured

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSchema(t *testing.T) {
	tests := []struct {
		name     string
		schemaFn func() *JSONSchema
		wantType SchemaType
	}{
		{"string", NewStringSchema, TypeString},
		{"number", NewNumberSchema, TypeNumber},
		{"integer", NewIntegerSchema, TypeInteger},
		{"boolean", NewBooleanSchema, TypeBoolean},
		{"object", NewObjectSchema, TypeObject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.schemaFn()
			assert.Equal(t, tt.wantType, schema.Type)
		})
	}
}

func TestNewArraySchema(t *testing.T) {
	items := NewStringSchema()
	schema := NewArraySchema(items)

	assert.Equal(t, TypeArray, schema.Type)
	assert.Equal(t, items, schema.Items)
}

func TestNewEnumSchema(t *testing.T) {
	schema := NewEnumSchema("a", "b", "c")

	assert.Equal(t, []any{"a", "b", "c"}, schema.Enum)
}

func TestObjectSchemaBuilder(t *testing.T) {
	schema := NewObjectSchema().
		WithTitle("Person").
		WithDescription("A person object").
		AddProperty("name", NewStringSchema().WithMinLength(1)).
		AddProperty("age", NewIntegerSchema().WithMinimum(0)).
		AddProperty("email", NewStringSchema().WithFormat(FormatEmail)).
		AddRequired("name", "age")

	assert.Equal(t, "Person", schema.Title)
	assert.Equal(t, "A person object", schema.Description)
	assert.Len(t, schema.Properties, 3)
	assert.Equal(t, []string{"name", "age"}, schema.Required)

	// 检查名称属性
	nameProp := schema.GetProperty("name")
	require.NotNil(t, nameProp)
	assert.Equal(t, TypeString, nameProp.Type)
	assert.Equal(t, 1, *nameProp.MinLength)

	// 检查年龄属性
	ageProp := schema.GetProperty("age")
	require.NotNil(t, ageProp)
	assert.Equal(t, TypeInteger, ageProp.Type)
	assert.Equal(t, 0.0, *ageProp.Minimum)

	// 检查电子邮件属性
	emailProp := schema.GetProperty("email")
	require.NotNil(t, emailProp)
	assert.Equal(t, FormatEmail, emailProp.Format)
}

func TestStringSchemaConstraints(t *testing.T) {
	schema := NewStringSchema().
		WithMinLength(5).
		WithMaxLength(100).
		WithPattern("^[a-z]+$").
		WithFormat(FormatEmail)

	assert.Equal(t, 5, *schema.MinLength)
	assert.Equal(t, 100, *schema.MaxLength)
	assert.Equal(t, "^[a-z]+$", schema.Pattern)
	assert.Equal(t, FormatEmail, schema.Format)
}

func TestNumericSchemaConstraints(t *testing.T) {
	schema := NewNumberSchema().
		WithMinimum(0).
		WithMaximum(100).
		WithExclusiveMinimum(-1).
		WithExclusiveMaximum(101).
		WithMultipleOf(0.5)

	assert.Equal(t, 0.0, *schema.Minimum)
	assert.Equal(t, 100.0, *schema.Maximum)
	assert.Equal(t, -1.0, *schema.ExclusiveMinimum)
	assert.Equal(t, 101.0, *schema.ExclusiveMaximum)
	assert.Equal(t, 0.5, *schema.MultipleOf)
}

func TestArraySchemaConstraints(t *testing.T) {
	schema := NewArraySchema(NewStringSchema()).
		WithMinItems(1).
		WithMaxItems(10).
		WithUniqueItems(true)

	assert.Equal(t, 1, *schema.MinItems)
	assert.Equal(t, 10, *schema.MaxItems)
	assert.True(t, *schema.UniqueItems)
}

func TestObjectSchemaConstraints(t *testing.T) {
	schema := NewObjectSchema().
		WithMinProperties(1).
		WithMaxProperties(10).
		WithAdditionalProperties(false)

	assert.Equal(t, 1, *schema.MinProperties)
	assert.Equal(t, 10, *schema.MaxProperties)
	assert.False(t, schema.AdditionalProperties.Allowed)
}

func TestSchemaWithDefault(t *testing.T) {
	schema := NewStringSchema().
		WithDefault("default_value").
		WithExamples("example1", "example2")

	assert.Equal(t, "default_value", schema.Default)
	assert.Equal(t, []any{"example1", "example2"}, schema.Examples)
}

func TestSchemaWithEnum(t *testing.T) {
	schema := NewStringSchema().WithEnum("pending", "active", "completed")

	assert.Equal(t, []any{"pending", "active", "completed"}, schema.Enum)
}

func TestSchemaWithConst(t *testing.T) {
	schema := NewStringSchema().WithConst("fixed_value")

	assert.Equal(t, "fixed_value", schema.Const)
}

func TestNestedObjectSchema(t *testing.T) {
	addressSchema := NewObjectSchema().
		AddProperty("street", NewStringSchema()).
		AddProperty("city", NewStringSchema()).
		AddProperty("zipCode", NewStringSchema().WithPattern("^\\d{5}$")).
		AddRequired("street", "city")

	personSchema := NewObjectSchema().
		AddProperty("name", NewStringSchema()).
		AddProperty("address", addressSchema).
		AddRequired("name")

	// 验证嵌入式结构
	assert.True(t, personSchema.HasProperty("address"))
	addrProp := personSchema.GetProperty("address")
	require.NotNil(t, addrProp)
	assert.Equal(t, TypeObject, addrProp.Type)
	assert.True(t, addrProp.HasProperty("street"))
	assert.True(t, addrProp.HasProperty("city"))
	assert.True(t, addrProp.HasProperty("zipCode"))
}

func TestArrayOfObjectsSchema(t *testing.T) {
	itemSchema := NewObjectSchema().
		AddProperty("id", NewIntegerSchema()).
		AddProperty("name", NewStringSchema()).
		AddRequired("id", "name")

	schema := NewArraySchema(itemSchema).
		WithMinItems(1).
		WithMaxItems(100)

	assert.Equal(t, TypeArray, schema.Type)
	assert.Equal(t, TypeObject, schema.Items.Type)
	assert.Len(t, schema.Items.Properties, 2)
}

func TestSchemaJSONSerialization(t *testing.T) {
	schema := NewObjectSchema().
		WithTitle("Task").
		WithDescription("A task object").
		AddProperty("status", NewStringSchema().WithEnum("pending", "done")).
		AddProperty("priority", NewIntegerSchema().WithMinimum(1).WithMaximum(5)).
		AddRequired("status")

	// 序列化为 JSON
	data, err := schema.ToJSON()
	require.NoError(t, err)

	// 切换回
	parsed, err := FromJSON(data)
	require.NoError(t, err)

	assert.Equal(t, schema.Title, parsed.Title)
	assert.Equal(t, schema.Description, parsed.Description)
	assert.Equal(t, schema.Type, parsed.Type)
	assert.Equal(t, schema.Required, parsed.Required)
}

func TestSchemaJSONIndent(t *testing.T) {
	schema := NewObjectSchema().
		WithTitle("Test").
		AddProperty("name", NewStringSchema())

	data, err := schema.ToJSONIndent()
	require.NoError(t, err)

	// 应包含缩进
	assert.Contains(t, string(data), "\n")
	assert.Contains(t, string(data), "  ")
}

func TestSchemaClone(t *testing.T) {
	original := NewObjectSchema().
		WithTitle("Original").
		WithDescription("Original description").
		AddProperty("name", NewStringSchema().WithMinLength(1)).
		AddProperty("tags", NewArraySchema(NewStringSchema()).WithMinItems(1)).
		AddRequired("name").
		WithAdditionalProperties(false)

	clone := original.Clone()

	// 校验复制为等效
	assert.Equal(t, original.Title, clone.Title)
	assert.Equal(t, original.Description, clone.Description)
	assert.Equal(t, original.Required, clone.Required)

	// 修改复制并验证原件不变
	clone.Title = "Modified"
	clone.Properties["name"].MinLength = nil
	clone.Required = append(clone.Required, "extra")

	assert.Equal(t, "Original", original.Title)
	assert.NotNil(t, original.Properties["name"].MinLength)
	assert.Len(t, original.Required, 1)
}

func TestSchemaCloneNil(t *testing.T) {
	var schema *JSONSchema
	clone := schema.Clone()
	assert.Nil(t, clone)
}

func TestIsRequired(t *testing.T) {
	schema := NewObjectSchema().
		AddProperty("name", NewStringSchema()).
		AddProperty("age", NewIntegerSchema()).
		AddRequired("name")

	assert.True(t, schema.IsRequired("name"))
	assert.False(t, schema.IsRequired("age"))
	assert.False(t, schema.IsRequired("nonexistent"))
}

func TestHasProperty(t *testing.T) {
	schema := NewObjectSchema().
		AddProperty("name", NewStringSchema())

	assert.True(t, schema.HasProperty("name"))
	assert.False(t, schema.HasProperty("age"))
}

func TestGetPropertyNil(t *testing.T) {
	schema := NewSchema(TypeObject)
	assert.Nil(t, schema.GetProperty("name"))
}

func TestAdditionalPropertiesMarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		ap       *AdditionalProperties
		expected string
	}{
		{
			name:     "nil",
			ap:       nil,
			expected: "null",
		},
		{
			name:     "false",
			ap:       &AdditionalProperties{Allowed: false},
			expected: "false",
		},
		{
			name:     "true",
			ap:       &AdditionalProperties{Allowed: true},
			expected: "true",
		},
		{
			name:     "schema",
			ap:       &AdditionalProperties{Allowed: true, Schema: NewStringSchema()},
			expected: `{"type":"string"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.ap)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestAdditionalPropertiesUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantAllowed bool
		wantSchema  bool
	}{
		{
			name:        "false",
			input:       "false",
			wantAllowed: false,
			wantSchema:  false,
		},
		{
			name:        "true",
			input:       "true",
			wantAllowed: true,
			wantSchema:  false,
		},
		{
			name:        "schema",
			input:       `{"type":"string"}`,
			wantAllowed: true,
			wantSchema:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ap AdditionalProperties
			err := json.Unmarshal([]byte(tt.input), &ap)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAllowed, ap.Allowed)
			if tt.wantSchema {
				assert.NotNil(t, ap.Schema)
			} else {
				assert.Nil(t, ap.Schema)
			}
		})
	}
}

func TestComplexSchemaRoundTrip(t *testing.T) {
	// 创建包含嵌入对象、阵列和各种约束的复杂方案
	schema := &JSONSchema{
		Schema:      "https://json-schema.org/draft/2020-12/schema",
		Title:       "TaskResult",
		Description: "Result of a task execution",
		Type:        TypeObject,
		Properties: map[string]*JSONSchema{
			"status": {
				Type:        TypeString,
				Enum:        []any{"success", "failure", "pending"},
				Description: "Task status",
			},
			"message": {
				Type:      TypeString,
				MinLength: intPtr(1),
				MaxLength: intPtr(1000),
			},
			"score": {
				Type:    TypeNumber,
				Minimum: floatPtr(0),
				Maximum: floatPtr(100),
			},
			"tags": {
				Type:        TypeArray,
				Items:       NewStringSchema(),
				MinItems:    intPtr(0),
				MaxItems:    intPtr(10),
				UniqueItems: boolPtr(true),
			},
			"metadata": {
				Type: TypeObject,
				AdditionalProperties: &AdditionalProperties{
					Allowed: true,
					Schema:  NewStringSchema(),
				},
			},
			"completedAt": {
				Type:   TypeString,
				Format: FormatDateTime,
			},
		},
		Required: []string{"status", "message"},
	}

	// 序列化
	data, err := schema.ToJSONIndent()
	require.NoError(t, err)

	// 淡化
	parsed, err := FromJSON(data)
	require.NoError(t, err)

	// 校验
	assert.Equal(t, schema.Schema, parsed.Schema)
	assert.Equal(t, schema.Title, parsed.Title)
	assert.Equal(t, schema.Type, parsed.Type)
	assert.Equal(t, schema.Required, parsed.Required)
	assert.Len(t, parsed.Properties, 6)

	// 校验嵌入属性
	statusProp := parsed.GetProperty("status")
	require.NotNil(t, statusProp)
	assert.Equal(t, []any{"success", "failure", "pending"}, statusProp.Enum)

	tagsProp := parsed.GetProperty("tags")
	require.NotNil(t, tagsProp)
	assert.Equal(t, TypeArray, tagsProp.Type)
	assert.True(t, *tagsProp.UniqueItems)
}

func TestSchemaWithComposition(t *testing.T) {
	// 测试全部
	schema := &JSONSchema{
		AllOf: []*JSONSchema{
			NewObjectSchema().AddProperty("name", NewStringSchema()),
			NewObjectSchema().AddProperty("age", NewIntegerSchema()),
		},
	}

	data, err := schema.ToJSON()
	require.NoError(t, err)

	parsed, err := FromJSON(data)
	require.NoError(t, err)

	assert.Len(t, parsed.AllOf, 2)
}

func TestSchemaWithConditional(t *testing.T) {
	schema := &JSONSchema{
		Type: TypeObject,
		If: &JSONSchema{
			Properties: map[string]*JSONSchema{
				"type": {Const: "premium"},
			},
		},
		Then: &JSONSchema{
			Properties: map[string]*JSONSchema{
				"discount": NewNumberSchema().WithMinimum(10),
			},
		},
		Else: &JSONSchema{
			Properties: map[string]*JSONSchema{
				"discount": NewNumberSchema().WithMaximum(5),
			},
		},
	}

	data, err := schema.ToJSON()
	require.NoError(t, err)

	parsed, err := FromJSON(data)
	require.NoError(t, err)

	assert.NotNil(t, parsed.If)
	assert.NotNil(t, parsed.Then)
	assert.NotNil(t, parsed.Else)
}

func TestSchemaWithDefs(t *testing.T) {
	schema := &JSONSchema{
		Type: TypeObject,
		Defs: map[string]*JSONSchema{
			"address": NewObjectSchema().
				AddProperty("street", NewStringSchema()).
				AddProperty("city", NewStringSchema()),
		},
		Properties: map[string]*JSONSchema{
			"homeAddress": {Ref: "#/$defs/address"},
			"workAddress": {Ref: "#/$defs/address"},
		},
	}

	data, err := schema.ToJSON()
	require.NoError(t, err)

	parsed, err := FromJSON(data)
	require.NoError(t, err)

	assert.NotNil(t, parsed.Defs)
	assert.NotNil(t, parsed.Defs["address"])
}

// 辅助功能
func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}

func boolPtr(b bool) *bool {
	return &b
}
