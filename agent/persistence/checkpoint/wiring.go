package checkpoint

import (
	"context"
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisClientAdapter struct {
	client *redis.Client
}

// NewRedisClientAdapter adapts go-redis to the checkpoint RedisClient contract.
func NewRedisClientAdapter(client *redis.Client) RedisClient {
	return redisClientAdapter{client: client}
}

func (c redisClientAdapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c redisClientAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	return c.client.Get(ctx, key).Bytes()
}

func (c redisClientAdapter) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c redisClientAdapter) Keys(ctx context.Context, pattern string) ([]string, error) {
	return c.client.Keys(ctx, pattern).Result()
}

func (c redisClientAdapter) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return c.client.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}

func (c redisClientAdapter) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:   key,
		Start: start,
		Stop:  stop,
		Rev:   true,
	}).Result()
}

func (c redisClientAdapter) ZRemRangeByScore(ctx context.Context, key string, min, max string) error {
	return c.client.ZRemRangeByScore(ctx, key, min, max).Err()
}

type postgreSQLClientAdapter struct {
	db *sql.DB
}

type sqlRowAdapter struct {
	row *sql.Row
}

func (r sqlRowAdapter) Scan(dest ...any) error { return r.row.Scan(dest...) }

type sqlRowsAdapter struct {
	rows *sql.Rows
}

func (r sqlRowsAdapter) Next() bool             { return r.rows.Next() }
func (r sqlRowsAdapter) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r sqlRowsAdapter) Close() error           { return r.rows.Close() }

// NewPostgreSQLClientAdapter adapts *sql.DB to the checkpoint PostgreSQLClient contract.
func NewPostgreSQLClientAdapter(db *sql.DB) PostgreSQLClient {
	return postgreSQLClientAdapter{db: db}
}

func (c postgreSQLClientAdapter) Exec(ctx context.Context, query string, args ...any) error {
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

func (c postgreSQLClientAdapter) QueryRow(ctx context.Context, query string, args ...any) Row {
	return sqlRowAdapter{row: c.db.QueryRowContext(ctx, query, args...)}
}

func (c postgreSQLClientAdapter) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqlRowsAdapter{rows: rows}, nil
}

// EnsurePostgreSQLSchema provisions the agent checkpoint table and indexes.
func EnsurePostgreSQLSchema(ctx context.Context, db PostgreSQLClient) error {
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
