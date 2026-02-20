package structured

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 模拟提供商是用于测试的模拟LLM供应商.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ChatResponse{
		Choices: []llm.ChatChoice{
			{Message: llm.Message{Content: m.response}},
		},
	}, nil
}

func (m *mockProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, nil
}

func (m *mockProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) SupportsNativeFunctionCalling() bool {
	return false
}

func (m *mockProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}

// 模拟Structured Provider支持本地结构输出.
type mockStructuredProvider struct {
	mockProvider
}

func (m *mockStructuredProvider) SupportsStructuredOutput() bool {
	return true
}

// TestTaskResult是结构化输出的测试结构.
type TestTaskResult struct {
	Status  string   `json:"status" jsonschema:"enum=success,failure,pending,required"`
	Message string   `json:"message" jsonschema:"required"`
	Score   float64  `json:"score" jsonschema:"minimum=0,maximum=100"`
	Tags    []string `json:"tags" jsonschema:"minItems=1"`
}

func TestNewStructuredOutput(t *testing.T) {
	provider := &mockProvider{}

	t.Run("creates structured output successfully", func(t *testing.T) {
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)
		assert.NotNil(t, so)
		assert.NotNil(t, so.Schema())
	})

	t.Run("fails with nil provider", func(t *testing.T) {
		so, err := NewStructuredOutput[TestTaskResult](nil)
		assert.Error(t, err)
		assert.Nil(t, so)
	})
}

func TestNewStructuredOutputWithSchema(t *testing.T) {
	provider := &mockProvider{}
	schema := NewObjectSchema()

	t.Run("creates with custom schema", func(t *testing.T) {
		so, err := NewStructuredOutputWithSchema[TestTaskResult](provider, schema)
		require.NoError(t, err)
		assert.NotNil(t, so)
		assert.Equal(t, schema, so.Schema())
	})

	t.Run("fails with nil provider", func(t *testing.T) {
		so, err := NewStructuredOutputWithSchema[TestTaskResult](nil, schema)
		assert.Error(t, err)
		assert.Nil(t, so)
	})

	t.Run("fails with nil schema", func(t *testing.T) {
		so, err := NewStructuredOutputWithSchema[TestTaskResult](provider, nil)
		assert.Error(t, err)
		assert.Nil(t, so)
	})
}

func TestStructuredOutput_Generate(t *testing.T) {
	validJSON := `{"status":"success","message":"Task completed","score":85.5,"tags":["test"]}`

	t.Run("generates valid output", func(t *testing.T) {
		provider := &mockProvider{response: validJSON}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		result, err := so.Generate(context.Background(), "Generate a task result")
		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		assert.Equal(t, "Task completed", result.Message)
		assert.Equal(t, 85.5, result.Score)
		assert.Equal(t, []string{"test"}, result.Tags)
	})

	t.Run("handles markdown code block", func(t *testing.T) {
		provider := &mockProvider{response: "```json\n" + validJSON + "\n```"}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		result, err := so.Generate(context.Background(), "Generate a task result")
		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
	})

	t.Run("handles response with extra text", func(t *testing.T) {
		provider := &mockProvider{response: "Here is the result:\n" + validJSON + "\nDone."}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		result, err := so.Generate(context.Background(), "Generate a task result")
		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
	})
}

func TestStructuredOutput_GenerateWithMessages(t *testing.T) {
	validJSON := `{"status":"pending","message":"In progress","score":50,"tags":["wip"]}`

	t.Run("generates from messages", func(t *testing.T) {
		provider := &mockProvider{response: validJSON}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		messages := []llm.Message{
			{Role: llm.RoleUser, Content: "Generate a task result"},
		}
		result, err := so.GenerateWithMessages(context.Background(), messages)
		require.NoError(t, err)
		assert.Equal(t, "pending", result.Status)
	})
}

func TestStructuredOutput_GenerateWithParse(t *testing.T) {
	validJSON := `{"status":"success","message":"Done","score":100,"tags":["complete"]}`

	t.Run("returns parse result with value", func(t *testing.T) {
		provider := &mockProvider{response: validJSON}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		result, err := so.GenerateWithParse(context.Background(), "Generate")
		require.NoError(t, err)
		assert.True(t, result.IsValid())
		assert.NotNil(t, result.Value)
		assert.Equal(t, "success", result.Value.Status)
		assert.NotEmpty(t, result.Raw)
	})

	t.Run("returns parse result with errors for invalid JSON", func(t *testing.T) {
		provider := &mockProvider{response: `{"status":"invalid_status","message":"","score":150,"tags":[]}`}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		result, err := so.GenerateWithParse(context.Background(), "Generate")
		require.NoError(t, err)
		assert.False(t, result.IsValid())
		assert.NotEmpty(t, result.Errors)
	})
}

func TestStructuredOutput_Parse(t *testing.T) {
	t.Run("parses valid JSON", func(t *testing.T) {
		provider := &mockProvider{}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		jsonStr := `{"status":"success","message":"OK","score":75,"tags":["a","b"]}`
		result, err := so.Parse(jsonStr)
		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		assert.Equal(t, "OK", result.Message)
		assert.Equal(t, 75.0, result.Score)
		assert.Equal(t, []string{"a", "b"}, result.Tags)
	})

	t.Run("fails on invalid JSON", func(t *testing.T) {
		provider := &mockProvider{}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		_, err = so.Parse(`{invalid}`)
		assert.Error(t, err)
	})

	t.Run("fails on schema validation error", func(t *testing.T) {
		provider := &mockProvider{}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		// 积分超过上限
		_, err = so.Parse(`{"status":"success","message":"OK","score":150,"tags":["a"]}`)
		assert.Error(t, err)
	})
}

func TestStructuredOutput_ParseWithResult(t *testing.T) {
	provider := &mockProvider{}
	so, err := NewStructuredOutput[TestTaskResult](provider)
	require.NoError(t, err)

	t.Run("returns detailed result for valid JSON", func(t *testing.T) {
		jsonStr := `{"status":"success","message":"OK","score":50,"tags":["x"]}`
		result := so.ParseWithResult(jsonStr)
		assert.True(t, result.IsValid())
		assert.Equal(t, jsonStr, result.Raw)
	})

	t.Run("returns errors for invalid JSON", func(t *testing.T) {
		jsonStr := `{"status":"unknown","message":"","score":-10,"tags":[]}`
		result := so.ParseWithResult(jsonStr)
		assert.False(t, result.IsValid())
		assert.NotEmpty(t, result.Errors)
	})
}

func TestStructuredOutput_ValidateValue(t *testing.T) {
	provider := &mockProvider{}
	so, err := NewStructuredOutput[TestTaskResult](provider)
	require.NoError(t, err)

	t.Run("validates valid value", func(t *testing.T) {
		value := &TestTaskResult{
			Status:  "success",
			Message: "OK",
			Score:   80,
			Tags:    []string{"test"},
		}
		err := so.ValidateValue(value)
		assert.NoError(t, err)
	})

	t.Run("fails on nil value", func(t *testing.T) {
		err := so.ValidateValue(nil)
		assert.Error(t, err)
	})

	t.Run("fails on invalid value", func(t *testing.T) {
		value := &TestTaskResult{
			Status:  "invalid",
			Message: "OK",
			Score:   200, // exceeds maximum
			Tags:    []string{},
		}
		err := so.ValidateValue(value)
		assert.Error(t, err)
	})
}

func TestStructuredOutput_NativeProvider(t *testing.T) {
	validJSON := `{"status":"success","message":"Native","score":90,"tags":["native"]}`

	t.Run("uses native structured output", func(t *testing.T) {
		provider := &mockStructuredProvider{mockProvider: mockProvider{response: validJSON}}
		so, err := NewStructuredOutput[TestTaskResult](provider)
		require.NoError(t, err)

		result, err := so.Generate(context.Background(), "Generate")
		require.NoError(t, err)
		assert.Equal(t, "success", result.Status)
		assert.Equal(t, "Native", result.Message)
	})
}

func TestStructuredOutput_ExtractJSON(t *testing.T) {
	provider := &mockProvider{}
	so, err := NewStructuredOutput[TestTaskResult](provider)
	require.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"key":"value"}`,
			expected: `{"key":"value"}`,
		},
		{
			name:     "markdown code block",
			input:    "```json\n{\"key\":\"value\"}\n```",
			expected: `{"key":"value"}`,
		},
		{
			name:     "markdown without language",
			input:    "```\n{\"key\":\"value\"}\n```",
			expected: `{"key":"value"}`,
		},
		{
			name:     "JSON with surrounding text",
			input:    "Here is the result: {\"key\":\"value\"} Done.",
			expected: `{"key":"value"}`,
		},
		{
			name:     "JSON array",
			input:    "Result: [1,2,3] end",
			expected: `[1,2,3]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := so.extractJSON(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseResult_IsValid(t *testing.T) {
	t.Run("valid when value present and no errors", func(t *testing.T) {
		result := &ParseResult[TestTaskResult]{
			Value:  &TestTaskResult{},
			Errors: nil,
		}
		assert.True(t, result.IsValid())
	})

	t.Run("invalid when value is nil", func(t *testing.T) {
		result := &ParseResult[TestTaskResult]{
			Value:  nil,
			Errors: nil,
		}
		assert.False(t, result.IsValid())
	})

	t.Run("invalid when errors present", func(t *testing.T) {
		result := &ParseResult[TestTaskResult]{
			Value:  &TestTaskResult{},
			Errors: []ParseError{{Message: "error"}},
		}
		assert.False(t, result.IsValid())
	})
}

// TestComplexStruct 测试结构化输出并有嵌入类型.
type TestComplexStruct struct {
	ID       int                    `json:"id" jsonschema:"required"`
	Name     string                 `json:"name" jsonschema:"required,minLength=1"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Items    []TestItem             `json:"items" jsonschema:"minItems=0"`
}

type TestItem struct {
	Key   string `json:"key" jsonschema:"required"`
	Value string `json:"value"`
}

func TestStructuredOutput_ComplexTypes(t *testing.T) {
	provider := &mockProvider{}

	t.Run("handles complex nested types", func(t *testing.T) {
		so, err := NewStructuredOutput[TestComplexStruct](provider)
		require.NoError(t, err)

		schema := so.Schema()
		assert.Equal(t, TypeObject, schema.Type)
		assert.Contains(t, schema.Properties, "id")
		assert.Contains(t, schema.Properties, "name")
		assert.Contains(t, schema.Properties, "items")
	})

	t.Run("parses complex JSON", func(t *testing.T) {
		so, err := NewStructuredOutput[TestComplexStruct](provider)
		require.NoError(t, err)

		jsonStr := `{
			"id": 1,
			"name": "test",
			"metadata": {"key": "value"},
			"items": [{"key": "k1", "value": "v1"}]
		}`

		result, err := so.Parse(jsonStr)
		require.NoError(t, err)
		assert.Equal(t, 1, result.ID)
		assert.Equal(t, "test", result.Name)
		assert.Len(t, result.Items, 1)
	})
}

// 基准测试
func BenchmarkStructuredOutput_Parse(b *testing.B) {
	provider := &mockProvider{}
	so, _ := NewStructuredOutput[TestTaskResult](provider)
	jsonStr := `{"status":"success","message":"OK","score":75,"tags":["a","b"]}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = so.Parse(jsonStr)
	}
}

func BenchmarkStructuredOutput_ExtractJSON(b *testing.B) {
	provider := &mockProvider{}
	so, _ := NewStructuredOutput[TestTaskResult](provider)
	input := "```json\n{\"status\":\"success\",\"message\":\"OK\",\"score\":75,\"tags\":[\"a\"]}\n```"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = so.extractJSON(input)
	}
}

func BenchmarkStructuredOutput_ValidateValue(b *testing.B) {
	provider := &mockProvider{}
	so, _ := NewStructuredOutput[TestTaskResult](provider)
	value := &TestTaskResult{
		Status:  "success",
		Message: "OK",
		Score:   80,
		Tags:    []string{"test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = so.ValidateValue(value)
	}
}

// 各种类型的测试计划生成
func TestStructuredOutput_SchemaGeneration(t *testing.T) {
	provider := &mockProvider{}

	t.Run("generates schema for simple struct", func(t *testing.T) {
		type Simple struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		so, err := NewStructuredOutput[Simple](provider)
		require.NoError(t, err)

		schema := so.Schema()
		assert.Equal(t, TypeObject, schema.Type)
		assert.Contains(t, schema.Properties, "name")
		assert.Contains(t, schema.Properties, "age")
	})

	t.Run("generates schema for struct with pointer fields", func(t *testing.T) {
		type WithPointer struct {
			Value *string `json:"value"`
		}
		so, err := NewStructuredOutput[WithPointer](provider)
		require.NoError(t, err)

		schema := so.Schema()
		assert.Contains(t, schema.Properties, "value")
	})

	t.Run("generates schema for struct with slice", func(t *testing.T) {
		type WithSlice struct {
			Items []string `json:"items"`
		}
		so, err := NewStructuredOutput[WithSlice](provider)
		require.NoError(t, err)

		schema := so.Schema()
		assert.Contains(t, schema.Properties, "items")
		assert.Equal(t, TypeArray, schema.Properties["items"].Type)
	})
}

// 测试 JSON 集合解析器
func TestParseResult_JSON(t *testing.T) {
	result := &ParseResult[TestTaskResult]{
		Value: &TestTaskResult{
			Status:  "success",
			Message: "OK",
			Score:   100,
			Tags:    []string{"test"},
		},
		Raw:    `{"status":"success"}`,
		Errors: nil,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"raw"`)
}
