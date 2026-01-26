// Package structured provides structured output support with JSON Schema validation.
package structured

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// SchemaGenerator generates JSON Schema from Go types using reflection.
type SchemaGenerator struct {
	// visited tracks types being processed to handle recursive types
	visited map[reflect.Type]bool
}

// NewSchemaGenerator creates a new SchemaGenerator instance.
func NewSchemaGenerator() *SchemaGenerator {
	return &SchemaGenerator{
		visited: make(map[reflect.Type]bool),
	}
}

// GenerateSchema generates a JSON Schema from a Go type.
// It supports structs, slices, maps, pointers, and basic types.
// Struct fields can use `json` tags for field names and `jsonschema` tags for validation constraints.
//
// Supported jsonschema tag options:
//   - required: mark field as required
//   - enum=a,b,c: enum values
//   - minimum=0: minimum value for numbers
//   - maximum=100: maximum value for numbers
//   - minLength=1: minimum string length
//   - maxLength=100: maximum string length
//   - pattern=^[a-z]+$: regex pattern for strings
//   - format=email: string format (email, uri, uuid, date-time, etc.)
//   - minItems=1: minimum array items
//   - maxItems=10: maximum array items
//   - description=...: field description
//   - default=...: default value
func (g *SchemaGenerator) GenerateSchema(t reflect.Type) (*JSONSchema, error) {
	// Reset visited map for each top-level call
	g.visited = make(map[reflect.Type]bool)
	return g.generateSchema(t)
}

// generateSchema is the internal recursive implementation.
func (g *SchemaGenerator) generateSchema(t reflect.Type) (*JSONSchema, error) {
	// Handle nil type
	if t == nil {
		return nil, fmt.Errorf("cannot generate schema for nil type")
	}

	// Dereference pointer types
	if t.Kind() == reflect.Ptr {
		return g.generateSchema(t.Elem())
	}

	// Check for recursive types
	if g.visited[t] {
		// Return a reference placeholder for recursive types
		return &JSONSchema{Type: TypeObject}, nil
	}

	switch t.Kind() {
	case reflect.String:
		return NewStringSchema(), nil

	case reflect.Bool:
		return NewBooleanSchema(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return NewIntegerSchema(), nil

	case reflect.Float32, reflect.Float64:
		return NewNumberSchema(), nil

	case reflect.Slice, reflect.Array:
		return g.generateArraySchema(t)

	case reflect.Map:
		return g.generateMapSchema(t)

	case reflect.Struct:
		return g.generateStructSchema(t)

	case reflect.Interface:
		// Interface{} maps to any type
		return &JSONSchema{}, nil

	default:
		return nil, fmt.Errorf("unsupported type: %s", t.Kind())
	}
}

// generateArraySchema generates schema for slice/array types.
func (g *SchemaGenerator) generateArraySchema(t reflect.Type) (*JSONSchema, error) {
	elemSchema, err := g.generateSchema(t.Elem())
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema for array element: %w", err)
	}
	return NewArraySchema(elemSchema), nil
}

// generateMapSchema generates schema for map types.
func (g *SchemaGenerator) generateMapSchema(t reflect.Type) (*JSONSchema, error) {
	// Maps are represented as objects with additionalProperties
	valueSchema, err := g.generateSchema(t.Elem())
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema for map value: %w", err)
	}

	schema := NewObjectSchema()
	schema.AdditionalProperties = &AdditionalProperties{
		Allowed: true,
		Schema:  valueSchema,
	}
	return schema, nil
}

// generateStructSchema generates schema for struct types.
func (g *SchemaGenerator) generateStructSchema(t reflect.Type) (*JSONSchema, error) {
	// Mark as visited to handle recursive types
	g.visited[t] = true
	defer func() { g.visited[t] = false }()

	schema := NewObjectSchema()
	schema.Type = TypeObject

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get field name from json tag or use field name
		fieldName := getJSONFieldName(field)
		if fieldName == "-" {
			continue // Skip fields with json:"-"
		}

		// Generate schema for field type
		fieldSchema, err := g.generateSchema(field.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to generate schema for field %s: %w", field.Name, err)
		}

		// Apply jsonschema tag constraints
		if err := applyJSONSchemaTag(fieldSchema, field); err != nil {
			return nil, fmt.Errorf("failed to apply jsonschema tag for field %s: %w", field.Name, err)
		}

		// Check if field is required
		if isFieldRequired(field) {
			schema.Required = append(schema.Required, fieldName)
		}

		schema.Properties[fieldName] = fieldSchema
	}

	return schema, nil
}

// getJSONFieldName extracts the field name from json tag or returns the struct field name.
func getJSONFieldName(field reflect.StructField) string {
	jsonTag := field.Tag.Get("json")
	if jsonTag == "" {
		return field.Name
	}

	// Parse json tag (format: "name,options")
	parts := strings.Split(jsonTag, ",")
	name := parts[0]

	if name == "" {
		return field.Name
	}

	return name
}

// isFieldRequired checks if a field is marked as required via jsonschema tag.
func isFieldRequired(field reflect.StructField) bool {
	jsonschemaTag := field.Tag.Get("jsonschema")
	if jsonschemaTag == "" {
		return false
	}

	options := parseTagOptions(jsonschemaTag)
	_, required := options["required"]
	return required
}

// applyJSONSchemaTag applies jsonschema tag constraints to a schema.
func applyJSONSchemaTag(schema *JSONSchema, field reflect.StructField) error {
	jsonschemaTag := field.Tag.Get("jsonschema")
	if jsonschemaTag == "" {
		return nil
	}

	options := parseTagOptions(jsonschemaTag)

	// Apply description
	if desc, ok := options["description"]; ok {
		schema.Description = desc
	}

	// Apply default value
	if def, ok := options["default"]; ok {
		schema.Default = parseDefaultValue(def, field.Type)
	}

	// Apply enum values
	if enumStr, ok := options["enum"]; ok {
		enumValues := strings.Split(enumStr, ",")
		schema.Enum = make([]any, len(enumValues))
		for i, v := range enumValues {
			schema.Enum[i] = strings.TrimSpace(v)
		}
	}

	// Apply string constraints
	if minLen, ok := options["minLength"]; ok {
		if v, err := strconv.Atoi(minLen); err == nil {
			schema.MinLength = &v
		}
	}
	if maxLen, ok := options["maxLength"]; ok {
		if v, err := strconv.Atoi(maxLen); err == nil {
			schema.MaxLength = &v
		}
	}
	if pattern, ok := options["pattern"]; ok {
		schema.Pattern = pattern
	}
	if format, ok := options["format"]; ok {
		schema.Format = StringFormat(format)
	}

	// Apply numeric constraints
	if min, ok := options["minimum"]; ok {
		if v, err := strconv.ParseFloat(min, 64); err == nil {
			schema.Minimum = &v
		}
	}
	if max, ok := options["maximum"]; ok {
		if v, err := strconv.ParseFloat(max, 64); err == nil {
			schema.Maximum = &v
		}
	}

	// Apply array constraints
	if minItems, ok := options["minItems"]; ok {
		if v, err := strconv.Atoi(minItems); err == nil {
			schema.MinItems = &v
		}
	}
	if maxItems, ok := options["maxItems"]; ok {
		if v, err := strconv.Atoi(maxItems); err == nil {
			schema.MaxItems = &v
		}
	}

	return nil
}

// parseTagOptions parses a jsonschema tag string into a map of options.
// Format: "option1,option2=value2,option3=value3"
func parseTagOptions(tag string) map[string]string {
	options := make(map[string]string)
	if tag == "" {
		return options
	}

	parts := splitTagParts(tag)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for key=value format
		if idx := strings.Index(part, "="); idx > 0 {
			key := part[:idx]
			value := part[idx+1:]
			options[key] = value
		} else {
			// Boolean option (e.g., "required")
			options[part] = ""
		}
	}

	return options
}

// splitTagParts splits a tag string by commas, but respects values that contain commas
// within enum values (e.g., "enum=a,b,c" should keep "a,b,c" together).
// The logic: after seeing '=', we're in a value. A comma ends the value only if
// the next segment looks like a new key (alphanumeric without '=' before any special chars,
// or is a known boolean option like "required").
func splitTagParts(tag string) []string {
	var parts []string
	var current strings.Builder
	inValue := false

	// Known boolean options that don't have '='
	knownBoolOptions := map[string]bool{
		"required": true,
	}

	for i := 0; i < len(tag); i++ {
		ch := tag[i]

		if ch == '=' {
			inValue = true
			current.WriteByte(ch)
		} else if ch == ',' && !inValue {
			parts = append(parts, current.String())
			current.Reset()
		} else if ch == ',' && inValue {
			// Check if this comma is part of an enum value or separates options
			// Look ahead to see if next part looks like a new option
			remaining := tag[i+1:]

			// Find the next segment (up to next comma or end)
			nextComma := strings.Index(remaining, ",")
			var nextSegment string
			if nextComma >= 0 {
				nextSegment = remaining[:nextComma]
			} else {
				nextSegment = remaining
			}
			nextSegment = strings.TrimSpace(nextSegment)

			// Check if next segment is a known boolean option
			if knownBoolOptions[nextSegment] {
				parts = append(parts, current.String())
				current.Reset()
				inValue = false
				continue
			}

			// Check if next segment looks like a key=value option
			// It should have '=' and the part before '=' should be a valid key (alphanumeric)
			if eqIdx := strings.Index(nextSegment, "="); eqIdx > 0 {
				potentialKey := nextSegment[:eqIdx]
				// Valid key: alphanumeric, no spaces
				isValidKey := true
				for _, c := range potentialKey {
					if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
						isValidKey = false
						break
					}
				}
				if isValidKey {
					parts = append(parts, current.String())
					current.Reset()
					inValue = false
					continue
				}
			}

			// This comma is part of the current value (e.g., enum values)
			current.WriteByte(ch)
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// parseDefaultValue parses a default value string to the appropriate type.
func parseDefaultValue(value string, t reflect.Type) any {
	// Dereference pointer
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return value
	case reflect.Bool:
		return value == "true"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			return v
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v, err := strconv.ParseUint(value, 10, 64); err == nil {
			return v
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			return v
		}
	}
	return value
}

// GenerateSchemaFromValue generates a JSON Schema from a value's type.
// This is a convenience function that extracts the type from a value.
func (g *SchemaGenerator) GenerateSchemaFromValue(v any) (*JSONSchema, error) {
	if v == nil {
		return nil, fmt.Errorf("cannot generate schema from nil value")
	}
	return g.GenerateSchema(reflect.TypeOf(v))
}
