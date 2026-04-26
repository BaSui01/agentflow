package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/api/handlers"
)

func registerServeHealthChecks(set *ServeHandlerSet, in ServeHandlerSetBuildInput) {
	set.HealthHandler = handlers.NewHealthHandler(in.Logger)
	if in.DB != nil {
		set.HealthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("database", func(ctx context.Context) error {
			sqlDB, err := in.DB.DB()
			if err != nil {
				return err
			}
			return sqlDB.PingContext(ctx)
		}))
	}
	if in.MongoClient != nil {
		set.HealthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("mongodb", func(ctx context.Context) error {
			return in.MongoClient.Ping(ctx)
		}))
	}
}
