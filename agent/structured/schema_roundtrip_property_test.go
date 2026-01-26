package structured

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 7: Schema 生成与解析 Round-Trip
// **Validates: Requirements 3.1, 3.5, 3.6, 4.1, 4.3**
//
// For any valid Go struct type T, generating JSON Schema from T, then validating
// a valid JSON instance of T against that Schema should pass; parsing that JSON
// back to type T should yield an equivalent value.

// RoundTripSimpleStruct represents a simple struct for round-trip testing.
type RoundTripSimpleStruct struct {
	Name    string  `json:"name" jsonschema:"required"`
	Age     int     `json:"age" jsonschema:"required,minimum=0,maximum=150"`
	Active  bool    `json:"active"`
	Balance float64 `json:"balance"`
}

// RoundTripNestedStruct represents a struct with nested objects for round-trip testing.
type RoundTripNestedStruct struct {
	ID      string               `json:"id" jsonschema:"required"`
	Profile RoundTripProfileInfo `json:"profile" jsonschema:"required"`
	Tags    []string             `json:"tags"`
}

// RoundTripProfileInfo is a nested struct for testing.
type RoundTripProfileInfo struct {
	FirstName string `json:"first_name" jsonschema:"required"`
	LastName  string `json:"last_name" jsonschema:"required"`
	Email     string `json:"email" jsonschema:"format=email"`
}

// RoundTripArrayStruct represents a struct with array fields for round-trip testing.
type RoundTripArrayStruct struct {
	Items  []int     `json:"items" jsonschema:"required,minItems=1"`
	Names  []string  `json:"names"`
	Scores []float64 `json:"scores"`
}

// RoundTripEnumStruct represents a struct with enum fields for round-trip testing.
type RoundTripEnumStruct struct {
	Status   string `json:"status" jsonschema:"required,enum=active,inactive,pending"`
	Priority string `json:"priority" jsonschema:"enum=low,medium,high"`
}

// RoundTripComplexStruct represents a complex struct with multiple features.
type RoundTripComplexStruct struct {
	ID       string             `json:"id" jsonschema:"required"`
	Name     string             `json:"name" jsonschema:"required,minLength=1,maxLength=100"`
	Count    int                `json:"count" jsonschema:"minimum=0"`
	Enabled  bool               `json:"enabled"`
	Score    float64            `json:"score" jsonschema:"minimum=0,maximum=100"`
	Tags     []string           `json:"tags"`
	Metadata map[string]string  `json:"metadata"`
	SubItems []RoundTripSubItem `json:"sub_items"`
}

// RoundTripSubItem is a nested struct for RoundTripComplexStruct.
type RoundTripSubItem struct {
	Key   string `json:"key" jsonschema:"required"`
	Value string `json:"value" jsonschema:"required"`
}

// TestProperty_SchemaRoundTrip_SimpleStruct tests round-trip for simple structs.
// **Validates: Requirements 3.1, 4.1, 4.3**
func TestProperty_SchemaRoundTrip_SimpleStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Generate random valid instance
		instance := RoundTripSimpleStruct{
			Name:    rapid.StringMatching(`[a-zA-Z]{1,50}`).Draw(rt, "name"),
			Age:     rapid.IntRange(0, 150).Draw(rt, "age"),
			Active:  rapid.Bool().Draw(rt, "active"),
			Balance: rapid.Float64Range(-10000, 10000).Draw(rt, "balance"),
		}

		// Step 1: Generate schema from Go struct type
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")
		require.NotNil(t, schema, "Schema should not be nil")

		// Step 2: Serialize instance to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// Step 3: Validate JSON against generated schema
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid instance should pass schema validation")

		// Step 4: Parse JSON back to struct type
		var parsed RoundTripSimpleStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// Step 5: Verify equivalence
		assert.Equal(t, instance.Name, parsed.Name, "Name should be equal")
		assert.Equal(t, instance.Age, parsed.Age, "Age should be equal")
		assert.Equal(t, instance.Active, parsed.Active, "Active should be equal")
		assert.InDelta(t, instance.Balance, parsed.Balance, 0.0001, "Balance should be approximately equal")
	})
}

// TestProperty_SchemaRoundTrip_NestedStruct tests round-trip for nested structs.
// **Validates: Requirements 3.5, 4.1, 4.3**
func TestProperty_SchemaRoundTrip_NestedStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Generate random valid instance with nested object
		instance := RoundTripNestedStruct{
			ID: rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(rt, "id"),
			Profile: RoundTripProfileInfo{
				FirstName: rapid.StringMatching(`[A-Z][a-z]{2,20}`).Draw(rt, "firstName"),
				LastName:  rapid.StringMatching(`[A-Z][a-z]{2,20}`).Draw(rt, "lastName"),
				Email:     rapid.StringMatching(`[a-z]{3,10}@[a-z]{3,10}\.[a-z]{2,4}`).Draw(rt, "email"),
			},
			Tags: rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,10}`), 0, 5).Draw(rt, "tags"),
		}

		// Step 1: Generate schema from Go struct type
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// Step 2: Serialize instance to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// Step 3: Validate JSON against generated schema
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid nested instance should pass schema validation")

		// Step 4: Parse JSON back to struct type
		var parsed RoundTripNestedStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// Step 5: Verify equivalence
		assert.Equal(t, instance.ID, parsed.ID, "ID should be equal")
		assert.Equal(t, instance.Profile.FirstName, parsed.Profile.FirstName, "FirstName should be equal")
		assert.Equal(t, instance.Profile.LastName, parsed.Profile.LastName, "LastName should be equal")
		assert.Equal(t, instance.Profile.Email, parsed.Profile.Email, "Email should be equal")
		assert.Equal(t, instance.Tags, parsed.Tags, "Tags should be equal")
	})
}

// TestProperty_SchemaRoundTrip_ArrayStruct tests round-trip for structs with arrays.
// **Validates: Requirements 3.5, 4.1, 4.3**
func TestProperty_SchemaRoundTrip_ArrayStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Generate random valid instance with arrays
		instance := RoundTripArrayStruct{
			Items:  rapid.SliceOfN(rapid.IntRange(-1000, 1000), 1, 10).Draw(rt, "items"),
			Names:  rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,15}`), 0, 5).Draw(rt, "names"),
			Scores: rapid.SliceOfN(rapid.Float64Range(0, 100), 0, 5).Draw(rt, "scores"),
		}

		// Step 1: Generate schema from Go struct type
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// Step 2: Serialize instance to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// Step 3: Validate JSON against generated schema
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid array instance should pass schema validation")

		// Step 4: Parse JSON back to struct type
		var parsed RoundTripArrayStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// Step 5: Verify equivalence
		assert.Equal(t, instance.Items, parsed.Items, "Items should be equal")
		assert.Equal(t, instance.Names, parsed.Names, "Names should be equal")
		require.Equal(t, len(instance.Scores), len(parsed.Scores), "Scores length should be equal")
		for i := range instance.Scores {
			assert.InDelta(t, instance.Scores[i], parsed.Scores[i], 0.0001, "Score[%d] should be approximately equal", i)
		}
	})
}

// TestProperty_SchemaRoundTrip_EnumStruct tests round-trip for structs with enums.
// **Validates: Requirements 3.5, 4.1, 4.3**
func TestProperty_SchemaRoundTrip_EnumStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Generate random valid instance with enum values
		statusValues := []string{"active", "inactive", "pending"}
		priorityValues := []string{"low", "medium", "high"}

		instance := RoundTripEnumStruct{
			Status:   statusValues[rapid.IntRange(0, 2).Draw(rt, "statusIdx")],
			Priority: priorityValues[rapid.IntRange(0, 2).Draw(rt, "priorityIdx")],
		}

		// Step 1: Generate schema from Go struct type
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// Step 2: Serialize instance to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// Step 3: Validate JSON against generated schema
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid enum instance should pass schema validation")

		// Step 4: Parse JSON back to struct type
		var parsed RoundTripEnumStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// Step 5: Verify equivalence
		assert.Equal(t, instance.Status, parsed.Status, "Status should be equal")
		assert.Equal(t, instance.Priority, parsed.Priority, "Priority should be equal")
	})
}

// TestProperty_SchemaRoundTrip_ComplexStruct tests round-trip for complex structs.
// **Validates: Requirements 3.1, 3.5, 3.6, 4.1, 4.3**
func TestProperty_SchemaRoundTrip_ComplexStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Generate random valid complex instance
		numSubItems := rapid.IntRange(0, 3).Draw(rt, "numSubItems")
		subItems := make([]RoundTripSubItem, numSubItems)
		for i := range subItems {
			subItems[i] = RoundTripSubItem{
				Key:   rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "subKey"),
				Value: rapid.StringMatching(`[a-zA-Z0-9]{5,20}`).Draw(rt, "subValue"),
			}
		}

		numMetadata := rapid.IntRange(0, 3).Draw(rt, "numMetadata")
		metadata := make(map[string]string, numMetadata)
		for i := 0; i < numMetadata; i++ {
			key := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "metaKey")
			value := rapid.StringMatching(`[a-zA-Z0-9]{5,15}`).Draw(rt, "metaValue")
			metadata[key] = value
		}

		instance := RoundTripComplexStruct{
			ID:       rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(rt, "id"),
			Name:     rapid.StringMatching(`[a-zA-Z]{1,50}`).Draw(rt, "name"),
			Count:    rapid.IntRange(0, 1000).Draw(rt, "count"),
			Enabled:  rapid.Bool().Draw(rt, "enabled"),
			Score:    rapid.Float64Range(0, 100).Draw(rt, "score"),
			Tags:     rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,10}`), 0, 5).Draw(rt, "tags"),
			Metadata: metadata,
			SubItems: subItems,
		}

		// Step 1: Generate schema from Go struct type
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// Step 2: Serialize instance to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// Step 3: Validate JSON against generated schema
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid complex instance should pass schema validation")

		// Step 4: Parse JSON back to struct type
		var parsed RoundTripComplexStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// Step 5: Verify equivalence
		assert.Equal(t, instance.ID, parsed.ID, "ID should be equal")
		assert.Equal(t, instance.Name, parsed.Name, "Name should be equal")
		assert.Equal(t, instance.Count, parsed.Count, "Count should be equal")
		assert.Equal(t, instance.Enabled, parsed.Enabled, "Enabled should be equal")
		assert.InDelta(t, instance.Score, parsed.Score, 0.0001, "Score should be approximately equal")
		assert.Equal(t, instance.Tags, parsed.Tags, "Tags should be equal")
		assert.Equal(t, instance.Metadata, parsed.Metadata, "Metadata should be equal")
		assert.Equal(t, len(instance.SubItems), len(parsed.SubItems), "SubItems length should be equal")
		for i := range instance.SubItems {
			assert.Equal(t, instance.SubItems[i].Key, parsed.SubItems[i].Key, "SubItem[%d].Key should be equal", i)
			assert.Equal(t, instance.SubItems[i].Value, parsed.SubItems[i].Value, "SubItem[%d].Value should be equal", i)
		}
	})
}

// TestProperty_SchemaRoundTrip_RequiredFieldsValidation tests that required fields are enforced.
// **Validates: Requirements 3.6**
func TestProperty_SchemaRoundTrip_RequiredFieldsValidation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Generate schema from RoundTripSimpleStruct which has required fields
		schema, err := generator.GenerateSchema(reflect.TypeOf(RoundTripSimpleStruct{}))
		require.NoError(t, err, "Schema generation should succeed")

		// Verify schema has required fields
		require.Contains(t, schema.Required, "name", "Schema should mark 'name' as required")
		require.Contains(t, schema.Required, "age", "Schema should mark 'age' as required")

		// Test that missing required field fails validation
		incompleteData := map[string]any{
			"active":  true,
			"balance": 100.0,
			// Missing "name" and "age"
		}
		jsonData, _ := json.Marshal(incompleteData)

		err = validator.Validate(jsonData, schema)
		require.Error(t, err, "Missing required fields should fail validation")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok, "Error should be ValidationErrors type")
		require.NotEmpty(t, validationErr.Errors, "Should have validation errors")
	})
}

// TestProperty_SchemaRoundTrip_SchemaGenerationConsistency tests schema generation is consistent.
// **Validates: Requirements 4.3**
func TestProperty_SchemaRoundTrip_SchemaGenerationConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator1 := NewSchemaGenerator()
		generator2 := NewSchemaGenerator()

		// Generate schema twice from the same type
		schema1, err := generator1.GenerateSchema(reflect.TypeOf(RoundTripSimpleStruct{}))
		require.NoError(t, err)

		schema2, err := generator2.GenerateSchema(reflect.TypeOf(RoundTripSimpleStruct{}))
		require.NoError(t, err)

		// Schemas should be equivalent
		json1, _ := json.Marshal(schema1)
		json2, _ := json.Marshal(schema2)
		assert.JSONEq(t, string(json1), string(json2), "Schema generation should be consistent")
	})
}

// TestProperty_SchemaRoundTrip_NilAndEmptyValues tests handling of nil and empty values.
// **Validates: Requirements 3.5, 4.1**
func TestProperty_SchemaRoundTrip_NilAndEmptyValues(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Test struct with optional fields (no required tag)
		type OptionalStruct struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}

		instance := OptionalStruct{
			Name:  rapid.StringMatching(`[a-z]{0,10}`).Draw(rt, "name"),
			Items: rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,10}`), 0, 3).Draw(rt, "items"),
		}

		// Step 1: Generate schema
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err)

		// Step 2: Serialize to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err)

		// Step 3: Validate
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Optional fields should pass validation")

		// Step 4: Parse back
		var parsed OptionalStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err)

		// Step 5: Verify equivalence
		assert.Equal(t, instance.Name, parsed.Name)
		assert.Equal(t, instance.Items, parsed.Items)
	})
}

// TestProperty_SchemaRoundTrip_NumericConstraints tests numeric constraint validation.
// **Validates: Requirements 3.1, 3.5**
func TestProperty_SchemaRoundTrip_NumericConstraints(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Test struct with numeric constraints
		type ConstrainedStruct struct {
			Score int     `json:"score" jsonschema:"required,minimum=0,maximum=100"`
			Rate  float64 `json:"rate" jsonschema:"minimum=0,maximum=1"`
		}

		// Generate valid instance within constraints
		instance := ConstrainedStruct{
			Score: rapid.IntRange(0, 100).Draw(rt, "score"),
			Rate:  rapid.Float64Range(0, 1).Draw(rt, "rate"),
		}

		// Step 1: Generate schema
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err)

		// Verify constraints are in schema
		scoreSchema := schema.Properties["score"]
		require.NotNil(t, scoreSchema)
		require.NotNil(t, scoreSchema.Minimum)
		require.NotNil(t, scoreSchema.Maximum)
		assert.Equal(t, float64(0), *scoreSchema.Minimum)
		assert.Equal(t, float64(100), *scoreSchema.Maximum)

		// Step 2: Serialize to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err)

		// Step 3: Validate
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid constrained instance should pass validation")

		// Step 4: Parse back
		var parsed ConstrainedStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err)

		// Step 5: Verify equivalence
		assert.Equal(t, instance.Score, parsed.Score)
		assert.InDelta(t, instance.Rate, parsed.Rate, 0.0001)
	})
}

// TestProperty_SchemaRoundTrip_MapFields tests round-trip for structs with map fields.
// **Validates: Requirements 3.5, 4.1, 4.3**
func TestProperty_SchemaRoundTrip_MapFields(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// Test struct with map field
		type MapStruct struct {
			ID       string            `json:"id" jsonschema:"required"`
			Settings map[string]string `json:"settings"`
		}

		// Generate random map
		numSettings := rapid.IntRange(0, 5).Draw(rt, "numSettings")
		settings := make(map[string]string, numSettings)
		for i := 0; i < numSettings; i++ {
			key := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "settingKey")
			value := rapid.StringMatching(`[a-zA-Z0-9]{5,15}`).Draw(rt, "settingValue")
			settings[key] = value
		}

		instance := MapStruct{
			ID:       rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(rt, "id"),
			Settings: settings,
		}

		// Step 1: Generate schema
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err)

		// Step 2: Serialize to JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err)

		// Step 3: Validate
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid map instance should pass validation")

		// Step 4: Parse back
		var parsed MapStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err)

		// Step 5: Verify equivalence
		assert.Equal(t, instance.ID, parsed.ID)
		assert.Equal(t, instance.Settings, parsed.Settings)
	})
}
