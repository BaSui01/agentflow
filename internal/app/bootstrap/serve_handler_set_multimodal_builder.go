package bootstrap

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/llm/observability"
	"go.uber.org/zap"
)

func buildServeMultimodal(set *ServeHandlerSet, in ServeHandlerSetBuildInput, llmRuntime *LLMHandlerRuntime) error {
	if !in.Cfg.Multimodal.Enabled {
		in.Logger.Info("Multimodal framework handler disabled by config")
		return nil
	}
	if _, err := ValidateMultimodalReferenceBackend(in.Cfg); err != nil {
		return err
	}

	redisClient, referenceStore, err := BuildMultimodalRedisReferenceStore(
		in.Cfg,
		in.Cfg.Multimodal.ReferenceStoreKeyPrefix,
		in.Cfg.Multimodal.ReferenceTTL,
		in.Logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize multimodal redis reference store: %w", err)
	}
	set.MultimodalRedis = redisClient
	set.HealthHandler.RegisterCheck(handlers.NewRedisHealthCheck("redis", func(ctx context.Context) error {
		return set.MultimodalRedis.Ping(ctx).Err()
	}))

	var ledger observability.Ledger
	if llmRuntime != nil {
		ledger = llmRuntime.Ledger
	}
	multimodalRuntime, err := BuildMultimodalRuntime(
		in.Cfg,
		set.Provider,
		set.BudgetManager,
		ledger,
		referenceStore,
		in.Logger,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize multimodal runtime: %w", err)
	}
	set.MultimodalHandler = multimodalRuntime.Handler
	in.Logger.Info("Multimodal framework handler initialized",
		zap.String("reference_store_backend", multimodalRuntime.ReferenceBackend),
		zap.Int("image_provider_count", multimodalRuntime.ImageProviderCount),
		zap.Int("video_provider_count", multimodalRuntime.VideoProviderCount),
		zap.Int64("reference_max_size_bytes", in.Cfg.Multimodal.ReferenceMaxSizeBytes),
		zap.Duration("reference_ttl", in.Cfg.Multimodal.ReferenceTTL),
	)
	return nil
}
