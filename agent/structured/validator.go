package structured

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// SchemaValidator validates JSON data against a JSONSchema.
type SchemaValidator interface {
	Validate(data []byte, schema *JSONSchema) error
}

// ParseError represents a validation error with field path.
type ParseError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *ParseError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationErrors represents multiple validation errors.
type ValidationErrors struct {
	Errors []ParseError `json:"errors"`
}

// Error implements the error interface.
func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	var msgs []string
	for _, err := range e.Errors {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("validation failed with %d errors: %s", len(e.Errors), strings.Join(msgs, "; "))
}

// DefaultValidator is the default implementation of SchemaValidator.
type DefaultValidator struct {
	// formatValidators holds custom format validators
	formatValidators map[StringFormat]func(string) bool
}

// NewValidator creates a new DefaultValidator with built-in format validators.
func NewValidator() *DefaultValidator {
	v := &DefaultValidator{
		formatValidators: make(map[StringFormat]func(string) bool),
	}
	v.registerBuiltinFormats()
	return v
}

// registerBuiltinFormats registers built-in format validators.
func (v *DefaultValidator) registerBuiltinFormats() {
	// Email format
	v.formatValidators[FormatEmail] = func(s string) bool {
		// Simple email regex
		pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// URI format
	v.formatValidators[FormatURI] = func(s string) bool {
		pattern := `^[a-zA-Z][a-zA-Z0-9+.-]*://`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// UUID format
	v.formatValidators[FormatUUID] = func(s string) bool {
		pattern := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// Date-time format (ISO 8601)
	v.formatValidators[FormatDateTime] = func(s string) bool {
		pattern := `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(.\d+)?(Z|[+-]\d{2}:\d{2})?$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// Date format
	v.formatValidators[FormatDate] = func(s string) bool {
		pattern := `^\d{4}-\d{2}-\d{2}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// Time format
	v.formatValidators[FormatTime] = func(s string) bool {
		pattern := `^\d{2}:\d{2}:\d{2}(.\d+)?(Z|[+-]\d{2}:\d{2})?$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// IPv4 format
	v.formatValidators[FormatIPv4] = func(s string) bool {
		pattern := `^(\d{1,3}\.){3}\d{1,3}$`
		matched, _ := regexp.MatchString(pattern, s)
		if !matched {
			return false
		}
		parts := strings.Split(s, ".")
		for _, part := range parts {
			var num int
			fmt.Sscanf(part, "%d", &num)
			if num < 0 || num > 255 {
				return false
			}
		}
		return true
	}

	// IPv6 format
	v.formatValidators[FormatIPv6] = func(s string) bool {
		pattern := `^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$|^::$|^([0-9a-fA-F]{1,4}:)*:([0-9a-fA-F]{1,4}:)*[0-9a-fA-F]{1,4}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// Hostname format
	v.formatValidators[FormatHostname] = func(s string) bool {
		pattern := `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched && len(s) <= 253
	}
}

// RegisterFormat registers a custom format validator.
func (v *DefaultValidator) RegisterFormat(format StringFormat, validator func(string) bool) {
	v.formatValidators[format] = validator
}

// Validate validates JSON data against a schema.
func (v *DefaultValidator) Validate(data []byte, schema *JSONSchema) error {
	if schema == nil {
		return nil
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return &ValidationErrors{
			Errors: []ParseError{{Path: "", Message: fmt.Sprintf("invalid JSON: %v", err)}},
		}
	}

	var errors []ParseError
	v.validateValue(value, schema, "", &errors)

	if len(errors) > 0 {
		return &ValidationErrors{Errors: errors}
	}
	return nil
}

// validateValue validates a value against a schema at the given path.
func (v *DefaultValidator) validateValue(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	if schema == nil {
		return
	}

	// Check const first
	if schema.Const != nil {
		if !v.equalValues(value, schema.Const) {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("value must be %v", schema.Const),
			})
		}
		return
	}

	// Check enum
	if len(schema.Enum) > 0 {
		found := false
		for _, enumVal := range schema.Enum {
			if v.equalValues(value, enumVal) {
				found = true
				break
			}
		}
		if !found {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("value must be one of: %v", schema.Enum),
			})
		}
	}

	// Validate based on type
	if schema.Type != "" {
		v.validateType(value, schema, path, errors)
	}
}

// validateType validates a value against its expected type.
func (v *DefaultValidator) validateType(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	switch schema.Type {
	case TypeString:
		v.validateString(value, schema, path, errors)
	case TypeNumber:
		v.validateNumber(value, schema, path, errors)
	case TypeInteger:
		v.validateInteger(value, schema, path, errors)
	case TypeBoolean:
		v.validateBoolean(value, schema, path, errors)
	case TypeNull:
		v.validateNull(value, schema, path, errors)
	case TypeObject:
		v.validateObject(value, schema, path, errors)
	case TypeArray:
		v.validateArray(value, schema, path, errors)
	}
}

// validateString validates a string value.
func (v *DefaultValidator) validateString(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	str, ok := value.(string)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected string, got %T", value),
		})
		return
	}

	// Check minLength
	if schema.MinLength != nil && len(str) < *schema.MinLength {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("string length %d is less than minimum %d", len(str), *schema.MinLength),
		})
	}

	// Check maxLength
	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("string length %d exceeds maximum %d", len(str), *schema.MaxLength),
		})
	}

	// Check pattern
	if schema.Pattern != "" {
		matched, err := regexp.MatchString(schema.Pattern, str)
		if err != nil {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("invalid pattern %q: %v", schema.Pattern, err),
			})
		} else if !matched {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("string does not match pattern %q", schema.Pattern),
			})
		}
	}

	// Check format
	if schema.Format != "" {
		if validator, ok := v.formatValidators[schema.Format]; ok {
			if !validator(str) {
				*errors = append(*errors, ParseError{
					Path:    path,
					Message: fmt.Sprintf("string does not match format %q", schema.Format),
				})
			}
		}
	}
}

// validateNumber validates a number value.
func (v *DefaultValidator) validateNumber(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	num, ok := v.toFloat64(value)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected number, got %T", value),
		})
		return
	}

	v.validateNumericConstraints(num, schema, path, errors)
}

// validateInteger validates an integer value.
func (v *DefaultValidator) validateInteger(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	num, ok := v.toFloat64(value)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected integer, got %T", value),
		})
		return
	}

	// Check if it's actually an integer
	if num != math.Trunc(num) {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected integer, got %v", num),
		})
		return
	}

	v.validateNumericConstraints(num, schema, path, errors)
}

// validateNumericConstraints validates numeric constraints.
func (v *DefaultValidator) validateNumericConstraints(num float64, schema *JSONSchema, path string, errors *[]ParseError) {
	// Check minimum
	if schema.Minimum != nil && num < *schema.Minimum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v is less than minimum %v", num, *schema.Minimum),
		})
	}

	// Check maximum
	if schema.Maximum != nil && num > *schema.Maximum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v exceeds maximum %v", num, *schema.Maximum),
		})
	}

	// Check exclusiveMinimum
	if schema.ExclusiveMinimum != nil && num <= *schema.ExclusiveMinimum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v must be greater than %v", num, *schema.ExclusiveMinimum),
		})
	}

	// Check exclusiveMaximum
	if schema.ExclusiveMaximum != nil && num >= *schema.ExclusiveMaximum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v must be less than %v", num, *schema.ExclusiveMaximum),
		})
	}

	// Check multipleOf
	if schema.MultipleOf != nil && *schema.MultipleOf != 0 {
		quotient := num / *schema.MultipleOf
		if quotient != math.Trunc(quotient) {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("value %v is not a multiple of %v", num, *schema.MultipleOf),
			})
		}
	}
}

// validateBoolean validates a boolean value.
func (v *DefaultValidator) validateBoolean(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	if _, ok := value.(bool); !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected boolean, got %T", value),
		})
	}
}

// validateNull validates a null value.
func (v *DefaultValidator) validateNull(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	if value != nil {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected null, got %T", value),
		})
	}
}

// validateObject validates an object value.
func (v *DefaultValidator) validateObject(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	obj, ok := value.(map[string]any)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected object, got %T", value),
		})
		return
	}

	// Check required fields
	for _, req := range schema.Required {
		val, exists := obj[req]
		if !exists {
			*errors = append(*errors, ParseError{
				Path:    v.joinPath(path, req),
				Message: "required field is missing",
			})
		} else if val == nil {
			*errors = append(*errors, ParseError{
				Path:    v.joinPath(path, req),
				Message: "required field must not be null",
			})
		}
	}

	// Check minProperties
	if schema.MinProperties != nil && len(obj) < *schema.MinProperties {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("object has %d properties, minimum is %d", len(obj), *schema.MinProperties),
		})
	}

	// Check maxProperties
	if schema.MaxProperties != nil && len(obj) > *schema.MaxProperties {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("object has %d properties, maximum is %d", len(obj), *schema.MaxProperties),
		})
	}

	// Validate properties
	for propName, propValue := range obj {
		propPath := v.joinPath(path, propName)

		// Check if property is defined in schema
		if propSchema, ok := schema.Properties[propName]; ok {
			v.validateValue(propValue, propSchema, propPath, errors)
		} else if schema.AdditionalProperties != nil {
			// Check additionalProperties
			if !schema.AdditionalProperties.Allowed && schema.AdditionalProperties.Schema == nil {
				*errors = append(*errors, ParseError{
					Path:    propPath,
					Message: "additional property not allowed",
				})
			} else if schema.AdditionalProperties.Schema != nil {
				v.validateValue(propValue, schema.AdditionalProperties.Schema, propPath, errors)
			}
		}

		// Check patternProperties
		for pattern, patternSchema := range schema.PatternProperties {
			matched, err := regexp.MatchString(pattern, propName)
			if err == nil && matched {
				v.validateValue(propValue, patternSchema, propPath, errors)
			}
		}
	}

	// Validate propertyNames
	if schema.PropertyNames != nil {
		for propName := range obj {
			v.validateValue(propName, schema.PropertyNames, v.joinPath(path, propName), errors)
		}
	}
}

// validateArray validates an array value.
func (v *DefaultValidator) validateArray(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	arr, ok := value.([]any)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected array, got %T", value),
		})
		return
	}

	// Check minItems
	if schema.MinItems != nil && len(arr) < *schema.MinItems {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("array has %d items, minimum is %d", len(arr), *schema.MinItems),
		})
	}

	// Check maxItems
	if schema.MaxItems != nil && len(arr) > *schema.MaxItems {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("array has %d items, maximum is %d", len(arr), *schema.MaxItems),
		})
	}

	// Check uniqueItems
	if schema.UniqueItems != nil && *schema.UniqueItems {
		seen := make(map[string]bool)
		for i, item := range arr {
			key := v.valueKey(item)
			if seen[key] {
				*errors = append(*errors, ParseError{
					Path:    fmt.Sprintf("%s[%d]", path, i),
					Message: "duplicate item in array with uniqueItems constraint",
				})
			}
			seen[key] = true
		}
	}

	// Validate items
	if schema.Items != nil {
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			v.validateValue(item, schema.Items, itemPath, errors)
		}
	}

	// Validate prefixItems
	if len(schema.PrefixItems) > 0 {
		for i, prefixSchema := range schema.PrefixItems {
			if i < len(arr) {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				v.validateValue(arr[i], prefixSchema, itemPath, errors)
			}
		}
	}

	// Validate contains
	if schema.Contains != nil {
		containsCount := 0
		for _, item := range arr {
			var itemErrors []ParseError
			v.validateValue(item, schema.Contains, "", &itemErrors)
			if len(itemErrors) == 0 {
				containsCount++
			}
		}

		minContains := 1
		if schema.MinContains != nil {
			minContains = *schema.MinContains
		}

		if containsCount < minContains {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("array must contain at least %d matching items, found %d", minContains, containsCount),
			})
		}

		if schema.MaxContains != nil && containsCount > *schema.MaxContains {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("array must contain at most %d matching items, found %d", *schema.MaxContains, containsCount),
			})
		}
	}
}

// toFloat64 converts a value to float64.
func (v *DefaultValidator) toFloat64(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// equalValues compares two values for equality.
func (v *DefaultValidator) equalValues(a, b any) bool {
	// Handle numeric comparison
	aNum, aIsNum := v.toFloat64(a)
	bNum, bIsNum := v.toFloat64(b)
	if aIsNum && bIsNum {
		return aNum == bNum
	}

	// Handle string comparison
	aStr, aIsStr := a.(string)
	bStr, bIsStr := b.(string)
	if aIsStr && bIsStr {
		return aStr == bStr
	}

	// Handle boolean comparison
	aBool, aIsBool := a.(bool)
	bBool, bIsBool := b.(bool)
	if aIsBool && bIsBool {
		return aBool == bBool
	}

	// Handle nil comparison
	if a == nil && b == nil {
		return true
	}

	// Use JSON serialization for complex types
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

// joinPath joins path segments.
func (v *DefaultValidator) joinPath(base, segment string) string {
	if base == "" {
		return segment
	}
	return base + "." + segment
}

// valueKey generates a unique key for a value (for uniqueItems check).
func (v *DefaultValidator) valueKey(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
