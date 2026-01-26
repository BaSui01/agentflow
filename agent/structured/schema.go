// Package structured provides structured output support with JSON Schema validation.
package structured

import (
	"encoding/json"
	"fmt"
)

// SchemaType represents JSON Schema types.
type SchemaType string

const (
	TypeString  SchemaType = "string"
	TypeNumber  SchemaType = "number"
	TypeInteger SchemaType = "integer"
	TypeBoolean SchemaType = "boolean"
	TypeNull    SchemaType = "null"
	TypeObject  SchemaType = "object"
	TypeArray   SchemaType = "array"
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
	FormatHostname StringFormat = "hostname"
	FormatIPv4     StringFormat = "ipv4"
	FormatIPv6     StringFormat = "ipv6"
)

// JSONSchema represents a JSON Schema definition.
// It supports nested objects, arrays, enums, and various validation constraints.
type JSONSchema struct {
	// Core schema metadata
	Schema      string `json:"$schema,omitempty"`
	ID          string `json:"$id,omitempty"`
	Ref         string `json:"$ref,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`

	// Type definition
	Type SchemaType `json:"type,omitempty"`

	// Object properties
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *AdditionalProperties  `json:"additionalProperties,omitempty"`
	MinProperties        *int                   `json:"minProperties,omitempty"`
	MaxProperties        *int                   `json:"maxProperties,omitempty"`
	PatternProperties    map[string]*JSONSchema `json:"patternProperties,omitempty"`
	PropertyNames        *JSONSchema            `json:"propertyNames,omitempty"`

	// Array items
	Items       *JSONSchema   `json:"items,omitempty"`
	PrefixItems []*JSONSchema `json:"prefixItems,omitempty"`
	Contains    *JSONSchema   `json:"contains,omitempty"`
	MinItems    *int          `json:"minItems,omitempty"`
	MaxItems    *int          `json:"maxItems,omitempty"`
	UniqueItems *bool         `json:"uniqueItems,omitempty"`
	MinContains *int          `json:"minContains,omitempty"`
	MaxContains *int          `json:"maxContains,omitempty"`

	// Enum and const
	Enum  []any `json:"enum,omitempty"`
	Const any   `json:"const,omitempty"`

	// String constraints
	MinLength *int         `json:"minLength,omitempty"`
	MaxLength *int         `json:"maxLength,omitempty"`
	Pattern   string       `json:"pattern,omitempty"`
	Format    StringFormat `json:"format,omitempty"`

	// Numeric constraints
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`
	MultipleOf       *float64 `json:"multipleOf,omitempty"`

	// Default value
	Default any `json:"default,omitempty"`

	// Examples
	Examples []any `json:"examples,omitempty"`

	// Composition keywords
	AllOf []*JSONSchema `json:"allOf,omitempty"`
	AnyOf []*JSONSchema `json:"anyOf,omitempty"`
	OneOf []*JSONSchema `json:"oneOf,omitempty"`
	Not   *JSONSchema   `json:"not,omitempty"`

	// Conditional keywords
	If   *JSONSchema `json:"if,omitempty"`
	Then *JSONSchema `json:"then,omitempty"`
	Else *JSONSchema `json:"else,omitempty"`

	// Definitions for reuse
	Defs map[string]*JSONSchema `json:"$defs,omitempty"`
}

// AdditionalProperties represents the additionalProperties field which can be
// either a boolean or a schema.
type AdditionalProperties struct {
	Allowed bool
	Schema  *JSONSchema
}

// MarshalJSON implements json.Marshaler for AdditionalProperties.
func (ap *AdditionalProperties) MarshalJSON() ([]byte, error) {
	if ap == nil {
		return json.Marshal(nil)
	}
	if ap.Schema != nil {
		return json.Marshal(ap.Schema)
	}
	return json.Marshal(ap.Allowed)
}

// UnmarshalJSON implements json.Unmarshaler for AdditionalProperties.
func (ap *AdditionalProperties) UnmarshalJSON(data []byte) error {
	// Try boolean first
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		ap.Allowed = b
		ap.Schema = nil
		return nil
	}

	// Try schema
	var schema JSONSchema
	if err := json.Unmarshal(data, &schema); err == nil {
		ap.Allowed = true
		ap.Schema = &schema
		return nil
	}

	return fmt.Errorf("additionalProperties must be boolean or schema")
}

// NewSchema creates a new JSONSchema with the specified type.
func NewSchema(t SchemaType) *JSONSchema {
	return &JSONSchema{Type: t}
}

// NewObjectSchema creates a new object schema.
func NewObjectSchema() *JSONSchema {
	return &JSONSchema{
		Type:       TypeObject,
		Properties: make(map[string]*JSONSchema),
	}
}

// NewArraySchema creates a new array schema with the specified items schema.
func NewArraySchema(items *JSONSchema) *JSONSchema {
	return &JSONSchema{
		Type:  TypeArray,
		Items: items,
	}
}

// NewStringSchema creates a new string schema.
func NewStringSchema() *JSONSchema {
	return &JSONSchema{Type: TypeString}
}

// NewNumberSchema creates a new number schema.
func NewNumberSchema() *JSONSchema {
	return &JSONSchema{Type: TypeNumber}
}

// NewIntegerSchema creates a new integer schema.
func NewIntegerSchema() *JSONSchema {
	return &JSONSchema{Type: TypeInteger}
}

// NewBooleanSchema creates a new boolean schema.
func NewBooleanSchema() *JSONSchema {
	return &JSONSchema{Type: TypeBoolean}
}

// NewEnumSchema creates a new enum schema with the specified values.
func NewEnumSchema(values ...any) *JSONSchema {
	return &JSONSchema{Enum: values}
}

// WithTitle sets the title and returns the schema for chaining.
func (s *JSONSchema) WithTitle(title string) *JSONSchema {
	s.Title = title
	return s
}

// WithDescription sets the description and returns the schema for chaining.
func (s *JSONSchema) WithDescription(desc string) *JSONSchema {
	s.Description = desc
	return s
}

// WithDefault sets the default value and returns the schema for chaining.
func (s *JSONSchema) WithDefault(def any) *JSONSchema {
	s.Default = def
	return s
}

// WithExamples sets the examples and returns the schema for chaining.
func (s *JSONSchema) WithExamples(examples ...any) *JSONSchema {
	s.Examples = examples
	return s
}

// AddProperty adds a property to an object schema.
func (s *JSONSchema) AddProperty(name string, prop *JSONSchema) *JSONSchema {
	if s.Properties == nil {
		s.Properties = make(map[string]*JSONSchema)
	}
	s.Properties[name] = prop
	return s
}

// AddRequired adds required field names to an object schema.
func (s *JSONSchema) AddRequired(names ...string) *JSONSchema {
	s.Required = append(s.Required, names...)
	return s
}

// WithMinLength sets the minimum length for string schema.
func (s *JSONSchema) WithMinLength(min int) *JSONSchema {
	s.MinLength = &min
	return s
}

// WithMaxLength sets the maximum length for string schema.
func (s *JSONSchema) WithMaxLength(max int) *JSONSchema {
	s.MaxLength = &max
	return s
}

// WithPattern sets the pattern for string schema.
func (s *JSONSchema) WithPattern(pattern string) *JSONSchema {
	s.Pattern = pattern
	return s
}

// WithFormat sets the format for string schema.
func (s *JSONSchema) WithFormat(format StringFormat) *JSONSchema {
	s.Format = format
	return s
}

// WithMinimum sets the minimum value for numeric schema.
func (s *JSONSchema) WithMinimum(min float64) *JSONSchema {
	s.Minimum = &min
	return s
}

// WithMaximum sets the maximum value for numeric schema.
func (s *JSONSchema) WithMaximum(max float64) *JSONSchema {
	s.Maximum = &max
	return s
}

// WithExclusiveMinimum sets the exclusive minimum value for numeric schema.
func (s *JSONSchema) WithExclusiveMinimum(min float64) *JSONSchema {
	s.ExclusiveMinimum = &min
	return s
}

// WithExclusiveMaximum sets the exclusive maximum value for numeric schema.
func (s *JSONSchema) WithExclusiveMaximum(max float64) *JSONSchema {
	s.ExclusiveMaximum = &max
	return s
}

// WithMultipleOf sets the multipleOf constraint for numeric schema.
func (s *JSONSchema) WithMultipleOf(val float64) *JSONSchema {
	s.MultipleOf = &val
	return s
}

// WithMinItems sets the minimum items for array schema.
func (s *JSONSchema) WithMinItems(min int) *JSONSchema {
	s.MinItems = &min
	return s
}

// WithMaxItems sets the maximum items for array schema.
func (s *JSONSchema) WithMaxItems(max int) *JSONSchema {
	s.MaxItems = &max
	return s
}

// WithUniqueItems sets the uniqueItems constraint for array schema.
func (s *JSONSchema) WithUniqueItems(unique bool) *JSONSchema {
	s.UniqueItems = &unique
	return s
}

// WithMinProperties sets the minimum properties for object schema.
func (s *JSONSchema) WithMinProperties(min int) *JSONSchema {
	s.MinProperties = &min
	return s
}

// WithMaxProperties sets the maximum properties for object schema.
func (s *JSONSchema) WithMaxProperties(max int) *JSONSchema {
	s.MaxProperties = &max
	return s
}

// WithAdditionalProperties sets the additionalProperties constraint.
func (s *JSONSchema) WithAdditionalProperties(allowed bool) *JSONSchema {
	s.AdditionalProperties = &AdditionalProperties{Allowed: allowed}
	return s
}

// WithAdditionalPropertiesSchema sets the additionalProperties to a schema.
func (s *JSONSchema) WithAdditionalPropertiesSchema(schema *JSONSchema) *JSONSchema {
	s.AdditionalProperties = &AdditionalProperties{Allowed: true, Schema: schema}
	return s
}

// WithEnum sets the enum values.
func (s *JSONSchema) WithEnum(values ...any) *JSONSchema {
	s.Enum = values
	return s
}

// WithConst sets the const value.
func (s *JSONSchema) WithConst(value any) *JSONSchema {
	s.Const = value
	return s
}

// Clone creates a deep copy of the schema.
func (s *JSONSchema) Clone() *JSONSchema {
	if s == nil {
		return nil
	}

	clone := &JSONSchema{
		Schema:      s.Schema,
		ID:          s.ID,
		Ref:         s.Ref,
		Title:       s.Title,
		Description: s.Description,
		Type:        s.Type,
		Pattern:     s.Pattern,
		Format:      s.Format,
		Default:     s.Default,
		Const:       s.Const,
	}

	// Clone properties
	if s.Properties != nil {
		clone.Properties = make(map[string]*JSONSchema, len(s.Properties))
		for k, v := range s.Properties {
			clone.Properties[k] = v.Clone()
		}
	}

	// Clone required
	if s.Required != nil {
		clone.Required = make([]string, len(s.Required))
		copy(clone.Required, s.Required)
	}

	// Clone items
	clone.Items = s.Items.Clone()

	// Clone prefix items
	if s.PrefixItems != nil {
		clone.PrefixItems = make([]*JSONSchema, len(s.PrefixItems))
		for i, item := range s.PrefixItems {
			clone.PrefixItems[i] = item.Clone()
		}
	}

	// Clone contains
	clone.Contains = s.Contains.Clone()

	// Clone enum
	if s.Enum != nil {
		clone.Enum = make([]any, len(s.Enum))
		copy(clone.Enum, s.Enum)
	}

	// Clone examples
	if s.Examples != nil {
		clone.Examples = make([]any, len(s.Examples))
		copy(clone.Examples, s.Examples)
	}

	// Clone numeric pointers
	if s.MinLength != nil {
		v := *s.MinLength
		clone.MinLength = &v
	}
	if s.MaxLength != nil {
		v := *s.MaxLength
		clone.MaxLength = &v
	}
	if s.Minimum != nil {
		v := *s.Minimum
		clone.Minimum = &v
	}
	if s.Maximum != nil {
		v := *s.Maximum
		clone.Maximum = &v
	}
	if s.ExclusiveMinimum != nil {
		v := *s.ExclusiveMinimum
		clone.ExclusiveMinimum = &v
	}
	if s.ExclusiveMaximum != nil {
		v := *s.ExclusiveMaximum
		clone.ExclusiveMaximum = &v
	}
	if s.MultipleOf != nil {
		v := *s.MultipleOf
		clone.MultipleOf = &v
	}
	if s.MinItems != nil {
		v := *s.MinItems
		clone.MinItems = &v
	}
	if s.MaxItems != nil {
		v := *s.MaxItems
		clone.MaxItems = &v
	}
	if s.UniqueItems != nil {
		v := *s.UniqueItems
		clone.UniqueItems = &v
	}
	if s.MinContains != nil {
		v := *s.MinContains
		clone.MinContains = &v
	}
	if s.MaxContains != nil {
		v := *s.MaxContains
		clone.MaxContains = &v
	}
	if s.MinProperties != nil {
		v := *s.MinProperties
		clone.MinProperties = &v
	}
	if s.MaxProperties != nil {
		v := *s.MaxProperties
		clone.MaxProperties = &v
	}

	// Clone additionalProperties
	if s.AdditionalProperties != nil {
		clone.AdditionalProperties = &AdditionalProperties{
			Allowed: s.AdditionalProperties.Allowed,
			Schema:  s.AdditionalProperties.Schema.Clone(),
		}
	}

	// Clone patternProperties
	if s.PatternProperties != nil {
		clone.PatternProperties = make(map[string]*JSONSchema, len(s.PatternProperties))
		for k, v := range s.PatternProperties {
			clone.PatternProperties[k] = v.Clone()
		}
	}

	// Clone propertyNames
	clone.PropertyNames = s.PropertyNames.Clone()

	// Clone composition keywords
	if s.AllOf != nil {
		clone.AllOf = make([]*JSONSchema, len(s.AllOf))
		for i, schema := range s.AllOf {
			clone.AllOf[i] = schema.Clone()
		}
	}
	if s.AnyOf != nil {
		clone.AnyOf = make([]*JSONSchema, len(s.AnyOf))
		for i, schema := range s.AnyOf {
			clone.AnyOf[i] = schema.Clone()
		}
	}
	if s.OneOf != nil {
		clone.OneOf = make([]*JSONSchema, len(s.OneOf))
		for i, schema := range s.OneOf {
			clone.OneOf[i] = schema.Clone()
		}
	}
	clone.Not = s.Not.Clone()

	// Clone conditional keywords
	clone.If = s.If.Clone()
	clone.Then = s.Then.Clone()
	clone.Else = s.Else.Clone()

	// Clone defs
	if s.Defs != nil {
		clone.Defs = make(map[string]*JSONSchema, len(s.Defs))
		for k, v := range s.Defs {
			clone.Defs[k] = v.Clone()
		}
	}

	return clone
}

// ToJSON serializes the schema to JSON.
func (s *JSONSchema) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// ToJSONIndent serializes the schema to indented JSON.
func (s *JSONSchema) ToJSONIndent() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// FromJSON deserializes a schema from JSON.
func FromJSON(data []byte) (*JSONSchema, error) {
	var schema JSONSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON schema: %w", err)
	}
	return &schema, nil
}

// IsRequired checks if a property is required.
func (s *JSONSchema) IsRequired(name string) bool {
	for _, req := range s.Required {
		if req == name {
			return true
		}
	}
	return false
}

// GetProperty returns a property schema by name.
func (s *JSONSchema) GetProperty(name string) *JSONSchema {
	if s.Properties == nil {
		return nil
	}
	return s.Properties[name]
}

// HasProperty checks if a property exists.
func (s *JSONSchema) HasProperty(name string) bool {
	if s.Properties == nil {
		return false
	}
	_, ok := s.Properties[name]
	return ok
}
