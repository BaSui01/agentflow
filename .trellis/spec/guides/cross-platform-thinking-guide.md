# Cross-Platform Thinking Guide

> **Purpose**: Catch platform-specific assumptions before they become bugs.

---

## The Problem

AgentFlow targets 4 platforms: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `windows/amd64` (from CI cross-compilation in `.github/workflows/ci.yml`).

Code that works on your dev machine may fail on other platforms.

---

## AgentFlow Platform Considerations

### File Paths

```go
// WRONG — hardcoded separator
path := "config" + "/" + "app.yaml"

// CORRECT — use filepath.Join
path := filepath.Join("config", "app.yaml")
```

The `config/watcher.go` uses `fsnotify` for cross-platform file watching — this is already handled. But any new file operations should use `path/filepath`, not string concatenation.

### Line Endings

Config files and migration SQL files may have different line endings across platforms.

```go
// CORRECT — normalize when reading user-provided text
strings.ReplaceAll(input, "\r\n", "\n")
```

### Environment Variables

AgentFlow config supports env var overrides (`config/loader.go`). Platform differences:

| Aspect | Linux/macOS | Windows |
|--------|-------------|---------|
| Case sensitivity | Case-sensitive | Case-insensitive |
| Path separator in PATH | `:` | `;` |
| Home directory | `$HOME` | `%USERPROFILE%` |

```go
// CORRECT — use os.UserHomeDir() instead of $HOME
home, err := os.UserHomeDir()
```

### Process Signals

The HTTP server in `cmd/agentflow/server.go` handles graceful shutdown via OS signals.

```go
// Linux/macOS: SIGTERM, SIGINT
// Windows: only SIGINT (Ctrl+C) is reliable
// Don't rely on SIGTERM on Windows
```

### Database Paths (SQLite)

SQLite is used for testing and development. File paths differ:

```go
// WRONG
db, err := gorm.Open(sqlite.Open("/tmp/test.db"))

// CORRECT — use os.TempDir() or t.TempDir() in tests
db, err := gorm.Open(sqlite.Open(filepath.Join(os.TempDir(), "test.db")))

// BEST in tests — use t.TempDir() (auto-cleaned)
db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")))
```

---

## Checklist for Cross-Platform Code

- [ ] File paths use `filepath.Join()`, not string concatenation
- [ ] No hardcoded `/tmp/` — use `os.TempDir()` or `t.TempDir()`
- [ ] No reliance on SIGTERM for Windows compatibility
- [ ] Environment variable access uses `os.Getenv()`, not shell expansion
- [ ] Home directory uses `os.UserHomeDir()`, not `$HOME`
- [ ] Tests use `t.TempDir()` for temporary files
- [ ] No platform-specific shell commands in Go code (use Go stdlib)

---

## When to Think About This

- [ ] Writing scripts or commands that users run directly
- [ ] Working with file paths or temporary files
- [ ] Adding new config loading or env var reading
- [ ] Writing tests that create files or directories
- [ ] Adding graceful shutdown or signal handling

→ See also: [Quality Guidelines](../backend/quality-guidelines.md) for CI cross-compilation targets
