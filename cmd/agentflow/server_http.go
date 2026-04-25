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
			Health:        s.handlers.healthHandler,
			Chat:          s.handlers.chatHandler,
			Agent:         s.handlers.agentHandler,
			APIKey:        s.handlers.apiKeyHandler,
			Tools:         s.handlers.toolRegistryHandler,
			ToolProviders: s.handlers.toolProviderHandler,
			ToolApprovals: s.handlers.toolApprovalHandler,
			AuthAudit:     s.handlers.authAuditHandler,
			Multimodal:    s.handlers.multimodalHandler,
			Protocol:      s.handlers.protocolHandler,
			RAG:           s.handlers.ragHandler,
			Workflow:      s.handlers.workflowHandler,
			ConfigAPI:     s.ops.configAPIHandler,
			Cost:          s.handlers.costHandler,
		},
		Version,
		BuildTime,
		GitCommit,
		s.getFirstAPIKey(),
		s.logger,
	)

	httpMiddlewares, err := bootstrap.BuildHTTPMiddlewares(s.cfg.Server, s.ops.metricsCollector, s.logger)
	if err != nil {
		return err
	}
	s.ops.rateLimiterCancel = httpMiddlewares.RateLimiterCancel
	s.ops.tenantRateLimiterCancel = httpMiddlewares.TenantRateLimiterCancel
	handler := mw.Chain(mux, httpMiddlewares.List...)

	// ========================================
	// 使用 internal/server.Manager
	// ========================================
	s.ops.httpManager = server.NewManager(handler, bootstrap.BuildHTTPServerConfig(s.cfg.Server), s.logger)
	return nil
}

// =============================================================================
// 📊 Metrics 服务器
// =============================================================================
//
// Metrics 端口默认仅绑定 loopback；若需要外部抓取，必须显式配置
// server.metrics_bind_address。pprof 默认关闭，仅在 enable_pprof=true 时启用。

// startMetricsServer 启动 Metrics 服务器
func (s *Server) startMetricsServer() error {
	mux := buildMetricsMux(s.cfg.Server.EnablePProf)
	s.ops.metricsManager = server.NewManager(mux, bootstrap.BuildMetricsServerConfig(s.cfg.Server), s.logger)
	return nil
}

func buildMetricsMux(enablePProf bool) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	if enablePProf {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
	return mux
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
