package bootstrap

import (
	"fmt"

	"github.com/BaSui01/agentflow/config"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"go.uber.org/zap"
)

// BuildMongoClient creates the MongoDB client used by runtime wiring.
func BuildMongoClient(cfg config.MongoDBConfig, logger *zap.Logger) (*mongoclient.Client, error) {
	connectCfg := mongoclient.ConnectConfig{
		URI:                cfg.URI,
		Host:               cfg.Host,
		Port:               cfg.Port,
		User:               cfg.User,
		Password:           cfg.Password,
		AuthSource:         cfg.AuthSource,
		ReplicaSet:         cfg.ReplicaSet,
		MaxPoolSize:        cfg.MaxPoolSize,
		MinPoolSize:        cfg.MinPoolSize,
		ConnectTimeout:     cfg.ConnectTimeout,
		Timeout:            cfg.Timeout,
		TLSEnabled:         cfg.TLSEnabled,
		TLSCAFile:          cfg.TLSCAFile,
		TLSCertFile:        cfg.TLSCertFile,
		TLSKeyFile:         cfg.TLSKeyFile,
		Database:           cfg.Database,
		HealthCheckInterval: cfg.HealthCheckInterval,
	}
	client, err := mongoclient.NewClient(connectCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	logger.Info("MongoDB client initialized",
		zap.String("database", cfg.Database),
	)
	return client, nil
}
