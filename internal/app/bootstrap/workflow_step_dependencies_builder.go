package bootstrap

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/agent/capabilities/planning"
	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	"github.com/BaSui01/agentflow/agent/runtime"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	ragcore "github.com/BaSui01/agentflow/rag/core"
	workflow "github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/engine"
	"go.uber.org/zap"
)

// WorkflowAgentResolver resolves an agent instance by ID for workflow agent steps.
type WorkflowAgentResolver func(ctx context.Context, agentID string) (agent.Agent, error)

// WorkflowRuntimeOptions carries optional runtime integrations for workflow steps.
type WorkflowRuntimeOptions struct {
	LLMGateway              llmcore.Gateway
	DefaultModel            string
	AgentResolver           WorkflowAgentResolver
	RetrievalStore          ragcore.VectorStore
	EmbeddingProvider       ragcore.EmbeddingProvider
	CheckpointStore         agentcheckpoint.Store
	WorkflowCheckpointStore workflow.CheckpointStore
	HITLManager             *hitl.InterruptManager
	AuthorizationService    usecase.AuthorizationService
}

const (
	defaultWorkflowCodeMaxBytes       = 64 * 1024
	defaultWorkflowCodeTimeoutSeconds = 30
	defaultWorkflowCodeMaxOutputBytes = 1024 * 1024
)

// buildStepDependencies assembles runtime dependencies for workflow steps.
func buildStepDependencies(opts WorkflowRuntimeOptions, logger *zap.Logger) engine.StepDependencies {
	toolRegistry, codeTool := buildHostedWorkflowTools(opts, logger)
	hitlManager := opts.HITLManager
	if hitlManager == nil {
		hitlManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), logger)
	}
	requester := planning.NewHITLInterruptAdapter(hitlManager)
	agentExecutor := resolverAgentExecutor{resolver: opts.AgentResolver}
	workflowTools := hostedToolRegistryAdapter{registry: toolRegistry, authorization: opts.AuthorizationService}

	return engine.StepDependencies{
		Gateway:       newWorkflowGatewayAdapter(opts.LLMGateway, opts.DefaultModel),
		ToolRegistry:  workflowTools,
		ChainRegistry: workflowTools,
		HumanHandler: hitlHumanInputHandler{
			requester:     requester,
			authorization: opts.AuthorizationService,
		},
		AgentExecutor: agentExecutor,
		AgentResolver: agentExecutor,
		CodeHandler: hostedCodeHandler{
			tool:          codeTool,
			authorization: opts.AuthorizationService,
			policy:        defaultWorkflowCodeExecutionPolicy(),
		}.Execute,
	}
}

func buildHostedWorkflowTools(opts WorkflowRuntimeOptions, logger *zap.Logger) (*hosted.ToolRegistry, *hosted.CodeExecTool) {
	registry := hosted.NewToolRegistry(logger)

	policy := defaultWorkflowCodeExecutionPolicy()
	sandboxCfg := runtime.DefaultSandboxConfig()
	sandboxCfg.Mode = runtime.ModeNative
	sandboxCfg.Timeout = policy.DefaultTimeout
	sandboxCfg.MaxOutputBytes = policy.MaxOutputBytes
	sandboxCfg.AllowedLanguages = append([]runtime.Language(nil), policy.AllowedLanguages...)
	sandbox := runtime.NewSandboxExecutor(sandboxCfg, runtime.NewRealProcessBackend(logger, false), logger)
	adapter := runtime.NewHostedAdapter(sandbox, logger)
	codeTool := hosted.NewCodeExecTool(hosted.CodeExecConfig{
		Executor: adapter,
		Timeout:  policy.DefaultTimeout,
		Logger:   logger,
	})
	registry.Register(codeTool)

	if opts.RetrievalStore != nil && opts.EmbeddingProvider != nil {
		retrievalTool := hosted.NewRetrievalTool(
			ragHostedRetrievalStore{store: opts.RetrievalStore, embedder: opts.EmbeddingProvider},
			10,
			logger,
		)
		registry.Register(retrievalTool)
	}

	return registry, codeTool
}

type workflowCodeExecutionPolicy struct {
	MaxCodeBytes        int
	DefaultTimeout      time.Duration
	MaxTimeout          time.Duration
	MaxOutputBytes      int
	AllowedLanguages    []runtime.Language
	AllowedLanguageTags []string
}

func defaultWorkflowCodeExecutionPolicy() workflowCodeExecutionPolicy {
	return workflowCodeExecutionPolicy{
		MaxCodeBytes:   defaultWorkflowCodeMaxBytes,
		DefaultTimeout: defaultWorkflowCodeTimeoutSeconds * time.Second,
		MaxTimeout:     defaultWorkflowCodeTimeoutSeconds * time.Second,
		MaxOutputBytes: defaultWorkflowCodeMaxOutputBytes,
		AllowedLanguages: []runtime.Language{
			runtime.LangPython,
			runtime.LangJavaScript,
			runtime.LangBash,
			runtime.LangGo,
		},
		AllowedLanguageTags: []string{"python", "javascript", "bash", "go"},
	}
}

func (p workflowCodeExecutionPolicy) normalized() workflowCodeExecutionPolicy {
	defaults := defaultWorkflowCodeExecutionPolicy()
	if p.MaxCodeBytes <= 0 {
		p.MaxCodeBytes = defaults.MaxCodeBytes
	}
	if p.DefaultTimeout <= 0 {
		p.DefaultTimeout = defaults.DefaultTimeout
	}
	if p.MaxTimeout <= 0 {
		p.MaxTimeout = defaults.MaxTimeout
	}
	if p.DefaultTimeout > p.MaxTimeout {
		p.DefaultTimeout = p.MaxTimeout
	}
	if p.MaxOutputBytes <= 0 {
		p.MaxOutputBytes = defaults.MaxOutputBytes
	}
	if len(p.AllowedLanguages) == 0 {
		p.AllowedLanguages = append([]runtime.Language(nil), defaults.AllowedLanguages...)
	}
	if len(p.AllowedLanguageTags) == 0 {
		p.AllowedLanguageTags = append([]string(nil), defaults.AllowedLanguageTags...)
	}
	return p
}
