package bootstrap

import (
	"context"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/pkg/metrics"
	mw "github.com/BaSui01/agentflow/pkg/middleware"
	"go.uber.org/zap"
)

// HTTPMiddlewares bundles middleware list and lifecycle cancels.
type HTTPMiddlewares struct {
	List                    []mw.Middleware
	RateLimiterCancel       context.CancelFunc
	TenantRateLimiterCancel context.CancelFunc
}

// BuildHTTPMiddlewares creates the default HTTP middleware chain.
func BuildHTTPMiddlewares(
	serverCfg config.ServerConfig,
	collector *metrics.Collector,
	logger *zap.Logger,
) HTTPMiddlewares {
	skipAuthPaths := []string{"/health", "/healthz", "/ready", "/readyz", "/version", "/metrics"}
	rateLimiterCtx, rateLimiterCancel := context.WithCancel(context.Background())
	tenantRateLimiterCtx, tenantRateLimiterCancel := context.WithCancel(context.Background())

	authMiddleware := BuildAuthMiddleware(serverCfg, skipAuthPaths, logger)

	middlewares := []mw.Middleware{
		mw.Recovery(logger),
		mw.RequestID(),
		mw.SecurityHeaders(),
		mw.MetricsMiddleware(collector),
		mw.OTelTracing(),
		mw.RequestLogger(logger),
		mw.CORS(serverCfg.CORSAllowedOrigins),
		mw.RateLimiter(rateLimiterCtx, float64(serverCfg.RateLimitRPS), serverCfg.RateLimitBurst, logger),
	}
	if authMiddleware != nil {
		middlewares = append(middlewares, authMiddleware)
	}
	middlewares = append(middlewares,
		mw.TenantRateLimiter(tenantRateLimiterCtx, float64(serverCfg.TenantRateLimitRPS), serverCfg.TenantRateLimitBurst, logger),
	)

	return HTTPMiddlewares{
		List:                    middlewares,
		RateLimiterCancel:       rateLimiterCancel,
		TenantRateLimiterCancel: tenantRateLimiterCancel,
	}
}
