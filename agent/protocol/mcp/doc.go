// Package mcp 实现 Anthropic Model Context Protocol (MCP) 规范。
//
// 本包提供 MCP 服务端与客户端的完整实现，包括资源管理、
// 工具定义、Prompt 模板以及 stdio/WebSocket 双传输层，
// 支持心跳重连与指数退避。
package mcp
