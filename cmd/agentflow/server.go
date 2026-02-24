package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/evaluation"
	"github.com/BaSui01/agentflow/agent/memory"
	mongostore "github.com/BaSui01/agentflow/agent/persistence/mongodb"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	llmfactory "github.com/BaSui01/agentflow/llm/factory"
	"github.com/BaSui01/agentflow/llm/tools"
	"github.com/BaSui01/agentflow/pkg/metrics"
	mw "github.com/BaSui01/agentflow/pkg/middleware"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"github.com/BaSui01/agentflow/pkg/server"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"github.com/BaSui01/agentflow/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// =============================================================================
// 🖥️ Server 结构（重构版）
// =============================================================================

// Server 是 AgentFlow 的主服务器
type Server struct {
	cfg        *config.Config
	configPath string
	logger     *zap.Logger

	// OpenTelemetry providers
	telemetry *telemetry.Providers

	// Database (optional — nil when DB is unavailable)
	db *gorm.DB

	// MongoDB (required — document store for prompts, conversations, runs, audit)
	mongoClient *mongoclient.Client

	// 服务器管理器
	httpManager    *server.Manager
	metricsManager *server.Manager

	// Handlers
	healthHandler *handlers.HealthHandler
	chatHandler   *handlers.ChatHandler
	agentHandler  *handlers.AgentHandler
	apiKeyHandler *handlers.APIKeyHandler

	// 指标收集器
	metricsCollector *metrics.Collector

	// 热更新管理器
	hotReloadManager *config.HotReloadManager
	configAPIHandler *config.ConfigAPIHandler

	// Rate limiter 生命周期管理
	rateLimiterCancel       context.CancelFunc
	tenantRateLimiterCancel context.CancelFunc

	// LLM provider (nil when API key not configured)
	provider llm.Provider

	// Agent resolver (nil when no LLM provider)
	resolver *agent.CachingResolver

	// AuditLogger for tool-level audit logging (backed by MongoDB)
	auditLogger *tools.DefaultAuditLogger

	// AB testing (backed by MongoDB experiment store)
	abTester *evaluation.ABTester

	// Enhanced memory system (backed by MongoDB stores)
	enhancedMemory *memory.EnhancedMemorySystem

	wg sync.WaitGroup
}

// NewServer 创建新的服务器实例
func NewServer(cfg *config.Config, configPath string, logger *zap.Logger, tp *telemetry.Providers, db *gorm.DB) *Server {
	return &Server{
		cfg:        cfg,
		configPath: configPath,
		logger:     logger,
		telemetry:  tp,
		db:         db,
	}
}

// =============================================================================
// 🚀 启动流程
// =============================================================================

// Start 启动所有服务
func (s *Server) Start() error {
	// 1. 初始化指标收集器
	s.metricsCollector = metrics.NewCollector("agentflow", s.logger)

	// 2. 初始化 MongoDB（必需）
	if err := s.initMongoDB(); err != nil {
		return fmt.Errorf("failed to init MongoDB: %w", err)
	}

	// 3. 初始化 Handlers
	if err := s.initHandlers(); err != nil {
		return fmt.Errorf("failed to init handlers: %w", err)
	}

	// 4. 初始化热更新管理器
	if err := s.initHotReloadManager(); err != nil {
		return fmt.Errorf("failed to init hot reload manager: %w", err)
	}

	// 5. 启动 HTTP 服务器
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// 6. 启动 Metrics 服务器
	if err := s.startMetricsServer(); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	s.logger.Info("All servers started",
		zap.Int("http_port", s.cfg.Server.HTTPPort),
		zap.Int("metrics_port", s.cfg.Server.MetricsPort),
		zap.Bool("hot_reload_enabled", s.configPath != ""),
	)

	return nil
}

// =============================================================================
// 🔧 初始化方法
// =============================================================================

// initMongoDB initializes the MongoDB client.
// MongoDB is required — startup fails if connection cannot be established.
func (s *Server) initMongoDB() error {
	client, err := mongoclient.NewClient(s.cfg.MongoDB, s.logger)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	s.mongoClient = client
	s.logger.Info("MongoDB client initialized",
		zap.String("database", s.cfg.MongoDB.Database),
	)
	return nil
}

// wireMongoStores creates MongoDB stores and wires them into the agent resolver,
// discovery registry, evaluation system, and enhanced memory system.
// Core stores (prompt, conversation, run, audit) are required — any failure is fatal.
// Extended stores (memory, episodic, knowledge graph, experiment, registry) are
// optional — failures are logged but do not prevent startup.
func (s *Server) wireMongoStores(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// PromptStore
	promptStore, err := mongostore.NewPromptStore(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB prompt store: %w", err)
	}
	resolver.WithPromptStore(mongostore.NewPromptStoreAdapter(promptStore))
	s.logger.Info("MongoDB prompt store initialized")

	// ConversationStore
	convStore, err := mongostore.NewConversationStore(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB conversation store: %w", err)
	}
	resolver.WithConversationStore(mongostore.NewConversationStoreAdapter(convStore))
	s.logger.Info("MongoDB conversation store initialized")

	// RunStore
	runStore, err := mongostore.NewRunStore(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB run store: %w", err)
	}
	resolver.WithRunStore(mongostore.NewRunStoreAdapter(runStore))
	s.logger.Info("MongoDB run store initialized")

	// AuditBackend — create and wire into an AuditLogger stored on Server.
	auditBackend, err := mongostore.NewAuditBackend(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB audit backend: %w", err)
	}
	s.auditLogger = tools.NewAuditLogger(&tools.AuditLoggerConfig{
		Backends: []tools.AuditBackend{auditBackend},
	}, s.logger)
	s.logger.Info("MongoDB audit backend initialized")

	// --- Extended stores (optional — failures are non-fatal) ---

	// MemoryStore — short-term memory backed by MongoDB
	memoryStore, err := mongostore.NewMemoryStore(ctx, s.mongoClient)
	if err != nil {
		s.logger.Warn("failed to create MongoDB memory store, enhanced memory disabled", zap.Error(err))
	}

	// EpisodicStore — episodic memory backed by MongoDB
	var episodicStore *mongostore.MongoEpisodicStore
	if memoryStore != nil {
		episodicStore, err = mongostore.NewEpisodicStore(ctx, s.mongoClient)
		if err != nil {
			s.logger.Warn("failed to create MongoDB episodic store", zap.Error(err))
		}
	}

	// KnowledgeGraph — semantic memory backed by MongoDB
	var knowledgeGraph *mongostore.MongoKnowledgeGraph
	if memoryStore != nil {
		knowledgeGraph, err = mongostore.NewKnowledgeGraph(ctx, s.mongoClient)
		if err != nil {
			s.logger.Warn("failed to create MongoDB knowledge graph", zap.Error(err))
		}
	}

	// Wire EnhancedMemorySystem when at least the memory store is available.
	if memoryStore != nil {
		memCfg := memory.DefaultEnhancedMemoryConfig()
		memCfg.EpisodicEnabled = episodicStore != nil
		memCfg.SemanticEnabled = knowledgeGraph != nil

		working := memory.NewInMemoryMemoryStore(memory.InMemoryMemoryStoreConfig{
			MaxEntries: memCfg.WorkingMemorySize,
		}, s.logger)

		var episodic memory.EpisodicStore
		if episodicStore != nil {
			episodic = episodicStore
		}
		var semantic memory.KnowledgeGraph
		if knowledgeGraph != nil {
			semantic = knowledgeGraph
		}

		s.enhancedMemory = memory.NewEnhancedMemorySystem(
			memoryStore, working, nil, episodic, semantic, memCfg, s.logger,
		)
		s.logger.Info("MongoDB enhanced memory system initialized",
			zap.Bool("episodic", episodicStore != nil),
			zap.Bool("semantic", knowledgeGraph != nil),
		)
	}

	// ExperimentStore — A/B testing backed by MongoDB
	expStore, err := mongostore.NewExperimentStore(ctx, s.mongoClient)
	if err != nil {
		s.logger.Warn("failed to create MongoDB experiment store, A/B testing disabled", zap.Error(err))
	} else {
		s.abTester = evaluation.NewABTester(expStore, s.logger)
		s.logger.Info("MongoDB experiment store initialized")
	}

	// RegistryStore — agent discovery persistence backed by MongoDB
	regStore, err := mongostore.NewRegistryStore(ctx, s.mongoClient)
	if err != nil {
		s.logger.Warn("failed to create MongoDB registry store, discovery persistence disabled", zap.Error(err))
	} else {
		discoveryRegistry.SetStore(regStore)
		s.logger.Info("MongoDB registry store initialized")
	}

	return nil
}

// initHandlers 初始化所有 handlers
func (s *Server) initHandlers() error {
	// 健康检查 handler
	s.healthHandler = handlers.NewHealthHandler(s.logger)

	// 初始化 LLM Provider（使用工厂函数）
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
			s.provider = provider
			s.chatHandler = handlers.NewChatHandler(provider, s.logger)
			s.logger.Info("Chat handler initialized",
				zap.String("provider", s.cfg.LLM.DefaultProvider))
		}
	} else {
		s.logger.Info("LLM API key not configured, chat endpoints disabled")
	}

	// Initialize agent handler with discovery registry and agent registry.
	// When an LLM provider is available, wire a resolver so execute/stream/plan
	// endpoints can create agent instances on demand instead of returning 501.
	discoveryRegistry := discovery.NewCapabilityRegistry(nil, s.logger)
	agentRegistry := agent.NewAgentRegistry(s.logger)

	if s.provider != nil {
		s.resolver = agent.NewCachingResolver(agentRegistry, s.provider, s.logger)

		// Wire MongoDB persistence stores into the resolver (required).
		if err := s.wireMongoStores(s.resolver); err != nil {
			return fmt.Errorf("failed to wire MongoDB stores: %w", err)
		}

		s.agentHandler = handlers.NewAgentHandler(discoveryRegistry, agentRegistry, s.logger, s.resolver.Resolve)
		s.logger.Info("Agent handler initialized with resolver")
	} else {
		s.agentHandler = handlers.NewAgentHandler(discoveryRegistry, agentRegistry, s.logger)
		s.logger.Info("Agent handler initialized without resolver (no LLM provider)")
	}

	// Initialize API key handler when database is available
	if s.db != nil {
		store := handlers.NewGormAPIKeyStore(s.db)
		s.apiKeyHandler = handlers.NewAPIKeyHandler(store, s.logger)
		s.logger.Info("API key handler initialized")
	} else {
		s.logger.Info("Database not available, API key management disabled")
	}

	s.logger.Info("Handlers initialized")
	return nil
}

// initHotReloadManager 初始化热更新管理器
func (s *Server) initHotReloadManager() error {
	opts := []config.HotReloadOption{
		config.WithHotReloadLogger(s.logger),
	}

	if s.configPath != "" {
		opts = append(opts, config.WithConfigPath(s.configPath))
	}

	s.hotReloadManager = config.NewHotReloadManager(s.cfg, opts...)

	// 注册配置变更回调
	s.hotReloadManager.OnChange(func(change config.ConfigChange) {
		s.logger.Info("Configuration changed",
			zap.String("path", change.Path),
			zap.String("source", change.Source),
			zap.Bool("requires_restart", change.RequiresRestart),
		)
	})

	// 注册配置重载回调
	s.hotReloadManager.OnReload(func(oldConfig, newConfig *config.Config) {
		s.logger.Info("Configuration reloaded")
		s.cfg = newConfig
	})

	// 启动热更新管理器
	ctx := context.Background()
	if err := s.hotReloadManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start hot reload manager: %w", err)
	}

	// 创建配置 API 处理器
	s.configAPIHandler = config.NewConfigAPIHandler(s.hotReloadManager)

	return nil
}

// =============================================================================
// 🌐 HTTP 服务器
// =============================================================================

// startHTTPServer 启动 HTTP 服务器（使用新的 handlers）
func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()

	// ========================================
	// 健康检查端点（使用新的 HealthHandler）
	// ========================================
	mux.HandleFunc("/health", s.healthHandler.HandleHealth)
	mux.HandleFunc("/healthz", s.healthHandler.HandleHealthz)
	mux.HandleFunc("/ready", s.healthHandler.HandleReady)
	mux.HandleFunc("/readyz", s.healthHandler.HandleReady)

	// 版本信息端点
	mux.HandleFunc("/version", s.healthHandler.HandleVersion(Version, BuildTime, GitCommit))

	// ========================================
	// API 路由
	// ========================================
	if s.chatHandler != nil {
		mux.HandleFunc("/api/v1/chat/completions", s.chatHandler.HandleCompletion)
		mux.HandleFunc("/api/v1/chat/completions/stream", s.chatHandler.HandleStream)
		s.logger.Info("Chat API routes registered")
	}

	// Agent API routes
	if s.agentHandler != nil {
		mux.HandleFunc("/api/v1/agents", s.agentHandler.HandleListAgents)
		mux.HandleFunc("/api/v1/agents/execute", s.agentHandler.HandleExecuteAgent)
		mux.HandleFunc("/api/v1/agents/execute/stream", s.agentHandler.HandleAgentStream)
		mux.HandleFunc("/api/v1/agents/plan", s.agentHandler.HandlePlanAgent)
		mux.HandleFunc("/api/v1/agents/health", s.agentHandler.HandleAgentHealth)
		s.logger.Info("Agent API routes registered")
	}

	// Provider / API Key CRUD routes
	if s.apiKeyHandler != nil {
		mux.HandleFunc("/api/v1/providers", s.apiKeyHandler.HandleListProviders)
		mux.HandleFunc("/api/v1/providers/{id}/api-keys", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				s.apiKeyHandler.HandleListAPIKeys(w, r)
			case http.MethodPost:
				s.apiKeyHandler.HandleCreateAPIKey(w, r)
			default:
				handlers.WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", s.logger)
			}
		})
		mux.HandleFunc("/api/v1/providers/{id}/api-keys/stats", s.apiKeyHandler.HandleAPIKeyStats)
		mux.HandleFunc("/api/v1/providers/{id}/api-keys/{keyId}", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				s.apiKeyHandler.HandleUpdateAPIKey(w, r)
			case http.MethodDelete:
				s.apiKeyHandler.HandleDeleteAPIKey(w, r)
			default:
				handlers.WriteErrorMessage(w, http.StatusMethodNotAllowed, types.ErrInvalidRequest, "method not allowed", s.logger)
			}
		})
		s.logger.Info("Provider API key routes registered")
	}

	// ========================================
	// 配置管理 API（需要独立认证保护）
	// 安全修复：配置 API 是敏感的管理端点，必须经过认证中间件保护，
	// 不依赖全局中间件链的顺序，而是显式包装认证检查。
	// ========================================
	if s.configAPIHandler != nil {
		configAuth := config.NewConfigAPIMiddleware(s.configAPIHandler, s.getFirstAPIKey())
		mux.HandleFunc("/api/v1/config", configAuth.RequireAuth(s.configAPIHandler.HandleConfig))
		mux.HandleFunc("/api/v1/config/reload", configAuth.RequireAuth(s.configAPIHandler.HandleReload))
		mux.HandleFunc("/api/v1/config/fields", configAuth.RequireAuth(s.configAPIHandler.HandleFields))
		mux.HandleFunc("/api/v1/config/changes", configAuth.RequireAuth(s.configAPIHandler.HandleChanges))
		s.logger.Info("Configuration API registered with authentication")
	}

	// ========================================
	// 构建中间件链
	// ========================================
	skipAuthPaths := []string{"/health", "/healthz", "/ready", "/readyz", "/version", "/metrics"}
	rateLimiterCtx, rateLimiterCancel := context.WithCancel(context.Background())
	s.rateLimiterCancel = rateLimiterCancel
	tenantRateLimiterCtx, tenantRateLimiterCancel := context.WithCancel(context.Background())
	s.tenantRateLimiterCancel = tenantRateLimiterCancel

	// Auth strategy: JWT preferred, fallback to API Key, skip if neither configured
	authMiddleware := s.buildAuthMiddleware(skipAuthPaths)

	middlewares := []mw.Middleware{
		mw.Recovery(s.logger),
		mw.RequestID(),
		mw.SecurityHeaders(),
		mw.MetricsMiddleware(s.metricsCollector),
		mw.OTelTracing(),
		mw.RequestLogger(s.logger),
		mw.CORS(s.cfg.Server.CORSAllowedOrigins),
		mw.RateLimiter(rateLimiterCtx, float64(s.cfg.Server.RateLimitRPS), s.cfg.Server.RateLimitBurst, s.logger),
	}
	if authMiddleware != nil {
		middlewares = append(middlewares, authMiddleware)
	}
	// Tenant rate limiter runs after auth (needs tenant_id in context)
	middlewares = append(middlewares,
		mw.TenantRateLimiter(tenantRateLimiterCtx, float64(s.cfg.Server.TenantRateLimitRPS), s.cfg.Server.TenantRateLimitBurst, s.logger),
	)

	handler := mw.Chain(mux, middlewares...)

	// ========================================
	// 使用 internal/server.Manager
	// ========================================
	serverConfig := server.Config{
		Addr:            fmt.Sprintf(":%d", s.cfg.Server.HTTPPort),
		ReadTimeout:     s.cfg.Server.ReadTimeout,
		WriteTimeout:    s.cfg.Server.WriteTimeout,
		IdleTimeout:     2 * s.cfg.Server.ReadTimeout, // 2x ReadTimeout
		MaxHeaderBytes:  1 << 20,                      // 1 MB
		ShutdownTimeout: s.cfg.Server.ShutdownTimeout,
	}

	s.httpManager = server.NewManager(handler, serverConfig, s.logger)

	// 启动服务器（非阻塞）
	if err := s.httpManager.Start(); err != nil {
		return err
	}

	s.logger.Info("HTTP server started", zap.Int("port", s.cfg.Server.HTTPPort))
	return nil
}

// =============================================================================
// 📊 Metrics 服务器
// =============================================================================

// startMetricsServer 启动 Metrics 服务器
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	serverConfig := server.Config{
		Addr:            fmt.Sprintf(":%d", s.cfg.Server.MetricsPort),
		ReadTimeout:     s.cfg.Server.ReadTimeout,
		WriteTimeout:    s.cfg.Server.WriteTimeout,
		ShutdownTimeout: s.cfg.Server.ShutdownTimeout,
	}

	s.metricsManager = server.NewManager(mux, serverConfig, s.logger)

	// 启动服务器（非阻塞）
	if err := s.metricsManager.Start(); err != nil {
		return err
	}

	s.logger.Info("Metrics server started", zap.Int("port", s.cfg.Server.MetricsPort))
	return nil
}

// getFirstAPIKey 返回配置中的第一个 API Key，用于配置 API 的独立认证。
// 如果未配置任何 API Key，返回空字符串（ConfigAPIMiddleware 会跳过认证检查）。
func (s *Server) getFirstAPIKey() string {
	if len(s.cfg.Server.APIKeys) > 0 {
		return s.cfg.Server.APIKeys[0]
	}
	return ""
}

// buildAuthMiddleware selects the authentication strategy based on configuration.
// Priority: JWT (if secret or public key configured) > API Key > nil (dev mode).
func (s *Server) buildAuthMiddleware(skipPaths []string) mw.Middleware {
	jwtCfg := s.cfg.Server.JWT
	hasJWT := jwtCfg.Secret != "" || jwtCfg.PublicKey != ""
	hasAPIKeys := len(s.cfg.Server.APIKeys) > 0

	switch {
	case hasJWT:
		s.logger.Info("Authentication: JWT enabled",
			zap.Bool("hmac", jwtCfg.Secret != ""),
			zap.Bool("rsa", jwtCfg.PublicKey != ""),
			zap.String("issuer", jwtCfg.Issuer),
		)
		return mw.JWTAuth(jwtCfg, skipPaths, s.logger)
	case hasAPIKeys:
		s.logger.Info("Authentication: API Key enabled",
			zap.Int("key_count", len(s.cfg.Server.APIKeys)),
		)
		return mw.APIKeyAuth(s.cfg.Server.APIKeys, skipPaths, s.cfg.Server.AllowQueryAPIKey, s.logger)
	default:
		if !s.cfg.Server.AllowNoAuth {
			s.logger.Warn("Authentication is disabled and allow_no_auth is false. " +
				"Set JWT or API key configuration, or set allow_no_auth=true to explicitly allow unauthenticated access.")
		} else {
			s.logger.Warn("Authentication is disabled (allow_no_auth=true). " +
				"This is not recommended for production use.")
		}
		return nil
	}
}

// =============================================================================
// 🛑 关闭流程
// =============================================================================

// WaitForShutdown 等待关闭信号并优雅关闭
func (s *Server) WaitForShutdown() {
	// 使用 httpManager 的 WaitForShutdown（它会监听信号）
	if s.httpManager != nil {
		s.httpManager.WaitForShutdown()
	}

	// 执行清理
	s.Shutdown()
}

// Shutdown 优雅关闭所有服务
func (s *Server) Shutdown() {
	s.logger.Info("Starting graceful shutdown...")

	timeout := s.cfg.Server.ShutdownTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 0. 停止 rate limiter 清理 goroutine
	if s.rateLimiterCancel != nil {
		s.rateLimiterCancel()
	}
	if s.tenantRateLimiterCancel != nil {
		s.tenantRateLimiterCancel()
	}

	// 1. 停止热更新管理器
	if s.hotReloadManager != nil {
		if err := s.hotReloadManager.Stop(); err != nil {
			s.logger.Error("Hot reload manager shutdown error", zap.Error(err))
		}
	}

	// 2. 关闭 HTTP 服务器（等待 in-flight 请求完成）
	if s.httpManager != nil {
		if err := s.httpManager.Shutdown(ctx); err != nil {
			s.logger.Error("HTTP server shutdown error", zap.Error(err))
		}
	}

	// 3. 关闭 Metrics 服务器
	if s.metricsManager != nil {
		if err := s.metricsManager.Shutdown(ctx); err != nil {
			s.logger.Error("Metrics server shutdown error", zap.Error(err))
		}
	}

	// 4. Flush and shutdown telemetry exporters
	// 必须在 HTTP/Metrics server 关闭之后执行，确保 in-flight 请求的 span/metric 不丢失
	if s.telemetry != nil {
		if err := s.telemetry.Shutdown(ctx); err != nil {
			s.logger.Error("Telemetry shutdown error", zap.Error(err))
		}
	}

	// 5. Teardown cached agent instances
	if s.resolver != nil {
		s.resolver.TeardownAll(ctx)
	}

	// 6. 关闭数据库连接
	if s.db != nil {
		if sqlDB, err := s.db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				s.logger.Error("Database close error", zap.Error(err))
			} else {
				s.logger.Info("Database connection closed")
			}
		}
	}

	// 7. 关闭 MongoDB 连接
	if err := s.mongoClient.Close(ctx); err != nil {
		s.logger.Error("MongoDB close error", zap.Error(err))
	} else {
		s.logger.Info("MongoDB connection closed")
	}

	// 7.5 关闭 AuditLogger
	if s.auditLogger != nil {
		if err := s.auditLogger.Close(); err != nil {
			s.logger.Error("AuditLogger close error", zap.Error(err))
		}
	}

	// 8. 等待所有 goroutine 完成
	s.wg.Wait()

	s.logger.Info("Graceful shutdown completed")
}
