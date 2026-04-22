package bootstrap

import (
	"context"
	"fmt"
	"strings"

	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	"github.com/BaSui01/agentflow/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BuildAgentCheckpointStore builds a single checkpoint store backend from config.
func BuildAgentCheckpointStore(cfg *config.Config, db *gorm.DB, logger *zap.Logger) (agentcheckpoint.Store, error) {
	if cfg == nil || !cfg.Agent.Checkpoint.Enabled {
		return nil, nil
	}

	backend := strings.ToLower(strings.TrimSpace(cfg.Agent.Checkpoint.Backend))
	switch backend {
	case "file":
		return agentcheckpoint.NewFileCheckpointStore(strings.TrimSpace(cfg.Agent.Checkpoint.FilePath), logger)
	case "redis":
		client := redis.NewClient(&redis.Options{
			Addr:         cfg.Redis.Addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
		})
		return agentcheckpoint.NewRedisCheckpointStore(
			agentcheckpoint.NewRedisClientAdapter(client),
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
		client := agentcheckpoint.NewPostgreSQLClientAdapter(sqlDB)
		if err := agentcheckpoint.EnsurePostgreSQLSchema(context.Background(), client); err != nil {
			return nil, fmt.Errorf("ensure checkpoint table: %w", err)
		}
		return agentcheckpoint.NewPostgreSQLCheckpointStore(client, logger), nil
	default:
		return nil, fmt.Errorf("unsupported checkpoint backend: %s", backend)
	}
}
