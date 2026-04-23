package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/capabilities/planning"
	"github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	ragcore "github.com/BaSui01/agentflow/rag/core"
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
	LLMGateway              llmcore.Gateway
	DefaultModel            string
	AgentResolver           WorkflowAgentResolver
	RetrievalStore          ragcore.VectorStore
	EmbeddingProvider       ragcore.EmbeddingProvider
	CheckpointStore         agentcheckpoint.Store
	WorkflowCheckpointStore workflow.CheckpointStore
	HITLManager             *hitl.InterruptManager
}

func buildStepDependencies(opts WorkflowRuntimeOptions, logger *zap.Logger) engine.StepDependencies {
	toolRegistry, codeTool := buildHostedWorkflowTools(opts, logger)
	hitlManager := opts.HITLManager
	if hitlManager == nil {
		hitlManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), logger)
	}
	requester := planning.NewHITLInterruptAdapter(hitlManager)
	_ = ensureAutoApproveHITL(hitlManager, logger)
	agentExecutor := resolverAgentExecutor{
		resolver: opts.AgentResolver,
	}

	return engine.StepDependencies{
		Gateway:       newWorkflowGatewayAdapter(opts.LLMGateway, opts.DefaultModel),
		ToolRegistry:  hostedToolRegistryAdapter{registry: toolRegistry},
		ChainRegistry: toolRegistry,
		HumanHandler:  hitlHumanInputHandler{requester: requester},
		AgentExecutor: agentExecutor,
		AgentResolver: agentExecutor,
		CodeHandler:   hostedCodeHandler{tool: codeTool}.Execute,
	}
}

func buildHostedWorkflowTools(opts WorkflowRuntimeOptions, logger *zap.Logger) (*hosted.ToolRegistry, *hosted.CodeExecTool) {
	registry := hosted.NewToolRegistry(logger)

	sandboxCfg := runtime.DefaultSandboxConfig()
	sandboxCfg.Mode = runtime.ModeNative
	sandboxCfg.AllowedLanguages = []runtime.Language{
		runtime.LangPython,
		runtime.LangJavaScript,
		runtime.LangBash,
		runtime.LangGo,
	}
	sandbox := runtime.NewSandboxExecutor(sandboxCfg, runtime.NewRealProcessBackend(logger, false), logger)
	adapter := runtime.NewHostedAdapter(sandbox, logger)
	codeTool := hosted.NewCodeExecTool(hosted.CodeExecConfig{
		Executor: adapter,
		Logger:   logger,
	})
	registry.Register(codeTool)

	_ = codeTool.Type()
	_ = codeTool.Description()
	_ = codeTool.Schema()

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

type workflowGatewayAdapter struct {
	gateway      llmcore.Gateway
	defaultModel string
}

func newWorkflowGatewayAdapter(gateway llmcore.Gateway, defaultModel string) core.GatewayLike {
	return &workflowGatewayAdapter{
		gateway:      gateway,
		defaultModel: defaultModel,
	}
}

func (g *workflowGatewayAdapter) Invoke(ctx context.Context, req *core.LLMRequest) (*core.LLMResponse, error) {
	if g.gateway == nil {
		return nil, fmt.Errorf("workflow LLM gateway is not configured")
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

	resp, err := g.gateway.Invoke(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  model,
		Payload:    completionReq,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	chatResp, ok := resp.Output.(*llm.ChatResponse)
	if !ok || chatResp == nil {
		return nil, fmt.Errorf("workflow gateway returned invalid chat output type %T", resp.Output)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("llm response has no choices")
	}

	out := &core.LLMResponse{
		Content: chatResp.Choices[0].Message.Content,
		Model:   chatResp.Model,
	}
	out.Usage = &core.LLMUsage{
		PromptTokens:     chatResp.Usage.PromptTokens,
		CompletionTokens: chatResp.Usage.CompletionTokens,
		TotalTokens:      chatResp.Usage.TotalTokens,
	}
	return out, nil
}

func (g *workflowGatewayAdapter) Stream(ctx context.Context, req *core.LLMRequest) (<-chan core.LLMStreamChunk, error) {
	if g.gateway == nil {
		return nil, fmt.Errorf("workflow LLM gateway is not configured")
	}

	model := req.Model
	if model == "" {
		model = g.defaultModel
	}

	streamReq := &llm.ChatRequest{
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
		StreamOptions: &llm.StreamOptions{
			IncludeUsage:      true,
			ChunkIncludeUsage: true,
		},
	}

	source, err := g.gateway.Stream(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  model,
		Payload:    streamReq,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return nil, err
	}

	out := make(chan core.LLMStreamChunk)
	go func() {
		defer close(out)

		var finalUsage *core.LLMUsage
		finalModel := model

		for chunk := range source {
			if chunk.Err != nil {
				out <- core.LLMStreamChunk{Err: chunk.Err}
				continue
			}
			if chunk.Usage != nil {
				usage := &core.LLMUsage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				}
				finalUsage = usage
			}

			streamChunk := core.LLMStreamChunk{
				Model: finalModel,
				Usage: finalUsage,
				Done:  chunk.Done,
			}

			if typed, ok := chunk.Output.(*llm.StreamChunk); ok && typed != nil {
				streamChunk.Delta = typed.Delta.Content
				streamChunk.ReasoningContent = typed.Delta.ReasoningContent
				if typed.Model != "" {
					streamChunk.Model = typed.Model
					finalModel = typed.Model
				}
				if typed.Usage != nil {
					usage := &core.LLMUsage{
						PromptTokens:     typed.Usage.PromptTokens,
						CompletionTokens: typed.Usage.CompletionTokens,
						TotalTokens:      typed.Usage.TotalTokens,
					}
					streamChunk.Usage = usage
					finalUsage = usage
				}
				if typed.Err != nil {
					streamChunk.Err = typed.Err
				}
			}

			out <- streamChunk
		}

		if finalUsage != nil {
			out <- core.LLMStreamChunk{
				Model: finalModel,
				Usage: finalUsage,
				Done:  true,
			}
		}
	}()

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
	requester planning.InterruptRequester
}

func (h hitlHumanInputHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (*core.HumanInputResult, error) {
	if h.requester == nil {
		return nil, fmt.Errorf("workflow hitl requester is not configured")
	}
	hitlOptions := make([]planning.ApprovalOption, 0, len(options))
	for idx, opt := range options {
		id := opt
		if id == "" {
			id = fmt.Sprintf("option_%d", idx+1)
		}
		hitlOptions = append(hitlOptions, planning.ApprovalOption{
			ID:    id,
			Label: opt,
		})
	}

	resp, err := h.requester.RequestApproval(ctx, planning.ApprovalRequest{
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

	optionID := resp.Action
	if data, ok := resp.Data.(map[string]any); ok {
		if oid, ok := data["option_id"].(string); ok && oid != "" {
			optionID = oid
		}
	}
	return &core.HumanInputResult{
		Value:    resp.Feedback,
		OptionID: optionID,
	}, nil
}

type resolverAgentExecutor struct {
	resolver WorkflowAgentResolver
}

func (e resolverAgentExecutor) ResolveAgent(ctx context.Context, agentID string) (agent.Agent, error) {
	if e.resolver == nil {
		return nil, fmt.Errorf("workflow agent resolver is not configured")
	}
	if agentID == "" {
		return nil, fmt.Errorf("workflow agent resolver requires agent id")
	}
	return e.resolver(ctx, agentID)
}

func (e resolverAgentExecutor) Execute(ctx context.Context, input map[string]any) (*core.AgentExecutionOutput, error) {
	if e.resolver == nil {
		return nil, fmt.Errorf("workflow agent resolver is not configured")
	}

	agentID, _ := input["agent_id"].(string)
	if agentID == "" {
		return nil, fmt.Errorf("workflow agent step requires input.agent_id")
	}
	content, _ := input["content"].(string)

	ag, err := e.ResolveAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	out, err := ag.Execute(ctx, &agent.Input{
		Content: content,
		Context: input,
	})
	if err != nil {
		return nil, err
	}
	return &core.AgentExecutionOutput{
		Content:      out.Content,
		TokensUsed:   out.TokensUsed,
		Cost:         out.Cost,
		Duration:     out.Duration,
		FinishReason: out.FinishReason,
	}, nil
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
	store    ragcore.VectorStore
	embedder ragcore.EmbeddingProvider
}

func buildWorkflowCheckpointManager(opts WorkflowRuntimeOptions) workflow.CheckpointManager {
	if opts.WorkflowCheckpointStore != nil {
		return checkpointStoreManagerAdapter{store: opts.WorkflowCheckpointStore}
	}
	if opts.CheckpointStore == nil {
		return nil
	}
	return workflowCheckpointManagerAdapter{manager: agent.NewCheckpointManagerFromNativeStore(opts.CheckpointStore, nil)}
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

type checkpointStoreManagerAdapter struct {
	store workflow.CheckpointStore
}

func (a checkpointStoreManagerAdapter) SaveCheckpoint(ctx context.Context, cp *workflow.EnhancedCheckpoint) error {
	return a.store.Save(ctx, cp)
}

func ensureAutoApproveHITL(manager *hitl.InterruptManager, logger *zap.Logger) *hitl.InterruptManager {
	if manager == nil {
		return nil
	}
	registered := manager.RegisterNamedHandler(hitl.InterruptTypeApproval, "workflow_auto_approve", func(ctx context.Context, interrupt *hitl.Interrupt) error {
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
	if logger != nil && registered {
		logger.Debug("workflow HITL auto-approve handler registered")
	}
	return manager
}

func (s ragHostedRetrievalStore) Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
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

	out := make([]types.RetrievalRecord, 0, len(results))
	for _, item := range results {
		out = append(out, types.RetrievalRecord{
			DocID:   item.Document.ID,
			Content: item.Document.Content,
			Score:   item.Score,
		})
	}
	return out, nil
}
