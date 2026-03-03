package routes

import (
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"go.uber.org/zap"
)

func RegisterSystem(mux *http.ServeMux, healthHandler *handlers.HealthHandler, version, buildTime, gitCommit string) {
	if healthHandler == nil {
		return
	}
	mux.HandleFunc("/health", healthHandler.HandleHealth)
	mux.HandleFunc("/healthz", healthHandler.HandleHealthz)
	mux.HandleFunc("/ready", healthHandler.HandleReady)
	mux.HandleFunc("/readyz", healthHandler.HandleReady)
	mux.HandleFunc("/version", healthHandler.HandleVersion(version, buildTime, gitCommit))
}

func RegisterChat(mux *http.ServeMux, chatHandler *handlers.ChatHandler, logger *zap.Logger) {
	if chatHandler == nil {
		return
	}
	mux.HandleFunc("/api/v1/chat/capabilities", chatHandler.HandleCapabilities)
	mux.HandleFunc("/api/v1/chat/completions", chatHandler.HandleCompletion)
	mux.HandleFunc("/api/v1/chat/completions/stream", chatHandler.HandleStream)
	logger.Info("Chat API routes registered")
}

func RegisterAgent(mux *http.ServeMux, agentHandler *handlers.AgentHandler, logger *zap.Logger) {
	if agentHandler == nil {
		return
	}
	mux.HandleFunc("/api/v1/agents", agentHandler.HandleListAgents)
	mux.HandleFunc("/api/v1/agents/{id}", agentHandler.HandleGetAgent)
	mux.HandleFunc("/api/v1/agents/capabilities", agentHandler.HandleCapabilities)
	mux.HandleFunc("/api/v1/agents/execute", agentHandler.HandleExecuteAgent)
	mux.HandleFunc("/api/v1/agents/execute/stream", agentHandler.HandleAgentStream)
	mux.HandleFunc("/api/v1/agents/plan", agentHandler.HandlePlanAgent)
	mux.HandleFunc("/api/v1/agents/health", agentHandler.HandleAgentHealth)
	logger.Info("Agent API routes registered")
}

func RegisterProvider(mux *http.ServeMux, apiKeyHandler *handlers.APIKeyHandler, logger *zap.Logger) {
	if apiKeyHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/providers", apiKeyHandler.HandleListProviders)
	mux.HandleFunc("GET /api/v1/providers/{id}/api-keys", apiKeyHandler.HandleListAPIKeys)
	mux.HandleFunc("POST /api/v1/providers/{id}/api-keys", apiKeyHandler.HandleCreateAPIKey)
	mux.HandleFunc("GET /api/v1/providers/{id}/api-keys/stats", apiKeyHandler.HandleAPIKeyStats)
	mux.HandleFunc("PUT /api/v1/providers/{id}/api-keys/{keyId}", apiKeyHandler.HandleUpdateAPIKey)
	mux.HandleFunc("DELETE /api/v1/providers/{id}/api-keys/{keyId}", apiKeyHandler.HandleDeleteAPIKey)
	logger.Info("Provider API key routes registered")
}

func RegisterTools(mux *http.ServeMux, toolHandler *handlers.ToolRegistryHandler, logger *zap.Logger) {
	if toolHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/tools", toolHandler.HandleList)
	mux.HandleFunc("POST /api/v1/tools", toolHandler.HandleCreate)
	mux.HandleFunc("GET /api/v1/tools/targets", toolHandler.HandleListTargets)
	mux.HandleFunc("POST /api/v1/tools/reload", toolHandler.HandleReload)
	mux.HandleFunc("PUT /api/v1/tools/{id}", toolHandler.HandleUpdate)
	mux.HandleFunc("DELETE /api/v1/tools/{id}", toolHandler.HandleDelete)
	logger.Info("Tool registry routes registered")
}

func RegisterMultimodal(mux *http.ServeMux, multimodalHandler *handlers.MultimodalHandler, logger *zap.Logger) {
	if multimodalHandler == nil {
		return
	}
	mux.HandleFunc("/api/v1/multimodal/capabilities", multimodalHandler.HandleCapabilities)
	mux.HandleFunc("/api/v1/multimodal/references", multimodalHandler.HandleUploadReference)
	mux.HandleFunc("/api/v1/multimodal/image", multimodalHandler.HandleImage)
	mux.HandleFunc("/api/v1/multimodal/video", multimodalHandler.HandleVideo)
	mux.HandleFunc("/api/v1/multimodal/plan", multimodalHandler.HandlePlan)
	mux.HandleFunc("/api/v1/multimodal/chat", multimodalHandler.HandleChat)
	logger.Info("Multimodal framework routes registered")
}

func RegisterProtocol(mux *http.ServeMux, protocolHandler *handlers.ProtocolHandler, logger *zap.Logger) {
	if protocolHandler == nil {
		return
	}
	mux.HandleFunc("/api/v1/mcp/resources", protocolHandler.HandleMCPListResources)
	mux.HandleFunc("/api/v1/mcp/resources/", protocolHandler.HandleMCPGetResource)
	mux.HandleFunc("/api/v1/mcp/tools", protocolHandler.HandleMCPListTools)
	mux.HandleFunc("/api/v1/mcp/tools/", protocolHandler.HandleMCPCallTool)
	mux.HandleFunc("/api/v1/a2a/.well-known/agent.json", protocolHandler.HandleA2AAgentCard)
	mux.HandleFunc("/api/v1/a2a/tasks", protocolHandler.HandleA2ASendTask)
	logger.Info("Protocol API routes registered (MCP + A2A)")
}

func RegisterRAG(mux *http.ServeMux, ragHandler *handlers.RAGHandler, logger *zap.Logger) {
	if ragHandler == nil {
		return
	}
	mux.HandleFunc("/api/v1/rag/capabilities", ragHandler.HandleCapabilities)
	mux.HandleFunc("/api/v1/rag/query", ragHandler.HandleQuery)
	mux.HandleFunc("/api/v1/rag/index", ragHandler.HandleIndex)
	logger.Info("RAG API routes registered")
}

func RegisterWorkflow(mux *http.ServeMux, workflowHandler *handlers.WorkflowHandler, logger *zap.Logger) {
	if workflowHandler == nil {
		return
	}
	mux.HandleFunc("/api/v1/workflows/capabilities", workflowHandler.HandleCapabilities)
	mux.HandleFunc("/api/v1/workflows/execute", workflowHandler.HandleExecute)
	mux.HandleFunc("/api/v1/workflows/parse", workflowHandler.HandleParse)
	mux.HandleFunc("/api/v1/workflows", workflowHandler.HandleList)
	logger.Info("Workflow API routes registered")
}

func RegisterConfig(mux *http.ServeMux, cfgHandler *config.ConfigAPIHandler, firstAPIKey string, logger *zap.Logger) {
	if cfgHandler == nil {
		return
	}
	configAuth := config.NewConfigAPIMiddleware(cfgHandler, firstAPIKey)
	withLogging := func(next http.HandlerFunc) http.HandlerFunc {
		return configAuth.LogRequests(configAuth.RequireAuth(next), func(method, path string, status int, duration time.Duration) {
			logger.Info("config api request",
				zap.String("method", method),
				zap.String("path", path),
				zap.Int("status", status),
				zap.Duration("duration", duration),
			)
		})
	}
	mux.HandleFunc("/api/v1/config", withLogging(cfgHandler.HandleConfig))
	mux.HandleFunc("/api/v1/config/reload", withLogging(cfgHandler.HandleReload))
	mux.HandleFunc("/api/v1/config/rollback", withLogging(cfgHandler.HandleRollback))
	mux.HandleFunc("/api/v1/config/fields", withLogging(cfgHandler.HandleFields))
	mux.HandleFunc("/api/v1/config/changes", withLogging(cfgHandler.HandleChanges))
	logger.Info("Configuration API registered with authentication")
}
