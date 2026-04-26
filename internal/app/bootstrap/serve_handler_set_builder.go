package bootstrap

import (
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
	Cfg         *config.Config
	DB          *gorm.DB
	MongoClient *mongoclient.Client
	Logger      *zap.Logger

	ToolApprovalManager *hitl.InterruptManager
	WorkflowHITLManager *hitl.InterruptManager

	WireMongoStores func(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error
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
