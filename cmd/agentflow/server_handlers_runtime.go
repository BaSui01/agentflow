package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/protocol/mcp"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/internal/bridge"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/budget"
	"github.com/BaSui01/agentflow/llm/cache"
	llmfactory "github.com/BaSui01/agentflow/llm/factory"
	llmmw "github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/dsl"
	"go.uber.org/zap"
)

func (s *Server) initHandlers() error {
	s.healthHandler = handlers.NewHealthHandler(s.logger)

	if s.cfg.LLM.APIKey != "" {
		provider, err := llmfactory.NewProviderFromConfig(s.cfg.LLM.DefaultProvider, llmfactory.ProviderConfig{
			APIKey:  s.cfg.LLM.APIKey,
			BaseURL: s.cfg.LLM.BaseURL,
			Timeout: s.cfg.LLM.Timeout,
		}, s.logger)
		if err != nil {
			s.logger.Warn("Failed to create LLM provider, chat endpoints disabled",
				zap.String("provider", s.cfg.LLM.DefaultProvider),
				zap.Error(err))
		} else {
			retryPolicy := llm.DefaultRetryPolicy()
			if s.cfg.LLM.MaxRetries >= 0 {
				retryPolicy.MaxRetries = s.cfg.LLM.MaxRetries
			}
			provider = llm.NewResilientProvider(provider, &llm.ResilientConfig{
				RetryPolicy:       retryPolicy,
				CircuitBreaker:    llm.DefaultCircuitBreakerConfig(),
				EnableIdempotency: true,
				IdempotencyTTL:    time.Hour,
			}, s.logger)

			if llmMetrics, mErr := observability.NewMetrics(); mErr != nil {
				s.logger.Warn("Failed to create LLM metrics", zap.Error(mErr))
			} else {
				s.llmMetrics = llmMetrics
			}
			costCalc := observability.NewCostCalculator()
			s.costTracker = observability.NewCostTracker(costCalc)

			if s.cfg.Budget.Enabled {
				budgetCfg := budget.BudgetConfig{
					MaxTokensPerMinute: s.cfg.Budget.MaxTokensPerMinute,
					MaxTokensPerDay:    s.cfg.Budget.MaxTokensPerDay,
					MaxCostPerDay:      s.cfg.Budget.MaxCostPerDay,
					AlertThreshold:     s.cfg.Budget.AlertThreshold,
				}
				s.budgetManager = budget.NewTokenBudgetManager(budgetCfg, s.logger)
				s.logger.Info("Budget manager initialized")
			}

			if s.cfg.Cache.Enabled {
				cacheCfg := &cache.CacheConfig{
					LocalMaxSize:    s.cfg.Cache.LocalMaxSize,
					LocalTTL:        s.cfg.Cache.LocalTTL,
					EnableLocal:     true,
					EnableRedis:     s.cfg.Cache.EnableRedis,
					RedisTTL:        s.cfg.Cache.RedisTTL,
					KeyStrategyType: s.cfg.Cache.KeyStrategy,
				}
				s.llmCache = cache.NewMultiLevelCache(nil, cacheCfg, s.logger)
				s.logger.Info("LLM cache initialized")
			}

			chain := llmmw.NewChain(
				llmmw.RecoveryMiddleware(func(v any) {
					s.logger.Error("LLM middleware panic recovered", zap.Any("panic", v))
				}),
				llmmw.LoggingMiddleware(s.logger.Sugar().Infof),
				llmmw.TimeoutMiddleware(s.cfg.LLM.Timeout),
			)
			if s.llmMetrics != nil {
				chain.Use(llmmw.MetricsMiddleware(&llmmw.OtelMetricsAdapter{Metrics: s.llmMetrics}))
			}
			if s.llmCache != nil {
				chain.Use(llmmw.CacheMiddleware(&llmmw.PromptCacheAdapter{Cache: s.llmCache}))
			}
			cleaner := llmmw.NewEmptyToolsCleaner()
			chain.UseFront(llmmw.TransformMiddleware(func(req *llm.ChatRequest) {
				if req != nil {
					_, _ = cleaner.Rewrite(context.Background(), req)
				}
			}, nil))

			provider = llmmw.NewMiddlewareProvider(provider, chain)

			s.provider = provider
			s.chatHandler = handlers.NewChatHandler(provider, s.logger)
			s.logger.Info("Chat handler initialized with middleware chain",
				zap.String("provider", s.cfg.LLM.DefaultProvider))
		}
	} else {
		s.logger.Info("LLM API key not configured, chat endpoints disabled")
	}

	discoveryRegistry := discovery.NewCapabilityRegistry(nil, s.logger)
	agentRegistry := agent.NewAgentRegistry(s.logger)

	bridgeAdapter := bridge.NewDiscoveryRegistrarAdapter(discoveryRegistry)
	_ = bridgeAdapter

	if s.provider != nil {
		s.resolver = agent.NewCachingResolver(agentRegistry, s.provider, s.logger)

		if err := s.wireMongoStores(s.resolver, discoveryRegistry); err != nil {
			return fmt.Errorf("failed to wire MongoDB stores: %w", err)
		}

		s.wireDefaultRuntimeAgent(agentRegistry)

		s.agentHandler = handlers.NewAgentHandler(discoveryRegistry, agentRegistry, s.logger, s.resolver.Resolve)
		s.logger.Info("Agent handler initialized with resolver")
	} else {
		s.agentHandler = handlers.NewAgentHandler(discoveryRegistry, agentRegistry, s.logger)
		s.logger.Info("Agent handler initialized without resolver (no LLM provider)")
	}

	if s.db != nil {
		store := handlers.NewGormAPIKeyStore(s.db)
		s.apiKeyHandler = handlers.NewAPIKeyHandler(store, s.logger)
		s.logger.Info("API key handler initialized")
	} else {
		s.logger.Info("Database not available, API key management disabled")
	}

	if s.cfg.Multimodal.Enabled {
		backend := strings.ToLower(strings.TrimSpace(s.cfg.Multimodal.ReferenceStoreBackend))
		if backend != "redis" {
			return fmt.Errorf("multimodal.reference_store_backend must be redis")
		}

		referenceStore, err := s.newMultimodalRedisReferenceStore(
			s.cfg.Multimodal.ReferenceStoreKeyPrefix,
			s.cfg.Multimodal.ReferenceTTL,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize multimodal redis reference store: %w", err)
		}

		multimodalCfg := handlers.MultimodalHandlerConfig{
			ChatProvider:         s.provider,
			OpenAIAPIKey:         firstNonEmpty(s.cfg.Multimodal.Image.OpenAIAPIKey, s.cfg.LLM.APIKey),
			OpenAIBaseURL:        firstNonEmpty(s.cfg.Multimodal.Image.OpenAIBaseURL, s.cfg.LLM.BaseURL),
			GoogleAPIKey:         firstNonEmpty(s.cfg.Multimodal.Video.GoogleAPIKey, s.cfg.Multimodal.Image.GeminiAPIKey),
			GoogleBaseURL:        s.cfg.Multimodal.Video.GoogleBaseURL,
			RunwayAPIKey:         s.cfg.Multimodal.Video.RunwayAPIKey,
			RunwayBaseURL:        s.cfg.Multimodal.Video.RunwayBaseURL,
			VeoAPIKey:            s.cfg.Multimodal.Video.VeoAPIKey,
			VeoBaseURL:           s.cfg.Multimodal.Video.VeoBaseURL,
			SoraAPIKey:           s.cfg.Multimodal.Video.SoraAPIKey,
			SoraBaseURL:          s.cfg.Multimodal.Video.SoraBaseURL,
			KlingAPIKey:          s.cfg.Multimodal.Video.KlingAPIKey,
			KlingBaseURL:         s.cfg.Multimodal.Video.KlingBaseURL,
			LumaAPIKey:           s.cfg.Multimodal.Video.LumaAPIKey,
			LumaBaseURL:          s.cfg.Multimodal.Video.LumaBaseURL,
			MiniMaxAPIKey:        s.cfg.Multimodal.Video.MiniMaxAPIKey,
			MiniMaxBaseURL:       s.cfg.Multimodal.Video.MiniMaxBaseURL,
			DefaultImageProvider: s.cfg.Multimodal.DefaultImageProvider,
			DefaultVideoProvider: s.cfg.Multimodal.DefaultVideoProvider,
			ReferenceMaxSize:     s.cfg.Multimodal.ReferenceMaxSizeBytes,
			ReferenceTTL:         s.cfg.Multimodal.ReferenceTTL,
			ReferenceStore:       referenceStore,
		}
		s.multimodalHandler = handlers.NewMultimodalHandlerFromConfig(multimodalCfg, s.logger)

		imageProviderCount := 0
		videoProviderCount := 0
		if multimodalCfg.OpenAIAPIKey != "" {
			imageProviderCount++
		}
		if multimodalCfg.GoogleAPIKey != "" {
			imageProviderCount++
			videoProviderCount++
		}
		if multimodalCfg.RunwayAPIKey != "" {
			videoProviderCount++
		}
		if multimodalCfg.VeoAPIKey != "" && multimodalCfg.GoogleAPIKey == "" {
			videoProviderCount++
		}
		if multimodalCfg.SoraAPIKey != "" {
			videoProviderCount++
		}
		if multimodalCfg.KlingAPIKey != "" {
			videoProviderCount++
		}
		if multimodalCfg.LumaAPIKey != "" {
			videoProviderCount++
		}
		if multimodalCfg.MiniMaxAPIKey != "" {
			videoProviderCount++
		}
		s.logger.Info("Multimodal framework handler initialized",
			zap.String("reference_store_backend", backend),
			zap.Int("image_provider_count", imageProviderCount),
			zap.Int("video_provider_count", videoProviderCount),
			zap.Int64("reference_max_size_bytes", multimodalCfg.ReferenceMaxSize),
			zap.Duration("reference_ttl", multimodalCfg.ReferenceTTL),
		)
	} else {
		s.logger.Info("Multimodal framework handler disabled by config")
	}

	mcpSrv := mcp.NewMCPServer("agentflow", "1.0.0", s.logger)
	a2aSrv := a2a.NewHTTPServer(&a2a.ServerConfig{Logger: s.logger})
	s.protocolHandler = handlers.NewProtocolHandler(mcpSrv, a2aSrv, s.logger)
	s.logger.Info("Protocol handler initialized (MCP + A2A)")

	s.initRAGHandler()

	dagExecutor := workflow.NewDAGExecutor(nil, s.logger)
	dslParser := dsl.NewParser()
	s.workflowHandler = handlers.NewWorkflowHandler(dagExecutor, dslParser, s.logger)
	s.logger.Info("Workflow handler initialized")

	s.logger.Info("Handlers initialized")
	return nil
}

func (s *Server) initRAGHandler() {
	if s.cfg.LLM.APIKey == "" {
		s.logger.Info("RAG handler disabled (no LLM API key for embedding)")
		return
	}

	embProvider, err := rag.NewEmbeddingProviderFromConfig(
		s.cfg,
		rag.EmbeddingProviderType(s.cfg.LLM.DefaultProvider),
	)
	if err != nil {
		s.logger.Warn("RAG handler disabled (failed to create embedding provider)",
			zap.String("provider", s.cfg.LLM.DefaultProvider),
			zap.Error(err))
		return
	}

	store := rag.NewInMemoryVectorStore(s.logger)
	s.ragHandler = handlers.NewRAGHandler(store, embProvider, s.logger)
	s.logger.Info("RAG handler initialized (in-memory store, embedding provider ready)",
		zap.String("provider", embProvider.Name()))
}
