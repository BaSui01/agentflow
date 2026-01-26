package structured

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test structs for schema generation

type SimpleStruct struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Active  bool   `json:"active"`
	Score   float64
	private string // unexported, should be skipped
}

type StructWithTags struct {
	Status  string   `json:"status" jsonschema:"required,enum=success,failure,pending"`
	Message string   `json:"message" jsonschema:"required,description=The result message"`
	Score   float64  `json:"score" jsonschema:"minimum=0,maximum=100"`
	Tags    []string `json:"tags" jsonschema:"minItems=1,maxItems=10"`
	Email   string   `json:"email" jsonschema:"format=email,pattern=^[a-z]+@[a-z]+\\.[a-z]+$"`
	Count   int      `json:"count" jsonschema:"minimum=0,maximum=1000,default=0"`
}

type NestedStruct struct {
	ID     string        `json:"id" jsonschema:"required"`
	Inner  SimpleStruct  `json:"inner"`
	InnerP *SimpleStruct `json:"inner_ptr,omitempty"`
}

type ArrayStruct struct {
	Items   []string       `json:"items"`
	Numbers []int          `json:"numbers"`
	Nested  []SimpleStruct `json:"nested"`
}

type MapStruct struct {
	StringMap map[string]string       `json:"string_map"`
	IntMap    map[string]int          `json:"int_map"`
	NestedMap map[string]SimpleStruct `json:"nested_map"`
}

type SkipFieldStruct struct {
	Visible string `json:"visible"`
	Hidden  string `json:"-"`
}

type StringLengthStruct struct {
	Short string `json:"short" jsonschema:"minLength=1,maxLength=10"`
	Long  string `json:"long" jsonschema:"minLength=100,maxLength=1000"`
}

func TestSchemaGenerator_BasicTypes(t *testing.T) {
	g := NewSchemaGenerator()

	tests := []struct {
		name     string
		input    any
		expected SchemaType
	}{
		{"string", "", TypeString},
		{"int", 0, TypeInteger},
		{"int64", int64(0), TypeInteger},
		{"float64", 0.0, TypeNumber},
		{"bool", false, TypeBoolean},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := g.GenerateSchema(reflect.TypeOf(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, schema.Type)
		})
	}
}

func TestSchemaGenerator_SimpleStruct(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(SimpleStruct{}))

	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)
	assert.Len(t, schema.Properties, 4) // 4 exported fields

	// Check field types
	assert.Equal(t, TypeString, schema.Properties["name"].Type)
	assert.Equal(t, TypeInteger, schema.Properties["age"].Type)
	assert.Equal(t, TypeBoolean, schema.Properties["active"].Type)
	assert.Equal(t, TypeNumber, schema.Properties["Score"].Type) // No json tag, uses field name

	// Unexported field should not be present
	assert.Nil(t, schema.Properties["private"])
}

func TestSchemaGenerator_StructWithTags(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(StructWithTags{}))

	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)

	// Check required fields
	assert.Contains(t, schema.Required, "status")
	assert.Contains(t, schema.Required, "message")
	assert.NotContains(t, schema.Required, "score")

	// Check enum
	statusSchema := schema.Properties["status"]
	assert.Equal(t, []any{"success", "failure", "pending"}, statusSchema.Enum)

	// Check description
	messageSchema := schema.Properties["message"]
	assert.Equal(t, "The result message", messageSchema.Description)

	// Check numeric constraints
	scoreSchema := schema.Properties["score"]
	require.NotNil(t, scoreSchema.Minimum)
	require.NotNil(t, scoreSchema.Maximum)
	assert.Equal(t, 0.0, *scoreSchema.Minimum)
	assert.Equal(t, 100.0, *scoreSchema.Maximum)

	// Check array constraints
	tagsSchema := schema.Properties["tags"]
	require.NotNil(t, tagsSchema.MinItems)
	require.NotNil(t, tagsSchema.MaxItems)
	assert.Equal(t, 1, *tagsSchema.MinItems)
	assert.Equal(t, 10, *tagsSchema.MaxItems)

	// Check format and pattern
	emailSchema := schema.Properties["email"]
	assert.Equal(t, StringFormat("email"), emailSchema.Format)
	assert.Equal(t, "^[a-z]+@[a-z]+\\.[a-z]+$", emailSchema.Pattern)

	// Check default value
	countSchema := schema.Properties["count"]
	assert.Equal(t, int64(0), countSchema.Default)
}

func TestSchemaGenerator_NestedStruct(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(NestedStruct{}))

	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)

	// Check nested struct
	innerSchema := schema.Properties["inner"]
	assert.Equal(t, TypeObject, innerSchema.Type)
	assert.Len(t, innerSchema.Properties, 4)

	// Check pointer to struct (should be same as non-pointer)
	innerPtrSchema := schema.Properties["inner_ptr"]
	assert.Equal(t, TypeObject, innerPtrSchema.Type)
	assert.Len(t, innerPtrSchema.Properties, 4)

	// Check required
	assert.Contains(t, schema.Required, "id")
}

func TestSchemaGenerator_ArrayTypes(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(ArrayStruct{}))

	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)

	// Check string array
	itemsSchema := schema.Properties["items"]
	assert.Equal(t, TypeArray, itemsSchema.Type)
	assert.Equal(t, TypeString, itemsSchema.Items.Type)

	// Check int array
	numbersSchema := schema.Properties["numbers"]
	assert.Equal(t, TypeArray, numbersSchema.Type)
	assert.Equal(t, TypeInteger, numbersSchema.Items.Type)

	// Check nested struct array
	nestedSchema := schema.Properties["nested"]
	assert.Equal(t, TypeArray, nestedSchema.Type)
	assert.Equal(t, TypeObject, nestedSchema.Items.Type)
	assert.Len(t, nestedSchema.Items.Properties, 4)
}

func TestSchemaGenerator_MapTypes(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(MapStruct{}))

	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)

	// Check string map
	stringMapSchema := schema.Properties["string_map"]
	assert.Equal(t, TypeObject, stringMapSchema.Type)
	require.NotNil(t, stringMapSchema.AdditionalProperties)
	assert.Equal(t, TypeString, stringMapSchema.AdditionalProperties.Schema.Type)

	// Check int map
	intMapSchema := schema.Properties["int_map"]
	assert.Equal(t, TypeObject, intMapSchema.Type)
	require.NotNil(t, intMapSchema.AdditionalProperties)
	assert.Equal(t, TypeInteger, intMapSchema.AdditionalProperties.Schema.Type)

	// Check nested map
	nestedMapSchema := schema.Properties["nested_map"]
	assert.Equal(t, TypeObject, nestedMapSchema.Type)
	require.NotNil(t, nestedMapSchema.AdditionalProperties)
	assert.Equal(t, TypeObject, nestedMapSchema.AdditionalProperties.Schema.Type)
}

func TestSchemaGenerator_SkipFields(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(SkipFieldStruct{}))

	require.NoError(t, err)
	assert.Len(t, schema.Properties, 1)
	assert.NotNil(t, schema.Properties["visible"])
	assert.Nil(t, schema.Properties["Hidden"])
}

func TestSchemaGenerator_StringLengthConstraints(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(StringLengthStruct{}))

	require.NoError(t, err)

	shortSchema := schema.Properties["short"]
	require.NotNil(t, shortSchema.MinLength)
	require.NotNil(t, shortSchema.MaxLength)
	assert.Equal(t, 1, *shortSchema.MinLength)
	assert.Equal(t, 10, *shortSchema.MaxLength)

	longSchema := schema.Properties["long"]
	require.NotNil(t, longSchema.MinLength)
	require.NotNil(t, longSchema.MaxLength)
	assert.Equal(t, 100, *longSchema.MinLength)
	assert.Equal(t, 1000, *longSchema.MaxLength)
}

func TestSchemaGenerator_Pointer(t *testing.T) {
	g := NewSchemaGenerator()

	// Pointer to basic type
	var strPtr *string
	schema, err := g.GenerateSchema(reflect.TypeOf(strPtr))
	require.NoError(t, err)
	assert.Equal(t, TypeString, schema.Type)

	// Pointer to struct
	var structPtr *SimpleStruct
	schema, err = g.GenerateSchema(reflect.TypeOf(structPtr))
	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)
	assert.Len(t, schema.Properties, 4)
}

func TestSchemaGenerator_Slice(t *testing.T) {
	g := NewSchemaGenerator()

	// Slice of strings
	var strSlice []string
	schema, err := g.GenerateSchema(reflect.TypeOf(strSlice))
	require.NoError(t, err)
	assert.Equal(t, TypeArray, schema.Type)
	assert.Equal(t, TypeString, schema.Items.Type)

	// Slice of ints
	var intSlice []int
	schema, err = g.GenerateSchema(reflect.TypeOf(intSlice))
	require.NoError(t, err)
	assert.Equal(t, TypeArray, schema.Type)
	assert.Equal(t, TypeInteger, schema.Items.Type)
}

func TestSchemaGenerator_GenerateSchemaFromValue(t *testing.T) {
	g := NewSchemaGenerator()

	// From struct value
	schema, err := g.GenerateSchemaFromValue(SimpleStruct{})
	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)
	assert.Len(t, schema.Properties, 4)

	// From pointer value
	schema, err = g.GenerateSchemaFromValue(&SimpleStruct{})
	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)

	// From nil value
	_, err = g.GenerateSchemaFromValue(nil)
	assert.Error(t, err)
}

func TestSchemaGenerator_NilType(t *testing.T) {
	g := NewSchemaGenerator()
	_, err := g.GenerateSchema(nil)
	assert.Error(t, err)
}

func TestSchemaGenerator_Interface(t *testing.T) {
	g := NewSchemaGenerator()

	type WithInterface struct {
		Data any `json:"data"`
	}

	schema, err := g.GenerateSchema(reflect.TypeOf(WithInterface{}))
	require.NoError(t, err)

	// Interface{} should produce an empty schema (any type)
	dataSchema := schema.Properties["data"]
	assert.NotNil(t, dataSchema)
	assert.Empty(t, dataSchema.Type)
}

func TestParseTagOptions(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected map[string]string
	}{
		{
			name:     "empty",
			tag:      "",
			expected: map[string]string{},
		},
		{
			name:     "required only",
			tag:      "required",
			expected: map[string]string{"required": ""},
		},
		{
			name:     "key=value",
			tag:      "minimum=0",
			expected: map[string]string{"minimum": "0"},
		},
		{
			name:     "multiple options",
			tag:      "required,minimum=0,maximum=100",
			expected: map[string]string{"required": "", "minimum": "0", "maximum": "100"},
		},
		{
			name:     "enum with commas",
			tag:      "enum=a,b,c,required",
			expected: map[string]string{"enum": "a,b,c", "required": ""},
		},
		{
			name:     "description with spaces",
			tag:      "description=This is a description",
			expected: map[string]string{"description": "This is a description"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTagOptions(tt.tag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test recursive struct handling
type RecursiveStruct struct {
	Name     string            `json:"name"`
	Children []RecursiveStruct `json:"children"`
}

func TestSchemaGenerator_RecursiveStruct(t *testing.T) {
	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(RecursiveStruct{}))

	require.NoError(t, err)
	assert.Equal(t, TypeObject, schema.Type)
	assert.NotNil(t, schema.Properties["name"])
	assert.NotNil(t, schema.Properties["children"])

	// Children should be an array
	childrenSchema := schema.Properties["children"]
	assert.Equal(t, TypeArray, childrenSchema.Type)
}

// Test all supported formats
func TestSchemaGenerator_AllFormats(t *testing.T) {
	type AllFormats struct {
		DateTime string `json:"date_time" jsonschema:"format=date-time"`
		Date     string `json:"date" jsonschema:"format=date"`
		Time     string `json:"time" jsonschema:"format=time"`
		Email    string `json:"email" jsonschema:"format=email"`
		URI      string `json:"uri" jsonschema:"format=uri"`
		UUID     string `json:"uuid" jsonschema:"format=uuid"`
		Hostname string `json:"hostname" jsonschema:"format=hostname"`
		IPv4     string `json:"ipv4" jsonschema:"format=ipv4"`
		IPv6     string `json:"ipv6" jsonschema:"format=ipv6"`
	}

	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(AllFormats{}))

	require.NoError(t, err)

	assert.Equal(t, StringFormat("date-time"), schema.Properties["date_time"].Format)
	assert.Equal(t, StringFormat("date"), schema.Properties["date"].Format)
	assert.Equal(t, StringFormat("time"), schema.Properties["time"].Format)
	assert.Equal(t, StringFormat("email"), schema.Properties["email"].Format)
	assert.Equal(t, StringFormat("uri"), schema.Properties["uri"].Format)
	assert.Equal(t, StringFormat("uuid"), schema.Properties["uuid"].Format)
	assert.Equal(t, StringFormat("hostname"), schema.Properties["hostname"].Format)
	assert.Equal(t, StringFormat("ipv4"), schema.Properties["ipv4"].Format)
	assert.Equal(t, StringFormat("ipv6"), schema.Properties["ipv6"].Format)
}

// Test default values for different types
func TestSchemaGenerator_DefaultValues(t *testing.T) {
	type WithDefaults struct {
		StringVal string  `json:"string_val" jsonschema:"default=hello"`
		IntVal    int     `json:"int_val" jsonschema:"default=42"`
		FloatVal  float64 `json:"float_val" jsonschema:"default=3.14"`
		BoolVal   bool    `json:"bool_val" jsonschema:"default=true"`
	}

	g := NewSchemaGenerator()
	schema, err := g.GenerateSchema(reflect.TypeOf(WithDefaults{}))

	require.NoError(t, err)

	assert.Equal(t, "hello", schema.Properties["string_val"].Default)
	assert.Equal(t, int64(42), schema.Properties["int_val"].Default)
	assert.Equal(t, 3.14, schema.Properties["float_val"].Default)
	assert.Equal(t, true, schema.Properties["bool_val"].Default)
}
