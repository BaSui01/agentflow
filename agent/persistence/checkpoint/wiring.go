package checkpoint

import (
	"context"
	"database/sql"
	"time"

	"github.com/BaSui01/agentflow/pkg/database"
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

// NewPostgreSQLClientAdapter adapts *sql.DB to the database.PostgreSQLClient contract.
func NewPostgreSQLClientAdapter(db *sql.DB) database.PostgreSQLClient {
	return database.NewSQLDBAdapter(db)
}

// EnsurePostgreSQLSchema provisions the agent checkpoint table and indexes.
func EnsurePostgreSQLSchema(ctx context.Context, db database.PostgreSQLClient) error {
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
