# Context 取消检查最佳实践

## 检查结果

经过全面检查，项目中所有长时间运行的循环都已经正确处理了 context 取消和退出机制：

### ✅ 已正确处理的模块

1. **agent/collaboration/multi_agent.go:515**
   - `StartRetryLoop` 有 `case <-ctx.Done()`

2. **agent/async_execution.go:299**
   - `autoCleanupLoop` 有 `case <-m.closeCh`

3. **agent/lifecycle.go:151**
   - `healthCheckLoop` 有 `case <-ctx.Done()` 和 `case <-stop`

4. **agent/discovery/integration.go:305**
   - 有 `case <-i.done` 退出机制

5. **llm/providers/async_poller.go**
   - `Poll` 函数正确处理了 `case <-ctx.Done()`
   - 有 `MaxAttempts` 限制防止无限循环

### 检查模式

所有长时间运行的操作都应该遵循以下模式：

```go
// 模式 1: 使用 context
for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-ticker.C:
        // 执行任务
    }
}

// 模式 2: 使用 stop channel
for {
    select {
    case <-stopCh:
        return
    case <-ticker.C:
        // 执行任务
    }
}

// 模式 3: 使用 Poll 函数 (推荐)
result, err := providers.Poll(ctx, providers.PollConfig{
    Interval:    5 * time.Second,
    MaxAttempts: 100, // 0 表示无限，依赖 ctx 超时
}, func(ctx context.Context) providers.PollResult[T] {
    // 检查逻辑
})
```

## 建议

虽然项目已经做得很好，但仍需注意以下场景：

1. **HTTP 请求** - 确保所有 HTTP 客户端都使用 `ctx` 参数
2. **数据库查询** - 使用支持 context 的方法
3. **文件 I/O** - 长时间文件操作需要检查 context
4. **goroutine 泄漏** - 确保所有 goroutine 都有退出路径

## 工具函数

项目已提供通用轮询函数 `llm/providers/async_poller.go:Poll`，建议统一使用：
- 自动处理 context 取消
- 支持最大尝试次数限制
- 类型安全
- 避免重复代码
