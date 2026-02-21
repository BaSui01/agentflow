// 包 api 提供 AgentFlow HTTP API 的 OpenAPI/Swagger 文档说明。
//
// # API 概述
//
// AgentFlow 提供 RESTful API，主要覆盖：
//   - 健康检查与就绪检查
//   - 运行时配置管理（热重载、字段查询、变更历史）
//   - 聊天完成（同步与流式）
//   - LLM 提供商与模型管理
//   - 智能路由与提供商选择
//   - 工具调用与函数执行
//   - A2A（Agent-to-Agent）协议交互
//
// # 身份验证
//
// 大多数 API 端点需要通过 `X-API-Key` 请求头进行认证：
//
//	X-API-Key: your-api-key
//
// # 基础地址
//
// API 默认基地址为：
//
//	http://localhost:8080
//
// # OpenAPI 规范
//
// OpenAPI 3.0 规范可通过以下位置获取：
//   - api/openapi.yaml（静态文件）
//   - /swagger/doc.json（使用 swag 时）
//
// # 生成文档
//
// 使用 swag 重新生成 Swagger 文档：
//
//	make docs-swagger
//
// 或手动执行：
//
//	swag init -g cmd/agentflow/main.go -o api --parseDependency --parseInternal
//
// # 查看文档
//
// 在 Swagger UI 中查看 API 文档：
//
//	make docs-serve
//
// 将在 http://localhost:8081 启动 Swagger UI 服务
//
// # 聊天完成类型
//
// 该包定义了聊天完成的请求/响应 DTO：
//
//   - [ChatRequest]：聊天完成请求，支持 Model、Messages、Tools、
//     Temperature、MaxTokens 等参数，以及 TraceID / TenantID 多租户字段
//   - [ChatResponse]：聊天完成响应，包含 Choices 列表和 Usage 统计
//   - [ChatChoice]：单个响应选择，含 FinishReason 和 Message
//   - [ChatUsage]：Token 使用统计（PromptTokens / CompletionTokens / TotalTokens）
//   - [StreamChunk]：流式响应块，用于 SSE 推送，含 Delta 增量内容
//
// # 消息与工具类型
//
//   - [Message]：对话消息，支持 system / user / assistant / tool 四种角色，
//     支持 ToolCalls、多模态 Images 和自定义 Metadata
//   - [ToolCall]：工具调用（类型别名，规范定义在 types.ToolCall）
//   - [ImageContent]：多模态图像内容，支持 URL 和 Base64 两种格式
//   - [ToolSchema]：工具定义，含 Name、Description 和 Parameters JSON Schema
//   - [ToolResult]：工具执行结果，含 Result JSON 和执行耗时
//   - [ToolInvokeRequest]：工具调用请求
//
// # 提供商与模型类型
//
//   - [LLMProvider]：LLM 提供商（如 OpenAI、Anthropic），含状态管理
//   - [LLMModel]：LLM 模型定义，含 ModelName 和 Enabled 状态
//   - [LLMProviderModel]：提供商-模型映射，含 RemoteModelName、BaseURL、
//     定价（PriceInput / PriceCompletion）、MaxTokens 和路由优先级
//   - [ProviderHealthResponse]：提供商健康检查结果（HTTP API DTO，
//     Latency 为 string 格式；框架内部请使用 llm.HealthStatus）
//
// # 路由类型
//
//   - [RoutingRequest]：路由请求，指定 Model 和 Strategy（cost / health /
//     qps / canary / tag）
//   - [ProviderSelection]：路由结果，含选中的 ProviderID、ModelID、
//     IsCanary 标志和使用的 Strategy
//
// # A2A 协议类型
//
//   - [AgentCard]：A2A 代理卡，描述代理的能力、工具和 Schema
//   - [Capability]：代理能力定义（task / query / stream）
//   - [ToolDefinition]：代理可用工具定义
//   - [A2ARequest]：A2A 调用请求，含 Task 描述和 Stream 标志
//   - [A2AResponse]：A2A 调用响应，含 Status 和 Result
//
// # 错误与列表响应类型
//
//   - [ErrorResponse]：统一错误响应包装
//   - [ErrorDetail]：错误详情，含 Code、Message、HTTPStatus 和 Retryable 标志
//   - [ProviderListResponse]：提供商列表响应
//   - [ModelListResponse]：模型列表响应
//   - [ToolListResponse]：工具列表响应
package api
