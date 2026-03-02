package bootstrap

import (
	"fmt"
	"net/http"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/api/routes"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/pkg/server"
	"go.uber.org/zap"
)

// HTTPRouteHandlers groups handler dependencies used by HTTP route registration.
type HTTPRouteHandlers struct {
	Health     *handlers.HealthHandler
	Chat       *handlers.ChatHandler
	Agent      *handlers.AgentHandler
	APIKey     *handlers.APIKeyHandler
	Multimodal *handlers.MultimodalHandler
	Protocol   *handlers.ProtocolHandler
	RAG        *handlers.RAGHandler
	Workflow   *handlers.WorkflowHandler
	ConfigAPI  *config.ConfigAPIHandler
}

// RegisterHTTPRoutes wires all API routes into the provided mux and logs route summary.
func RegisterHTTPRoutes(
	mux *http.ServeMux,
	handlers HTTPRouteHandlers,
	version string,
	buildTime string,
	gitCommit string,
	firstAPIKey string,
	logger *zap.Logger,
) {
	routes.RegisterSystem(mux, handlers.Health, version, buildTime, gitCommit)
	routes.RegisterChat(mux, handlers.Chat, logger)
	routes.RegisterAgent(mux, handlers.Agent, logger)
	routes.RegisterProvider(mux, handlers.APIKey, logger)
	routes.RegisterMultimodal(mux, handlers.Multimodal, logger)
	routes.RegisterProtocol(mux, handlers.Protocol, logger)
	routes.RegisterRAG(mux, handlers.RAG, logger)
	routes.RegisterWorkflow(mux, handlers.Workflow, logger)
	routes.RegisterConfig(mux, handlers.ConfigAPI, firstAPIKey, logger)

	logger.Info("HTTP routes registered",
		zap.Strings("routes", []string{
			"/health",
			"/healthz",
			"/ready",
			"/readyz",
			"/version",
			"/api/v1/chat/completions",
			"/api/v1/agents/*",
			"/api/v1/providers/*",
			"/api/v1/multimodal/*",
			"/api/v1/mcp/*",
			"/api/v1/rag/*",
			"/api/v1/workflows/*",
			"/api/v1/config/*",
			"/metrics",
		}))
}

// BuildHTTPServerConfig creates the HTTP server manager configuration.
func BuildHTTPServerConfig(serverCfg config.ServerConfig) server.Config {
	return server.Config{
		Addr:            fmt.Sprintf(":%d", serverCfg.HTTPPort),
		ReadTimeout:     serverCfg.ReadTimeout,
		WriteTimeout:    serverCfg.WriteTimeout,
		IdleTimeout:     2 * serverCfg.ReadTimeout, // keep existing 2x read-timeout policy
		MaxHeaderBytes:  1 << 20,                   // 1 MB
		ShutdownTimeout: serverCfg.ShutdownTimeout,
	}
}

// BuildMetricsServerConfig creates the metrics server manager configuration.
func BuildMetricsServerConfig(serverCfg config.ServerConfig) server.Config {
	return server.Config{
		Addr:            fmt.Sprintf(":%d", serverCfg.MetricsPort),
		ReadTimeout:     serverCfg.ReadTimeout,
		WriteTimeout:    serverCfg.WriteTimeout,
		ShutdownTimeout: serverCfg.ShutdownTimeout,
	}
}
