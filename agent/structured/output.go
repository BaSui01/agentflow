// Package structured provides structured output support with JSON Schema validation.
package structured

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/BaSui01/agentflow/llm"
)

// StructuredOutputProvider extends llm.Provider with structured output capability detection.
// Providers that support native structured output (like OpenAI's JSON mode) should implement this.
type StructuredOutputProvider interface {
	llm.Provider
	// SupportsStructuredOutput returns true if the provider supports native structured output.
	SupportsStructuredOutput() bool
}

// ParseResult represents the result of parsing structured output.
type ParseResult[T any] struct {
	Value  *T           `json:"value,omitempty"`
	Raw    string       `json:"raw"`
	Errors []ParseError `json:"errors,omitempty"`
}

// IsValid returns true if parsing was successful with no errors.
func (r *ParseResult[T]) IsValid() bool {
	return r.Value != nil && len(r.Errors) == 0
}

// StructuredOutput is a generic structured output processor that generates
// type-safe outputs from LLM providers.
type StructuredOutput[T any] struct {
	schema    *JSONSchema
	provider  llm.Provider
	validator SchemaValidator
	generator *SchemaGenerator
}

// NewStructuredOutput creates a new structured output processor for type T.
// It automatically generates a JSON Schema from the type parameter.
func NewStructuredOutput[T any](provider llm.Provider) (*StructuredOutput[T], error) {
	if provider == nil {
		return nil, fmt.Errorf("provider cannot be nil")
	}

	generator := NewSchemaGenerator()
	var zero T
	schema, err := generator.GenerateSchema(reflect.TypeOf(zero))
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema for type %T: %w", zero, err)
	}

	return &StructuredOutput[T]{
		schema:    schema,
		provider:  provider,
		validator: NewValidator(),
		generator: generator,
	}, nil
}

// NewStructuredOutputWithSchema creates a new structured output processor with a custom schema.
func NewStructuredOutputWithSchema[T any](provider llm.Provider, schema *JSONSchema) (*StructuredOutput[T], error) {
	if provider == nil {
		return nil, fmt.Errorf("provider cannot be nil")
	}
	if schema == nil {
		return nil, fmt.Errorf("schema cannot be nil")
	}

	return &StructuredOutput[T]{
		schema:    schema,
		provider:  provider,
		validator: NewValidator(),
		generator: NewSchemaGenerator(),
	}, nil
}

// Schema returns the JSON Schema used for validation.
func (s *StructuredOutput[T]) Schema() *JSONSchema {
	return s.schema
}

// Generate generates a structured output from a prompt string.
// It uses native structured output if the provider supports it,
// otherwise falls back to prompt engineering.
func (s *StructuredOutput[T]) Generate(ctx context.Context, prompt string) (*T, error) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}
	return s.GenerateWithMessages(ctx, messages)
}

// GenerateWithMessages generates a structured output from a list of messages.
// It uses native structured output if the provider supports it,
// otherwise falls back to prompt engineering.
func (s *StructuredOutput[T]) GenerateWithMessages(ctx context.Context, messages []llm.Message) (*T, error) {
	if s.supportsNativeStructuredOutput() {
		return s.generateNative(ctx, messages)
	}
	return s.generateWithPromptEngineering(ctx, messages)
}

// GenerateWithParse generates structured output and returns detailed parse result.
func (s *StructuredOutput[T]) GenerateWithParse(ctx context.Context, prompt string) (*ParseResult[T], error) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}
	return s.GenerateWithMessagesAndParse(ctx, messages)
}

// GenerateWithMessagesAndParse generates structured output from messages and returns detailed parse result.
func (s *StructuredOutput[T]) GenerateWithMessagesAndParse(ctx context.Context, messages []llm.Message) (*ParseResult[T], error) {
	var raw string
	var value *T
	var parseErrors []ParseError

	if s.supportsNativeStructuredOutput() {
		v, r, err := s.generateNativeWithRaw(ctx, messages)
		if err != nil {
			return nil, err
		}
		raw = r
		value = v
	} else {
		v, r, errs, err := s.generateWithPromptEngineeringDetailed(ctx, messages)
		if err != nil {
			return nil, err
		}
		raw = r
		value = v
		parseErrors = errs
	}

	return &ParseResult[T]{
		Value:  value,
		Raw:    raw,
		Errors: parseErrors,
	}, nil
}

// supportsNativeStructuredOutput checks if the provider supports native structured output.
func (s *StructuredOutput[T]) supportsNativeStructuredOutput() bool {
	if sp, ok := s.provider.(StructuredOutputProvider); ok {
		return sp.SupportsStructuredOutput()
	}
	return false
}

// generateNative uses the provider's native structured output capability.
func (s *StructuredOutput[T]) generateNative(ctx context.Context, messages []llm.Message) (*T, error) {
	value, _, err := s.generateNativeWithRaw(ctx, messages)
	return value, err
}

// generateNativeWithRaw uses native structured output and returns raw response.
func (s *StructuredOutput[T]) generateNativeWithRaw(ctx context.Context, messages []llm.Message) (*T, string, error) {
	// Build schema JSON for the request
	schemaJSON, err := json.Marshal(s.schema)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Add system message with schema instruction
	systemMsg := llm.Message{
		Role: llm.RoleSystem,
		Content: fmt.Sprintf(
			"You must respond with valid JSON that conforms to the following JSON Schema:\n%s\n\nRespond only with the JSON object, no additional text.",
			string(schemaJSON),
		),
	}

	// Prepend system message
	allMessages := append([]llm.Message{systemMsg}, messages...)

	req := &llm.ChatRequest{
		Messages: allMessages,
	}

	resp, err := s.provider.Completion(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("provider completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, "", fmt.Errorf("no response choices returned")
	}

	raw := resp.Choices[0].Message.Content

	// Parse and validate the response
	value, err := s.parseAndValidate(raw)
	if err != nil {
		return nil, raw, err
	}

	return value, raw, nil
}

// generateWithPromptEngineering uses prompt engineering to get structured output.
func (s *StructuredOutput[T]) generateWithPromptEngineering(ctx context.Context, messages []llm.Message) (*T, error) {
	value, _, _, err := s.generateWithPromptEngineeringDetailed(ctx, messages)
	return value, err
}

// generateWithPromptEngineeringDetailed uses prompt engineering and returns detailed results.
func (s *StructuredOutput[T]) generateWithPromptEngineeringDetailed(ctx context.Context, messages []llm.Message) (*T, string, []ParseError, error) {
	// Build schema JSON for the prompt
	schemaJSON, err := s.schema.ToJSONIndent()
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Create a detailed system prompt for structured output
	systemPrompt := s.buildStructuredOutputPrompt(string(schemaJSON))

	systemMsg := llm.Message{
		Role:    llm.RoleSystem,
		Content: systemPrompt,
	}

	// Prepend system message
	allMessages := append([]llm.Message{systemMsg}, messages...)

	req := &llm.ChatRequest{
		Messages: allMessages,
	}

	resp, err := s.provider.Completion(ctx, req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("provider completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, "", nil, fmt.Errorf("no response choices returned")
	}

	raw := resp.Choices[0].Message.Content

	// Extract JSON from the response (handle markdown code blocks, etc.)
	jsonStr := s.extractJSON(raw)

	// Parse and validate
	value, parseErrors := s.parseAndValidateDetailed(jsonStr)

	return value, raw, parseErrors, nil
}

// buildStructuredOutputPrompt creates a detailed prompt for structured output generation.
func (s *StructuredOutput[T]) buildStructuredOutputPrompt(schemaJSON string) string {
	var sb strings.Builder

	sb.WriteString("You are a helpful assistant that generates structured JSON output.\n\n")
	sb.WriteString("IMPORTANT INSTRUCTIONS:\n")
	sb.WriteString("1. You MUST respond with valid JSON that conforms to the schema below.\n")
	sb.WriteString("2. Do NOT include any text before or after the JSON.\n")
	sb.WriteString("3. Do NOT wrap the JSON in markdown code blocks.\n")
	sb.WriteString("4. Ensure all required fields are present and have valid values.\n")
	sb.WriteString("5. Follow all constraints specified in the schema (enum values, min/max, patterns, etc.).\n\n")
	sb.WriteString("JSON Schema:\n")
	sb.WriteString("```json\n")
	sb.WriteString(schemaJSON)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Respond with ONLY the JSON object.")

	return sb.String()
}

// extractJSON extracts JSON from a response that may contain markdown or other text.
func (s *StructuredOutput[T]) extractJSON(response string) string {
	response = strings.TrimSpace(response)

	// Try to extract from markdown code block
	if strings.Contains(response, "```") {
		// Match ```json ... ``` or ``` ... ```
		re := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")
		matches := re.FindStringSubmatch(response)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// Try to find JSON object boundaries
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		return response[start : end+1]
	}

	// Try to find JSON array boundaries
	start = strings.Index(response, "[")
	end = strings.LastIndex(response, "]")
	if start >= 0 && end > start {
		return response[start : end+1]
	}

	return response
}

// parseAndValidate parses JSON and validates against schema.
func (s *StructuredOutput[T]) parseAndValidate(jsonStr string) (*T, error) {
	value, errors := s.parseAndValidateDetailed(jsonStr)
	if len(errors) > 0 {
		return nil, &ValidationErrors{Errors: errors}
	}
	return value, nil
}

// parseAndValidateDetailed parses JSON and returns detailed validation errors.
func (s *StructuredOutput[T]) parseAndValidateDetailed(jsonStr string) (*T, []ParseError) {
	var errors []ParseError

	// Validate against schema first
	if err := s.validator.Validate([]byte(jsonStr), s.schema); err != nil {
		if ve, ok := err.(*ValidationErrors); ok {
			errors = append(errors, ve.Errors...)
		} else {
			errors = append(errors, ParseError{
				Path:    "",
				Message: err.Error(),
			})
		}
	}

	// Parse into target type
	var value T
	if err := json.Unmarshal([]byte(jsonStr), &value); err != nil {
		errors = append(errors, ParseError{
			Path:    "",
			Message: fmt.Sprintf("JSON parse error: %v", err),
		})
		return nil, errors
	}

	if len(errors) > 0 {
		return &value, errors
	}

	return &value, nil
}

// ValidateValue validates a value against the schema.
func (s *StructuredOutput[T]) ValidateValue(value *T) error {
	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return s.validator.Validate(data, s.schema)
}

// Parse parses a JSON string into the target type with validation.
func (s *StructuredOutput[T]) Parse(jsonStr string) (*T, error) {
	return s.parseAndValidate(jsonStr)
}

// ParseWithResult parses a JSON string and returns detailed result.
func (s *StructuredOutput[T]) ParseWithResult(jsonStr string) *ParseResult[T] {
	value, errors := s.parseAndValidateDetailed(jsonStr)
	return &ParseResult[T]{
		Value:  value,
		Raw:    jsonStr,
		Errors: errors,
	}
}
