# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

AgentFlow uses **GORM v1.31.1** as the primary ORM, with support for three database backends:

| Backend | Driver | Usage |
|---------|--------|-------|
| PostgreSQL | `gorm.io/driver/postgres` v1.6.0 | Primary production database |
| MySQL | `github.com/go-sql-driver/mysql` v1.8.1 | Supported via migrations |
| SQLite | `github.com/glebarez/sqlite` v1.11.0 | Testing and development |

Additional infrastructure:
- **Redis** (`github.com/redis/go-redis/v9`) — short-term memory cache
- **Vector stores** — Qdrant (default), Weaviate, Milvus, Pinecone (via `rag/` package)

---

## Query Patterns

### GORM Fluent Query Builder (Standard)

All application queries use GORM's fluent API. **No raw SQL in application code.**

```go
// Simple filtered query (llm/apikey_pool.go:69-72)
err := p.db.WithContext(ctx).
    Where("provider_id = ? AND enabled = TRUE", p.providerID).
    Order("priority ASC, weight DESC").
    Find(&keys).Error

// Chained query with joins (llm/router_multi_provider.go:76-81)
query := r.db.WithContext(ctx).Table("sc_llm_provider_models").
    Select("sc_llm_provider_models.*, p.code as provider_code, p.status as provider_status, m.model_name").
    Joins("JOIN sc_llm_providers p ON p.id = sc_llm_provider_models.provider_id").
    Joins("JOIN sc_llm_models m ON m.id = sc_llm_provider_models.model_id").
    Where("m.model_name = ? AND sc_llm_provider_models.enabled = TRUE AND p.status = ?",
        modelName, LLMProviderStatusActive)
```

### Context Propagation

Always pass `context.Context` via `db.WithContext(ctx)`:

```go
// CORRECT
err := p.db.WithContext(ctx).Where(...).Find(&result).Error

// WRONG — missing context
err := p.db.Where(...).Find(&result).Error
```

---

## Model Definitions

### GORM Model Pattern

Models are defined in `llm/types.go` with struct tags for GORM, JSON, and validation:

```go
// llm/types.go:34-46
type LLMProvider struct {
    ID          uint              `gorm:"primaryKey" json:"id"`
    Code        string            `gorm:"size:50;not null;uniqueIndex" json:"code"`
    Name        string            `gorm:"size:200;not null" json:"name"`
    Description string            `gorm:"type:text" json:"description"`
    Status      LLMProviderStatus `gorm:"default:1" json:"status"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}

// Explicit table name (required)
func (LLMProvider) TableName() string { return "sc_llm_providers" }
```

### Required Conventions

- Always define `TableName()` method — use `sc_` prefix
- Use `gorm:"primaryKey"` for ID fields
- Use `gorm:"not null"` for required fields
- Use `gorm:"uniqueIndex"` or `gorm:"index:idx_name"` for indexed fields
- Include both `gorm` and `json` struct tags

---

## Migrations

### Dual Migration Strategy

The project uses two migration approaches:

#### 1. `golang-migrate` for Versioned SQL Migrations (Production)

Located at `internal/migration/migrator.go`, with embedded SQL files:

```
internal/migration/migrations/
├── postgres/
│   ├── 000001_init_schema.up.sql
│   └── 000001_init_schema.down.sql
├── mysql/
│   └── 000001_init_schema.up.sql
└── sqlite/
    └── 000001_init_schema.up.sql
```

CLI commands via `cmd/agentflow/migrate.go`:
```bash
agentflow migrate up          # Apply all pending migrations
agentflow migrate down        # Rollback last migration
agentflow migrate status      # Show migration status
agentflow migrate version     # Show current version
agentflow migrate goto N      # Migrate to specific version
agentflow migrate force N     # Force set version (no migration run)
agentflow migrate reset       # Rollback all migrations
```

#### 2. GORM `AutoMigrate` for Development

Used in `llm/db_init.go` for quick schema sync:

```go
// llm/db_init.go:11-27
func InitDatabase(db *gorm.DB) error {
    err := db.AutoMigrate(
        &LLMProvider{},
        &LLMModel{},
        &LLMProviderModel{},
        &LLMProviderAPIKey{},
    )
    // ...
}
```

`SeedExampleData()` seeds 13 providers, 52 models, and 52 provider-model mappings.

### Creating a New Migration

```bash
# Create migration files manually:
# internal/migration/migrations/postgres/000002_add_feature.up.sql
# internal/migration/migrations/postgres/000002_add_feature.down.sql
```

---

## Connection Management

### Pool Manager (`internal/database/pool.go`)

```go
type PoolManager struct {
    db     *gorm.DB
    mu     sync.RWMutex
    config PoolConfig
}
```

Features:
- Configurable: `MaxIdleConns`, `MaxOpenConns`, `ConnMaxLifetime`, `ConnMaxIdleTime`
- Background health check loop with configurable interval
- Thread-safe access via `sync.RWMutex`
- Exposes `DB()`, `Ping()`, `Stats()`, `Close()`

### Transaction Patterns

```go
// Simple transaction (internal/database/pool.go:213)
func (pm *PoolManager) WithTransaction(ctx context.Context, fn TransactionFunc) error {
    return db.WithContext(ctx).Transaction(fn)
}

// Transaction with exponential backoff retry (internal/database/pool.go:230)
func (pm *PoolManager) WithTransactionRetry(ctx context.Context, maxRetries int, fn TransactionFunc) error {
    // Exponential backoff: 100ms, 200ms, 400ms...
}
```

---

## Naming Conventions

| Element | Convention | Example |
|---------|-----------|---------|
| Table names | `sc_` prefix, snake_case | `sc_llm_providers`, `sc_llm_models` |
| Column names | snake_case | `model_name`, `provider_id`, `rate_limit_rpm` |
| Index names | `idx_` prefix | `idx_model_provider` |
| Primary keys | `id` (uint, auto-increment) | `gorm:"primaryKey"` |
| Foreign keys | `{table}_id` | `provider_id`, `model_id` |
| Timestamps | `created_at`, `updated_at` | Auto-managed by GORM |
| Boolean columns | Descriptive name | `enabled`, `is_default` |
| Decimal columns | Explicit type | `gorm:"type:decimal(10,6)"` |

---

## Common Mistakes

### 1. Forgetting `WithContext(ctx)`

Always propagate context for cancellation and tracing support.

### 2. Using Raw SQL in Application Code

Use GORM's fluent API. Raw SQL is only acceptable in migration files.

### 3. Missing `TableName()` Method

Every GORM model must define `TableName()` with the `sc_` prefix. Without it, GORM will auto-generate a table name that doesn't match the schema.

### 4. Not Handling `gorm.ErrRecordNotFound`

```go
// CORRECT
err := db.WithContext(ctx).Where("id = ?", id).First(&record).Error
if errors.Is(err, gorm.ErrRecordNotFound) {
    return nil, nil // Not found is not an error
}
if err != nil {
    return nil, fmt.Errorf("query failed: %w", err)
}

// WRONG — treats not-found as an error
```

### 5. Forgetting Migration Down Files

Every `.up.sql` must have a corresponding `.down.sql` for rollback support.
