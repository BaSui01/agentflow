# Implementation Plan: Multi-Provider Support

## Overview

This implementation plan adds support for five new LLM providers (xAI Grok, Zhipu AI GLM, MiniMax, Alibaba Qwen, and DeepSeek) to the AgentFlow system. The implementation follows an incremental approach, starting with configuration, then implementing OpenAI-compatible providers (which can share code), followed by providers with custom formats, and finally comprehensive testing.

## Tasks

- [x] 1. Add configuration structs for all new providers
  - Add GrokConfig, GLMConfig, MiniMaxConfig, QwenConfig, and DeepSeekConfig to providers/config.go
  - Each config should have APIKey, BaseURL, Model, and Timeout fields
  - Follow the existing pattern from OpenAIConfig and ClaudeConfig
  - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

- [-] 2. Implement xAI Grok Provider (OpenAI-compatible)
  - [x] 2.1 Create providers/grok/provider.go with GrokProvider struct
    - Implement NewGrokProvider constructor with default BaseURL "https://api.x.ai"
    - Initialize HTTP client with timeout (default 30s)
    - Initialize RewriterChain with EmptyToolsCleaner
    - _Requirements: 1.1, 1.2, 6.6_
  
  - [x] 2.2 Implement core Provider interface methods
    - Implement Name() returning "grok"
    - Implement SupportsNativeFunctionCalling() returning true
    - Implement HealthCheck() using GET /v1/models endpoint
    - _Requirements: 1.1, 1.6, 8.1, 8.2, 8.5_
  
  - [x] 2.3 Implement Completion() method
    - Apply RewriterChain to request
    - Handle credential override from context
    - Convert llm.ChatRequest to OpenAI format (reuse existing conversion functions)
    - Build HTTP request with Bearer Token authentication
    - Send POST to /v1/chat/completions
    - Parse response and convert to llm.ChatResponse
    - Map errors using mapError function
    - _Requirements: 1.3, 1.4, 1.5, 1.7, 1.8, 7.1, 7.3, 9.1-9.8, 15.3_
  
  - [x] 2.4 Implement Stream() method
    - Apply RewriterChain to request
    - Handle credential override from context
    - Convert llm.ChatRequest to OpenAI format with stream=true
    - Build HTTP request with Bearer Token authentication
    - Parse SSE response and emit StreamChunk messages
    - Handle [DONE] marker and errors
    - _Requirements: 1.5, 10.1, 10.2, 10.3, 10.4, 10.5_
  
  - [-] 2.5 Write property test for default BaseURL configuration
    - **Property 1: Default BaseURL Configuration**
    - **Validates: Requirements 1.2**
  
  - [ ] 2.6 Write property test for Bearer Token authentication
    - **Property 2: Bearer Token Authentication**
    - **Validates: Requirements 1.3**
  
  - [ ] 2.7 Write property test for model selection priority
    - **Property 5: Default Model Selection Priority**
    - **Validates: Requirements 1.7, 14.1, 14.2, 14.3**

- [ ] 3. Implement Alibaba Qwen Provider (OpenAI-compatible)
  - [ ] 3.1 Create providers/qwen/provider.go with QwenProvider struct
    - Implement NewQwenProvider constructor with default BaseURL "https://dashscope.aliyuncs.com/compatible-mode/v1"
    - Initialize HTTP client and RewriterChain
    - _Requirements: 4.1, 4.2_
  
  - [ ] 3.2 Implement Provider interface methods
    - Implement Name() returning "qwen"
    - Implement SupportsNativeFunctionCalling() returning true
    - Implement HealthCheck(), Completion(), and Stream() following Grok pattern
    - Use default model "qwen-plus"
    - _Requirements: 4.1, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_
  
  - [ ] 3.3 Write property test for OpenAI format conversion
    - **Property 3: OpenAI Format Conversion for Compatible Providers**
    - **Validates: Requirements 4.4**

- [ ] 4. Implement DeepSeek Provider (OpenAI-compatible)
  - [ ] 4.1 Create providers/deepseek/provider.go with DeepSeekProvider struct
    - Implement NewDeepSeekProvider constructor with default BaseURL "https://api.deepseek.com"
    - Initialize HTTP client and RewriterChain
    - _Requirements: 5.1, 5.2_
  
  - [ ] 4.2 Implement Provider interface methods
    - Implement Name() returning "deepseek"
    - Implement SupportsNativeFunctionCalling() returning true
    - Implement HealthCheck(), Completion(), and Stream() following Grok pattern
    - Use default model "deepseek-chat"
    - _Requirements: 5.1, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8_
  
  - [ ] 4.3 Write property test for credential override
    - **Property 6: Credential Override from Context**
    - **Validates: Requirements 5.8**

- [ ] 5. Checkpoint - Verify OpenAI-compatible providers work
  - Ensure all tests pass for Grok, Qwen, and DeepSeek providers
  - Ask the user if questions arise

- [ ] 6. Implement Zhipu AI GLM Provider
  - [ ] 6.1 Create providers/glm/provider.go with GLMProvider struct
    - Implement NewGLMProvider constructor with default BaseURL "https://open.bigmodel.cn/api/paas/v4"
    - Initialize HTTP client and RewriterChain
    - _Requirements: 2.1, 2.2_
  
  - [ ] 6.2 Implement Provider interface methods
    - Implement Name() returning "glm"
    - Implement SupportsNativeFunctionCalling() returning true
    - Implement HealthCheck() (verify endpoint during implementation)
    - Use default model "glm-4-plus"
    - _Requirements: 2.1, 2.5, 2.6_
  
  - [ ] 6.3 Implement Completion() and Stream() methods
    - Start with OpenAI-compatible format assumption
    - Apply RewriterChain and handle credential override
    - Implement GLM-specific error code mapping if needed
    - _Requirements: 2.3, 2.4, 2.7, 2.8_
  
  - [ ] 6.4 Write property test for GLM error mapping
    - **Property 12: HTTP Status to Error Code Mapping**
    - **Validates: Requirements 2.8, 9.1-9.8**

- [ ] 7. Implement MiniMax Provider (Custom Format)
  - [ ] 7.1 Create providers/minimax/provider.go with MiniMaxProvider struct
    - Implement NewMiniMaxProvider constructor with default BaseURL "https://api.minimax.chat/v1"
    - Initialize HTTP client and RewriterChain
    - Define miniMaxMessage, miniMaxTool, miniMaxRequest, miniMaxResponse types
    - _Requirements: 3.1, 3.2_
  
  - [ ] 7.2 Implement message and tool conversion functions
    - Implement convertToMiniMaxMessages() to convert llm.Message to miniMaxMessage
    - Implement convertToMiniMaxTools() to convert llm.ToolSchema to miniMaxTool
    - Handle tool calls in XML format: <tool_calls>JSON</tool_calls>
    - _Requirements: 11.1, 11.5, 12.1-12.7_
  
  - [ ] 7.3 Implement tool call parsing function
    - Implement parseMiniMaxToolCalls() to extract tool calls from XML tags
    - Parse JSON lines within <tool_calls> tags
    - Generate unique tool call IDs
    - _Requirements: 11.3, 11.4_
  
  - [ ] 7.4 Implement Provider interface methods
    - Implement Name() returning "minimax"
    - Implement SupportsNativeFunctionCalling() returning true
    - Implement HealthCheck() (verify endpoint during implementation)
    - Use default model "abab6.5s-chat"
    - _Requirements: 3.1, 3.5, 3.6_
  
  - [ ] 7.5 Implement Completion() method
    - Apply RewriterChain and handle credential override
    - Convert request using MiniMax-specific functions
    - Parse response and extract tool calls from XML
    - Convert to llm.ChatResponse
    - _Requirements: 3.3, 3.4, 3.7_
  
  - [ ] 7.6 Implement Stream() method
    - Apply RewriterChain and handle credential override
    - Convert request with stream=true
    - Parse SSE response
    - Extract tool calls from XML in streaming chunks
    - _Requirements: 3.4, 10.1, 10.2, 10.3_
  
  - [ ] 7.7 Write property test for MiniMax tool call parsing
    - **Property 19: Tool Call Response Parsing**
    - **Validates: Requirements 3.8, 11.3**
  
  - [ ] 7.8 Write unit test for XML tool call format
    - Test parseMiniMaxToolCalls() with various XML inputs
    - Test single and multiple tool calls
    - Test malformed XML handling
    - _Requirements: 11.3_

- [ ] 8. Checkpoint - Verify all providers are implemented
  - Ensure all five providers compile and basic tests pass
  - Ask the user if questions arise

- [ ] 9. Implement shared utility functions
  - [ ] 9.1 Create or update error mapping function
    - Ensure mapError() handles all HTTP status codes correctly
    - Add quota/credit detection for 400 errors
    - Include provider name in all errors
    - _Requirements: 9.1-9.8_
  
  - [ ] 9.2 Create model selection helper function
    - Implement chooseModel(requestModel, configModel, defaultModel) function
    - Test priority: request > config > default
    - _Requirements: 14.1, 14.2, 14.3_
  
  - [ ] 9.3 Write property test for error mapping
    - **Property 12: HTTP Status to Error Code Mapping**
    - **Validates: Requirements 9.1-9.8**

- [ ] 10. Implement RewriterChain integration
  - [ ] 10.1 Write property test for RewriterChain application
    - **Property 8: RewriterChain Application**
    - **Validates: Requirements 7.1, 7.4**
  
  - [ ] 10.2 Write property test for RewriterChain error handling
    - **Property 9: RewriterChain Error Handling**
    - **Validates: Requirements 7.3**

- [ ] 11. Implement tool calling support tests
  - [ ] 11.1 Write property test for tool schema conversion
    - **Property 17: Tool Schema Conversion**
    - **Validates: Requirements 11.1**
  
  - [ ] 11.2 Write property test for tool choice preservation
    - **Property 18: Tool Choice Preservation**
    - **Validates: Requirements 11.2**
  
  - [ ] 11.3 Write property test for tool result conversion
    - **Property 20: Tool Result Message Conversion**
    - **Validates: Requirements 11.5**
  
  - [ ] 11.4 Write property test for tool calling in both modes
    - **Property 21: Tool Calling in Both Modes**
    - **Validates: Requirements 11.6**

- [ ] 12. Implement streaming support tests
  - [ ] 12.1 Write property test for streaming request format
    - **Property 13: Streaming Request Format**
    - **Validates: Requirements 10.1**
  
  - [ ] 12.2 Write property test for SSE response parsing
    - **Property 14: SSE Response Parsing**
    - **Validates: Requirements 10.2, 10.3**
  
  - [ ] 12.3 Write property test for stream error handling
    - **Property 15: Stream Error Handling**
    - **Validates: Requirements 10.5**
  
  - [ ] 12.4 Write property test for tool call accumulation
    - **Property 16: Tool Call Accumulation in Streaming**
    - **Validates: Requirements 10.6**
  
  - [ ] 12.5 Write unit test for [DONE] marker handling
    - Test that stream closes on [DONE]
    - Test that channel is properly closed
    - _Requirements: 10.4_

- [ ] 13. Implement message and response conversion tests
  - [ ] 13.1 Write property test for message role conversion
    - **Property 22: Message Role Conversion**
    - **Validates: Requirements 12.1, 12.2, 12.3, 12.4**
  
  - [ ] 13.2 Write property test for message content preservation
    - **Property 23: Message Content Preservation**
    - **Validates: Requirements 12.5, 12.6, 12.7**
  
  - [ ] 13.3 Write property test for response field extraction
    - **Property 24: Response Field Extraction**
    - **Validates: Requirements 13.1-13.7**

- [ ] 14. Implement HTTP client and context tests
  - [ ] 14.1 Write property test for default timeout configuration
    - **Property 7: Default Timeout Configuration**
    - **Validates: Requirements 6.6, 15.1**
  
  - [ ] 14.2 Write property test for HTTP headers configuration
    - **Property 25: HTTP Headers Configuration**
    - **Validates: Requirements 15.3, 15.4, 15.5**
  
  - [ ] 14.3 Write property test for context propagation
    - **Property 26: Context Propagation**
    - **Validates: Requirements 16.1, 16.4**
  
  - [ ] 14.4 Write property test for context cancellation
    - **Property 27: Context Cancellation Handling**
    - **Validates: Requirements 16.2, 16.3**

- [ ] 15. Implement health check tests
  - [ ] 15.1 Write property test for health check request execution
    - **Property 10: Health Check Request Execution**
    - **Validates: Requirements 8.1, 8.5**
  
  - [ ] 15.2 Write property test for health check latency measurement
    - **Property 11: Health Check Latency Measurement**
    - **Validates: Requirements 8.2**
  
  - [ ] 15.3 Write unit test for health check success case
    - Test that HTTP 200 returns Healthy=true
    - _Requirements: 8.3_
  
  - [ ] 15.4 Write unit test for health check failure case
    - Test that HTTP errors return Healthy=false
    - _Requirements: 8.4_

- [ ] 16. Implement dual completion mode tests
  - [ ] 16.1 Write property test for dual completion mode support
    - **Property 4: Dual Completion Mode Support**
    - **Validates: Requirements 1.5, 2.4, 3.4, 4.5, 5.5**

- [ ] 17. Add integration tests (optional)
  - [ ] 17.1 Create integration test for Grok provider
    - Test end-to-end completion with real API (requires API key)
    - Test streaming response
    - Test tool calling flow
    - _Requirements: 1.1-1.8_
  
  - [ ] 17.2 Create integration test for Qwen provider
    - Test end-to-end completion with real API (requires API key)
    - _Requirements: 4.1-4.8_
  
  - [ ] 17.3 Create integration test for DeepSeek provider
    - Test end-to-end completion with real API (requires API key)
    - _Requirements: 5.1-5.8_
  
  - [ ] 17.4 Create integration test for GLM provider
    - Test end-to-end completion with real API (requires API key)
    - _Requirements: 2.1-2.8_
  
  - [ ] 17.5 Create integration test for MiniMax provider
    - Test end-to-end completion with real API (requires API key)
    - Test XML tool call format
    - _Requirements: 3.1-3.8_

- [ ] 18. Final checkpoint - Comprehensive testing
  - Run all unit tests and property tests
  - Verify all 27 correctness properties are tested
  - Ensure test coverage is adequate
  - Ask the user if questions arise

- [ ] 19. Documentation and examples
  - [ ] 19.1 Add usage examples for each provider
    - Create example code showing how to initialize each provider
    - Show basic completion and streaming examples
    - Show tool calling examples
  
  - [ ] 19.2 Update README or documentation
    - Document the five new providers
    - List supported models for each provider
    - Document configuration options
    - Include API key setup instructions

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation at key milestones
- Property tests validate universal correctness properties with minimum 100 iterations
- Unit tests validate specific examples and edge cases
- Integration tests are optional and require valid API keys
- OpenAI-compatible providers (Grok, Qwen, DeepSeek) can share significant code
- MiniMax requires custom XML parsing for tool calls
- GLM format should be verified during implementation (assumed OpenAI-compatible)
