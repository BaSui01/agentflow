# AgentFlow 最佳实践指南

本文档提供使用 AgentFlow 框架的最佳实践建议。

## 目录

- [Agent 设计](#agent-设计)
- [性能优化](#性能优化)
- [错误处理](#错误处理)
- [安全最佳实践](#安全最佳实践)
- [测试策略](#测试策略)

---

## Agent 设计

### 1. 单一职责原则

每个 Agent 应该专注于一个特定任务：

```go
// ✅ 好的设计：专注于翻译
translatorAgent := agent.NewBaseAgent(agent.Config{
    Name: "translator",
    Type: agent.TypeTranslator,
    // ...
})

// ❌ 避免：一个 Agent 做太多事情
superAgent := agent.NewBaseAgent(agent.Config{
    Name: "super-agent",
    // 同时处理翻译、分析、总结...
})
```

### 2. 使用合适的提示词

```go
config := agent.Config{
    PromptBundle: agent.PromptBundle{
        SystemPrompt: `你是一个专业的翻译助手。
规则：
1. 保持原文的语气和风格
2. 专业术语使用标准翻译
3. 如果不确定，保留原文并标注`,
    },
}
```

### 3. 合理配置工具

只注册 Agent 需要的工具：

```go
config := agent.Config{
    Tools: []string{
        "search",      // 只注册需要的工具
        "calculator",
    },
}
```

---

## 性能优化

### 1. 使用缓存

```go
// LLM 响应缓存
cacheConfig := cache.Config{
    L1Size:     1000,           // 本地缓存大小
    L2Enabled:  true,           // 启用 Redis 缓存
    TTL:        time.Hour,      // 缓存过期时间
}

provider := cache.NewCachedProvider(baseProvider, cacheConfig)
```

### 2. 批量处理

```go
// 批量嵌入
embeddings, err := embedder.EmbedBatch(ctx, []string{
    "document 1",
    "document 2",
    "document 3",
})
```

### 3. 并发控制

```go
// 使用信号量限制并发
sem := make(chan struct{}, 10) // 最多 10 个并发

for _, task := range tasks {
    sem <- struct{}{}
    go func(t Task) {
        defer func() { <-sem }()
        process(t)
    }(task)
}
```

### 4. 合理的 Token 限制

```go
config := agent.Config{
    MaxTokens:   2048,  // 根据任务复杂度设置
    Temperature: 0.7,   // 创意任务用高温度，精确任务用低温度
}
```

---

## 错误处理

### 1. 使用重试机制

```go
retryConfig := retry.Config{
    MaxRetries:  3,
    InitialWait: time.Second,
    MaxWait:     time.Minute,
    Multiplier:  2.0,
}

provider := retry.NewRetryProvider(baseProvider, retryConfig)
```

### 2. 熔断器保护

```go
cbConfig := circuitbreaker.Config{
    MaxFailures:   5,
    ResetTimeout:  time.Minute,
    HalfOpenMax:   3,
}

provider := circuitbreaker.NewCircuitBreakerProvider(baseProvider, cbConfig)
```

### 3. 优雅降级

```go
func executeWithFallback(ctx context.Context, input *Input) (*Output, error) {
    output, err := primaryAgent.Execute(ctx, input)
    if err != nil {
        // 降级到备用 Agent
        return fallbackAgent.Execute(ctx, input)
    }
    return output, nil
}
```

### 4. 详细的错误日志

```go
if err != nil {
    logger.Error("agent execution failed",
        zap.String("agent_id", agent.ID()),
        zap.String("trace_id", input.TraceID),
        zap.Error(err),
        zap.Duration("duration", time.Since(start)),
    )
}
```

---

## 安全最佳实践

### 1. 使用 Guardrails

```go
config := agent.Config{
    Guardrails: &guardrails.GuardrailsConfig{
        MaxInputLength:     10000,
        BlockedKeywords:    []string{"password", "secret"},
        PIIDetectionEnabled: true,
        InjectionDetection:  true,
    },
}
```

### 2. 输入验证

```go
// 自定义验证器
validator := guardrails.NewCustomValidator(func(ctx context.Context, content string) (*guardrails.ValidationResult, error) {
    if containsSensitiveData(content) {
        return &guardrails.ValidationResult{
            Valid: false,
            Errors: []guardrails.ValidationError{
                {Code: "SENSITIVE_DATA", Message: "输入包含敏感数据"},
            },
        }, nil
    }
    return &guardrails.ValidationResult{Valid: true}, nil
})
```

### 3. 输出过滤

```go
// 过滤敏感信息
filter := guardrails.NewRegexFilter(&guardrails.RegexFilterConfig{
    Patterns: []string{
        `\b\d{3}-\d{2}-\d{4}\b`,  // SSN
        `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,  // Email
    },
    Replacement: "[REDACTED]",
})
```

### 4. API Key 管理

```go
// 使用 API Key 池
pool := llm.NewAPIKeyPool([]string{
    os.Getenv("OPENAI_API_KEY_1"),
    os.Getenv("OPENAI_API_KEY_2"),
})

provider := openai.NewProvider(openai.Config{
    APIKeyPool: pool,
})
```

---

## 测试策略

### 1. 单元测试

```go
func TestAgent_Execute(t *testing.T) {
    // 使用 mock provider
    mockProvider := &MockProvider{
        Response: &llm.ChatResponse{
            Message: llm.Message{Content: "test response"},
        },
    }

    agent := NewBaseAgent(config, mockProvider, nil, nil, nil, zap.NewNop())

    output, err := agent.Execute(ctx, &Input{Content: "test"})

    assert.NoError(t, err)
    assert.Equal(t, "test response", output.Content)
}
```

### 2. 集成测试

```go
func TestAgent_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // 使用真实 provider
    provider := openai.NewProvider(openai.Config{
        APIKey: os.Getenv("OPENAI_API_KEY"),
    })

    agent := NewBaseAgent(config, provider, nil, nil, nil, zap.NewNop())

    output, err := agent.Execute(ctx, &Input{Content: "Hello"})

    assert.NoError(t, err)
    assert.NotEmpty(t, output.Content)
}
```

### 3. 基准测试

```go
func BenchmarkAgent_Execute(b *testing.B) {
    agent := setupAgent()
    input := &Input{Content: "test"}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        agent.Execute(context.Background(), input)
    }
}
```

---

## 监控和可观测性

### 1. 指标收集

```go
// 启用 Prometheus 指标
observability := observability.NewSystem(&observability.Config{
    MetricsEnabled: true,
    MetricsPort:    9090,
})

agent.EnableObservability(observability)
```

### 2. 分布式追踪

```go
// 启用 OpenTelemetry 追踪
tracer := otel.Tracer("agentflow")

ctx, span := tracer.Start(ctx, "agent.execute")
defer span.End()

output, err := agent.Execute(ctx, input)
```

### 3. 结构化日志

```go
logger := zap.NewProduction()

logger.Info("agent execution completed",
    zap.String("agent_id", agent.ID()),
    zap.String("trace_id", input.TraceID),
    zap.Int("tokens_used", output.TokensUsed),
    zap.Duration("duration", output.Duration),
)
```

---

## 总结

遵循这些最佳实践可以帮助你：

1. **设计更好的 Agent** - 单一职责、合理配置
2. **提升性能** - 缓存、批处理、并发控制
3. **增强可靠性** - 重试、熔断、降级
4. **保障安全** - Guardrails、输入验证、输出过滤
5. **便于维护** - 测试、监控、日志

更多信息请参考：
- [API 参考](../api/README.md)
- [教程](../tutorials/)
- [故障排查](./troubleshooting.md)

---

## 成本控制最佳实践

### Token 计数器选择

| 场景 | 推荐 | 原因 |
|------|------|------|
| OpenAI 模型 | `TiktokenTokenizer` | 精确计数，支持 o200k_base/cl100k_base 编码 |
| 非 OpenAI 模型 | `EstimatorTokenizer` | 无需下载模型数据，CJK 字符优化 |
| 混合场景 | `GetTokenizerOrEstimator` | 自动回退，优先精确计数 |

### 预算管理建议

```go
// 1. 启动时注册所有 OpenAI tokenizer
tokenizer.RegisterOpenAITokenizers()

// 2. 请求前预估 Token 消耗
tok := tokenizer.GetTokenizerOrEstimator(model)
estimated, _ := tok.CountMessages(messages)
if estimated > budgetLimit {
    // 裁剪消息或拒绝请求
}

// 3. 请求后记录实际消耗
resp, _ := provider.Completion(ctx, req)
actualTokens := resp.Usage.TotalTokens
```

### 双模型架构降本

- 工具调用密集型任务：设置 `toolProvider` 为便宜模型（如 GPT-4o-mini），主 Provider 为贵模型
- 预期成本降低 40-60%（工具调用通常占总 Token 的 60-80%）

---

## Config Hot Reload 回滚最佳实践

### 基本配置

```go
import "github.com/BaSui01/agentflow/config"

manager := config.NewHotReloadManager(cfg,
    config.WithConfigPath("/etc/agentflow/config.yaml"),
    config.WithHotReloadLogger(logger),
    config.WithMaxHistorySize(20),       // 保留 20 个历史版本
    config.WithValidateFunc(func(newConfig *config.Config) error {
        // 自定义验证逻辑
        if newConfig.Agent.MaxTokens > 100000 {
            return fmt.Errorf("max_tokens too large")
        }
        return nil
    }),
)

manager.Start(ctx)
defer manager.Stop()
```

### 回滚策略

```go
// 手动回滚到上一个版本
manager.Rollback()

// 回滚到指定版本
manager.RollbackToVersion(3)

// 查看配置历史
history := manager.GetConfigHistory()
for _, snapshot := range history {
    fmt.Printf("版本 %d: %s (来源: %s)\n",
        snapshot.Version, snapshot.Timestamp, snapshot.Source)
}
```

### 监控与告警

```go
// 监听配置变更
manager.OnChange(func(change config.ConfigChange) {
    if change.RequiresRestart {
        alertOps("配置变更需要重启: " + change.Path)
    }
})

// 监听回滚事件
manager.OnRollback(func(event config.RollbackEvent) {
    alertOps(fmt.Sprintf("配置回滚! 原因: %s, 恢复到版本: %d",
        event.Reason, event.Version))
})
```

### 生产环境注意事项

1. **热重载字段**：Log.Level、Agent.MaxIterations、Agent.Temperature 等可热重载
2. **需重启字段**：Server.HTTPPort、Database.Host、Redis.Addr 等需要重启
3. **敏感字段**：Password、APIKey 等字段在变更日志中自动脱敏
4. **自动回滚**：回调函数执行失败时自动回滚到上一个有效配置
5. **环形缓冲**：历史记录使用环形缓冲，默认保留最近 10 个版本

---

## API 中间件安全配置

`cmd/agentflow/middleware.go` 提供生产级 API 安全中间件。

### 推荐中间件链

```go
import "go.uber.org/zap"

logger, _ := zap.NewProduction()

handler := Chain(apiHandler,
    Recovery(logger),                    // 1. Panic 恢复（最外层）
    RequestLogger(logger),               // 2. 请求日志
    CORS([]string{"https://app.example.com"}), // 3. CORS
    RateLimiter(100, 200, logger),       // 4. 限流（100 RPS，突发 200）
    APIKeyAuth(                          // 5. 认证（最内层）
        []string{"key-1", "key-2"},
        []string{"/health", "/metrics"}, // 跳过认证的路径
        logger,
    ),
)
```

### API Key 认证

- 支持 `X-API-Key` Header 和 `api_key` Query Parameter
- 可配置跳过路径（健康检查、指标端点等）

### 限流配置建议

| 场景 | RPS | Burst | 说明 |
|------|-----|-------|------|
| 开发环境 | 1000 | 2000 | 宽松限制 |
| 生产 API | 100 | 200 | 标准限制 |
| 公开端点 | 10 | 20 | 严格限制 |

### CORS 配置

```go
// 允许特定域名
CORS([]string{"https://app.example.com", "https://admin.example.com"})

// 允许所有域名（仅开发环境）
CORS(nil)
```

> 限流器基于 IP 地址，自动清理 3 分钟内无活动的访客记录。
