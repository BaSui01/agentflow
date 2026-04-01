package main

import (
	"net/http"
	"net/http/pprof"

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
			Health:        s.healthHandler,
			Chat:          s.chatHandler,
			Agent:         s.agentHandler,
			APIKey:        s.apiKeyHandler,
			Tools:         s.toolRegistryHandler,
			ToolProviders: s.toolProviderHandler,
			ToolApprovals: s.toolApprovalHandler,
			Multimodal:    s.multimodalHandler,
			Protocol:      s.protocolHandler,
			RAG:           s.ragHandler,
			Workflow:      s.workflowHandler,
			ConfigAPI:     s.configAPIHandler,
			Cost:          s.costHandler,
		},
		Version,
		BuildTime,
		GitCommit,
		s.getFirstAPIKey(),
		s.logger,
	)

	httpMiddlewares, err := bootstrap.BuildHTTPMiddlewares(s.cfg.Server, s.metricsCollector, s.logger)
	if err != nil {
		return err
	}
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
//
// 已知安全风险 (X-005): Metrics 与 pprof 服务无认证、无限流，暴露于独立端口。
// 生产环境应通过网络隔离或反向代理限制访问。

// startMetricsServer 启动 Metrics 服务器
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

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
