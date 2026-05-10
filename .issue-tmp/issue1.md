## 🔴 优先级：HIGH（潜在 goroutine 泄漏）

## 问题描述

架构 review 中发现，约 15 处生产代码在 `go func()` 内部使用了 `context.Background()` 而非传播父 context。当父 context 取消时，这些孤儿 goroutine 仍会继续执行，存在 goroutine 泄漏与资源浪费风险。

## 受影响位置

| 文件 | 行号 | 场景 |
|------|------|------|
| `agent/runtime/execution.go` | 142, 315, 507 | 父 context 取消后回退到 `context.Background()` |
| `agent/capabilities/tools/registry_health.go` | 92 | 健康检查未传播调用方 context |
| `agent/collaboration/federation/discovery_bridge.go` | 124, 135, 146 | 多次使用 `context.Background()` + WithTimeout，丢失父 context |
| `agent/execution/protocol/a2a/server_helper.go` | 295 | 服务端回调使用 Background |
| `agent/execution/protocol/a2a/server_handler.go` | 169 | 同上 |
| `agent/execution/protocol/mcp/sse_transport.go` | 60 | SSE 传输层使用 Background |

## 复现思路

写一个测试，启动一个会触发上述 goroutine 的执行流程，然后取消父 context，再用 `goleak.VerifyNone` 断言无残留 goroutine。当前会失败。

## 建议方案

1. **优先方案**：将父 context 传播到子 goroutine，使用 `context.WithCancel(parent)` 或 `context.WithTimeout(parent, ...)`
2. **回退方案**：若必须脱离父生命周期（如审计日志、清理任务），使用专门的 `lifecycleCtx` + 显式 `Stop()` 方法关闭
3. **测试**：扩展 `go.uber.org/goleak` 覆盖到至少 `agent/runtime`、`agent/capabilities/tools`、`agent/collaboration/federation`、`agent/execution/protocol/*` 包

## TDD 流程建议

1. **Red**：在 `agent/runtime/execution_test.go` 添加用例，启动执行 → 取消 ctx → goleak 检测应失败
2. **Green**：将 `context.Background()` 替换为传播的父 ctx，让测试通过
3. **Refactor**：抽取 helper（如 `derivedCtx(parent, timeout)`）统一所有 detached 场景

## 标签
`bug` `enhancement` `tech-debt`
