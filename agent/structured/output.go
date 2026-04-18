package structured

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// ParseResult代表了解析结构化输出的结果.
type ParseResult[T any] struct {
	Value  *T             `json:"value,omitempty"`
	Raw    string         `json:"raw"`
	Errors []ParseError   `json:"errors,omitempty"`
	Usage  *llm.ChatUsage `json:"usage,omitempty"`
}

// IsValid 如果解析成功且没有出错, 则返回为真 。
func (r *ParseResult[T]) IsValid() bool {
	return r.Value != nil && len(r.Errors) == 0
}

// 结构化输出是一个通用结构化输出处理器，生成
// 基于 llmcore.Gateway 的类型安全输出。
type StructuredOutput[T any] struct {
	schema    *JSONSchema
	gateway   llmcore.Gateway
	validator SchemaValidator
	generator *SchemaGenerator
}

// NewStructuredOutput为T型创建了新的结构化输出处理器.
// 它从类型参数中自动生成了JSON Schema.
func NewStructuredOutput[T any](gateway llmcore.Gateway) (*StructuredOutput[T], error) {
	if gateway == nil {
		return nil, fmt.Errorf("gateway cannot be nil")
	}

	generator := NewSchemaGenerator()
	var zero T
	schema, err := generator.GenerateSchema(reflect.TypeOf(zero))
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema for type %T: %w", zero, err)
	}

	return &StructuredOutput[T]{
		schema:    schema,
		gateway:   gateway,
		validator: NewValidator(),
		generator: generator,
	}, nil
}

// NewStructured Output With Schema 创建了自定义的自定义计划的新结构化输出处理器.
func NewStructuredOutputWithSchema[T any](gateway llmcore.Gateway, schema *JSONSchema) (*StructuredOutput[T], error) {
	if gateway == nil {
		return nil, fmt.Errorf("gateway cannot be nil")
	}
	if schema == nil {
		return nil, fmt.Errorf("schema cannot be nil")
	}

	return &StructuredOutput[T]{
		schema:    schema,
		gateway:   gateway,
		validator: NewValidator(),
		generator: NewSchemaGenerator(),
	}, nil
}

// Schema返回用于验证的JSON Schema.
func (s *StructuredOutput[T]) Schema() *JSONSchema {
	return s.schema
}

// Generate 从 prompt 生成结构化输出。
// 结构化 schema 约束统一通过 llmcore.Gateway 下发，
// provider 差异由 llm 层处理。
func (s *StructuredOutput[T]) Generate(ctx context.Context, prompt string) (*T, error) {
	return s.GenerateWithRequest(ctx, &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	})
}

// 生成 Messages 从信件列表中生成结构化输出 。
func (s *StructuredOutput[T]) GenerateWithMessages(ctx context.Context, messages []types.Message) (*T, error) {
	return s.GenerateWithRequest(ctx, &llm.ChatRequest{
		Messages: messages,
	})
}

// GenerateWithRequest 从完整 ChatRequest 生成结构化输出，并保留调用方的模型与采样参数。
func (s *StructuredOutput[T]) GenerateWithRequest(ctx context.Context, req *llm.ChatRequest) (*T, error) {
	result, err := s.GenerateWithRequestAndParse(ctx, req)
	if err != nil {
		return nil, err
	}
	if !result.IsValid() {
		return nil, &ValidationErrors{Errors: result.Errors}
	}
	return result.Value, nil
}

// 生成 WithParse 生成结构化输出并返回详细解析结果 。
func (s *StructuredOutput[T]) GenerateWithParse(ctx context.Context, prompt string) (*ParseResult[T], error) {
	return s.GenerateWithRequestAndParse(ctx, &llm.ChatRequest{
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
	})
}

// 生成与Messages AndParse 从消息中生成结构化输出并返回详细解析结果.
func (s *StructuredOutput[T]) GenerateWithMessagesAndParse(ctx context.Context, messages []types.Message) (*ParseResult[T], error) {
	return s.GenerateWithRequestAndParse(ctx, &llm.ChatRequest{
		Messages: messages,
	})
}

// GenerateWithRequestAndParse 从完整 ChatRequest 生成结构化输出并返回详细解析结果。
func (s *StructuredOutput[T]) GenerateWithRequestAndParse(ctx context.Context, req *llm.ChatRequest) (*ParseResult[T], error) {
	value, raw, usage, parseErrors, err := s.generateWithGatewayDetailed(ctx, req)
	if err != nil {
		return nil, err
	}

	return &ParseResult[T]{
		Value:  value,
		Raw:    raw,
		Errors: parseErrors,
		Usage:  usage,
	}, nil
}

// generateWithGatewayDetailed 通过 llmcore.Gateway 统一入口生成结构化输出。
func (s *StructuredOutput[T]) generateWithGatewayDetailed(ctx context.Context, req *llm.ChatRequest) (*T, string, *llm.ChatUsage, []ParseError, error) {
	if req == nil {
		return nil, "", nil, nil, fmt.Errorf("chat request cannot be nil")
	}

	// 为请求构建 JSON Schema
	schemaJSON, err := json.Marshal(s.schema)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	// 将 schema 转换为 map[string]any 用于 ResponseFormat
	var schemaMap map[string]any
	if err := json.Unmarshal(schemaJSON, &schemaMap); err != nil {
		return nil, "", nil, nil, fmt.Errorf("failed to unmarshal schema to map: %w", err)
	}

	strict := true
	reqCopy := *req
	reqCopy.ResponseFormat = &llm.ResponseFormat{
		Type: llm.ResponseFormatJSONSchema,
		JSONSchema: &llm.JSONSchemaParam{
			Name:   "structured_output",
			Schema: schemaMap,
			Strict: &strict,
		},
	}

	resp, err := s.invokeChat(ctx, &reqCopy)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("gateway invoke failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, "", nil, nil, fmt.Errorf("no response choices returned")
	}

	raw := resp.Choices[0].Message.Content
	value, parseErrors := s.parseAndValidateDetailed(raw)
	usage := resp.Usage

	return value, raw, &usage, parseErrors, nil
}

func (s *StructuredOutput[T]) invokeChat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if s.gateway == nil {
		return nil, fmt.Errorf("gateway is not configured")
	}
	resp, err := s.gateway.Invoke(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  req.Model,
		TraceID:    req.TraceID,
		Payload:    req,
	})
	if err != nil {
		return nil, err
	}
	chatResp, ok := resp.Output.(*llm.ChatResponse)
	if !ok || chatResp == nil {
		return nil, fmt.Errorf("invalid chat response from gateway")
	}
	return chatResp, nil
}

// 解析AndValidate 解析JSON 并验证与计划。
func (s *StructuredOutput[T]) parseAndValidate(jsonStr string) (*T, error) {
	value, errors := s.parseAndValidateDetailed(jsonStr)
	if len(errors) > 0 {
		return nil, &ValidationErrors{Errors: errors}
	}
	return value, nil
}

// parseAndValidate Detailed pares JSON并返回详细的验证错误。
func (s *StructuredOutput[T]) parseAndValidateDetailed(jsonStr string) (*T, []ParseError) {
	var errors []ParseError

	// 先对计划进行验证
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

	// 分析为目标类型
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

// 校验Value 对照 schema 验证一个值 。
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

// 解析将 JSON 字符串解析为目标类型,并进行验证。
func (s *StructuredOutput[T]) Parse(jsonStr string) (*T, error) {
	return s.parseAndValidate(jsonStr)
}

// ParseWithResult 解析出 JSON 字符串并返回详细结果 。
func (s *StructuredOutput[T]) ParseWithResult(jsonStr string) *ParseResult[T] {
	value, errors := s.parseAndValidateDetailed(jsonStr)
	return &ParseResult[T]{
		Value:  value,
		Raw:    jsonStr,
		Errors: errors,
	}
}
