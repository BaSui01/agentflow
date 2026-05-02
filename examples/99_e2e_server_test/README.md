# 99_e2e_server_test

端到端 HTTP 服务测试，验证 API 路由和 Handler 的完整链路。

**主入口**: `main.go` — package main
**核心测试函数**: `testHealthEndpoint`

## 用途
模拟真实 HTTP 请求验证服务端到端流程，包含健康检查、请求路由和响应格式。
