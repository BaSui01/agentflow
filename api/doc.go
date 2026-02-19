// 包 api 为 AgentFlow API 提供 OpenAPI/Swagger 文档。
//
// 该软件包包含OpenAPI 3.0规范和相关文档
// 对于 AgentFlow HTTP API。
//
// # API 概述
//
// AgentFlow 提供 RESTful API 用于：
//   - 健康检查与就绪检查
//   - 运行时配置管理（热重载、字段查询、变更历史）
//
// # 身份验证
//
// 大多数 API 端点需要通过 X-API-Key 标头进行身份验证：
//
//	X-API-Key: your-api-key
//
// # 基本网址
//
// API 的默认基本 URL 为：
//
//	http://localhost:8080
//
// # OpenAPI 规范
//
// OpenAPI 3.0 规范可在以下位置获取：
//   - api/openapi.yaml（静态文件）
//   - /swagger/doc.json（使用 swag 时）
//
// # 生成文档
//
// 要使用 swag 重新生成 Swagger 文档：
//
//	make docs-swagger
//
// 或者手动：
//
//	swag init -g cmd/agentflow/main.go -o api --parseDependency --parseInternal
//
// # 查看文档
//
// 要在 Swagger UI 中查看 API 文档：
//
//	make docs-serve
//
// 这将在 http://localhost:8081 启动 Swagger UI 服务器
package api
