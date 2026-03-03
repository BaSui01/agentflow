package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BuildAgentCheckpointStore builds a single checkpoint store backend from config.
func BuildAgentCheckpointStore(cfg *config.Config, db *gorm.DB, logger *zap.Logger) (agent.CheckpointStore, error) {
	if cfg == nil || !cfg.Agent.Checkpoint.Enabled {
		return nil, nil
	}

	backend := strings.ToLower(strings.TrimSpace(cfg.Agent.Checkpoint.Backend))
	switch backend {
	case "file":
		return agent.NewFileCheckpointStore(strings.TrimSpace(cfg.Agent.Checkpoint.FilePath), logger)
	case "redis":
		client := redis.NewClient(&redis.Options{
			Addr:         cfg.Redis.Addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
		})
		return agent.NewRedisCheckpointStore(
			redisCheckpointClient{client: client},
			strings.TrimSpace(cfg.Agent.Checkpoint.RedisPrefix),
			cfg.Agent.Checkpoint.RedisTTL,
			logger,
		), nil
	case "postgres":
		if db == nil {
			return nil, fmt.Errorf("database is required for postgres checkpoint backend")
		}
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("get sql db from gorm: %w", err)
		}
		client := sqlCheckpointClient{db: sqlDB}
		if err := ensureCheckpointTable(context.Background(), client); err != nil {
			return nil, fmt.Errorf("ensure checkpoint table: %w", err)
		}
		return agent.NewPostgreSQLCheckpointStore(client, logger), nil
	default:
		return nil, fmt.Errorf("unsupported checkpoint backend: %s", backend)
	}
}

type redisCheckpointClient struct {
	client *redis.Client
}

func (c redisCheckpointClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c redisCheckpointClient) Get(ctx context.Context, key string) ([]byte, error) {
	return c.client.Get(ctx, key).Bytes()
}

func (c redisCheckpointClient) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c redisCheckpointClient) Keys(ctx context.Context, pattern string) ([]string, error) {
	return c.client.Keys(ctx, pattern).Result()
}

func (c redisCheckpointClient) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return c.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}

func (c redisCheckpointClient) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:   key,
		Start: start,
		Stop:  stop,
		Rev:   true,
	}).Result()
}

func (c redisCheckpointClient) ZRemRangeByScore(ctx context.Context, key string, min, max string) error {
	return c.client.ZRemRangeByScore(ctx, key, min, max).Err()
}

type sqlCheckpointClient struct {
	db *sql.DB
}

type sqlRow struct {
	row *sql.Row
}

func (r sqlRow) Scan(dest ...any) error { return r.row.Scan(dest...) }

type sqlRows struct {
	rows *sql.Rows
}

func (r sqlRows) Next() bool             { return r.rows.Next() }
func (r sqlRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r sqlRows) Close() error           { return r.rows.Close() }
func (c sqlCheckpointClient) Exec(ctx context.Context, query string, args ...any) error {
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

func (c sqlCheckpointClient) QueryRow(ctx context.Context, query string, args ...any) agent.Row {
	return sqlRow{row: c.db.QueryRowContext(ctx, query, args...)}
}

func (c sqlCheckpointClient) Query(ctx context.Context, query string, args ...any) (agent.Rows, error) {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqlRows{rows: rows}, nil
}

func ensureCheckpointTable(ctx context.Context, db agent.PostgreSQLClient) error {
	const createTable = `
CREATE TABLE IF NOT EXISTS agent_checkpoints (
	id TEXT PRIMARY KEY,
	thread_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	version INTEGER NOT NULL,
	state TEXT NOT NULL,
	data BYTEA NOT NULL,
	created_at TIMESTAMP NOT NULL,
	parent_id TEXT
);
`
	const indexByCreated = `
CREATE INDEX IF NOT EXISTS idx_agent_checkpoints_thread_created
	ON agent_checkpoints(thread_id, created_at DESC);
`
	const indexByVersion = `
CREATE INDEX IF NOT EXISTS idx_agent_checkpoints_thread_version
	ON agent_checkpoints(thread_id, version ASC);
`
	if err := db.Exec(ctx, createTable); err != nil {
		return err
	}
	if err := db.Exec(ctx, indexByCreated); err != nil {
		return err
	}
	return db.Exec(ctx, indexByVersion)
}
