package types

import (
	"encoding/json"
	"fmt"
)

// SchemaType represents JSON Schema types.
type SchemaType string

const (
	SchemaTypeString  SchemaType = "string"
	SchemaTypeNumber  SchemaType = "number"
	SchemaTypeInteger SchemaType = "integer"
	SchemaTypeBoolean SchemaType = "boolean"
	SchemaTypeNull    SchemaType = "null"
	SchemaTypeObject  SchemaType = "object"
	SchemaTypeArray   SchemaType = "array"
)

// StringFormat represents common string format constraints.
type StringFormat string

const (
	FormatDateTime StringFormat = "date-time"
	FormatDate     StringFormat = "date"
	FormatTime     StringFormat = "time"
	FormatEmail    StringFormat = "email"
	FormatURI      StringFormat = "uri"
	FormatUUID     StringFormat = "uuid"
)

// JSONSchema represents a JSON Schema definition.
type JSONSchema struct {
	Schema      string `json:"$schema,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`

	Type SchemaType `json:"type,omitempty"`

	// Object properties
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`

	// Array items
	Items    *JSONSchema `json:"items,omitempty"`
	MinItems *int        `json:"minItems,omitempty"`
	MaxItems *int        `json:"maxItems,omitempty"`

	// Enum and const
	Enum  []any `json:"enum,omitempty"`
	Const any   `json:"const,omitempty"`

	// String constraints
	MinLength *int         `json:"minLength,omitempty"`
	MaxLength *int         `json:"maxLength,omitempty"`
	Pattern   string       `json:"pattern,omitempty"`
	Format    StringFormat `json:"format,omitempty"`

	// Numeric constraints
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`

	// Default value
	Default any `json:"default,omitempty"`
}

// NewObjectSchema creates a new object schema.
func NewObjectSchema() *JSONSchema {
	return &JSONSchema{
		Type:       SchemaTypeObject,
		Properties: make(map[string]*JSONSchema),
	}
}

// NewArraySchema creates a new array schema.
func NewArraySchema(items *JSONSchema) *JSONSchema {
	return &JSONSchema{
		Type:  SchemaTypeArray,
		Items: items,
	}
}

// NewStringSchema creates a new string schema.
func NewStringSchema() *JSONSchema {
	return &JSONSchema{Type: SchemaTypeString}
}

// NewNumberSchema creates a new number schema.
func NewNumberSchema() *JSONSchema {
	return &JSONSchema{Type: SchemaTypeNumber}
}

// NewIntegerSchema creates a new integer schema.
func NewIntegerSchema() *JSONSchema {
	return &JSONSchema{Type: SchemaTypeInteger}
}

// NewBooleanSchema creates a new boolean schema.
func NewBooleanSchema() *JSONSchema {
	return &JSONSchema{Type: SchemaTypeBoolean}
}

// NewEnumSchema creates a new enum schema.
func NewEnumSchema(values ...any) *JSONSchema {
	return &JSONSchema{Enum: values}
}

// AddProperty adds a property to an object schema.
func (s *JSONSchema) AddProperty(name string, prop *JSONSchema) *JSONSchema {
	if s.Properties == nil {
		s.Properties = make(map[string]*JSONSchema)
	}
	s.Properties[name] = prop
	return s
}

// AddRequired adds required field names.
func (s *JSONSchema) AddRequired(names ...string) *JSONSchema {
	s.Required = append(s.Required, names...)
	return s
}

// WithDescription sets the description.
func (s *JSONSchema) WithDescription(desc string) *JSONSchema {
	s.Description = desc
	return s
}

// ToJSON serializes the schema to JSON.
func (s *JSONSchema) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// FromJSON deserializes a schema from JSON.
func FromJSON(data []byte) (*JSONSchema, error) {
	var schema JSONSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON schema: %w", err)
	}
	return &schema, nil
}
