package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/workflow"
	"gorm.io/gorm"
)

func BuildWorkflowPostgreSQLCheckpointStore(ctx context.Context, db *gorm.DB) (workflow.CheckpointStore, error) {
	if db == nil {
		return nil, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	return workflow.NewPostgreSQLCheckpointStore(ctx, sqlDB)
}
