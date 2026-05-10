package bootstrap

import (
	"context"
	"fmt"
	"time"

	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const defaultServeChatServiceTimeout = 30 * time.Second

// ServeHandlerSetBuildInput defines dependencies for serve-time handler assembly.
type ServeHandlerSetBuildInput struct {
	Ctx         context.Context // lifecycle context for servers; defaults to context.Background() if nil (#12)
	Cfg         *config.Config
	DB          *gorm.DB
	MongoClient *mongoclient.Client
	Logger      *zap.Logger

	ToolApprovalManager *hitl.InterruptManager
	WorkflowHITLManager *hitl.InterruptManager

	WireMongoStores func(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error
}

// lifecycleCtx returns a non-nil context from input, defaulting to Background().
func (in *ServeHandlerSetBuildInput) lifecycleCtx() context.Context {
	if in.Ctx != nil {
		return in.Ctx
	}
	return context.Background()
}

// ServeHandlerSet aggregates handlers and runtime dependencies built at startup.
// It is composed of three sub-sets for better organization and reduced field count.
type ServeHandlerSet struct {
	HTTPHandlerSet
	LLMRuntimeSet
	StorageSet

	ChatService usecase.ChatService

	ToolingRuntime    *AgentToolingRuntime
	CapabilityCatalog *CapabilityCatalog
}

// BuildServeHandlerSet builds serve-time handlers and runtime dependencies in one entry.
func BuildServeHandlerSet(in ServeHandlerSetBuildInput) (*ServeHandlerSet, error) {
	if in.Cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if in.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	set := &ServeHandlerSet{}
	modelCatalog, err := BuildModelCatalog(in.Cfg.LLM.ModelCatalogPath)
	if err != nil {
		return nil, err
	}
	set.ModelCatalog = modelCatalog
	registerServeHealthChecks(set, in)

	llmRuntime, err := buildServeLLMRuntime(set, in)
	if err != nil {
		return nil, err
	}
	buildServeAgentRegistries(set, in.Logger)
	buildServeAPIKeyHandler(set, in)

	if err := buildServeMultimodal(set, in, llmRuntime); err != nil {
		return nil, err
	}
	protocolRuntime := buildServeProtocolHandler(set, in)
	if err := buildServeRAGHandler(set, in); err != nil {
		return nil, err
	}
	authorizationService, err := buildServeToolingBundle(set, in, protocolRuntime)
	if err != nil {
		return nil, err
	}
	if err := buildServeChatHandler(set, in, llmRuntime); err != nil {
		return nil, err
	}
	if err := buildServeAgentHandler(set, in, llmRuntime); err != nil {
		return nil, err
	}
	if err := buildServeWorkflowHandler(set, in, llmRuntime, authorizationService); err != nil {
		return nil, err
	}

	in.Logger.Info("Handlers initialized")
	return set, nil
}
