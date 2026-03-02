package main

import (
	"net/http"

	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	mw "github.com/BaSui01/agentflow/pkg/middleware"
	"github.com/BaSui01/agentflow/pkg/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()

	bootstrap.RegisterHTTPRoutes(
		mux,
		bootstrap.HTTPRouteHandlers{
			Health:     s.healthHandler,
			Chat:       s.chatHandler,
			Agent:      s.agentHandler,
			APIKey:     s.apiKeyHandler,
			Multimodal: s.multimodalHandler,
			Protocol:   s.protocolHandler,
			RAG:        s.ragHandler,
			Workflow:   s.workflowHandler,
			ConfigAPI:  s.configAPIHandler,
		},
		Version,
		BuildTime,
		GitCommit,
		s.getFirstAPIKey(),
		s.logger,
	)

	httpMiddlewares := bootstrap.BuildHTTPMiddlewares(s.cfg.Server, s.metricsCollector, s.logger)
	s.rateLimiterCancel = httpMiddlewares.RateLimiterCancel
	s.tenantRateLimiterCancel = httpMiddlewares.TenantRateLimiterCancel
	handler := mw.Chain(mux, httpMiddlewares.List...)

	// ========================================
	// 使用 internal/server.Manager
	// ========================================
	s.httpManager = server.NewManager(handler, bootstrap.BuildHTTPServerConfig(s.cfg.Server), s.logger)
	return nil
}

// =============================================================================
// 📊 Metrics 服务器
// =============================================================================

// startMetricsServer 启动 Metrics 服务器
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	s.metricsManager = server.NewManager(mux, bootstrap.BuildMetricsServerConfig(s.cfg.Server), s.logger)
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

// =============================================================================
// 🛑 关闭流程
// =============================================================================

// WaitForShutdown 等待关闭信号并优雅关闭
