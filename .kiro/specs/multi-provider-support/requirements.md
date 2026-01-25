# Requirements Document

## Introduction

本文档定义了扩展 AgentFlow 项目 LLM Provider 支持的需求。当前项目已实现 OpenAI、Claude 和 Gemini（配置存在但未实现）的 Provider。本需求旨在扩展支持五个主流 LLM 提供商：xAI Grok、Zhipu AI (GLM)、MiniMax、Alibaba Qwen 和 DeepSeek，以提供更广泛的模型选择和更好的中文支持。

## Glossary

- **Provider**: 实现 llm.Provider 接口的 LLM 服务提供商适配器
- **System**: AgentFlow LLM Provider 系统
- **Grok_Provider**: xAI Grok 的 Provider 实现
- **GLM_Provider**: Zhipu AI (GLM) 的 Provider 实现
- **MiniMax_Provider**: MiniMax 的 Provider 实现
- **Qwen_Provider**: Alibaba Qwen (通义千问) 的 Provider 实现
- **DeepSeek_Provider**: DeepSeek 的 Provider 实现
- **OpenAI_Compatible_Format**: 与 OpenAI API 兼容的请求/响应格式
- **Native_Function_Calling**: LLM 原生支持的工具调用（Function Calling）能力
- **Credential_Override**: 从 context 中获取的 API Key 覆盖配置中的默认值
- **RewriterChain**: 请求预处理中间件链，用于清理和转换请求
- **Health_Check**: 轻量级健康检查，用于路由探活和降级决策
- **Stream_Response**: 流式响应，通过 SSE (Server-Sent Events) 返回增量内容
- **Tool_Call**: LLM 返回的工具调用请求，包含工具名称和参数

## Requirements

### Requirement 1: xAI Grok Provider Implementation

**User Story:** 作为开发者，我希望系统支持 xAI Grok 模型，以便使用 Grok 4 系列的 256K context 能力。

#### Acceptance Criteria

1. THE System SHALL implement Grok_Provider that conforms to llm.Provider interface
2. WHEN Grok_Provider is initialized, THE System SHALL use https://api.x.ai/v1 as the default base URL
3. WHEN making API requests, THE Grok_Provider SHALL use Bearer Token authentication with the API key
4. WHEN converting requests, THE Grok_Provider SHALL use OpenAI_Compatible_Format for messages and tools
5. THE Grok_Provider SHALL support both streaming and non-streaming completion modes
6. THE Grok_Provider SHALL return true for SupportsNativeFunctionCalling method
7. WHEN no model is specified, THE Grok_Provider SHALL default to "grok-beta" model
8. WHEN Credential_Override is present in context, THE Grok_Provider SHALL use the overridden API key

### Requirement 2: Zhipu AI (GLM) Provider Implementation

**User Story:** 作为开发者，我希望系统支持 Zhipu AI GLM 模型，以便利用其中文优化能力和开源友好特性。

#### Acceptance Criteria

1. THE System SHALL implement GLM_Provider that conforms to llm.Provider interface
2. WHEN GLM_Provider is initialized, THE System SHALL use https://open.bigmodel.cn/api/paas/v4 as the default base URL
3. WHEN making API requests, THE GLM_Provider SHALL use Bearer Token authentication with the API key
4. THE GLM_Provider SHALL support both streaming and non-streaming completion modes
5. THE GLM_Provider SHALL return true for SupportsNativeFunctionCalling method
6. WHEN no model is specified, THE GLM_Provider SHALL default to "glm-4-plus" model
7. WHEN Credential_Override is present in context, THE GLM_Provider SHALL use the overridden API key
8. THE GLM_Provider SHALL correctly map GLM-specific error codes to llm.ErrorCode

### Requirement 3: MiniMax Provider Implementation

**User Story:** 作为开发者，我希望系统支持 MiniMax 模型，以便使用其多模态能力（文本、语音、图像、视频）。

#### Acceptance Criteria

1. THE System SHALL implement MiniMax_Provider that conforms to llm.Provider interface
2. WHEN MiniMax_Provider is initialized, THE System SHALL use https://api.minimax.chat/v1 as the default base URL
3. WHEN making API requests, THE MiniMax_Provider SHALL use Bearer Token authentication with the API key
4. THE MiniMax_Provider SHALL support both streaming and non-streaming completion modes
5. THE MiniMax_Provider SHALL return true for SupportsNativeFunctionCalling method
6. WHEN no model is specified, THE MiniMax_Provider SHALL default to "abab6.5s-chat" model
7. WHEN Credential_Override is present in context, THE MiniMax_Provider SHALL use the overridden API key
8. THE MiniMax_Provider SHALL correctly handle MiniMax-specific response format

### Requirement 4: Alibaba Qwen Provider Implementation

**User Story:** 作为开发者，我希望系统支持 Alibaba Qwen (通义千问) 模型，以便使用其 128K context 和中文优化能力。

#### Acceptance Criteria

1. THE System SHALL implement Qwen_Provider that conforms to llm.Provider interface
2. WHEN Qwen_Provider is initialized, THE System SHALL use https://dashscope.aliyuncs.com/compatible-mode/v1 as the default base URL
3. WHEN making API requests, THE Qwen_Provider SHALL use Bearer Token authentication with the API key
4. WHEN converting requests, THE Qwen_Provider SHALL use OpenAI_Compatible_Format for messages and tools
5. THE Qwen_Provider SHALL support both streaming and non-streaming completion modes
6. THE Qwen_Provider SHALL return true for SupportsNativeFunctionCalling method
7. WHEN no model is specified, THE Qwen_Provider SHALL default to "qwen-plus" model
8. WHEN Credential_Override is present in context, THE Qwen_Provider SHALL use the overridden API key

### Requirement 5: DeepSeek Provider Implementation

**User Story:** 作为开发者，我希望系统支持 DeepSeek 模型，以便使用其超高性价比和 thinking mode 能力。

#### Acceptance Criteria

1. THE System SHALL implement DeepSeek_Provider that conforms to llm.Provider interface
2. WHEN DeepSeek_Provider is initialized, THE System SHALL use https://api.deepseek.com as the default base URL
3. WHEN making API requests, THE DeepSeek_Provider SHALL use Bearer Token authentication with the API key
4. WHEN converting requests, THE DeepSeek_Provider SHALL use OpenAI_Compatible_Format for messages and tools
5. THE DeepSeek_Provider SHALL support both streaming and non-streaming completion modes
6. THE DeepSeek_Provider SHALL return true for SupportsNativeFunctionCalling method
7. WHEN no model is specified, THE DeepSeek_Provider SHALL default to "deepseek-chat" model
8. WHEN Credential_Override is present in context, THE DeepSeek_Provider SHALL use the overridden API key

### Requirement 6: Configuration Support

**User Story:** 作为系统管理员，我希望能够配置所有新增 Provider 的参数，以便灵活管理不同环境的设置。

#### Acceptance Criteria

1. THE System SHALL define GrokConfig struct with fields: APIKey, BaseURL, Model, Timeout
2. THE System SHALL define GLMConfig struct with fields: APIKey, BaseURL, Model, Timeout
3. THE System SHALL define MiniMaxConfig struct with fields: APIKey, BaseURL, Model, Timeout
4. THE System SHALL define QwenConfig struct with fields: APIKey, BaseURL, Model, Timeout
5. THE System SHALL define DeepSeekConfig struct with fields: APIKey, BaseURL, Model, Timeout
6. WHEN Timeout is not specified in configuration, THE System SHALL use 30 seconds as default timeout
7. THE System SHALL support loading configuration from JSON and YAML formats

### Requirement 7: Request Preprocessing

**User Story:** 作为开发者，我希望所有 Provider 使用统一的请求预处理逻辑，以便保持一致的行为。

#### Acceptance Criteria

1. WHEN a Provider receives a ChatRequest, THE System SHALL apply RewriterChain before processing
2. THE RewriterChain SHALL include EmptyToolsCleaner middleware
3. WHEN RewriterChain execution fails, THE System SHALL return llm.Error with code ErrInvalidRequest
4. THE System SHALL apply RewriterChain to both Completion and Stream methods

### Requirement 8: Health Check Implementation

**User Story:** 作为运维人员，我希望所有 Provider 实现健康检查，以便监控服务可用性和延迟。

#### Acceptance Criteria

1. WHEN HealthCheck is called, THE System SHALL send a lightweight request to the Provider's API endpoint
2. THE System SHALL measure the latency of the health check request
3. WHEN the health check succeeds (HTTP 200), THE System SHALL return HealthStatus with Healthy=true
4. WHEN the health check fails, THE System SHALL return HealthStatus with Healthy=false and the error
5. THE System SHALL use GET /v1/models endpoint for OpenAI-compatible Providers
6. THE System SHALL complete health checks within the configured timeout period

### Requirement 9: Error Handling and Mapping

**User Story:** 作为开发者，我希望所有 Provider 统一映射错误码，以便上层能够正确处理不同类型的错误。

#### Acceptance Criteria

1. WHEN HTTP status is 401, THE System SHALL map to llm.ErrUnauthorized
2. WHEN HTTP status is 403, THE System SHALL map to llm.ErrForbidden
3. WHEN HTTP status is 429, THE System SHALL map to llm.ErrRateLimited with Retryable=true
4. WHEN HTTP status is 400, THE System SHALL map to llm.ErrInvalidRequest
5. WHEN HTTP status is 503, 502, or 504, THE System SHALL map to llm.ErrUpstreamError with Retryable=true
6. WHEN HTTP status is 5xx, THE System SHALL map to llm.ErrUpstreamError with Retryable=true
7. WHEN error response contains quota or credit keywords, THE System SHALL map to llm.ErrQuotaExceeded
8. THE System SHALL include the Provider name in all error responses

### Requirement 10: Streaming Response Handling

**User Story:** 作为开发者，我希望所有 Provider 正确处理流式响应，以便支持实时交互场景。

#### Acceptance Criteria

1. WHEN Stream method is called, THE System SHALL set stream=true in the API request
2. THE System SHALL parse SSE (Server-Sent Events) format responses
3. WHEN receiving data lines, THE System SHALL parse JSON and emit StreamChunk
4. WHEN receiving [DONE] marker, THE System SHALL close the stream channel
5. WHEN stream parsing fails, THE System SHALL emit StreamChunk with error
6. THE System SHALL accumulate tool call arguments across multiple chunks for Providers that send partial JSON
7. WHEN stream completes, THE System SHALL close the response body and channel

### Requirement 11: Tool Calling Support

**User Story:** 作为开发者，我希望所有新增 Provider 支持原生工具调用，以便 Agent 能够使用工具增强能力。

#### Acceptance Criteria

1. WHEN ChatRequest contains Tools, THE System SHALL convert them to Provider-specific format
2. WHEN ChatRequest contains ToolChoice, THE System SHALL include it in the API request
3. WHEN LLM response contains tool calls, THE System SHALL parse them into llm.ToolCall format
4. THE System SHALL preserve tool call ID, name, and arguments in the conversion
5. WHEN handling tool results, THE System SHALL convert llm.RoleTool messages to Provider-specific format
6. THE System SHALL support tool calling in both streaming and non-streaming modes

### Requirement 12: Message Format Conversion

**User Story:** 作为开发者，我希望系统正确转换消息格式，以便不同 Provider 能够理解统一的 llm.Message 格式。

#### Acceptance Criteria

1. WHEN converting messages, THE System SHALL map llm.RoleSystem to Provider-specific system message format
2. WHEN converting messages, THE System SHALL map llm.RoleUser to Provider-specific user message format
3. WHEN converting messages, THE System SHALL map llm.RoleAssistant to Provider-specific assistant message format
4. WHEN converting messages, THE System SHALL map llm.RoleTool to Provider-specific tool result format
5. WHEN a message contains ToolCalls, THE System SHALL convert them to Provider-specific tool call format
6. WHEN a message contains ToolCallID, THE System SHALL include it in the Provider-specific format
7. THE System SHALL preserve message content, name, and metadata during conversion

### Requirement 13: Response Format Conversion

**User Story:** 作为开发者，我希望系统将 Provider 响应转换为统一格式，以便上层代码能够统一处理不同 Provider 的响应。

#### Acceptance Criteria

1. WHEN receiving Provider response, THE System SHALL extract response ID and map to ChatResponse.ID
2. WHEN receiving Provider response, THE System SHALL extract model name and map to ChatResponse.Model
3. WHEN receiving Provider response, THE System SHALL set ChatResponse.Provider to the Provider name
4. WHEN receiving Provider response, THE System SHALL convert choices to llm.ChatChoice format
5. WHEN receiving usage information, THE System SHALL map to llm.ChatUsage format
6. WHEN receiving timestamp, THE System SHALL convert to ChatResponse.CreatedAt
7. THE System SHALL preserve finish reason in the conversion

### Requirement 14: Default Model Selection

**User Story:** 作为开发者，我希望每个 Provider 有合理的默认模型，以便在未指定模型时能够正常工作。

#### Acceptance Criteria

1. WHEN ChatRequest.Model is specified, THE System SHALL use the specified model
2. WHEN ChatRequest.Model is empty and config has Model, THE System SHALL use the configured model
3. WHEN both ChatRequest.Model and config Model are empty, THE System SHALL use Provider-specific default model
4. THE Grok_Provider default model SHALL be "grok-beta"
5. THE GLM_Provider default model SHALL be "glm-4-plus"
6. THE MiniMax_Provider default model SHALL be "abab6.5s-chat"
7. THE Qwen_Provider default model SHALL be "qwen-plus"
8. THE DeepSeek_Provider default model SHALL be "deepseek-chat"

### Requirement 15: HTTP Client Configuration

**User Story:** 作为开发者，我希望所有 Provider 使用合理的 HTTP 客户端配置，以便处理网络超时和连接问题。

#### Acceptance Criteria

1. WHEN creating a Provider, THE System SHALL create an HTTP client with configured timeout
2. WHEN timeout is not configured, THE System SHALL use 30 seconds as default
3. THE System SHALL set appropriate HTTP headers for each Provider
4. THE System SHALL set Content-Type to application/json for all requests
5. THE System SHALL set Accept header to application/json for non-streaming requests
6. THE System SHALL properly close HTTP response bodies after reading

### Requirement 16: Context Propagation

**User Story:** 作为开发者，我希望 Provider 正确传播 context，以便支持请求取消和超时控制。

#### Acceptance Criteria

1. WHEN creating HTTP requests, THE System SHALL use context.Context from the method parameter
2. WHEN context is cancelled, THE System SHALL abort the HTTP request
3. WHEN context has deadline, THE System SHALL respect the deadline
4. THE System SHALL propagate context to all HTTP requests including health checks
