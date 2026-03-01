package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	mw "github.com/BaSui01/agentflow/pkg/middleware"
	"github.com/BaSui01/agentflow/pkg/server"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"github.com/BaSui01/agentflow/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()

	routes.RegisterSystem(mux, s.healthHandler, Version, BuildTime, GitCommit)
	routes.RegisterChat(mux, s.chatHandler, s.logger)
	routes.RegisterAgent(mux, s.agentHandler, s.logger)
	routes.RegisterProvider(mux, s.apiKeyHandler, s.logger)
	routes.RegisterMultimodal(mux, s.multimodalHandler, s.logger)
	routes.RegisterProtocol(mux, s.protocolHandler, s.logger)
	routes.RegisterRAG(mux, s.ragHandler, s.logger)
	routes.RegisterWorkflow(mux, s.workflowHandler, s.logger)
	routes.RegisterConfig(mux, s.configAPIHandler, s.getFirstAPIKey(), s.logger)

	middlewares := s.buildHTTPMiddlewares()
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

func (s *Server) buildHTTPMiddlewares() []mw.Middleware {
	skipAuthPaths := []string{"/health", "/healthz", "/ready", "/readyz", "/version", "/metrics"}
	rateLimiterCtx, rateLimiterCancel := context.WithCancel(context.Background())
	s.rateLimiterCancel = rateLimiterCancel
	tenantRateLimiterCtx, tenantRateLimiterCancel := context.WithCancel(context.Background())
	s.tenantRateLimiterCancel = tenantRateLimiterCancel

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
	middlewares = append(middlewares,
		mw.TenantRateLimiter(tenantRateLimiterCtx, float64(s.cfg.Server.TenantRateLimitRPS), s.cfg.Server.TenantRateLimitBurst, s.logger),
	)

	return middlewares
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
// 如果未配置任何 API Key，返回空字符串（ConfigAPIMiddleware 本身不再增加额外认证约束，
// 但路由仍可能受全局认证中间件保护）。
func (s *Server) getFirstAPIKey() string {
	if len(s.cfg.Server.APIKeys) > 0 {
		return s.cfg.Server.APIKeys[0]
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *Server) newMultimodalRedisReferenceStore(keyPrefix string, ttl time.Duration) (handlers.ReferenceStore, error) {
	addr := strings.TrimSpace(s.cfg.Redis.Addr)
	if addr == "" {
		return nil, fmt.Errorf("redis address is required when multimodal reference_store_backend=redis")
	}

	var (
		opts *redis.Options
		err  error
	)

	if strings.HasPrefix(addr, "redis://") || strings.HasPrefix(addr, "rediss://") {
		parsed, parseErr := url.Parse(addr)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid redis url: %w", parseErr)
		}
		scheme := strings.ToLower(parsed.Scheme)
		host := parsed.Hostname()
		if scheme == "redis" && !isLoopbackHost(host) {
			return nil, fmt.Errorf("insecure redis:// is only allowed for loopback hosts, use rediss:// for %q", host)
		}

		opts, err = redis.ParseURL(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid redis url: %w", err)
		}
		if s.cfg.Redis.Password != "" && opts.Password == "" {
			opts.Password = s.cfg.Redis.Password
		}
		if s.cfg.Redis.DB != 0 && opts.DB == 0 {
			opts.DB = s.cfg.Redis.DB
		}
		if s.cfg.Redis.PoolSize > 0 {
			opts.PoolSize = s.cfg.Redis.PoolSize
		}
		if s.cfg.Redis.MinIdleConns > 0 {
			opts.MinIdleConns = s.cfg.Redis.MinIdleConns
		}
		if scheme == "rediss" && opts.TLSConfig == nil {
			opts.TLSConfig = tlsutil.DefaultTLSConfig()
		}
		if scheme == "redis" && isLoopbackHost(host) {
			s.logger.Warn("using insecure redis:// for loopback host in multimodal reference store",
				zap.String("host", host))
		}
	} else {
		host := hostFromAddr(addr)
		if !isLoopbackHost(host) {
			return nil, fmt.Errorf("non-loopback redis address %q requires rediss:// scheme", host)
		}

		opts = &redis.Options{
			Addr:         addr,
			Password:     s.cfg.Redis.Password,
			DB:           s.cfg.Redis.DB,
			PoolSize:     s.cfg.Redis.PoolSize,
			MinIdleConns: s.cfg.Redis.MinIdleConns,
		}
		s.logger.Warn("using insecure plaintext redis connection for loopback host in multimodal reference store",
			zap.String("host", host))
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	s.multimodalRedis = client
	return handlers.NewRedisReferenceStore(client, keyPrefix, ttl, s.logger), nil
}

func hostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(strings.Trim(host, "[]"))
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// buildAuthMiddleware selects the authentication strategy based on configuration.
// Priority: JWT (if secret or public key configured) > API Key > fail-closed.
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
		return mw.APIKeyAuth(s.cfg.Server.APIKeys, skipPaths, s.logger)
	default:
		if s.cfg.Server.AllowNoAuth {
			s.logger.Warn("Authentication is disabled (allow_no_auth=true). " +
				"This is not recommended for production use.")
			return nil
		}
		s.logger.Error("Authentication is required but no JWT/API key is configured; server will reject protected requests")
		skipSet := make(map[string]struct{}, len(skipPaths))
		for _, p := range skipPaths {
			skipSet[p] = struct{}{}
		}
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, skip := skipSet[r.URL.Path]; skip {
					next.ServeHTTP(w, r)
					return
				}
				api.WriteJSONResponse(w, http.StatusServiceUnavailable, api.Response{
					Success: false,
					Error: &api.ErrorInfo{
						Code:    string(types.ErrServiceUnavailable),
						Message: "authentication is not configured",
					},
					Timestamp: time.Now().UTC(),
					RequestID: w.Header().Get("X-Request-ID"),
				})
			})
		}
	}
}

// =============================================================================
// 🛑 关闭流程
// =============================================================================

// WaitForShutdown 等待关闭信号并优雅关闭
