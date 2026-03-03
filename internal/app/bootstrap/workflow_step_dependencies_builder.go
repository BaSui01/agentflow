package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/browser"
	"github.com/BaSui01/agentflow/agent/deliberation"
	"github.com/BaSui01/agentflow/agent/execution"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/engine"
	"go.uber.org/zap"
)

// WorkflowAgentResolver resolves an agent instance by ID for workflow agent steps.
type WorkflowAgentResolver func(ctx context.Context, agentID string) (agent.Agent, error)

// WorkflowRuntimeOptions carries optional runtime integrations for workflow steps.
type WorkflowRuntimeOptions struct {
	LLMProvider       llm.Provider
	DefaultModel      string
	AgentResolver     WorkflowAgentResolver
	RetrievalStore    rag.VectorStore
	EmbeddingProvider rag.EmbeddingProvider
	CheckpointStore   agent.CheckpointStore
	HITLManager       *hitl.InterruptManager
}

func buildStepDependencies(opts WorkflowRuntimeOptions, logger *zap.Logger) engine.StepDependencies {
	toolRegistry, codeTool := buildHostedWorkflowTools(opts, logger)
	hitlManager := opts.HITLManager
	if hitlManager == nil {
		hitlManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), logger)
	}
	requester := deliberation.NewHITLInterruptAdapter(hitlManager)
	_ = ensureAutoApproveHITL(hitlManager, logger)

	return engine.StepDependencies{
		Gateway:      newLLMProviderGateway(opts.LLMProvider, opts.DefaultModel),
		ToolRegistry: hostedToolRegistryAdapter{registry: toolRegistry},
		HumanHandler: hitlHumanInputHandler{requester: requester},
		AgentExecutor: resolverAgentExecutor{
			resolver: opts.AgentResolver,
		},
		CodeHandler: hostedCodeHandler{tool: codeTool}.Execute,
	}
}

func buildHostedWorkflowTools(opts WorkflowRuntimeOptions, logger *zap.Logger) (*hosted.ToolRegistry, *hosted.CodeExecTool) {
	registry := hosted.NewToolRegistry(logger)

	sandboxCfg := execution.DefaultSandboxConfig()
	sandboxCfg.Mode = execution.ModeNative
	sandboxCfg.AllowedLanguages = []execution.Language{
		execution.LangPython,
		execution.LangJavaScript,
		execution.LangBash,
		execution.LangGo,
	}
	sandbox := execution.NewSandboxExecutor(sandboxCfg, execution.NewRealProcessBackend(logger, false), logger)
	adapter := execution.NewHostedAdapter(sandbox, logger)
	codeTool := hosted.NewCodeExecTool(hosted.CodeExecConfig{
		Executor: adapter,
		Logger:   logger,
	})
	registry.Register(codeTool)

	_ = codeTool.Type()
	_ = codeTool.Description()
	_ = codeTool.Schema()

	browserFactory := browser.NewChromeDPBrowserFactory(logger)
	browserTool := browser.NewBrowserTool(browserFactory, browser.DefaultBrowserConfig(), logger)
	browserHostedTool := hosted.NewBrowserAutomationTool(browserTool, logger)
	registry.Register(browserHostedTool)
	_ = browserHostedTool.Type()
	_ = browserHostedTool.Description()
	_ = browserHostedTool.Schema()

	if opts.RetrievalStore != nil && opts.EmbeddingProvider != nil {
		retrievalTool := hosted.NewRetrievalTool(
			ragHostedRetrievalStore{
				store:    opts.RetrievalStore,
				embedder: opts.EmbeddingProvider,
			},
			10,
			logger,
		)
		registry.Register(retrievalTool)
		_ = retrievalTool.Type()
		_ = retrievalTool.Description()
		_ = retrievalTool.Schema()
	}

	return registry, codeTool
}

type llmProviderGateway struct {
	provider     llm.Provider
	defaultModel string
}

func newLLMProviderGateway(provider llm.Provider, defaultModel string) core.GatewayLike {
	return &llmProviderGateway{
		provider:     provider,
		defaultModel: defaultModel,
	}
}

func (g *llmProviderGateway) Invoke(ctx context.Context, req *core.LLMRequest) (*core.LLMResponse, error) {
	if g.provider == nil {
		return nil, fmt.Errorf("workflow LLM provider is not configured")
	}

	model := req.Model
	if model == "" {
		model = g.defaultModel
	}

	completionReq := &llm.ChatRequest{
		Model: model,
		Messages: []types.Message{
			{
				Role:    types.RoleUser,
				Content: req.Prompt,
			},
		},
		MaxTokens:   req.MaxTokens,
		Temperature: float32(req.Temperature),
		Metadata:    req.Metadata,
	}

	resp, err := g.provider.Completion(ctx, completionReq)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm response has no choices")
	}

	out := &core.LLMResponse{
		Content: resp.Choices[0].Message.Content,
		Model:   resp.Model,
	}
	out.Usage = &core.LLMUsage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	return out, nil
}

type hostedToolRegistryAdapter struct {
	registry *hosted.ToolRegistry
}

func (a hostedToolRegistryAdapter) ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("workflow tool registry is not configured")
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal tool params: %w", err)
	}

	raw, err := a.registry.Execute(ctx, name, payload)
	if err != nil {
		return nil, err
	}

	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw), nil
	}
	return out, nil
}

type hitlHumanInputHandler struct {
	requester deliberation.InterruptRequester
}

func (h hitlHumanInputHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (any, error) {
	if h.requester == nil {
		return nil, fmt.Errorf("workflow hitl requester is not configured")
	}
	hitlOptions := make([]deliberation.ApprovalOption, 0, len(options))
	for idx, opt := range options {
		id := opt
		if id == "" {
			id = fmt.Sprintf("option_%d", idx+1)
		}
		hitlOptions = append(hitlOptions, deliberation.ApprovalOption{
			ID:    id,
			Label: opt,
		})
	}

	resp, err := h.requester.RequestApproval(ctx, deliberation.ApprovalRequest{
		Title:       "Workflow human input required",
		Description: prompt,
		Options:     hitlOptions,
		Timeout:     30 * time.Second,
		Data: map[string]any{
			"input_type": inputType,
		},
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"action":   resp.Action,
		"feedback": resp.Feedback,
		"data":     resp.Data,
	}, nil
}

type resolverAgentExecutor struct {
	resolver WorkflowAgentResolver
}

func (e resolverAgentExecutor) Execute(ctx context.Context, input any) (any, error) {
	if e.resolver == nil {
		return nil, fmt.Errorf("workflow agent resolver is not configured")
	}

	inputMap, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("workflow agent step input must be map[string]any")
	}

	agentID, _ := inputMap["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("workflow agent step requires input.agent_id")
	}
	content, _ := inputMap["content"].(string)

	ag, err := e.resolver(ctx, agentID)
	if err != nil {
		return nil, err
	}

	out, err := ag.Execute(ctx, &agent.Input{
		Content: content,
		Context: inputMap,
	})
	if err != nil {
		return nil, err
	}
	return out.Content, nil
}

type hostedCodeHandler struct {
	tool *hosted.CodeExecTool
}

func (h hostedCodeHandler) Execute(ctx context.Context, input core.StepInput) (map[string]any, error) {
	if h.tool == nil {
		return nil, fmt.Errorf("workflow code tool is not configured")
	}

	language, _ := input.Data["language"].(string)
	if language == "" {
		language = "python"
	}
	code, _ := input.Data["code"].(string)
	if code == "" {
		return nil, fmt.Errorf("workflow code step requires input.code")
	}

	payload, err := json.Marshal(map[string]any{
		"language": language,
		"code":     code,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal code execution request: %w", err)
	}

	raw, err := h.tool.Execute(ctx, payload)
	if err != nil {
		return nil, err
	}

	var output hosted.CodeExecOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, fmt.Errorf("decode code execution output: %w", err)
	}

	return map[string]any{
		"stdout":    output.Stdout,
		"stderr":    output.Stderr,
		"exit_code": output.ExitCode,
		"duration":  output.Duration.String(),
	}, nil
}

type ragHostedRetrievalStore struct {
	store    rag.VectorStore
	embedder rag.EmbeddingProvider
}

type workflowCheckpointManagerAdapter struct {
	manager *agent.CheckpointManager
}

func (a workflowCheckpointManagerAdapter) SaveCheckpoint(ctx context.Context, cp *workflow.EnhancedCheckpoint) error {
	if a.manager == nil {
		return fmt.Errorf("workflow checkpoint manager is not configured")
	}
	if cp == nil {
		return fmt.Errorf("workflow checkpoint is nil")
	}

	payload := &agent.Checkpoint{
		ID:       cp.ID,
		ThreadID: cp.ThreadID,
		AgentID:  "workflow",
		Version:  cp.Version,
		State:    agent.StateReady,
		Messages: []agent.CheckpointMessage{},
		Metadata: map[string]any{
			"workflow_id":       cp.WorkflowID,
			"node_id":           cp.NodeID,
			"completed_nodes":   cp.CompletedNodes,
			"pending_nodes":     cp.PendingNodes,
			"has_snapshot":      cp.Snapshot != nil,
			"checkpoint_source": "workflow_dag",
		},
		CreatedAt: cp.CreatedAt,
		ParentID:  cp.ParentID,
		ExecutionContext: &agent.ExecutionContext{
			WorkflowID:  cp.WorkflowID,
			CurrentNode: cp.NodeID,
			NodeResults: cp.NodeResults,
			Variables:   cp.Variables,
		},
	}
	return a.manager.SaveCheckpoint(ctx, payload)
}

func buildWorkflowCheckpointManager(opts WorkflowRuntimeOptions) workflow.CheckpointManager {
	if opts.CheckpointStore == nil {
		return nil
	}
	return workflowCheckpointManagerAdapter{manager: agent.NewCheckpointManager(opts.CheckpointStore, nil)}
}

func ensureAutoApproveHITL(manager *hitl.InterruptManager, logger *zap.Logger) *hitl.InterruptManager {
	if manager == nil {
		return nil
	}
	manager.RegisterHandler(hitl.InterruptTypeApproval, func(ctx context.Context, interrupt *hitl.Interrupt) error {
		if interrupt == nil {
			return nil
		}
		optionID := "approve"
		if len(interrupt.Options) > 0 {
			optionID = interrupt.Options[0].ID
		}
		return manager.ResolveInterrupt(ctx, interrupt.ID, &hitl.Response{
			OptionID: optionID,
			Comment:  "auto approved by workflow runtime",
			Approved: true,
		})
	})
	if logger != nil {
		logger.Debug("workflow HITL auto-approve handler registered")
	}
	return manager
}

func (s ragHostedRetrievalStore) Retrieve(ctx context.Context, query string, topK int) ([]hosted.RetrievalResult, error) {
	if s.store == nil || s.embedder == nil {
		return nil, fmt.Errorf("workflow retrieval dependencies are not configured")
	}

	emb, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	results, err := s.store.Search(ctx, emb, topK)
	if err != nil {
		return nil, err
	}

	out := make([]hosted.RetrievalResult, 0, len(results))
	for _, item := range results {
		out = append(out, hosted.RetrievalResult{
			DocumentID: item.Document.ID,
			Content:    item.Document.Content,
			Score:      item.Score,
			Metadata:   item.Document.Metadata,
		})
	}
	return out, nil
}
