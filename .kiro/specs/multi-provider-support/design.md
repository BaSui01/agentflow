# Design Document: Multi-Provider Support

## Overview

This design extends the AgentFlow LLM Provider system to support five additional mainstream LLM providers: xAI Grok, Zhipu AI (GLM), MiniMax, Alibaba Qwen, and DeepSeek. The implementation follows the existing architecture pattern established by OpenAI and Claude providers, ensuring consistency in error handling, streaming, tool calling, and request preprocessing.

### Key Design Decisions

1. **OpenAI-Compatible Providers**: Grok, Qwen, and DeepSeek use OpenAI-compatible API formats, allowing significant code reuse
2. **Unified Error Mapping**: All providers map HTTP status codes to standardized llm.ErrorCode values
3. **Consistent Middleware**: All providers use RewriterChain for request preprocessing
4. **Native Tool Calling**: All five providers support native function calling capabilities
5. **Bearer Token Authentication**: All providers use Bearer Token authentication (consistent with OpenAI pattern)

### Research Findings

Based on official documentation review:

- **xAI Grok** ([docs.x.ai](https://docs.x.ai)): Fully OpenAI-compatible API at https://api.x.ai, supports function calling with OpenAI format
- **DeepSeek** ([api-docs.deepseek.com](https://api-docs.deepseek.com)): OpenAI-compatible API, supports function calling with strict mode (beta)
- **Alibaba Qwen** ([alibabacloud.com](https://www.alibabacloud.com/help/en/model-studio)): OpenAI-compatible via DashScope at https://dashscope.aliyuncs.com/compatible-mode/v1
- **Zhipu AI GLM** ([docs.z.ai](https://docs.z.ai)): Uses Bearer Token authentication, API format needs verification during implementation
- **MiniMax** ([platform.minimax.io](https://platform.minimax.io)): Supports function calling with custom format, needs specific handling

## Architecture

### Component Structure

```
providers/
├── config.go                    # Configuration structs for all providers
├── grok/
│   └── provider.go             # xAI Grok implementation
├── glm/
│   └── provider.go             # Zhipu AI GLM implementation
├── minimax/
│   └── provider.go             # MiniMax implementation
├── qwen/
│   └── provider.go             # Alibaba Qwen implementation
└── deepseek/
    └── provider.go             # DeepSeek implementation
```

### Provider Interface Implementation

Each provider implements the `llm.Provider` interface:

```go
type Provider interface {
    Name() string
    HealthCheck(ctx context.Context) (*HealthStatus, error)
    Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
    SupportsNativeFunctionCalling() bool
}
```

### Shared Patterns

All providers follow these patterns established by OpenAI and Claude implementations:

1. **Configuration**: Each provider has a dedicated config struct with APIKey, BaseURL, Model, and Timeout fields
2. **HTTP Client**: Each provider creates an http.Client with configured timeout (default 30s)
3. **Credential Override**: Support for runtime API key override via context
4. **Request Preprocessing**: Apply RewriterChain (with EmptyToolsCleaner) before processing
5. **Error Mapping**: Map HTTP status codes to llm.ErrorCode with appropriate retry flags
6. **Streaming**: Parse SSE format responses and emit StreamChunk messages

## Components and Interfaces

### 1. Configuration Structs (providers/config.go)

Add five new configuration structs following the existing pattern:

```go
// GrokConfig xAI Grok Provider configuration
type GrokConfig struct {
    APIKey  string        `json:"api_key" yaml:"api_key"`
    BaseURL string        `json:"base_url" yaml:"base_url"`
    Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
    Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// GLMConfig Zhipu AI GLM Provider configuration
type GLMConfig struct {
    APIKey  string        `json:"api_key" yaml:"api_key"`
    BaseURL string        `json:"base_url" yaml:"base_url"`
    Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
    Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// MiniMaxConfig MiniMax Provider configuration
type MiniMaxConfig struct {
    APIKey  string        `json:"api_key" yaml:"api_key"`
    BaseURL string        `json:"base_url" yaml:"base_url"`
    Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
    Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// QwenConfig Alibaba Qwen Provider configuration
type QwenConfig struct {
    APIKey  string        `json:"api_key" yaml:"api_key"`
    BaseURL string        `json:"base_url" yaml:"base_url"`
    Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
    Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DeepSeekConfig DeepSeek Provider configuration
type DeepSeekConfig struct {
    APIKey  string        `json:"api_key" yaml:"api_key"`
    BaseURL string        `json:"base_url" yaml:"base_url"`
    Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
    Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}
```

### 2. OpenAI-Compatible Providers (Grok, Qwen, DeepSeek)

These three providers can share significant code with the OpenAI implementation since they use OpenAI-compatible APIs.

#### Provider Structure

```go
type GrokProvider struct {
    cfg           providers.GrokConfig
    client        *http.Client
    logger        *zap.Logger
    rewriterChain *middleware.RewriterChain
}

func NewGrokProvider(cfg providers.GrokConfig, logger *zap.Logger) *GrokProvider {
    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 30 * time.Second
    }
    
    // Set default BaseURL if not provided
    if cfg.BaseURL == "" {
        cfg.BaseURL = "https://api.x.ai"
    }
    
    return &GrokProvider{
        cfg: cfg,
        client: &http.Client{Timeout: timeout},
        logger: logger,
        rewriterChain: middleware.NewRewriterChain(
            middleware.NewEmptyToolsCleaner(),
        ),
    }
}
```

#### Request/Response Types

Reuse OpenAI types (openAIMessage, openAIToolCall, openAIRequest, openAIResponse) since the API format is identical.

#### Key Methods

**Name()**: Return provider identifier ("grok", "qwen", "deepseek")

**SupportsNativeFunctionCalling()**: Return true

**HealthCheck()**: 
- Send GET request to `/v1/models` endpoint
- Measure latency
- Return HealthStatus with Healthy=true on 200, false otherwise

**Completion()**:
1. Apply RewriterChain to request
2. Extract API key (config or context override)
3. Convert llm.ChatRequest to openAIRequest format
4. Build HTTP request with Bearer Token authentication
5. Send POST to `/v1/chat/completions`
6. Parse response and convert to llm.ChatResponse
7. Map errors to llm.Error with appropriate codes

**Stream()**:
1. Apply RewriterChain to request
2. Extract API key (config or context override)
3. Convert llm.ChatRequest to openAIRequest with stream=true
4. Build HTTP request with Bearer Token authentication
5. Send POST to `/v1/chat/completions`
6. Parse SSE response line by line
7. Emit StreamChunk for each data event
8. Close channel on [DONE] or error

#### Default Models

- **Grok**: "grok-beta"
- **Qwen**: "qwen-plus"
- **DeepSeek**: "deepseek-chat"

### 3. GLM Provider (Zhipu AI)

GLM uses Bearer Token authentication and likely follows OpenAI-compatible format based on research.

#### Provider Structure

```go
type GLMProvider struct {
    cfg           providers.GLMConfig
    client        *http.Client
    logger        *zap.Logger
    rewriterChain *middleware.RewriterChain
}

func NewGLMProvider(cfg providers.GLMConfig, logger *zap.Logger) *GLMProvider {
    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 30 * time.Second
    }
    
    if cfg.BaseURL == "" {
        cfg.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
    }
    
    return &GLMProvider{
        cfg: cfg,
        client: &http.Client{Timeout: timeout},
        logger: logger,
        rewriterChain: middleware.NewRewriterChain(
            middleware.NewEmptyToolsCleaner(),
        ),
    }
}
```

#### Implementation Strategy

1. Start with OpenAI-compatible format assumption
2. Use Bearer Token authentication
3. Endpoint: `/chat/completions` (verify during implementation)
4. Default model: "glm-4-plus"
5. Adjust format if needed based on actual API responses

### 4. MiniMax Provider

MiniMax has a custom function calling format that requires specific handling.

#### Provider Structure

```go
type MiniMaxProvider struct {
    cfg           providers.MiniMaxConfig
    client        *http.Client
    logger        *zap.Logger
    rewriterChain *middleware.RewriterChain
}

func NewMiniMaxProvider(cfg providers.MiniMaxConfig, logger *zap.Logger) *MiniMaxProvider {
    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 30 * time.Second
    }
    
    if cfg.BaseURL == "" {
        cfg.BaseURL = "https://api.minimax.chat/v1"
    }
    
    return &MiniMaxProvider{
        cfg: cfg,
        client: &http.Client{Timeout: timeout},
        logger: logger,
        rewriterChain: middleware.NewRewriterChain(
            middleware.NewEmptyToolsCleaner(),
        ),
    }
}
```

#### Request/Response Types

Based on MiniMax documentation, tool calls are returned in `<tool_calls>` XML tags with JSON content:

```go
type miniMaxMessage struct {
    Role    string `json:"role"`
    Content string `json:"content,omitempty"`
    Name    string `json:"name,omitempty"`
}

type miniMaxTool struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    Parameters  json.RawMessage `json:"parameters"`
}

type miniMaxRequest struct {
    Model       string           `json:"model"`
    Messages    []miniMaxMessage `json:"messages"`
    Tools       []miniMaxTool    `json:"tools,omitempty"`
    MaxTokens   int              `json:"max_tokens,omitempty"`
    Temperature float32          `json:"temperature,omitempty"`
    Stream      bool             `json:"stream,omitempty"`
}

type miniMaxResponse struct {
    ID      string `json:"id"`
    Model   string `json:"model"`
    Choices []struct {
        Index        int              `json:"index"`
        FinishReason string           `json:"finish_reason"`
        Message      miniMaxMessage   `json:"message"`
    } `json:"choices"`
    Usage *struct {
        PromptTokens     int `json:"prompt_tokens"`
        CompletionTokens int `json:"completion_tokens"`
        TotalTokens      int `json:"total_tokens"`
    } `json:"usage,omitempty"`
}
```

#### Tool Call Parsing

MiniMax returns tool calls in the message content as XML:

```
<tool_calls>
{"name": "get_weather", "arguments": {"location": "Shanghai"}}
</tool_calls>
```

Parse this format:

```go
func parseMiniMaxToolCalls(content string) []llm.ToolCall {
    // Extract content between <tool_calls> tags
    pattern := regexp.MustCompile(`<tool_calls>(.*?)</tool_calls>`)
    matches := pattern.FindStringSubmatch(content)
    if len(matches) < 2 {
        return nil
    }
    
    toolCallsContent := strings.TrimSpace(matches[1])
    var toolCalls []llm.ToolCall
    
    // Parse each line as JSON
    for _, line := range strings.Split(toolCallsContent, "\n") {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        
        var call struct {
            Name      string          `json:"name"`
            Arguments json.RawMessage `json:"arguments"`
        }
        
        if err := json.Unmarshal([]byte(line), &call); err != nil {
            continue
        }
        
        toolCalls = append(toolCalls, llm.ToolCall{
            ID:        generateToolCallID(), // Generate unique ID
            Name:      call.Name,
            Arguments: call.Arguments,
        })
    }
    
    return toolCalls
}
```

#### Default Model

"abab6.5s-chat"

## Data Models

### Error Mapping

All providers use consistent error mapping:

```go
func mapError(status int, msg string, provider string) *llm.Error {
    switch status {
    case http.StatusUnauthorized:
        return &llm.Error{
            Code:       llm.ErrUnauthorized,
            Message:    msg,
            HTTPStatus: status,
            Provider:   provider,
        }
    case http.StatusForbidden:
        return &llm.Error{
            Code:       llm.ErrForbidden,
            Message:    msg,
            HTTPStatus: status,
            Provider:   provider,
        }
    case http.StatusTooManyRequests:
        return &llm.Error{
            Code:       llm.ErrRateLimited,
            Message:    msg,
            HTTPStatus: status,
            Retryable:  true,
            Provider:   provider,
        }
    case http.StatusBadRequest:
        // Check for quota/credit keywords
        if strings.Contains(strings.ToLower(msg), "quota") ||
           strings.Contains(strings.ToLower(msg), "credit") {
            return &llm.Error{
                Code:       llm.ErrQuotaExceeded,
                Message:    msg,
                HTTPStatus: status,
                Provider:   provider,
            }
        }
        return &llm.Error{
            Code:       llm.ErrInvalidRequest,
            Message:    msg,
            HTTPStatus: status,
            Provider:   provider,
        }
    case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
        return &llm.Error{
            Code:       llm.ErrUpstreamError,
            Message:    msg,
            HTTPStatus: status,
            Retryable:  true,
            Provider:   provider,
        }
    case 529: // Model overloaded (Claude-specific but can apply to others)
        return &llm.Error{
            Code:       llm.ErrModelOverloaded,
            Message:    msg,
            HTTPStatus: status,
            Retryable:  true,
            Provider:   provider,
        }
    default:
        return &llm.Error{
            Code:       llm.ErrUpstreamError,
            Message:    msg,
            HTTPStatus: status,
            Retryable:  status >= 500,
            Provider:   provider,
        }
    }
}
```

### Message Conversion

For OpenAI-compatible providers, reuse existing conversion functions:

```go
func convertMessages(msgs []llm.Message) []openAIMessage {
    // Reuse from OpenAI provider
}

func convertTools(tools []llm.ToolSchema) []openAITool {
    // Reuse from OpenAI provider
}
```

For MiniMax, implement custom conversion:

```go
func convertToMiniMaxMessages(msgs []llm.Message) []miniMaxMessage {
    out := make([]miniMaxMessage, 0, len(msgs))
    for _, m := range msgs {
        mm := miniMaxMessage{
            Role:    string(m.Role),
            Content: m.Content,
            Name:    m.Name,
        }
        
        // If message has tool calls, format them as XML
        if len(m.ToolCalls) > 0 {
            toolCallsXML := "<tool_calls>\n"
            for _, tc := range m.ToolCalls {
                callJSON, _ := json.Marshal(map[string]interface{}{
                    "name":      tc.Name,
                    "arguments": json.RawMessage(tc.Arguments),
                })
                toolCallsXML += string(callJSON) + "\n"
            }
            toolCallsXML += "</tool_calls>"
            mm.Content = toolCallsXML
        }
        
        out = append(out, mm)
    }
    return out
}

func convertToMiniMaxTools(tools []llm.ToolSchema) []miniMaxTool {
    if len(tools) == 0 {
        return nil
    }
    out := make([]miniMaxTool, 0, len(tools))
    for _, t := range tools {
        out = append(out, miniMaxTool{
            Name:        t.Name,
            Description: t.Description,
            Parameters:  t.Parameters,
        })
    }
    return out
}
```

### Response Conversion

Convert provider-specific responses to llm.ChatResponse:

```go
func toChatResponse(providerResp interface{}, provider string) *llm.ChatResponse {
    // Extract common fields: ID, Model, Choices, Usage
    // Convert to llm.ChatResponse format
    // Set Provider field
}
```

## Correctness Properties

A property is a characteristic or behavior that should hold true across all valid executions of a system—essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.

### Property 1: Default BaseURL Configuration

*For any* provider (Grok, GLM, MiniMax, Qwen, DeepSeek), when initialized with an empty BaseURL in configuration, the provider should use its documented default BaseURL.

**Validates: Requirements 1.2, 2.2, 3.2, 4.2, 5.2**

### Property 2: Bearer Token Authentication

*For any* provider and any API request (Completion, Stream, HealthCheck), the HTTP request should include an Authorization header with "Bearer <api_key>" format.

**Validates: Requirements 1.3, 2.3, 3.3, 4.3, 5.3**

### Property 3: OpenAI Format Conversion for Compatible Providers

*For any* llm.ChatRequest with messages and tools, when converted by OpenAI-compatible providers (Grok, Qwen, DeepSeek), the resulting request format should match OpenAI API specification.

**Validates: Requirements 1.4, 4.4, 5.4**

### Property 4: Dual Completion Mode Support

*For any* provider and any valid ChatRequest, both Completion() and Stream() methods should successfully process the request without errors (given valid credentials and network).

**Validates: Requirements 1.5, 2.4, 3.4, 4.5, 5.5**

### Property 5: Default Model Selection Priority

*For any* provider, when selecting a model, the priority should be: (1) ChatRequest.Model if specified, (2) Config.Model if specified, (3) Provider-specific default model.

**Validates: Requirements 1.7, 2.6, 3.6, 4.7, 5.7, 14.1, 14.2, 14.3**

### Property 6: Credential Override from Context

*For any* provider, when a ChatRequest context contains a Credential_Override with a non-empty API key, the HTTP request should use the overridden key instead of the configured key.

**Validates: Requirements 1.8, 2.7, 3.7, 4.8, 5.8**

### Property 7: Default Timeout Configuration

*For any* provider configuration with Timeout set to zero or unspecified, the provider should initialize with a 30-second HTTP client timeout.

**Validates: Requirements 6.6, 15.1, 15.2**

### Property 8: RewriterChain Application

*For any* provider, when Completion() or Stream() is called with a ChatRequest containing empty tools array, the RewriterChain should remove the empty tools before sending the API request.

**Validates: Requirements 7.1, 7.4**

### Property 9: RewriterChain Error Handling

*For any* provider, when RewriterChain execution fails, the provider should return an llm.Error with Code=ErrInvalidRequest and HTTPStatus=400.

**Validates: Requirements 7.3**

### Property 10: Health Check Request Execution

*For any* OpenAI-compatible provider (Grok, Qwen, DeepSeek), when HealthCheck() is called, the provider should send a GET request to the "/v1/models" endpoint.

**Validates: Requirements 8.1, 8.5**

### Property 11: Health Check Latency Measurement

*For any* provider, when HealthCheck() completes (success or failure), the returned HealthStatus should contain a non-zero Latency value representing the request duration.

**Validates: Requirements 8.2**

### Property 12: HTTP Status to Error Code Mapping

*For any* provider and any HTTP error response, the error mapping should follow these rules:
- 401 → ErrUnauthorized
- 403 → ErrForbidden
- 429 → ErrRateLimited (Retryable=true)
- 400 → ErrInvalidRequest (or ErrQuotaExceeded if message contains "quota"/"credit")
- 503/502/504 → ErrUpstreamError (Retryable=true)
- 5xx → ErrUpstreamError (Retryable=true)

**Validates: Requirements 9.1, 9.2, 9.3, 9.4, 9.5, 9.6, 9.7, 9.8**

### Property 13: Streaming Request Format

*For any* provider, when Stream() is called with a ChatRequest, the HTTP request body should include "stream": true field.

**Validates: Requirements 10.1**

### Property 14: SSE Response Parsing

*For any* provider streaming response, when receiving SSE data lines starting with "data: ", the provider should parse the JSON content and emit corresponding StreamChunk messages.

**Validates: Requirements 10.2, 10.3**

### Property 15: Stream Error Handling

*For any* provider streaming response, when encountering invalid JSON in SSE data, the provider should emit a StreamChunk with a non-nil Err field containing an llm.Error.

**Validates: Requirements 10.5**

### Property 16: Tool Call Accumulation in Streaming

*For any* provider that sends partial tool call JSON across multiple stream chunks, the accumulated tool call arguments should form valid JSON when all chunks are combined.

**Validates: Requirements 10.6**

### Property 17: Tool Schema Conversion

*For any* provider and any ChatRequest with non-empty Tools array, the provider should convert each llm.ToolSchema to the provider-specific tool format preserving name, description, and parameters.

**Validates: Requirements 11.1**

### Property 18: Tool Choice Preservation

*For any* provider and any ChatRequest with non-empty ToolChoice string, the provider should include the tool_choice field in the API request with the same value.

**Validates: Requirements 11.2**

### Property 19: Tool Call Response Parsing

*For any* provider response containing tool calls, the parsed llm.ChatResponse should contain llm.ToolCall objects with valid ID, Name, and Arguments fields.

**Validates: Requirements 11.3, 11.4**

### Property 20: Tool Result Message Conversion

*For any* provider and any llm.Message with Role=RoleTool, the provider should convert it to the provider-specific tool result format including the ToolCallID reference.

**Validates: Requirements 11.5**

### Property 21: Tool Calling in Both Modes

*For any* provider and any ChatRequest with Tools, both Completion() and Stream() should successfully handle tool calls and return/emit tool call information.

**Validates: Requirements 11.6**

### Property 22: Message Role Conversion

*For any* provider and any llm.Message array, the provider should correctly map each llm.Role (System, User, Assistant, Tool) to the provider-specific role format.

**Validates: Requirements 12.1, 12.2, 12.3, 12.4**

### Property 23: Message Content Preservation

*For any* provider and any llm.Message with Content, Name, ToolCalls, or ToolCallID fields, the provider should preserve all non-empty fields during conversion to provider format.

**Validates: Requirements 12.5, 12.6, 12.7**

### Property 24: Response Field Extraction

*For any* provider response, the converted llm.ChatResponse should contain the response ID, model name, provider name, choices array, and usage information (if present in provider response).

**Validates: Requirements 13.1, 13.2, 13.3, 13.4, 13.5, 13.6, 13.7**

### Property 25: HTTP Headers Configuration

*For any* provider and any HTTP request (Completion, Stream, HealthCheck), the request should include Content-Type: application/json header and Authorization header with Bearer token.

**Validates: Requirements 15.3, 15.4, 15.5**

### Property 26: Context Propagation

*For any* provider and any method call (Completion, Stream, HealthCheck), the HTTP request should be created with the context.Context parameter from the method call.

**Validates: Requirements 16.1, 16.4**

### Property 27: Context Cancellation Handling

*For any* provider, when the context passed to Completion() or Stream() is already cancelled, the method should return an error without making an HTTP request.

**Validates: Requirements 16.2, 16.3**

## Error Handling

### Error Response Parsing

All providers must parse error responses from the API and extract meaningful error messages:

```go
func readErrMsg(body io.Reader) string {
    data, _ := io.ReadAll(body)
    
    // Try to parse as JSON error response
    var errResp struct {
        Error struct {
            Message string `json:"message"`
            Type    string `json:"type"`
            Code    any    `json:"code"`
        } `json:"error"`
    }
    
    if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
        return errResp.Error.Message
    }
    
    // Fallback to raw body
    return string(data)
}
```

### Retry Logic

Errors with `Retryable=true` should be handled by upper layers with exponential backoff:
- Rate limit errors (429)
- Upstream errors (5xx)
- Model overloaded errors (529)

### Error Context

All errors should include:
- Provider name
- HTTP status code
- Original error message
- Appropriate error code from llm.ErrorCode

## Testing Strategy

### Unit Testing

Unit tests should focus on:

1. **Configuration Initialization**
   - Test default BaseURL setting for each provider
   - Test default timeout setting
   - Test custom configuration values

2. **Message Conversion**
   - Test conversion of different message roles
   - Test tool call formatting
   - Test tool result formatting
   - Test MiniMax XML tool call parsing

3. **Error Mapping**
   - Test each HTTP status code maps to correct llm.ErrorCode
   - Test quota detection in error messages
   - Test error message extraction

4. **Model Selection**
   - Test priority: request model > config model > default model
   - Test each provider's default model

5. **Authentication**
   - Test Bearer Token header formatting
   - Test credential override from context

6. **Request Preprocessing**
   - Test RewriterChain removes empty tools
   - Test RewriterChain error handling

### Property-Based Testing

Property tests should be configured with minimum 100 iterations and tagged with feature name and property number.

Example test structure:

```go
// Feature: multi-provider-support, Property 5: Default Model Selection Priority
func TestProperty5_DefaultModelSelectionPriority(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // Generate random combinations of request model, config model
        requestModel := rapid.StringMatching(`^[a-z0-9-]*$`).Draw(t, "requestModel")
        configModel := rapid.StringMatching(`^[a-z0-9-]*$`).Draw(t, "configModel")
        
        // Test model selection logic
        selectedModel := chooseModel(requestModel, configModel, "default-model")
        
        // Verify priority
        if requestModel != "" {
            assert.Equal(t, requestModel, selectedModel)
        } else if configModel != "" {
            assert.Equal(t, configModel, selectedModel)
        } else {
            assert.Equal(t, "default-model", selectedModel)
        }
    })
}
```

Key property tests:

1. **Property 5**: Model selection priority across all providers
2. **Property 6**: Credential override behavior
3. **Property 8**: RewriterChain removes empty tools
4. **Property 12**: Error code mapping for all status codes
5. **Property 17**: Tool schema conversion preserves fields
6. **Property 19**: Tool call parsing extracts all fields
7. **Property 22**: Message role conversion correctness
8. **Property 23**: Message content preservation
9. **Property 24**: Response field extraction completeness

### Integration Testing

Integration tests should verify:

1. **End-to-End Completion**
   - Test actual API calls with valid credentials (optional, requires API keys)
   - Test streaming responses
   - Test tool calling flows

2. **Health Checks**
   - Test health check against live endpoints (optional)
   - Test health check timeout handling

3. **Error Scenarios**
   - Test with invalid API keys (401)
   - Test with rate limiting (429)
   - Test with network errors

### Test Organization

```
providers/
├── grok/
│   ├── provider.go
│   ├── provider_test.go          # Unit tests
│   └── provider_property_test.go # Property tests
├── glm/
│   ├── provider.go
│   ├── provider_test.go
│   └── provider_property_test.go
├── minimax/
│   ├── provider.go
│   ├── provider_test.go
│   └── provider_property_test.go
├── qwen/
│   ├── provider.go
│   ├── provider_test.go
│   └── provider_property_test.go
└── deepseek/
    ├── provider.go
    ├── provider_test.go
    └── provider_property_test.go
```

### Testing Best Practices

1. **Mock HTTP Responses**: Use httptest.Server for unit tests to avoid real API calls
2. **Test Error Paths**: Ensure all error conditions are tested
3. **Test Edge Cases**: Empty strings, nil values, malformed JSON
4. **Property Test Coverage**: Focus on conversion logic and business rules
5. **Integration Test Isolation**: Use separate API keys for testing, implement cleanup

