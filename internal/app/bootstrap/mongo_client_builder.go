package bootstrap

import (
	"fmt"

	"github.com/BaSui01/agentflow/config"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"go.uber.org/zap"
)

// BuildMongoClient creates the MongoDB client used by runtime wiring.
func BuildMongoClient(cfg config.MongoDBConfig, logger *zap.Logger) (*mongoclient.Client, error) {
	client, err := mongoclient.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	logger.Info("MongoDB client initialized",
		zap.String("database", cfg.Database),
	)
	return client, nil
}
