package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/config"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// MainProviderBuilder constructs the handler runtime's main text provider chain
// for a configured startup mode.
type MainProviderBuilder func(ctx context.Context, cfg *config.Config, db *gorm.DB, logger *zap.Logger) (llm.Provider, error)

var (
	mainProviderBuildersMu sync.RWMutex
	mainProviderBuilders   = map[string]MainProviderBuilder{
		config.LLMMainProviderModeLegacy: buildLegacyMainProvider,
	}
)

// RegisterMainProviderBuilder registers or replaces a startup builder for a
// main-provider mode such as "legacy" or "channel_routed".
func RegisterMainProviderBuilder(mode string, builder MainProviderBuilder) error {
	normalizedMode := normalizeMainProviderMode(mode)
	if normalizedMode == "" {
		return fmt.Errorf("main provider mode is required")
	}
	if builder == nil {
		return fmt.Errorf("main provider builder %q is nil", normalizedMode)
	}

	mainProviderBuildersMu.Lock()
	defer mainProviderBuildersMu.Unlock()
	mainProviderBuilders[normalizedMode] = builder
	return nil
}

// UnregisterMainProviderBuilder removes a registered startup builder.
func UnregisterMainProviderBuilder(mode string) {
	normalizedMode := normalizeMainProviderMode(mode)
	if normalizedMode == "" {
		return
	}

	mainProviderBuildersMu.Lock()
	defer mainProviderBuildersMu.Unlock()
	delete(mainProviderBuilders, normalizedMode)
}

// BuildMainProvider resolves the configured builder mode and constructs the
// main text provider for runtime composition.
func BuildMainProvider(ctx context.Context, cfg *config.Config, db *gorm.DB, logger *zap.Logger) (llm.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required for llm main provider")
	}

	normalizedMode := normalizeMainProviderMode(cfg.LLM.MainProviderMode)

	mainProviderBuildersMu.RLock()
	builder, ok := mainProviderBuilders[normalizedMode]
	mainProviderBuildersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no main provider builder registered for mode %q", normalizedMode)
	}

	provider, err := builder(ctx, cfg, db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build llm main provider for mode %q: %w", normalizedMode, err)
	}
	return provider, nil
}

func buildLegacyMainProvider(ctx context.Context, cfg *config.Config, db *gorm.DB, logger *zap.Logger) (llm.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required for legacy multi-provider router runtime")
	}
	if db == nil {
		return nil, fmt.Errorf("database is required for legacy multi-provider router runtime")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	factory := llmrouter.VendorChatProviderFactory{
		Timeout: cfg.LLM.Timeout,
		Logger:  logger,
	}
	router := llmrouter.NewMultiProviderRouter(db, factory, llmrouter.RouterOptions{Logger: logger})
	if err := router.InitAPIKeyPools(ctx); err != nil {
		router.Stop()
		return nil, fmt.Errorf("failed to initialize llm router api key pools: %w", err)
	}
	if len(router.GetAPIKeyStats()) == 0 {
		router.Stop()
		return nil, fmt.Errorf("no active provider api keys found in llm router pool")
	}

	logger.Info("LLM main provider initialized",
		zap.String("mode", config.LLMMainProviderModeLegacy),
		zap.String("entry", "multi-provider-router"))

	return llmrouter.NewRoutedChatProvider(router, llmrouter.RoutedChatProviderOptions{
		DefaultStrategy: llmrouter.StrategyQPSBased,
		Logger:          logger,
	}), nil
}

func normalizeMainProviderMode(raw string) string {
	return strings.TrimSpace(config.NormalizeLLMMainProviderMode(raw))
}
