package rag

import (
	"encoding/json"
	"testing"
)

func TestRetrievalToolSchema(t *testing.T) {
	schema := RetrievalToolSchema()

	if schema.Name != ToolNameRetrieve {
		t.Errorf("expected name %q, got %q", ToolNameRetrieve, schema.Name)
	}

	if schema.Description == "" {
		t.Error("description should not be empty")
	}

	// Validate JSON parameters
	var params map[string]interface{}
	if err := json.Unmarshal(schema.Parameters, &params); err != nil {
		t.Errorf("invalid JSON parameters: %v", err)
	}

	// Check required fields exist
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties not found or invalid type")
	}

	requiredFields := []string{"query", "top_k", "collection", "filter"}
	for _, field := range requiredFields {
		if _, exists := properties[field]; !exists {
			t.Errorf("field %q not found in properties", field)
		}
	}

	// Check required array contains "query"
	required, ok := params["required"].([]interface{})
	if !ok || len(required) == 0 {
		t.Error("required array should contain at least one element")
	} else if required[0] != "query" {
		t.Errorf("first required field should be 'query', got %v", required[0])
	}
}

func TestRerankToolSchema(t *testing.T) {
	schema := RerankToolSchema()

	if schema.Name != ToolNameRerank {
		t.Errorf("expected name %q, got %q", ToolNameRerank, schema.Name)
	}

	if schema.Description == "" {
		t.Error("description should not be empty")
	}

	// Validate JSON parameters
	var params map[string]interface{}
	if err := json.Unmarshal(schema.Parameters, &params); err != nil {
		t.Errorf("invalid JSON parameters: %v", err)
	}

	// Check required fields exist
	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties not found or invalid type")
	}

	requiredFields := []string{"query", "documents", "top_n"}
	for _, field := range requiredFields {
		if _, exists := properties[field]; !exists {
			t.Errorf("field %q not found in properties", field)
		}
	}

	// Check required array
	required, ok := params["required"].([]interface{})
	if !ok || len(required) != 2 {
		t.Errorf("required array should have 2 elements, got %v", required)
	}
}

func TestGetRAGToolSchemas(t *testing.T) {
	schemas := GetRAGToolSchemas()

	if len(schemas) != 2 {
		t.Errorf("expected 2 schemas, got %d", len(schemas))
	}

	expectedNames := map[string]bool{
		ToolNameRetrieve: false,
		ToolNameRerank:   false,
	}

	for _, schema := range schemas {
		if _, exists := expectedNames[schema.Name]; !exists {
			t.Errorf("unexpected schema name: %s", schema.Name)
		}
		expectedNames[schema.Name] = true
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("schema %q not found in results", name)
		}
	}
}
