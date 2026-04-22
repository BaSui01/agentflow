package checkpoint

import (
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

func checkpointLogger(logger *zap.Logger, store string) *zap.Logger {
	if logger == nil {
		logger = zap.NewNop()
	}
	return logger.With(zap.String("store", store))
}

func nextCheckpointID() string {
	counter := atomic.AddUint64(&checkpointIDCounter, 1)
	return fmt.Sprintf("ckpt_%d%d", time.Now().UnixNano(), counter)
}

var checkpointIDCounter uint64
