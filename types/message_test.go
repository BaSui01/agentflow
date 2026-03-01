package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Message constructors
// ============================================================

func TestNewMessage(t *testing.T) {
	msg := NewMessage(RoleUser, "hello")
	assert.Equal(t, RoleUser, msg.Role)
	assert.Equal(t, "hello", msg.Content)
	assert.NotZero(t, msg.Timestamp)
}

func TestNewSystemMessage(t *testing.T) {
	msg := NewSystemMessage("you are helpful")
	assert.Equal(t, RoleSystem, msg.Role)
	assert.Equal(t, "you are helpful", msg.Content)
}

func TestNewUserMessage(t *testing.T) {
	msg := NewUserMessage("hi")
	assert.Equal(t, RoleUser, msg.Role)
	assert.Equal(t, "hi", msg.Content)
}

func TestNewAssistantMessage(t *testing.T) {
	msg := NewAssistantMessage("hello there")
	assert.Equal(t, RoleAssistant, msg.Role)
	assert.Equal(t, "hello there", msg.Content)
}

func TestNewToolMessage(t *testing.T) {
	msg := NewToolMessage("tc-1", "calculator", "42")
	assert.Equal(t, RoleTool, msg.Role)
	assert.Equal(t, "42", msg.Content)
	assert.Equal(t, "calculator", msg.Name)
	assert.Equal(t, "tc-1", msg.ToolCallID)
	assert.NotZero(t, msg.Timestamp)
}

// ============================================================
// Message builder methods
// ============================================================

func TestMessage_WithToolCalls(t *testing.T) {
	msg := NewAssistantMessage("thinking")
	calls := []ToolCall{
		{ID: "tc-1", Name: "search", Arguments: json.RawMessage(`{"q":"test"}`)},
	}
	result := msg.WithToolCalls(calls)
	assert.Len(t, result.ToolCalls, 1)
	assert.Equal(t, "tc-1", result.ToolCalls[0].ID)
	// Original should not be modified (value receiver)
	assert.Empty(t, msg.ToolCalls)
}

func TestMessage_WithImages(t *testing.T) {
	msg := NewUserMessage("look at this")
	images := []ImageContent{
		{Type: "url", URL: "https://example.com/img.png"},
	}
	result := msg.WithImages(images)
	assert.Len(t, result.Images, 1)
	assert.Equal(t, "url", result.Images[0].Type)
	assert.Empty(t, msg.Images)
}

func TestMessage_WithMetadata(t *testing.T) {
	msg := NewUserMessage("hello")
	result := msg.WithMetadata(map[string]string{"source": "test"})
	assert.NotNil(t, result.Metadata)
	assert.Nil(t, msg.Metadata)
}

// ============================================================
// ToolResult
// ============================================================

func TestToolResult_ToMessage_Success(t *testing.T) {
	tr := ToolResult{
		ToolCallID: "tc-1",
		Name:       "calculator",
		Result:     json.RawMessage(`{"answer":42}`),
	}
	msg := tr.ToMessage()
	assert.Equal(t, RoleTool, msg.Role)
	assert.Equal(t, `{"answer":42}`, msg.Content)
	assert.Equal(t, "calculator", msg.Name)
	assert.Equal(t, "tc-1", msg.ToolCallID)
}

func TestToolResult_ToMessage_Error(t *testing.T) {
	tr := ToolResult{
		ToolCallID: "tc-1",
		Name:       "calculator",
		Error:      "division by zero",
	}
	msg := tr.ToMessage()
	assert.Equal(t, "Error: division by zero", msg.Content)
}

func TestToolResult_IsError(t *testing.T) {
	assert.False(t, ToolResult{}.IsError())
	assert.True(t, ToolResult{Error: "fail"}.IsError())
}

// ============================================================
// Error — constructors and helpers
// ============================================================

func TestNewError(t *testing.T) {
	err := NewError(ErrRateLimit, "too many requests")
	assert.Equal(t, ErrRateLimit, err.Code)
	assert.Equal(t, "too many requests", err.Message)
}

func TestError_Error_WithCause(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewError(ErrUpstreamError, "upstream failed").WithCause(cause)
	assert.Contains(t, err.Error(), "[UPSTREAM_ERROR]")
	assert.Contains(t, err.Error(), "upstream failed")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestError_Error_WithoutCause(t *testing.T) {
	err := NewError(ErrInvalidRequest, "bad input")
	assert.Equal(t, "[INVALID_REQUEST] bad input", err.Error())
}

func TestError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := NewError(ErrInternalError, "internal").WithCause(cause)
	assert.True(t, errors.Is(err, cause))
}

func TestError_WithHTTPStatus(t *testing.T) {
	err := NewError(ErrRateLimit, "rate limited").WithHTTPStatus(429)
	assert.Equal(t, 429, err.HTTPStatus)
}

func TestError_WithRetryable(t *testing.T) {
	err := NewError(ErrRateLimit, "rate limited").WithRetryable(true)
	assert.True(t, err.Retryable)
}

func TestError_WithProvider(t *testing.T) {
	err := NewError(ErrUpstreamError, "fail").WithProvider("openai")
	assert.Equal(t, "openai", err.Provider)
}

func TestIsRetryable(t *testing.T) {
	retryable := NewError(ErrRateLimit, "rate limited").WithRetryable(true)
	assert.True(t, IsRetryable(retryable))

	notRetryable := NewError(ErrInvalidRequest, "bad").WithRetryable(false)
	assert.False(t, IsRetryable(notRetryable))

	// Standard error
	assert.False(t, IsRetryable(errors.New("plain error")))

	// Wrapped typed error
	wrapped := fmt.Errorf("wrapped: %w", retryable)
	assert.True(t, IsRetryable(wrapped))
}

func TestGetErrorCode(t *testing.T) {
	err := NewError(ErrRateLimit, "rate limited")
	assert.Equal(t, ErrRateLimit, GetErrorCode(err))

	assert.Equal(t, ErrorCode(""), GetErrorCode(errors.New("plain")))

	wrapped := fmt.Errorf("wrapped: %w", err)
	assert.Equal(t, ErrRateLimit, GetErrorCode(wrapped))
}

func TestWrapError_NilError(t *testing.T) {
	result := WrapError(nil, ErrInternalError, "msg")
	assert.Nil(t, result)
}

func TestWrapError_AlreadyTyped(t *testing.T) {
	original := NewError(ErrRateLimit, "rate limited")
	result := WrapError(original, ErrInternalError, "should not change")
	assert.Equal(t, ErrRateLimit, result.Code)
	assert.Same(t, original, result)
}

func TestWrapError_StandardError(t *testing.T) {
	stdErr := errors.New("connection refused")
	result := WrapError(stdErr, ErrUpstreamError, "upstream failed")
	assert.Equal(t, ErrUpstreamError, result.Code)
	assert.Equal(t, "upstream failed", result.Message)
	assert.True(t, errors.Is(result, stdErr))
}

func TestWrapErrorf(t *testing.T) {
	stdErr := errors.New("timeout")
	result := WrapErrorf(stdErr, ErrTimeout, "request to %s timed out", "api.example.com")
	assert.Equal(t, ErrTimeout, result.Code)
	assert.Equal(t, "request to api.example.com timed out", result.Message)
}

func TestWrapErrorf_NilError(t *testing.T) {
	result := WrapErrorf(nil, ErrInternalError, "msg %s", "arg")
	assert.Nil(t, result)
}

func TestAsError(t *testing.T) {
	typed := NewError(ErrRateLimit, "rate limited")
	got, ok := AsError(typed)
	assert.True(t, ok)
	assert.Equal(t, typed, got)

	_, ok = AsError(errors.New("plain"))
	assert.False(t, ok)

	wrapped := fmt.Errorf("wrapped: %w", typed)
	got, ok = AsError(wrapped)
	assert.True(t, ok)
	assert.Equal(t, typed, got)
}

func TestIsErrorCode(t *testing.T) {
	err := NewError(ErrRateLimit, "rate limited")
	assert.True(t, IsErrorCode(err, ErrRateLimit))
	assert.False(t, IsErrorCode(err, ErrInternalError))
	assert.False(t, IsErrorCode(errors.New("plain"), ErrRateLimit))
}

// ============================================================
// Convenience error constructors
// ============================================================

func TestNewInvalidRequestError(t *testing.T) {
	err := NewInvalidRequestError("bad input")
	assert.Equal(t, ErrInvalidRequest, err.Code)
	assert.Equal(t, 400, err.HTTPStatus)
	assert.False(t, err.Retryable)
}

func TestNewAuthenticationError(t *testing.T) {
	err := NewAuthenticationError("invalid token")
	assert.Equal(t, ErrAuthentication, err.Code)
	assert.Equal(t, 401, err.HTTPStatus)
	assert.False(t, err.Retryable)
}

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("model not found")
	assert.Equal(t, ErrModelNotFound, err.Code)
	assert.Equal(t, 404, err.HTTPStatus)
	assert.False(t, err.Retryable)
}

func TestNewRateLimitError(t *testing.T) {
	err := NewRateLimitError("too many requests")
	assert.Equal(t, ErrRateLimit, err.Code)
	assert.Equal(t, 429, err.HTTPStatus)
	assert.True(t, err.Retryable)
}

func TestNewInternalError(t *testing.T) {
	err := NewInternalError("something broke")
	assert.Equal(t, ErrInternalError, err.Code)
	assert.Equal(t, 500, err.HTTPStatus)
	assert.False(t, err.Retryable)
}

func TestNewServiceUnavailableError(t *testing.T) {
	err := NewServiceUnavailableError("service down")
	assert.Equal(t, ErrServiceUnavailable, err.Code)
	assert.Equal(t, 503, err.HTTPStatus)
	assert.True(t, err.Retryable)
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("request timed out")
	assert.Equal(t, ErrTimeout, err.Code)
	assert.Equal(t, 504, err.HTTPStatus)
	assert.True(t, err.Retryable)
}

// ============================================================
// Context helpers — additional coverage
// ============================================================

func TestContextHelpers_EmptyValues(t *testing.T) {
	ctx := context.Background()

	_, ok := TraceID(ctx)
	assert.False(t, ok)

	_, ok = TenantID(ctx)
	assert.False(t, ok)

	_, ok = UserID(ctx)
	assert.False(t, ok)

	_, ok = RunID(ctx)
	assert.False(t, ok)

	_, ok = LLMModel(ctx)
	assert.False(t, ok)

	_, ok = PromptBundleVersion(ctx)
	assert.False(t, ok)

	_, ok = Roles(ctx)
	assert.False(t, ok)
}

func TestContextHelpers_EmptyString(t *testing.T) {
	ctx := WithTraceID(context.Background(), "")
	_, ok := TraceID(ctx)
	assert.False(t, ok, "empty string should return false")
}

func TestContextHelpers_Roles(t *testing.T) {
	ctx := WithRoles(context.Background(), []string{"admin", "user"})
	roles, ok := Roles(ctx)
	assert.True(t, ok)
	assert.Equal(t, []string{"admin", "user"}, roles)
}

func TestContextHelpers_EmptyRoles(t *testing.T) {
	ctx := WithRoles(context.Background(), []string{})
	_, ok := Roles(ctx)
	assert.False(t, ok, "empty roles should return false")
}

// ============================================================
// AgentConfig
// ============================================================

func TestAgentConfig_Validate_Valid(t *testing.T) {
	cfg := &AgentConfig{
		Core: CoreConfig{ID: "agent-1", Name: "Test Agent"},
		LLM:  LLMConfig{Model: "gpt-4"},
	}
	assert.NoError(t, cfg.Validate())
}

func TestAgentConfig_Validate_MissingID(t *testing.T) {
	cfg := &AgentConfig{
		Core: CoreConfig{Name: "Test"},
		LLM:  LLMConfig{Model: "gpt-4"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent ID is required")
}

func TestAgentConfig_Validate_MissingName(t *testing.T) {
	cfg := &AgentConfig{
		Core: CoreConfig{ID: "agent-1"},
		LLM:  LLMConfig{Model: "gpt-4"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent name is required")
}

func TestAgentConfig_Validate_MissingModel(t *testing.T) {
	cfg := &AgentConfig{
		Core: CoreConfig{ID: "agent-1", Name: "Test"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM model is required")
}

func TestAgentConfig_FeatureFlags(t *testing.T) {
	cfg := &AgentConfig{}

	assert.False(t, cfg.IsReflectionEnabled())
	assert.False(t, cfg.IsToolSelectionEnabled())
	assert.False(t, cfg.IsGuardrailsEnabled())
	assert.False(t, cfg.IsMemoryEnabled())
	assert.False(t, cfg.IsSkillsEnabled())
	assert.False(t, cfg.IsMCPEnabled())
	assert.False(t, cfg.IsObservabilityEnabled())

	cfg.Features.Reflection = &ReflectionConfig{Enabled: true}
	cfg.Features.ToolSelection = &ToolSelectionConfig{Enabled: true}
	cfg.Features.Guardrails = &GuardrailsConfig{Enabled: true}
	cfg.Features.Memory = &MemoryConfig{Enabled: true}
	cfg.Extensions.Skills = &SkillsConfig{Enabled: true}
	cfg.Extensions.MCP = &MCPConfig{Enabled: true}
	cfg.Extensions.Observability = &ObservabilityConfig{Enabled: true}

	assert.True(t, cfg.IsReflectionEnabled())
	assert.True(t, cfg.IsToolSelectionEnabled())
	assert.True(t, cfg.IsGuardrailsEnabled())
	assert.True(t, cfg.IsMemoryEnabled())
	assert.True(t, cfg.IsSkillsEnabled())
	assert.True(t, cfg.IsMCPEnabled())
	assert.True(t, cfg.IsObservabilityEnabled())
}

func TestAgentConfig_FeatureFlags_DisabledExplicitly(t *testing.T) {
	cfg := &AgentConfig{
		Features: FeaturesConfig{
			Reflection: &ReflectionConfig{Enabled: false},
		},
	}
	assert.False(t, cfg.IsReflectionEnabled())
}

// ============================================================
// Default config constructors
// ============================================================

func TestDefaultReflectionConfig(t *testing.T) {
	cfg := DefaultReflectionConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 3, cfg.MaxIterations)
	assert.InDelta(t, 0.7, cfg.MinQuality, 0.001)
}

func TestDefaultToolSelectionConfig(t *testing.T) {
	cfg := DefaultToolSelectionConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 5, cfg.MaxTools)
	assert.Equal(t, "hybrid", cfg.Strategy)
}

func TestDefaultPromptEnhancerConfig(t *testing.T) {
	cfg := DefaultPromptEnhancerConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "basic", cfg.Mode)
}

func TestDefaultGuardrailsConfig(t *testing.T) {
	cfg := DefaultGuardrailsConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 10000, cfg.MaxInputLength)
	assert.True(t, cfg.PIIDetection)
	assert.True(t, cfg.InjectionDetection)
}

func TestDefaultMemoryConfig(t *testing.T) {
	cfg := DefaultMemoryConfig()
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.EnableLongTerm)
	assert.True(t, cfg.EnableEpisodic)
}

func TestDefaultObservabilityConfig(t *testing.T) {
	cfg := DefaultObservabilityConfig()
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.MetricsEnabled)
	assert.True(t, cfg.TracingEnabled)
	assert.Equal(t, "info", cfg.LogLevel)
}

// ============================================================
// ExtensionRegistry
// ============================================================

func TestExtensionRegistry_AllFalseByDefault(t *testing.T) {
	reg := &ExtensionRegistry{}
	assert.False(t, reg.HasReflection())
	assert.False(t, reg.HasToolSelection())
	assert.False(t, reg.HasPromptEnhancer())
	assert.False(t, reg.HasSkills())
	assert.False(t, reg.HasMCP())
	assert.False(t, reg.HasEnhancedMemory())
	assert.False(t, reg.HasObservability())
	assert.False(t, reg.HasGuardrails())
}

// ============================================================
// JSONSchema
// ============================================================

func TestNewObjectSchema(t *testing.T) {
	s := NewObjectSchema()
	assert.Equal(t, SchemaTypeObject, s.Type)
	assert.NotNil(t, s.Properties)
}

func TestNewArraySchema(t *testing.T) {
	items := NewStringSchema()
	s := NewArraySchema(items)
	assert.Equal(t, SchemaTypeArray, s.Type)
	assert.Equal(t, items, s.Items)
}

func TestNewStringSchema(t *testing.T) {
	s := NewStringSchema()
	assert.Equal(t, SchemaTypeString, s.Type)
}

func TestNewNumberSchema(t *testing.T) {
	s := NewNumberSchema()
	assert.Equal(t, SchemaTypeNumber, s.Type)
}

func TestNewIntegerSchema(t *testing.T) {
	s := NewIntegerSchema()
	assert.Equal(t, SchemaTypeInteger, s.Type)
}

func TestNewBooleanSchema(t *testing.T) {
	s := NewBooleanSchema()
	assert.Equal(t, SchemaTypeBoolean, s.Type)
}

func TestNewEnumSchema(t *testing.T) {
	s := NewEnumSchema("a", "b", "c")
	assert.Len(t, s.Enum, 3)
}

func TestJSONSchema_AddProperty(t *testing.T) {
	s := NewObjectSchema()
	s.AddProperty("name", NewStringSchema())
	s.AddProperty("age", NewIntegerSchema())

	assert.Len(t, s.Properties, 2)
	assert.Equal(t, SchemaTypeString, s.Properties["name"].Type)
	assert.Equal(t, SchemaTypeInteger, s.Properties["age"].Type)
}

func TestJSONSchema_AddProperty_NilProperties(t *testing.T) {
	s := &JSONSchema{Type: SchemaTypeObject}
	s.AddProperty("name", NewStringSchema())
	assert.NotNil(t, s.Properties)
	assert.Len(t, s.Properties, 1)
}

func TestJSONSchema_AddRequired(t *testing.T) {
	s := NewObjectSchema().
		AddProperty("name", NewStringSchema()).
		AddRequired("name")
	assert.Equal(t, []string{"name"}, s.Required)
}

func TestJSONSchema_WithDescription(t *testing.T) {
	s := NewStringSchema().WithDescription("A name field")
	assert.Equal(t, "A name field", s.Description)
}

func TestJSONSchema_ToJSON(t *testing.T) {
	s := NewObjectSchema().
		AddProperty("name", NewStringSchema().WithDescription("User name")).
		AddRequired("name")

	data, err := s.ToJSON()
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "object", parsed["type"])
}

func TestFromJSON(t *testing.T) {
	jsonStr := `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`
	s, err := FromJSON([]byte(jsonStr))
	require.NoError(t, err)
	assert.Equal(t, SchemaTypeObject, s.Type)
	assert.Contains(t, s.Required, "name")
	assert.NotNil(t, s.Properties["name"])
}

func TestFromJSON_Invalid(t *testing.T) {
	_, err := FromJSON([]byte(`{invalid`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

// ============================================================
// EstimateTokenizer — additional coverage
// ============================================================

func TestEstimateTokenizer_ChineseText(t *testing.T) {
	tok := NewEstimateTokenizer()
	count := tok.CountTokens("你好世界")
	assert.Greater(t, count, 0)
}

func TestEstimateTokenizer_MixedText(t *testing.T) {
	tok := NewEstimateTokenizer()
	count := tok.CountTokens("Hello 你好")
	assert.Greater(t, count, 0)
}

func TestEstimateTokenizer_SingleChar(t *testing.T) {
	tok := NewEstimateTokenizer()
	count := tok.CountTokens("a")
	assert.Equal(t, 1, count)
}

func TestEstimateTokenizer_MessageWithName(t *testing.T) {
	tok := NewEstimateTokenizer()
	msg := Message{
		Role:    RoleUser,
		Content: "hello",
		Name:    "user1",
	}
	count := tok.CountMessageTokens(msg)
	assert.Greater(t, count, tok.CountTokens("hello"))
}

func TestEstimateTokenizer_EmptyMessages(t *testing.T) {
	tok := NewEstimateTokenizer()
	count := tok.CountMessagesTokens(nil)
	assert.Equal(t, 0, count)
}

func TestEstimateTokenizer_EmptyTools(t *testing.T) {
	tok := NewEstimateTokenizer()
	count := tok.EstimateToolTokens(nil)
	assert.Equal(t, 0, count)
}

