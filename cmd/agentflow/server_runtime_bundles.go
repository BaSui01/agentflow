package main

import (
	"context"

	agentmemory "github.com/BaSui01/agentflow/agent/capabilities/memory"
	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/observability/evaluation"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm/cache"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/pkg/metrics"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"github.com/BaSui01/agentflow/pkg/server"
	pkgservice "github.com/BaSui01/agentflow/pkg/service"
	"github.com/BaSui01/agentflow/pkg/telemetry"
	"github.com/BaSui01/agentflow/rag/core"
	workflowpkg "github.com/BaSui01/agentflow/workflow/core"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type serverInfraBundle struct {
	telemetry *telemetry.Providers
	db        *gorm.DB

	mongoClient       *mongoclient.Client
	multimodalRedis   *redis.Client
	toolApprovalRedis *redis.Client

	auditLogger    *llmtools.DefaultAuditLogger
	abTester       *evaluation.ABTester
	enhancedMemory *agentmemory.EnhancedMemorySystem
}

type serverOpsBundle struct {
	httpManager     *server.Manager
	metricsManager  *server.Manager
	serviceRegistry *pkgservice.Registry

	metricsCollector *metrics.Collector
	hotReloadManager *config.HotReloadManager
	configAPIHandler *config.ConfigAPIHandler

	rateLimiterCancel       context.CancelFunc
	tenantRateLimiterCancel context.CancelFunc
}

type serverHandlerBundle struct {
	healthHandler       *handlers.HealthHandler
	chatHandler         *handlers.ChatHandler
	agentHandler        *handlers.AgentHandler
	apiKeyHandler       *handlers.APIKeyHandler
	toolRegistryHandler *handlers.ToolRegistryHandler
	toolProviderHandler *handlers.ToolProviderHandler
	toolApprovalHandler *handlers.ToolApprovalHandler
	authAuditHandler    *handlers.AuthorizationAuditHandler
	ragHandler          *handlers.RAGHandler
	workflowHandler     *handlers.WorkflowHandler
	protocolHandler     *handlers.ProtocolHandler
	multimodalHandler   *handlers.MultimodalHandler
	costHandler         *handlers.CostHandler
}

type serverTextRuntimeBundle struct {
	chatService usecase.ChatService

	provider     llm.Provider
	toolProvider llm.Provider

	budgetManager *llmpolicy.TokenBudgetManager
	costTracker   *observability.CostTracker
	llmCache      *cache.MultiLevelCache
	llmMetrics    *observability.Metrics
}

type serverToolingBundle struct {
	discoveryRegistry *discovery.CapabilityRegistry
	agentRegistry     *agent.AgentRegistry
	toolingRuntime    *bootstrap.AgentToolingRuntime

	toolApprovalManager *hitl.InterruptManager
	capabilityCatalog   *bootstrap.CapabilityCatalog
}

type serverWorkflowBundle struct {
	resolver *agent.CachingResolver

	workflowHITLManager *hitl.InterruptManager

	checkpointStore         agentcheckpoint.Store
	checkpointManager       *agent.CheckpointManager
	workflowCheckpointStore workflowpkg.CheckpointStore
	ragStore                core.VectorStore
	ragEmbedding            core.EmbeddingProvider
}
