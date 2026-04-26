# Logging Guidelines

> AgentFlow 项目的日志规范和最佳实践。

---

## Overview

AgentFlow 使用 **Uber Zap** (`go.uber.org/zap`) 作为结构化日志库。Zap 提供高性能的结构化日志，支持 JSON 和 Console 两种输出格式。

**核心原则**:
- 使用结构化字段而非字符串拼接
- 根据环境选择适当的日志格式（开发环境用 Console，生产环境用 JSON）
- 敏感信息（API Key、密码等）不得记录到日志
- 使用适当的日志级别

---

## Log Levels

| Level | 使用场景 | 示例 |
|-------|----------|------|
| **Debug** | 调试信息，仅在开发环境启用 | 详细的函数调用参数、中间状态 |
| **Info** | 正常业务流程记录 | 请求处理完成、Agent 启动 |
| **Warn** | 非致命问题，可继续执行 | 降级处理、配置使用默认值 |
| **Error** | 错误发生，请求处理失败 | 数据库连接失败、API 调用错误 |
| **Fatal** | 致命错误，程序无法继续 | 配置加载失败、端口被占用 |

---

## Structured Logging

### 基本用法

```go
import "go.uber.org/zap"

// ✅ 使用结构化字段
logger.Info("agent started",
    zap.String("agent_id", agent.ID),
    zap.String("model", agent.Model),
    zap.Int("tool_count", len(agent.Tools)),
)

// ❌ 不要这样做 - 字符串拼接
logger.Info(fmt.Sprintf("agent %s started with model %s", agent.ID, agent.Model))
```

### 字段类型

```go
// 字符串
zap.String("key", "value")
zap.Stringer("uuid", uuid)  // 实现了 String() 接口的类型

// 数值
zap.Int("count", 42)
zap.Int64("timestamp", time.Now().Unix())
zap.Float64("score", 0.95)

// 布尔
zap.Bool("retryable", true)

// 错误
zap.Error(err)

// 持续时间
zap.Duration("elapsed", time.Since(start))

// 嵌套对象
zap.Any("config", config)
zap.Reflect("response", resp)

// 省略空值
zap.String("optional", value, zap.Skip())
```

### Logger 创建

```go
// bootstrap/bootstrap.go
func NewLogger(cfg config.LogConfig) *zap.Logger {
    var level zapcore.Level
    switch cfg.Level {
    case "debug":
        level = zapcore.DebugLevel
    case "info":
        level = zapcore.InfoLevel
    case "warn":
        level = zapcore.WarnLevel
    case "error":
        level = zapcore.ErrorLevel
    default:
        level = zapcore.InfoLevel
    }

    var encoderConfig zapcore.EncoderConfig
    if cfg.Format == "console" {
        // 开发环境: 带颜色的 Console 格式
        encoderConfig = zap.NewDevelopmentEncoderConfig()
        encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
    } else {
        // 生产环境: JSON 格式
        encoderConfig = zap.NewProductionEncoderConfig()
        encoderConfig.TimeKey = "timestamp"
        encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
    }

    zapConfig := zap.Config{
        Level:            zap.NewAtomicLevelAt(level),
        Development:      cfg.Format == "console",
        Encoding:         cfg.Format,
        EncoderConfig:    encoderConfig,
        OutputPaths:      cfg.OutputPaths,
        ErrorOutputPaths: []string{"stderr"},
    }

    logger, err := zapConfig.Build(
        zap.AddCaller(),
        zap.AddStacktrace(zapcore.ErrorLevel),
    )
    if err != nil {
        // 回退到 stderr
        fallbackCore := zapcore.NewCore(
            zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
            zapcore.Lock(os.Stderr),
            zapcore.ErrorLevel,
        )
        return zap.New(fallbackCore)
    }
    return logger
}
```

---

## What to Log

### ✅ 应该记录的事件

**服务生命周期**:
```go
logger.Info("server starting",
    zap.String("address", cfg.Server.Address),
    zap.Int("port", cfg.Server.Port),
)

logger.Info("agent initialized",
    zap.String("agent_id", agent.ID),
    zap.String("provider", agent.Provider),
)
```

**请求处理**:
```go
logger.Info("request completed",
    zap.String("trace_id", traceID),
    zap.String("method", c.Request.Method),
    zap.String("path", c.Request.URL.Path),
    zap.Duration("latency", latency),
    zap.Int("status", c.Writer.Status()),
)
```

**业务关键操作**:
```go
logger.Info("tool executed",
    zap.String("tool_name", toolCall.Name),
    zap.String("call_id", toolCall.ID),
    zap.Duration("duration", duration),
)

logger.Info("checkpoint saved",
    zap.String("checkpoint_id", cp.ID),
    zap.Int("message_count", len(cp.Messages)),
)
```

**错误**:
```go
logger.Error("provider call failed",
    zap.String("provider", provider),
    zap.String("model", model),
    zap.Error(err),
    zap.Duration("retry_after", backoff),
)
```

**性能指标**:
```go
logger.Debug("token usage",
    zap.Int("prompt_tokens", resp.Usage.PromptTokens),
    zap.Int("completion_tokens", resp.Usage.CompletionTokens),
    zap.Int("total_tokens", resp.Usage.TotalTokens),
)
```

---

## What NOT to Log

### ❌ 禁止记录的敏感信息

| 类型 | 示例 | 原因 |
|------|------|------|
| API Key | `sk-abc123...` | 凭证泄露风险 |
| 密码 | `password123` | 安全合规 |
| Token | JWT、Session Token | 会话劫持风险 |
| PII | 邮箱、手机号、身份证号 | 隐私保护 |
| 完整请求体 | 包含敏感字段的 JSON | 数据泄露 |

```go
// ❌ 错误: 记录 API Key
logger.Info("request sent",
    zap.String("api_key", req.APIKey),  // 禁止!
)

// ✅ 正确: 只记录关键标识
logger.Info("request sent",
    zap.String("provider", provider),
    zap.String("model", model),
)

// ❌ 错误: 记录完整配置（可能包含凭证）
logger.Debug("config loaded", zap.Any("config", cfg))

// ✅ 正确: 只记录非敏感配置项
logger.Debug("config loaded",
    zap.String("log_level", cfg.Log.Level),
    zap.String("server_address", cfg.Server.Address),
)
```

---

## Logger 注入模式

### 构造函数注入

```go
// ✅ 推荐: 通过构造函数注入 Logger
type AgentService struct {
    store  AgentStore
    logger *zap.Logger
}

func NewAgentService(store AgentStore, logger *zap.Logger) *AgentService {
    if logger == nil {
        logger = zap.NewNop()  // 无操作 Logger 作为默认值
    }
    return &AgentService{
        store:  store,
        logger: logger,
    }
}
```

### Context 传递（可选）

```go
// 在 Context 中存储 Logger
func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
    return context.WithValue(ctx, loggerKey, logger)
}

func LoggerFromContext(ctx context.Context) *zap.Logger {
    if logger, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
        return logger
    }
    return zap.NewNop()
}
```

---

## Named Loggers

为不同组件创建命名 Logger:

```go
// ✅ 使用 Named 创建组件特定的 Logger
agentLogger := logger.Named("agent")
llmLogger := logger.Named("llm")
apiLogger := logger.Named("api")

// 输出: {"logger": "agent", "msg": "starting..."}
agentLogger.Info("starting agent")
```

---

## Logger 字段复用

### With 方法添加固定字段

```go
// ✅ 为特定操作创建带固定字段的 Logger
opLogger := logger.With(
    zap.String("trace_id", traceID),
    zap.String("agent_id", agentID),
)

// 后续所有日志都包含这些字段
opLogger.Info("processing started")
opLogger.Info("step 1 completed")
opLogger.Error("processing failed", zap.Error(err))
```

---

## Testing with Loggers

### 使用 Nop Logger

```go
import "go.uber.org/zap"

func TestAgentService(t *testing.T) {
    // ✅ 测试中使用无操作 Logger
    logger := zap.NewNop()
    service := NewAgentService(mockStore, logger)

    // 测试逻辑...
}
```

### 使用观察 Logger（ObservedLogs）

```go
import "go.uber.org/zap/zaptest/observer"

func TestAgentServiceLogs(t *testing.T) {
    // 创建观察 Logger
    observedZapCore, observedLogs := observer.New(zap.InfoLevel)
    logger := zap.New(observedZapCore)

    service := NewAgentService(mockStore, logger)
    service.Process(ctx, req)

    // 验证日志
    logs := observedLogs.All()
    require.Len(t, logs, 1)
    assert.Equal(t, "agent started", logs[0].Message)
    assert.Equal(t, "agent-123", logs[0].ContextMap()["agent_id"])
}
```

---

## Log Sampling

高频日志应该启用采样:

```go
// ✅ 对高频日志启用采样
core := zapcore.NewCore(
    encoder,
    writer,
    level,
)

// 每秒最多记录 100 条，首次 10 条不采样
sampledCore := zapcore.NewSamplerWithOptions(
    core,
    time.Second,
    100,  // 每秒最多记录数
    10,   // 首次不采样的数量
)

logger := zap.New(sampledCore)
```

---

## Performance Considerations

### Debug 日志检查

```go
// ✅ 检查日志级别避免不必要的计算
if logger.Core().Enabled(zap.DebugLevel) {
    // 只有 Debug 级别才执行昂贵的序列化
    logger.Debug("detailed state",
        zap.Any("state", expensiveSerialize(state)),
    )
}
```

### 延迟字段求值

```go
// ✅ 使用 zap.Stringer 延迟序列化
logger.Debug("processing",
    zap.Stringer("request", req),  // 只在需要时调用 req.String()
)
```
