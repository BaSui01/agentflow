# Logging Guidelines

> How logging is done in this project.

---

## Overview

AgentFlow uses **`go.uber.org/zap` v1.27.1** exclusively as its structured logging library. It is imported in 150+ source files. The `Sugar()` (printf-style) API is never used — all logging uses zap's structured field API.

Fallback: The standard library `log` package appears in a few places (`examples/*/main.go` for quick demos, `llm/canary.go` and `agent/persistence/factory.go` for fallback/fatal logging). New code should use zap.

---

## Log Levels

| Level | When to Use | Example |
|-------|-------------|---------|
| `Debug` | Low-level operational details: browser navigation, pool acquire/release, event dispatch, subagent spawn | `d.logger.Debug("navigating", zap.String("url", url))` |
| `Info` | Lifecycle events, state transitions, feature initialization, task completion, config reload | `b.logger.Info("state transition", zap.String("from", ...), zap.String("to", ...))` |
| `Warn` | Recoverable failures, degraded operations, config validation failures, rollback events | `b.logger.Warn("failed to load memory", zap.Error(err))` |
| `Error` | Operation failures, screenshot/vision failures, config reload failures | `m.logger.Error("Failed to reload configuration", zap.Error(err))` |
| `Fatal` | Server startup failure only (used once in `cmd/agentflow/main.go:138`) | `logger.Fatal("Failed to start server", zap.Error(err))` |

### Rules

- `Fatal` should only be used at application startup in `main.go`
- `Error` means something broke and needs attention
- `Warn` means something is degraded but the system continues
- `Info` is for significant state changes visible in production logs
- `Debug` is for development/troubleshooting, disabled in production

---

## Structured Logging

### Logger Injection Pattern

Logger is passed via constructor injection as `*zap.Logger` and stored as a struct field named `logger`:

```go
// agent/builder.go:94-101
func (b *AgentBuilder) WithLogger(logger *zap.Logger) *AgentBuilder {
    if logger == nil {
        b.errors = append(b.errors, fmt.Errorf("logger cannot be nil"))
        return b
    }
    b.logger = logger
    return b
}
```

### Default to `zap.NewNop()`

When no logger is provided, default to a no-op logger:

```go
// agent/builder.go:237
if b.logger == nil {
    b.logger = zap.NewNop()
}
```

This pattern is consistent across `config/watcher.go:130`, `config/hotreload.go:360`, and others.

### Logger Enrichment

Add context fields at construction time using `logger.With()`:

```go
// agent/base.go:181
logger: logger.With(
    zap.String("agent_id", cfg.ID),
    zap.String("agent_type", string(cfg.Type)),
),
```

### Structured Fields

Always use typed field constructors:

```go
zap.String("path", filePath)       // String fields
zap.Error(err)                     // Error fields (always use this for errors)
zap.Int("count", n)                // Integer fields
zap.Bool("enabled", true)          // Boolean fields
zap.Duration("elapsed", dur)       // Duration fields
zap.Any("config", cfg)             // Dynamic/complex values
zap.Strings("tags", tagList)       // String slices
```

### Multi-Field Logging

```go
// config/hotreload.go:684-701
fields := []zap.Field{
    zap.String("path", change.Path),
    zap.String("source", change.Source),
    zap.Bool("requires_restart", change.RequiresRestart),
}
if !known || !field.Sensitive {
    fields = append(fields,
        zap.Any("old_value", change.OldValue),
        zap.Any("new_value", change.NewValue),
    )
}
m.logger.Info("Configuration changed", fields...)
```

---

## Log Configuration

Configured in `cmd/agentflow/main.go:218-272`, driven by YAML config:

```yaml
# deployments/docker/config.example.yaml:130-143
log:
  level: "info"           # debug, info, warn, error
  format: "json"          # "json" or "console"
  output_paths:
    - "stdout"
  enable_caller: true
  enable_stacktrace: false
```

| Mode | Format | Timestamps | Colors | Stack Traces |
|------|--------|-----------|--------|-------------|
| Production | JSON | ISO8601, key: `"timestamp"` | No | At Error level |
| Development | Console | Colored | Yes | At Error level |

Both modes add caller info (`zap.AddCaller()`) and stack traces at Error level (`zap.AddStacktrace(zapcore.ErrorLevel)`).

### Application Startup

```go
// cmd/agentflow/main.go:126-131
logger := initLogger(cfg.Log)
defer logger.Sync()

logger.Info("Starting AgentFlow",
    zap.String("version", Version),
    zap.String("build_time", BuildTime),
    zap.String("git_commit", GitCommit),
)
```

---

## What to Log

- Application startup/shutdown with version info
- State transitions (agent lifecycle, circuit breaker state changes)
- Configuration changes (with sensitive field redaction)
- Request/response metadata (method, path, status, duration, remote_addr)
- Error conditions with full context (`zap.Error(err)`)
- Performance-relevant events (cache hits/misses, retry attempts)
- Security events (auth failures, guardrails violations)

---

## What NOT to Log

- API keys, passwords, tokens, secrets (auto-redacted in config change logs via `hotReloadableFields` sensitive flag)
- Full request/response bodies (use Debug level only if needed)
- PII (user emails, names, etc.)
- Raw SQL queries with parameter values
- Large binary data or file contents
- High-frequency events at Info level (use Debug)

### Sensitive Field Handling

The hot reload system (`config/hotreload.go`) automatically redacts sensitive fields:

```go
// Fields marked as sensitive are not logged in change events
if !known || !field.Sensitive {
    fields = append(fields,
        zap.Any("old_value", change.OldValue),
        zap.Any("new_value", change.NewValue),
    )
}
```

---

## Anti-Patterns

### 1. Using `log.Printf` Instead of Zap

```go
// WRONG
log.Printf("failed to connect: %v", err)

// CORRECT
logger.Error("failed to connect", zap.Error(err))
```

### 2. Using `Sugar()` API

```go
// WRONG — not used in this project
logger.Sugar().Infof("processing %d items", count)

// CORRECT
logger.Info("processing items", zap.Int("count", count))
```

### 3. String Formatting in Log Messages

```go
// WRONG — loses structured data
logger.Info(fmt.Sprintf("user %s logged in from %s", userID, ip))

// CORRECT
logger.Info("user logged in", zap.String("user_id", userID), zap.String("ip", ip))
```

### 4. Logging Sensitive Data

```go
// WRONG
logger.Info("API key loaded", zap.String("key", apiKey))

// CORRECT
logger.Info("API key loaded", zap.String("provider", providerName))
```
