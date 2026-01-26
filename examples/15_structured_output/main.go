// Package main demonstrates the Structured Output module for type-safe LLM responses.
// This example shows JSON Schema generation, validation, and structured output parsing.
package main

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/BaSui01/agentflow/agent/structured"
)

// TaskResult represents a task execution result.
type TaskResult struct {
	Status      string   `json:"status" jsonschema:"required,enum=success,failure,pending"`
	Message     string   `json:"message" jsonschema:"required,minLength=1"`
	Score       float64  `json:"score" jsonschema:"minimum=0,maximum=100"`
	Tags        []string `json:"tags" jsonschema:"minItems=1"`
	CompletedAt *string  `json:"completed_at,omitempty" jsonschema:"format=date-time"`
}

// UserProfile represents a user profile with nested objects.
type UserProfile struct {
	ID       string   `json:"id" jsonschema:"required,format=uuid"`
	Name     string   `json:"name" jsonschema:"required,minLength=1,maxLength=100"`
	Email    string   `json:"email" jsonschema:"required,format=email"`
	Age      int      `json:"age" jsonschema:"minimum=0,maximum=150"`
	Role     string   `json:"role" jsonschema:"enum=admin,user,guest"`
	Settings Settings `json:"settings"`
}

// Settings represents user settings.
type Settings struct {
	Theme        string `json:"theme" jsonschema:"enum=light,dark,auto"`
	Notification bool   `json:"notification"`
	Language     string `json:"language" jsonschema:"pattern=^[a-z]{2}$"`
}

func main() {
	fmt.Println("=== AgentFlow Structured Output Example ===")

	// 1. Schema Generation
	demonstrateSchemaGeneration()

	// 2. Schema Validation
	demonstrateSchemaValidation()

	// 3. Manual Schema Building
	demonstrateManualSchema()
}

// demonstrateSchemaGeneration shows how to generate JSON Schema from Go types.
func demonstrateSchemaGeneration() {
	fmt.Println("--- 1. Schema Generation ---")

	generator := structured.NewSchemaGenerator()

	// Generate schema from TaskResult type
	schema, err := generator.GenerateSchema(reflect.TypeOf(TaskResult{}))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Print schema as JSON
	schemaJSON, _ := schema.ToJSONIndent()
	fmt.Printf("TaskResult Schema:\n%s\n\n", schemaJSON)

	// Generate schema from UserProfile (nested struct)
	schema2, _ := generator.GenerateSchema(reflect.TypeOf(UserProfile{}))
	schemaJSON2, _ := schema2.ToJSONIndent()
	fmt.Printf("UserProfile Schema:\n%s\n\n", schemaJSON2)
}

// demonstrateSchemaValidation shows how to validate JSON against a schema.
func demonstrateSchemaValidation() {
	fmt.Println("--- 2. Schema Validation ---")

	generator := structured.NewSchemaGenerator()
	validator := structured.NewValidator()

	// Generate schema
	schema, _ := generator.GenerateSchema(reflect.TypeOf(TaskResult{}))

	// Valid JSON
	validJSON := `{
		"status": "success",
		"message": "Task completed successfully",
		"score": 95.5,
		"tags": ["important", "reviewed"]
	}`

	// Invalid JSON (missing required field, invalid enum)
	invalidJSON := `{
		"status": "unknown",
		"message": "",
		"score": 150,
		"tags": []
	}`

	fmt.Println("Valid JSON:")
	if err := validator.Validate([]byte(validJSON), schema); err != nil {
		fmt.Printf("  ✗ Validation failed: %v\n", err)
	} else {
		fmt.Println("  ✓ Validation passed")
	}

	fmt.Println("\nInvalid JSON:")
	if err := validator.Validate([]byte(invalidJSON), schema); err != nil {
		fmt.Printf("  ✗ Validation errors:\n")
		if ve, ok := err.(*structured.ValidationErrors); ok {
			for _, e := range ve.Errors {
				fmt.Printf("    - %s: %s\n", e.Path, e.Message)
			}
		}
	} else {
		fmt.Println("  ✓ Validation passed")
	}
	fmt.Println()
}

// demonstrateManualSchema shows how to build schemas programmatically.
func demonstrateManualSchema() {
	fmt.Println("--- 3. Manual Schema Building ---")

	// Build a schema for an API response
	schema := structured.NewObjectSchema().
		WithTitle("APIResponse").
		WithDescription("Standard API response format")

	// Add properties
	schema.AddProperty("success", structured.NewBooleanSchema().
		WithDescription("Whether the request was successful"))

	schema.AddProperty("code", structured.NewIntegerSchema().
		WithMinimum(100).
		WithMaximum(599).
		WithDescription("HTTP status code"))

	schema.AddProperty("message", structured.NewStringSchema().
		WithMinLength(1).
		WithDescription("Response message"))

	// Add data property with nested object
	dataSchema := structured.NewObjectSchema()
	dataSchema.AddProperty("items", structured.NewArraySchema(
		structured.NewObjectSchema().
			AddProperty("id", structured.NewStringSchema()).
			AddProperty("name", structured.NewStringSchema()),
	).WithMinItems(0))
	dataSchema.AddProperty("total", structured.NewIntegerSchema().WithMinimum(0))

	schema.AddProperty("data", dataSchema)

	// Mark required fields
	schema.AddRequired("success", "code", "message")

	// Print the schema
	schemaJSON, _ := schema.ToJSONIndent()
	fmt.Printf("APIResponse Schema:\n%s\n\n", schemaJSON)

	// Validate sample data
	validator := structured.NewValidator()
	sampleData := map[string]any{
		"success": true,
		"code":    200,
		"message": "OK",
		"data": map[string]any{
			"items": []any{
				map[string]any{"id": "1", "name": "Item 1"},
				map[string]any{"id": "2", "name": "Item 2"},
			},
			"total": 2,
		},
	}

	dataBytes, _ := json.Marshal(sampleData)
	if err := validator.Validate(dataBytes, schema); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
	} else {
		fmt.Println("Sample data validation: ✓ Passed")
	}
}
