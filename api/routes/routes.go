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
	mux.HandleFunc("GET /health", healthHandler.HandleHealth)
	mux.HandleFunc("GET /healthz", healthHandler.HandleHealthz)
	mux.HandleFunc("GET /ready", healthHandler.HandleReady)
	mux.HandleFunc("GET /readyz", healthHandler.HandleReady)
	mux.HandleFunc("GET /version", healthHandler.HandleVersion(version, buildTime, gitCommit))
}

func RegisterChat(mux *http.ServeMux, chatHandler *handlers.ChatHandler, logger *zap.Logger) {
	if chatHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/chat/capabilities", chatHandler.HandleCapabilities)
	mux.HandleFunc("POST /api/v1/chat/completions", chatHandler.HandleCompletion)
	mux.HandleFunc("POST /api/v1/chat/completions/stream", chatHandler.HandleStream)
	mux.HandleFunc("POST /v1/chat/completions", chatHandler.HandleOpenAICompatChatCompletions)
	mux.HandleFunc("POST /v1/responses", chatHandler.HandleOpenAICompatResponses)
	logger.Info("Chat API routes registered")
}

func RegisterAgent(mux *http.ServeMux, agentHandler *handlers.AgentHandler, logger *zap.Logger) {
	if agentHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/agents", agentHandler.HandleListAgents)
	mux.HandleFunc("GET /api/v1/agents/{id}", agentHandler.HandleGetAgent)
	mux.HandleFunc("GET /api/v1/agents/capabilities", agentHandler.HandleCapabilities)
	mux.HandleFunc("POST /api/v1/agents/execute", agentHandler.HandleExecuteAgent)
	mux.HandleFunc("POST /api/v1/agents/execute/stream", agentHandler.HandleAgentStream)
	mux.HandleFunc("POST /api/v1/agents/execute/interrupt", agentHandler.HandleAgentInterrupt)
	mux.HandleFunc("GET /api/v1/agents/health", agentHandler.HandleAgentHealth)
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

func RegisterTools(mux *http.ServeMux, toolHandler *handlers.ToolRegistryHandler, providerHandler *handlers.ToolProviderHandler, logger *zap.Logger) {
	if toolHandler == nil && providerHandler == nil {
		return
	}
	if toolHandler != nil {
		mux.HandleFunc("GET /api/v1/tools", toolHandler.HandleList)
		mux.HandleFunc("POST /api/v1/tools", toolHandler.HandleCreate)
		mux.HandleFunc("GET /api/v1/tools/targets", toolHandler.HandleListTargets)
		mux.HandleFunc("POST /api/v1/tools/reload", toolHandler.HandleReload)
		mux.HandleFunc("PUT /api/v1/tools/{id}", toolHandler.HandleUpdate)
		mux.HandleFunc("DELETE /api/v1/tools/{id}", toolHandler.HandleDelete)
	}
	if providerHandler != nil {
		mux.HandleFunc("GET /api/v1/tools/providers", providerHandler.HandleList)
		mux.HandleFunc("PUT /api/v1/tools/providers/{provider}", providerHandler.HandleUpsert)
		mux.HandleFunc("DELETE /api/v1/tools/providers/{provider}", providerHandler.HandleDelete)
		mux.HandleFunc("POST /api/v1/tools/providers/reload", providerHandler.HandleReload)
	}
	logger.Info("Tool registry routes registered")
}

func RegisterMultimodal(mux *http.ServeMux, multimodalHandler *handlers.MultimodalHandler, logger *zap.Logger) {
	if multimodalHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/multimodal/capabilities", multimodalHandler.HandleCapabilities)
	mux.HandleFunc("POST /api/v1/multimodal/references", multimodalHandler.HandleUploadReference)
	mux.HandleFunc("POST /api/v1/multimodal/image", multimodalHandler.HandleImage)
	mux.HandleFunc("POST /api/v1/multimodal/video", multimodalHandler.HandleVideo)
	mux.HandleFunc("POST /api/v1/multimodal/plan", multimodalHandler.HandlePlan)
	mux.HandleFunc("POST /api/v1/multimodal/chat", multimodalHandler.HandleChat)
	logger.Info("Multimodal framework routes registered")
}

func RegisterProtocol(mux *http.ServeMux, protocolHandler *handlers.ProtocolHandler, logger *zap.Logger) {
	if protocolHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/mcp/resources", protocolHandler.HandleMCPListResources)
	mux.HandleFunc("GET /api/v1/mcp/resources/", protocolHandler.HandleMCPGetResource)
	mux.HandleFunc("GET /api/v1/mcp/tools", protocolHandler.HandleMCPListTools)
	mux.HandleFunc("POST /api/v1/mcp/tools/", protocolHandler.HandleMCPCallTool)
	mux.HandleFunc("GET /api/v1/a2a/.well-known/agent.json", protocolHandler.HandleA2AAgentCard)
	mux.HandleFunc("POST /api/v1/a2a/tasks", protocolHandler.HandleA2ASendTask)
	logger.Info("Protocol API routes registered (MCP + A2A)")
}

func RegisterRAG(mux *http.ServeMux, ragHandler *handlers.RAGHandler, logger *zap.Logger) {
	if ragHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/rag/capabilities", ragHandler.HandleCapabilities)
	mux.HandleFunc("POST /api/v1/rag/query", ragHandler.HandleQuery)
	mux.HandleFunc("POST /api/v1/rag/index", ragHandler.HandleIndex)
	logger.Info("RAG API routes registered")
}

func RegisterWorkflow(mux *http.ServeMux, workflowHandler *handlers.WorkflowHandler, logger *zap.Logger) {
	if workflowHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/workflows/capabilities", workflowHandler.HandleCapabilities)
	mux.HandleFunc("POST /api/v1/workflows/execute", workflowHandler.HandleExecute)
	mux.HandleFunc("POST /api/v1/workflows/parse", workflowHandler.HandleParse)
	mux.HandleFunc("GET /api/v1/workflows", workflowHandler.HandleList)
	logger.Info("Workflow API routes registered")
}

func RegisterCost(mux *http.ServeMux, costHandler *handlers.CostHandler, logger *zap.Logger) {
	if costHandler == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/cost/summary", costHandler.HandleSummary)
	mux.HandleFunc("GET /api/v1/cost/records", costHandler.HandleRecords)
	mux.HandleFunc("POST /api/v1/cost/reset", costHandler.HandleReset)
	logger.Info("Cost API routes registered")
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
	mux.HandleFunc("GET /api/v1/config", withLogging(cfgHandler.HandleConfig))
	mux.HandleFunc("PUT /api/v1/config", withLogging(cfgHandler.HandleConfig))
	mux.HandleFunc("OPTIONS /api/v1/config", withLogging(cfgHandler.HandleConfig))
	mux.HandleFunc("POST /api/v1/config/reload", withLogging(cfgHandler.HandleReload))
	mux.HandleFunc("OPTIONS /api/v1/config/reload", withLogging(cfgHandler.HandleReload))
	mux.HandleFunc("POST /api/v1/config/rollback", withLogging(cfgHandler.HandleRollback))
	mux.HandleFunc("OPTIONS /api/v1/config/rollback", withLogging(cfgHandler.HandleRollback))
	mux.HandleFunc("GET /api/v1/config/fields", withLogging(cfgHandler.HandleFields))
	mux.HandleFunc("OPTIONS /api/v1/config/fields", withLogging(cfgHandler.HandleFields))
	mux.HandleFunc("GET /api/v1/config/changes", withLogging(cfgHandler.HandleChanges))
	mux.HandleFunc("OPTIONS /api/v1/config/changes", withLogging(cfgHandler.HandleChanges))
	logger.Info("Configuration API registered with authentication")
}
