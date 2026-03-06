package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/cache"
	llmmw "github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/llm/providers/vendor"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// LLMHandlerRuntime groups LLM-related runtime dependencies used by API handlers.
type LLMHandlerRuntime struct {
	Provider      llm.Provider
	ToolProvider  llm.Provider
	BudgetManager *llmpolicy.TokenBudgetManager
	CostTracker   *observability.CostTracker
	Ledger        observability.Ledger
	Cache         *cache.MultiLevelCache
	Metrics       *observability.Metrics
	PolicyManager *llmpolicy.Manager
}

// BuildLLMHandlerRuntime creates the LLM runtime required by handler layer.
// The main provider entry is multi-provider router + routed provider.
func BuildLLMHandlerRuntime(cfg *config.Config, db *gorm.DB, logger *zap.Logger) (*LLMHandlerRuntime, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required for multi-provider router runtime")
	}

	baseProvider, err := buildRoutedMainProvider(cfg, db, logger)
	if err != nil {
		return nil, err
	}

	retryPolicy := llmpolicy.DefaultRetryPolicy()
	if cfg.LLM.MaxRetries >= 0 {
		retryPolicy.MaxRetries = cfg.LLM.MaxRetries
	}

	var provider llm.Provider = llm.NewResilientProvider(baseProvider, &llm.ResilientConfig{
		RetryPolicy:       retryPolicy,
		CircuitBreaker:    llm.DefaultCircuitBreakerConfig(),
		EnableIdempotency: true,
		IdempotencyTTL:    time.Hour,
	}, logger)

	var llmMetrics *observability.Metrics
	if metrics, mErr := observability.NewMetrics(); mErr != nil {
		logger.Warn("Failed to create LLM metrics", zap.Error(mErr))
	} else {
		llmMetrics = metrics
	}

	costTracker := observability.NewCostTracker(observability.NewCostCalculator())
	ledger := observability.NewCostTrackerLedger(costTracker)

	var budgetManager *llmpolicy.TokenBudgetManager
	if cfg.Budget.Enabled {
		budgetManager = llmpolicy.NewTokenBudgetManager(llmpolicy.BudgetConfig{
			MaxTokensPerMinute: cfg.Budget.MaxTokensPerMinute,
			MaxTokensPerDay:    cfg.Budget.MaxTokensPerDay,
			MaxCostPerDay:      cfg.Budget.MaxCostPerDay,
			AlertThreshold:     cfg.Budget.AlertThreshold,
		}, logger)
		logger.Info("Budget manager initialized")
	}

	policyManager := llmpolicy.NewManager(llmpolicy.ManagerConfig{
		Budget:      budgetManager,
		RetryPolicy: retryPolicy,
	})

	var llmCache *cache.MultiLevelCache
	if cfg.Cache.Enabled {
		llmCache = cache.NewMultiLevelCache(nil, &cache.CacheConfig{
			LocalMaxSize:    cfg.Cache.LocalMaxSize,
			LocalTTL:        cfg.Cache.LocalTTL,
			EnableLocal:     true,
			EnableRedis:     cfg.Cache.EnableRedis,
			RedisTTL:        cfg.Cache.RedisTTL,
			KeyStrategyType: cfg.Cache.KeyStrategy,
		}, logger)
		logger.Info("LLM cache initialized")
	}

	chain := llmmw.NewChain(
		llmmw.RecoveryMiddleware(func(v any) {
			logger.Error("LLM middleware panic recovered", zap.Any("panic", v))
		}),
		llmmw.LoggingMiddleware(logger.Sugar().Infof),
		llmmw.TimeoutMiddleware(cfg.LLM.Timeout),
	)
	if llmMetrics != nil {
		chain.Use(llmmw.MetricsMiddleware(&llmmw.OtelMetricsAdapter{Metrics: llmMetrics}))
	}
	if llmCache != nil {
		chain.Use(llmmw.CacheMiddleware(&llmmw.PromptCacheAdapter{Cache: llmCache}))
	}
	cleaner := llmmw.NewEmptyToolsCleaner()
	chain.UseFront(llmmw.TransformMiddleware(func(req *llm.ChatRequest) {
		if req != nil {
			if _, err := cleaner.Rewrite(context.Background(), req); err != nil {
				logger.Warn("empty tools cleaner rewrite failed", zap.Error(err))
			}
		}
	}, nil))

	provider = llmmw.NewMiddlewareProvider(provider, chain)
	toolProvider := buildToolProviderOrFallback(cfg, logger, provider)

	return &LLMHandlerRuntime{
		Provider:      provider,
		ToolProvider:  toolProvider,
		BudgetManager: budgetManager,
		CostTracker:   costTracker,
		Ledger:        ledger,
		Cache:         llmCache,
		Metrics:       llmMetrics,
		PolicyManager: policyManager,
	}, nil
}

func buildRoutedMainProvider(cfg *config.Config, db *gorm.DB, logger *zap.Logger) (llm.Provider, error) {
	factory := &vendorRouterProviderFactory{
		timeout: cfg.LLM.Timeout,
		logger:  logger,
	}
	router := llmrouter.NewMultiProviderRouter(db, factory, llmrouter.RouterOptions{Logger: logger})
	if err := router.InitAPIKeyPools(context.Background()); err != nil {
		router.Stop()
		return nil, fmt.Errorf("failed to initialize llm router api key pools: %w", err)
	}
	if len(router.GetAPIKeyStats()) == 0 {
		router.Stop()
		return nil, fmt.Errorf("no active provider api keys found in llm router pool")
	}

	logger.Info("LLM main provider initialized with multi-provider router")
	return llmrouter.NewRoutedChatProvider(router, llmrouter.RoutedChatProviderOptions{
		DefaultStrategy: llmrouter.StrategyQPSBased,
		Logger:          logger,
	}), nil
}

type vendorRouterProviderFactory struct {
	timeout time.Duration
	logger  *zap.Logger
}

func (f *vendorRouterProviderFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (llmrouter.Provider, error) {
	return vendor.NewChatProviderFromConfig(providerCode, vendor.ChatProviderConfig{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: strings.TrimSpace(baseURL),
		Timeout: f.timeout,
	}, f.logger)
}

func buildToolProviderOrFallback(cfg *config.Config, logger *zap.Logger, mainProvider llm.Provider) llm.Provider {
	if mainProvider == nil {
		return nil
	}

	toolProviderCode := strings.TrimSpace(firstNonEmpty(cfg.LLM.ToolProvider, cfg.LLM.DefaultProvider))
	toolAPIKey := strings.TrimSpace(firstNonEmpty(cfg.LLM.ToolAPIKey, cfg.LLM.APIKey))
	toolBaseURL := strings.TrimSpace(firstNonEmpty(cfg.LLM.ToolBaseURL, cfg.LLM.BaseURL))
	toolTimeout := cfg.LLM.ToolTimeout
	if toolTimeout <= 0 {
		toolTimeout = cfg.LLM.Timeout
	}

	toolRetryPolicy := llmpolicy.DefaultRetryPolicy()
	toolMaxRetries := cfg.LLM.ToolMaxRetries
	if toolMaxRetries == 0 {
		toolMaxRetries = cfg.LLM.MaxRetries
	}
	if toolMaxRetries >= 0 {
		toolRetryPolicy.MaxRetries = toolMaxRetries
	}

	toolCfgUnspecified := strings.TrimSpace(cfg.LLM.ToolProvider) == "" &&
		strings.TrimSpace(cfg.LLM.ToolAPIKey) == "" &&
		strings.TrimSpace(cfg.LLM.ToolBaseURL) == "" &&
		cfg.LLM.ToolTimeout == 0 &&
		cfg.LLM.ToolMaxRetries == 0
	if toolCfgUnspecified {
		logger.Info("Tool provider uses main provider (no explicit tool LLM config)")
		return mainProvider
	}

	toolProvider, err := vendor.NewChatProviderFromConfig(toolProviderCode, vendor.ChatProviderConfig{
		APIKey:  toolAPIKey,
		BaseURL: toolBaseURL,
		Timeout: toolTimeout,
	}, logger)
	if err != nil {
		logger.Warn("Failed to create tool LLM provider, fallback to main provider",
			zap.String("provider", toolProviderCode),
			zap.Error(err))
		return mainProvider
	}

	toolProvider = llm.NewResilientProvider(toolProvider, &llm.ResilientConfig{
		RetryPolicy:       toolRetryPolicy,
		CircuitBreaker:    llm.DefaultCircuitBreakerConfig(),
		EnableIdempotency: true,
		IdempotencyTTL:    time.Hour,
	}, logger)

	logger.Info("Tool LLM provider initialized",
		zap.String("provider", toolProviderCode),
		zap.Duration("timeout", toolTimeout),
		zap.Int("max_retries", toolRetryPolicy.MaxRetries))
	return toolProvider
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
