package checkpoint

import (
	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"

	"go.uber.org/zap"
)

func checkpointLogger(logger *zap.Logger, store string) *zap.Logger {
	return checkpointcore.Logger(logger, store)
}

func nextCheckpointID() string {
	return checkpointcore.NextCheckpointID(&checkpointIDCounter)
}

var checkpointIDCounter uint64
