# Database Guidelines

> AgentFlow 项目的数据库规范和最佳实践。

---

## Overview

AgentFlow 使用两种数据库技术：

1. **关系型数据库** (GORM): MySQL、PostgreSQL、SQLite - 用于结构化数据存储
2. **文档数据库** (MongoDB): 用于会话、记忆、审计等灵活 schema 数据

---

## Query Patterns

### GORM 模式

#### 基础 CRUD

```go
// ✅ 查询单条记录
func (s *GormToolProviderStore) GetByProvider(provider string) (ToolProviderConfig, error) {
    var row ToolProviderConfig
    err := s.db.Where("provider = ?", provider).First(&row).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return row, ErrNotFound
    }
    return row, err
}

// ✅ 查询多条记录
func (s *GormToolProviderStore) List() ([]ToolProviderConfig, error) {
    var rows []ToolProviderConfig
    err := s.db.Order("priority ASC, id ASC").Limit(500).Find(&rows).Error
    return rows, err
}

// ✅ 创建记录
func (s *GormToolProviderStore) Create(row *ToolProviderConfig) error {
    return s.db.Create(row).Error
}

// ✅ 更新记录 - 使用 Updates 更新指定字段
func (s *GormToolProviderStore) Update(row *ToolProviderConfig, updates map[string]any) error {
    return s.db.Model(row).Updates(updates).Error
}

// ✅ 删除记录
func (s *GormToolProviderStore) DeleteByProvider(provider string) (int64, error) {
    result := s.db.Where("provider = ?", provider).Delete(&ToolProviderConfig{})
    return result.RowsAffected, result.Error
}
```

#### 错误处理

```go
// ✅ 正确处理记录不存在的情况
import "gorm.io/gorm"

func (s *Store) Get(id string) (*Model, error) {
    var m Model
    err := s.db.First(&m, "id = ?", id).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, types.NewNotFoundError("record not found")
    }
    if err != nil {
        return nil, types.WrapError(err, types.ErrInternalError, "database query failed")
    }
    return &m, nil
}
```

### MongoDB 模式

#### 文档结构定义

```go
// ✅ 定义 BSON 文档结构
package mongodb

import "go.mongodb.org/mongo-driver/v2/bson"

// ConversationDocument is the MongoDB document for a single conversation.
type ConversationDocument struct {
    ID         string            `bson:"_id"         json:"id"`
    ParentID   string            `bson:"parent_id"   json:"parent_id,omitempty"`
    AgentID    string            `bson:"agent_id"    json:"agent_id"`
    TenantID   string            `bson:"tenant_id"   json:"tenant_id"`
    UserID     string            `bson:"user_id"     json:"user_id"`
    Title      string            `bson:"title"       json:"title,omitempty"`
    Messages   []MessageDocument `bson:"messages"    json:"messages"`
    Metadata   map[string]any    `bson:"metadata"    json:"metadata,omitempty"`
    Archived   bool              `bson:"archived"    json:"archived"`
    CreatedAt  time.Time         `bson:"created_at"  json:"created_at"`
    UpdatedAt  time.Time         `bson:"updated_at"  json:"updated_at"`
}
```

#### CRUD 操作

```go
// ✅ 创建文档
func (s *MongoConversationStore) Create(ctx context.Context, doc *ConversationDocument) error {
    doc.CreatedAt = time.Now()
    doc.UpdatedAt = time.Now()

    _, err := s.collection.InsertOne(ctx, doc)
    return err
}

// ✅ 查询单条文档
func (s *MongoConversationStore) GetByID(ctx context.Context, id string) (*ConversationDocument, error) {
    var doc ConversationDocument
    err := s.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
    if err == mongo.ErrNoDocuments {
        return nil, types.NewNotFoundError("conversation not found")
    }
    if err != nil {
        return nil, err
    }
    return &doc, nil
}

// ✅ 更新文档 - 使用 bson.D 保证顺序
func (s *MongoConversationStore) Update(ctx context.Context, id string, updates bson.D) error {
    updates = append(updates, bson.E{Key: "updated_at", Value: time.Now()})

    _, err := s.collection.UpdateOne(
        ctx,
        bson.M{"_id": id},
        bson.D{{Key: "$set", Value: updates}},
    )
    return err
}

// ✅ 查询列表 - 带分页
func (s *MongoConversationStore) List(ctx context.Context, filter ConversationFilter) ([]*ConversationDocument, int64, error) {
    // 构建查询条件
    query := bson.M{}
    if filter.AgentID != "" {
        query["agent_id"] = filter.AgentID
    }
    if filter.TenantID != "" {
        query["tenant_id"] = filter.TenantID
    }

    // 计数
    total, err := s.collection.CountDocuments(ctx, query)
    if err != nil {
        return nil, 0, err
    }

    // 查询
    opts := options.Find().
        SetSort(bson.D{{Key: "created_at", Value: -1}}).
        SetSkip(int64(filter.Offset)).
        SetLimit(int64(filter.Limit))

    cursor, err := s.collection.Find(ctx, query, opts)
    if err != nil {
        return nil, 0, err
    }
    defer cursor.Close(ctx)

    var docs []*ConversationDocument
    if err := cursor.All(ctx, &docs); err != nil {
        return nil, 0, err
    }

    return docs, total, nil
}
```

---

## Migrations

### 使用 golang-migrate

项目使用 `golang-migrate/migrate` 进行数据库迁移。

```go
// 在 bootstrap 中初始化迁移
import (
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(dbURL string, migrationsPath string) error {
    m, err := migrate.New(
        "file://"+migrationsPath,
        dbURL,
    )
    if err != nil {
        return err
    }

    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return err
    }
    return nil
}
```

### 迁移文件命名

```
migrations/
├── 000001_create_agents_table.up.sql
├── 000001_create_agents_table.down.sql
├── 000002_add_agent_config.up.sql
├── 000002_add_agent_config.down.sql
└── ...
```

**命名规则**:
- 格式: `{version}_{description}.{direction}.sql`
- `version`: 6 位数字，递增
- `description`: 下划线分隔的描述
- `direction`: `up` 或 `down`

### 迁移内容示例

```sql
-- 000001_create_agents_table.up.sql
CREATE TABLE IF NOT EXISTS agents (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    model VARCHAR(100) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    config JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_agents_provider ON agents(provider);
CREATE INDEX idx_agents_created_at ON agents(created_at);

-- 000001_create_agents_table.down.sql
DROP TABLE IF EXISTS agents;
```

---

## Naming Conventions

### 表名

| 约定 | 示例 | 说明 |
|------|------|------|
| 复数形式 | `agents`, `conversations` | 表存储多条记录 |
| 小写下划线 | `tool_providers` | snake_case |
| 避免缩写 | `conversations` 而非 `conv` | 可读性优先 |

### 列名

```sql
-- ✅ 推荐命名
CREATE TABLE agents (
    id VARCHAR(36) PRIMARY KEY,           -- ID 字段
    name VARCHAR(255) NOT NULL,           -- 名称
    created_at TIMESTAMP,                 -- 创建时间
    updated_at TIMESTAMP,                 -- 更新时间
    deleted_at TIMESTAMP,                 -- 软删除时间（GORM）
    config JSONB                          -- JSON 配置
);

-- ❌ 不推荐
CREATE TABLE Agents (
    ID VARCHAR(36),
    agentName VARCHAR(255),               -- 驼峰命名
    createdAt TIMESTAMP                   -- 驼峰命名
);
```

### 索引命名

```sql
-- ✅ 推荐命名: idx_{table}_{column(s)}
CREATE INDEX idx_agents_provider ON agents(provider);
CREATE INDEX idx_agents_provider_model ON agents(provider, model);
CREATE UNIQUE INDEX idx_agents_name_unique ON agents(name) WHERE deleted_at IS NULL;

-- ❌ 不推荐
CREATE INDEX provider_idx ON agents(provider);  -- 表名在后
```

### MongoDB 集合命名

```go
// ✅ 使用复数形式，小写下划线
const conversationsCollection = "conversations"
const agentMemoriesCollection = "agent_memories"

// ❌ 不推荐
const convColl = "conv"  // 缩写
const AgentMemory = "AgentMemory"  // 大写
```

---

## Common Mistakes

### ❌ 错误: 在循环中执行查询

```go
// 不要这样做 - N+1 查询问题
for _, agentID := range agentIDs {
    var agent Agent
    db.First(&agent, "id = ?", agentID)  // N 次查询!
}

// ✅ 正确做法 - 使用 IN 查询
var agents []Agent
db.Where("id IN ?", agentIDs).Find(&agents)  // 1 次查询
```

### ❌ 错误: 不处理上下文取消

```go
// 不要这样做
func (s *Store) Get(ctx context.Context, id string) (*Model, error) {
    var m Model
    err := s.db.First(&m, "id = ?", id).Error  // 不检查 context
    return &m, err
}

// ✅ 正确做法 - 传递 context
func (s *Store) Get(ctx context.Context, id string) (*Model, error) {
    var m Model
    err := s.db.WithContext(ctx).First(&m, "id = ?", id).Error
    return &m, err
}
```

### ❌ 错误: 全表更新/删除

```go
// 不要这样做 - 忘记 WHERE 条件
result := db.Model(&Agent{}).Update("status", "inactive")  // 更新所有记录!

// ✅ 正确做法 - 明确指定条件
result := db.Model(&Agent{}).Where("id = ?", id).Update("status", "inactive")
```

### ❌ 错误: 在 MongoDB 中使用无索引查询

```go
// 不要这样做 - 对大集合执行无索引查询
filter := bson.M{"metadata.custom_field": value}  // 无索引

// ✅ 正确做法 - 确保查询字段有索引
// 在集合创建时定义索引
collection.Indexes().CreateOne(ctx, mongo.IndexModel{
    Keys: bson.D{{Key: "metadata.tenant_id", Value: 1}},
})
```

### ❌ 错误: 在日志中记录敏感数据

```go
// 不要这样做
logger.Info("query executed",
    zap.String("sql", "SELECT * FROM users WHERE api_key = 'secret'"),
)

// ✅ 正确做法 - 参数化查询，不在日志中记录敏感值
logger.Debug("database query",
    zap.String("table", "users"),
    zap.String("operation", "SELECT"),
)
```

---

## Connection Pool

### GORM 连接池配置

```go
import (
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func NewPostgresClient(dsn string) (*gorm.DB, error) {
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent), // 使用自定义日志
    })
    if err != nil {
        return nil, err
    }

    sqlDB, err := db.DB()
    if err != nil {
        return nil, err
    }

    // 连接池配置
    sqlDB.SetMaxOpenConns(25)        // 最大打开连接数
    sqlDB.SetMaxIdleConns(10)        // 最大空闲连接数
    sqlDB.SetConnMaxLifetime(5 * time.Minute)  // 连接最大生命周期
    sqlDB.SetConnMaxIdleTime(1 * time.Minute)  // 空闲连接最大时间

    return db, nil
}
```

### MongoDB 连接配置

```go
import "go.mongodb.org/mongo-driver/v2/mongo/options"

func NewMongoClient(uri string) (*mongo.Client, error) {
    opts := options.Client().
        ApplyURI(uri).
        SetMaxPoolSize(100).                    // 最大连接池大小
        SetMinPoolSize(10).                     // 最小连接池大小
        SetMaxConnIdleTime(30 * time.Second).   // 连接最大空闲时间
        SetServerSelectionTimeout(5 * time.Second)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    client, err := mongo.Connect(ctx, opts)
    if err != nil {
        return nil, err
    }

    // 验证连接
    if err := client.Ping(ctx, nil); err != nil {
        return nil, err
    }

    return client, nil
}
```

---

## Transaction Pattern

### GORM 事务

```go
func (s *Store) Transfer(ctx context.Context, fromID, toID string, amount int64) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 扣减源账户
        if err := tx.Model(&Account{}).Where("id = ?", fromID).
            UpdateColumn("balance", gorm.Expr("balance - ?", amount)).Error; err != nil {
            return err
        }

        // 增加目标账户
        if err := tx.Model(&Account{}).Where("id = ?", toID).
            UpdateColumn("balance", gorm.Expr("balance + ?", amount)).Error; err != nil {
            return err
        }

        // 记录交易
        if err := tx.Create(&Transaction{From: fromID, To: toID, Amount: amount}).Error; err != nil {
            return err
        }

        return nil
    })
}
```

### MongoDB 事务（多文档）

```go
func (s *Store) Transfer(ctx context.Context, fromID, toID string, amount int64) error {
    session, err := s.client.StartSession()
    if err != nil {
        return err
    }
    defer session.EndSession(ctx)

    _, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
        // 扣减源账户
        _, err := s.accounts.UpdateOne(sessCtx,
            bson.M{"_id": fromID},
            bson.M{"$inc": bson.M{"balance": -amount}},
        )
        if err != nil {
            return nil, err
        }

        // 增加目标账户
        _, err = s.accounts.UpdateOne(sessCtx,
            bson.M{"_id": toID},
            bson.M{"$inc": bson.M{"balance": amount}},
        )
        if err != nil {
            return nil, err
        }

        return nil, nil
    })

    return err
}
```
