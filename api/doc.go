// Package api provides OpenAPI/Swagger documentation for the AgentFlow API.
//
// This package contains the OpenAPI 3.0 specification and related documentation
// for the AgentFlow HTTP API.
//
// # API Overview
//
// AgentFlow provides a RESTful API for:
//   - Chat completions with multi-provider LLM routing
//   - Provider and model management
//   - A2A (Agent-to-Agent) protocol endpoints
//   - MCP (Model Context Protocol) tool invocation
//   - Health monitoring and metrics
//
// # Authentication
//
// Most API endpoints require authentication via the X-API-Key header:
//
//	X-API-Key: your-api-key
//
// # Base URL
//
// The default base URL for the API is:
//
//	http://localhost:8080
//
// # OpenAPI Specification
//
// The OpenAPI 3.0 specification is available at:
//   - api/openapi.yaml (static file)
//   - /swagger/doc.json (when swag is used)
//
// # Generating Documentation
//
// To regenerate Swagger documentation using swag:
//
//	make docs-swagger
//
// Or manually:
//
//	swag init -g cmd/agentflow/main.go -o api --parseDependency --parseInternal
//
// # Viewing Documentation
//
// To view the API documentation in Swagger UI:
//
//	make docs-serve
//
// This will start a Swagger UI server at http://localhost:8081
package api
