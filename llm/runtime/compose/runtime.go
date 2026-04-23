package compose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm/cache"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	llmmw "github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
	"go.uber.org/zap"
)

// Runtime groups the LLM-facing runtime dependencies assembled around a main
// provider chain.
type Runtime struct {
	Gateway       llmcore.Gateway
	ToolGateway   llmcore.Gateway
	Provider      llmcore.Provider
	ToolProvider  llmcore.Provider
	BudgetManager *llmpolicy.TokenBudgetManager
	CostTracker   *observability.CostTracker
	Ledger        observability.Ledger
	Cache         *cache.MultiLevelCache
	Metrics       *observability.Metrics
	PolicyManager *llmpolicy.Manager
}

// Config controls runtime composition around an already-constructed main
// provider. It is storage-agnostic and can be reused by external projects.
type Config struct {
	Timeout    time.Duration
	MaxRetries int
	Budget     BudgetConfig
	Cache      CacheConfig
	Tool       ToolProviderConfig
}

// BudgetConfig controls token and cost policy assembly.
type BudgetConfig struct {
	Enabled             bool
	MaxTokensPerRequest int
	MaxTokensPerMinute  int
	MaxTokensPerHour    int
	MaxTokensPerDay     int
	MaxCostPerRequest   float64
	MaxCostPerDay       float64
	AlertThreshold      float64
	AutoThrottle        bool
	ThrottleDelay       time.Duration
}

// CacheConfig controls prompt-cache assembly.
type CacheConfig struct {
	Enabled      bool
	LocalMaxSize int
	LocalTTL     time.Duration
	EnableRedis  bool
	RedisTTL     time.Duration
	KeyStrategy  string
}

// ToolProviderConfig describes an optional dedicated tool-calling provider. If
// the provider/api key/baseURL/timeout/max-retry fields are all left empty or
// zero, the runtime reuses the main provider for tools.
type ToolProviderConfig struct {
	Provider        string
	DefaultProvider string
	APIKey          string
	DefaultAPIKey   string
	BaseURL         string
	DefaultBaseURL  string
	Timeout         time.Duration
	MaxRetries      int
}

// Build assembles the shared runtime chain around the supplied main provider.
func Build(cfg Config, mainProvider llmcore.Provider, logger *zap.Logger) (*Runtime, error) {
	if mainProvider == nil {
		return nil, fmt.Errorf("main provider is required for llm runtime composition")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	retryPolicy := llmpolicy.DefaultRetryPolicy()
	if cfg.MaxRetries >= 0 {
		retryPolicy.MaxRetries = cfg.MaxRetries
	}

	var provider llmcore.Provider = llmcore.NewResilientProvider(mainProvider, &llmcore.ResilientConfig{
		RetryPolicy:       retryPolicy,
		CircuitBreaker:    llmcore.DefaultCircuitBreakerConfig(),
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
			MaxTokensPerRequest: cfg.Budget.MaxTokensPerRequest,
			MaxTokensPerMinute:  cfg.Budget.MaxTokensPerMinute,
			MaxTokensPerHour:    cfg.Budget.MaxTokensPerHour,
			MaxTokensPerDay:     cfg.Budget.MaxTokensPerDay,
			MaxCostPerRequest:   cfg.Budget.MaxCostPerRequest,
			MaxCostPerDay:       cfg.Budget.MaxCostPerDay,
			AlertThreshold:      cfg.Budget.AlertThreshold,
			AutoThrottle:        cfg.Budget.AutoThrottle,
			ThrottleDelay:       cfg.Budget.ThrottleDelay,
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
		llmmw.TimeoutMiddleware(cfg.Timeout),
	)
	if llmMetrics != nil {
		chain.Use(llmmw.MetricsMiddleware(&llmmw.OtelMetricsAdapter{Metrics: llmMetrics}))
	}
	if llmCache != nil {
		chain.Use(llmmw.CacheMiddleware(&llmmw.PromptCacheAdapter{Cache: llmCache}))
	}
	cleaner := llmmw.NewEmptyToolsCleaner()
	chain.UseFront(llmmw.TransformMiddleware(func(req *llmcore.ChatRequest) {
		if req != nil {
			if _, err := cleaner.Rewrite(context.Background(), req); err != nil {
				logger.Warn("empty tools cleaner rewrite failed", zap.Error(err))
			}
		}
	}, nil))

	provider = llmmw.NewMiddlewareProvider(provider, chain)
	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider:  provider,
		Ledger:        ledger,
		PolicyManager: policyManager,
		Logger:        logger,
	})
	providerAdapter := llmgateway.NewChatProviderAdapter(gateway, provider)
	toolProvider := buildToolProviderOrFallback(cfg, logger, provider)
	toolProviderAdapter := providerAdapter
	toolGateway := gateway
	if toolProvider != nil && toolProvider != provider {
		toolGateway = llmgateway.New(llmgateway.Config{
			ChatProvider:  toolProvider,
			Ledger:        ledger,
			PolicyManager: policyManager,
			Logger:        logger,
		})
		toolProviderAdapter = llmgateway.NewChatProviderAdapter(toolGateway, toolProvider)
	}

	return &Runtime{
		Gateway:       gateway,
		ToolGateway:   toolGateway,
		Provider:      providerAdapter,
		ToolProvider:  toolProviderAdapter,
		BudgetManager: budgetManager,
		CostTracker:   costTracker,
		Ledger:        ledger,
		Cache:         llmCache,
		Metrics:       llmMetrics,
		PolicyManager: policyManager,
	}, nil
}

func buildToolProviderOrFallback(cfg Config, logger *zap.Logger, mainProvider llmcore.Provider) llmcore.Provider {
	if mainProvider == nil {
		return nil
	}

	if toolProviderUnspecified(cfg.Tool) {
		logger.Info("Tool provider uses main provider (no explicit tool LLM config)")
		return mainProvider
	}

	toolProviderCode := strings.TrimSpace(firstNonEmpty(cfg.Tool.Provider, cfg.Tool.DefaultProvider))
	toolAPIKey := strings.TrimSpace(firstNonEmpty(cfg.Tool.APIKey, cfg.Tool.DefaultAPIKey))
	toolBaseURL := strings.TrimSpace(firstNonEmpty(cfg.Tool.BaseURL, cfg.Tool.DefaultBaseURL))
	toolTimeout := cfg.Tool.Timeout
	if toolTimeout <= 0 {
		toolTimeout = cfg.Timeout
	}

	toolRetryPolicy := llmpolicy.DefaultRetryPolicy()
	toolMaxRetries := cfg.Tool.MaxRetries
	if toolMaxRetries == 0 {
		toolMaxRetries = cfg.MaxRetries
	}
	if toolMaxRetries >= 0 {
		toolRetryPolicy.MaxRetries = toolMaxRetries
	}

	toolFactory := llmrouter.VendorChatProviderFactory{
		Timeout: toolTimeout,
		Logger:  logger,
	}
	toolProvider, err := toolFactory.CreateProvider(toolProviderCode, toolAPIKey, toolBaseURL)
	if err != nil {
		logger.Warn("Failed to create tool LLM provider, fallback to main provider",
			zap.String("provider", toolProviderCode),
			zap.Error(err))
		return mainProvider
	}

	toolProvider = llmcore.NewResilientProvider(toolProvider, &llmcore.ResilientConfig{
		RetryPolicy:       toolRetryPolicy,
		CircuitBreaker:    llmcore.DefaultCircuitBreakerConfig(),
		EnableIdempotency: true,
		IdempotencyTTL:    time.Hour,
	}, logger)

	logger.Info("Tool LLM provider initialized",
		zap.String("provider", toolProviderCode),
		zap.Duration("timeout", toolTimeout),
		zap.Int("max_retries", toolRetryPolicy.MaxRetries))
	return toolProvider
}

func toolProviderUnspecified(cfg ToolProviderConfig) bool {
	return strings.TrimSpace(cfg.Provider) == "" &&
		strings.TrimSpace(cfg.APIKey) == "" &&
		strings.TrimSpace(cfg.BaseURL) == "" &&
		cfg.Timeout == 0 &&
		cfg.MaxRetries == 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
