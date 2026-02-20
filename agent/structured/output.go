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

// 结构输出输出输出扩展为 llm. 提供商具有结构化输出能力检测.
// 支持本土结构输出(如OpenAI的JSON模式)的提供商应当执行.
type StructuredOutputProvider interface {
	llm.Provider
	// 如果提供者支持本地结构输出, 则支持StructuredOutput 返回 true 。
	SupportsStructuredOutput() bool
}

// ParseResult代表了解析结构化输出的结果.
type ParseResult[T any] struct {
	Value  *T           `json:"value,omitempty"`
	Raw    string       `json:"raw"`
	Errors []ParseError `json:"errors,omitempty"`
}

// IsValid 如果解析成功且没有出错, 则返回为真 。
func (r *ParseResult[T]) IsValid() bool {
	return r.Value != nil && len(r.Errors) == 0
}

// 结构化输出是一个通用结构化输出处理器,生成
// LLM 提供者的类型安全输出。
type StructuredOutput[T any] struct {
	schema    *JSONSchema
	provider  llm.Provider
	validator SchemaValidator
	generator *SchemaGenerator
}

// NewStructuredOutput为T型创建了新的结构化输出处理器.
// 它从类型参数中自动生成了JSON Schema.
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

// NewStructured Output With Schema 创建了自定义的自定义计划的新结构化输出处理器.
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

// Schema返回用于验证的JSON Schema.
func (s *StructuredOutput[T]) Schema() *JSONSchema {
	return s.schema
}

// 生成从快取字符串生成结构化输出 。
// 它使用本地结构输出 如果提供者支持它,
// 否则会回到即时工程
func (s *StructuredOutput[T]) Generate(ctx context.Context, prompt string) (*T, error) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}
	return s.GenerateWithMessages(ctx, messages)
}

// 生成 Messages 从信件列表中生成结构化输出 。
// 它使用本地结构输出 如果提供者支持它,
// 否则会回到即时工程
func (s *StructuredOutput[T]) GenerateWithMessages(ctx context.Context, messages []llm.Message) (*T, error) {
	if s.supportsNativeStructuredOutput() {
		return s.generateNative(ctx, messages)
	}
	return s.generateWithPromptEngineering(ctx, messages)
}

// 生成 WithParse 生成结构化输出并返回详细解析结果 。
func (s *StructuredOutput[T]) GenerateWithParse(ctx context.Context, prompt string) (*ParseResult[T], error) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}
	return s.GenerateWithMessagesAndParse(ctx, messages)
}

// 生成与Messages AndParse 从消息中生成结构化输出并返回详细解析结果.
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

// 支持 NativeStructured Output 检查,如果提供者支持本地结构输出。
func (s *StructuredOutput[T]) supportsNativeStructuredOutput() bool {
	if sp, ok := s.provider.(StructuredOutputProvider); ok {
		return sp.SupportsStructuredOutput()
	}
	return false
}

// 生成 Native 使用提供者的本地结构输出能力.
func (s *StructuredOutput[T]) generateNative(ctx context.Context, messages []llm.Message) (*T, error) {
	value, _, err := s.generateNativeWithRaw(ctx, messages)
	return value, err
}

// 生成 NativeWithRaw 使用本地结构输出并返回原始响应。
func (s *StructuredOutput[T]) generateNativeWithRaw(ctx context.Context, messages []llm.Message) (*T, string, error) {
	// 为请求构建 JSON 计划
	schemaJSON, err := json.Marshal(s.schema)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal schema: %w", err)
	}

	// 添加带有计划指令的系统消息
	systemMsg := llm.Message{
		Role: llm.RoleSystem,
		Content: fmt.Sprintf(
			"You must respond with valid JSON that conforms to the following JSON Schema:\n%s\n\nRespond only with the JSON object, no additional text.",
			string(schemaJSON),
		),
	}

	// 预收系统消息
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

	// 解析并验证响应
	value, err := s.parseAndValidate(raw)
	if err != nil {
		return nil, raw, err
	}

	return value, raw, nil
}

// 生成WithPromptEngineering 使用即时工程来获得结构化输出.
func (s *StructuredOutput[T]) generateWithPromptEngineering(ctx context.Context, messages []llm.Message) (*T, error) {
	value, _, _, err := s.generateWithPromptEngineeringDetailed(ctx, messages)
	return value, err
}

// 生成与Prompt工程 详细使用即时工程并返回详细结果.
func (s *StructuredOutput[T]) generateWithPromptEngineeringDetailed(ctx context.Context, messages []llm.Message) (*T, string, []ParseError, error) {
	// 构建快捷的 JSON 计划
	schemaJSON, err := s.schema.ToJSONIndent()
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	// 为结构化输出创建详细系统提示
	systemPrompt := s.buildStructuredOutputPrompt(string(schemaJSON))

	systemMsg := llm.Message{
		Role:    llm.RoleSystem,
		Content: systemPrompt,
	}

	// 预收系统消息
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

	// 从响应中提取 JSON( 处理下标记代码块等)
	jsonStr := s.extractJSON(raw)

	// 解析和验证
	value, parseErrors := s.parseAndValidateDetailed(jsonStr)

	return value, raw, parseErrors, nil
}

// 构建StructuredOutputPrompt为结构化输出生成创建了详细提示.
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

// 取出JSON取出JSON从可能包含平分或其他文字的响应中取出.
func (s *StructuredOutput[T]) extractJSON(response string) string {
	response = strings.TrimSpace(response)

	// 尝试从 markdown 代码块提取
	if strings.Contains(response, "```") {
		// 火柴・杰森・・・或者・・・・・・
		re := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")
		matches := re.FindStringSubmatch(response)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// 尝试找到 JSON 对象边界
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		return response[start : end+1]
	}

	// 尝试找到 JSON 阵列边界
	start = strings.Index(response, "[")
	end = strings.LastIndex(response, "]")
	if start >= 0 && end > start {
		return response[start : end+1]
	}

	return response
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
