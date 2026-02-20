// 包 api 提供 AgentFlow HTTP API 的 OpenAPI/Swagger 文档说明。
//
// # API 概述
//
// AgentFlow 提供 RESTful API，主要覆盖：
//   - 健康检查与就绪检查
//   - 运行时配置管理（热重载、字段查询、变更历史）
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
package api
