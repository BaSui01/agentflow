package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
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
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
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
	workflowMaxInt                    = int64(1<<(strconv.IntSize-1) - 1)
	workflowMinInt                    = -workflowMaxInt - 1
)

type workflowCodeExecutionPolicy struct {
	MaxCodeBytes        int
	DefaultTimeout      time.Duration
	MaxTimeout          time.Duration
	MaxOutputBytes      int
	AllowedLanguages    []runtime.Language
	AllowedLanguageTags []string
}

type workflowCodeExecutionRequest struct {
	Language       string
	Code           string
	TimeoutSeconds int
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

func buildStepDependencies(opts WorkflowRuntimeOptions, logger *zap.Logger) engine.StepDependencies {
	toolRegistry, codeTool := buildHostedWorkflowTools(opts, logger)
	hitlManager := opts.HITLManager
	if hitlManager == nil {
		hitlManager = hitl.NewInterruptManager(hitl.NewInMemoryInterruptStore(), logger)
	}
	requester := planning.NewHITLInterruptAdapter(hitlManager)
	agentExecutor := resolverAgentExecutor{
		resolver: opts.AgentResolver,
	}
	workflowTools := hostedToolRegistryAdapter{
		registry:      toolRegistry,
		authorization: opts.AuthorizationService,
	}

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

	completionReq := &llmcore.ChatRequest{
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
	chatResp, ok := resp.Output.(*llmcore.ChatResponse)
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

	streamReq := &llmcore.ChatRequest{
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
		StreamOptions: &llmcore.StreamOptions{
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

			if typed, ok := chunk.Output.(*llmcore.StreamChunk); ok && typed != nil {
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
	registry      *hosted.ToolRegistry
	authorization usecase.AuthorizationService
}

func (a hostedToolRegistryAdapter) ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("workflow tool registry is not configured")
	}

	payload, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal tool params: %w", err)
	}

	if err := a.authorize(ctx, name, payload, cloneAnyMap(params)); err != nil {
		return nil, err
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

func (a hostedToolRegistryAdapter) Execute(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if a.registry == nil {
		return nil, fmt.Errorf("workflow tool registry is not configured")
	}
	if err := a.authorize(ctx, name, args, workflowArgumentsFromRaw(args)); err != nil {
		return nil, err
	}
	return a.registry.Execute(ctx, name, args)
}

func (a hostedToolRegistryAdapter) authorize(ctx context.Context, name string, raw json.RawMessage, args map[string]any) error {
	if a.authorization == nil {
		return nil
	}

	tool, ok := a.registry.Get(name)
	if !ok {
		return fmt.Errorf("tool not found: %s", name)
	}
	resourceKind, riskTier, toolType, hostedRisk := workflowHostedToolAuthorizationShape(tool, name)
	authContext := map[string]any{
		"arguments":        args,
		"args_fingerprint": workflowRawFingerprint(raw),
		"metadata": map[string]string{
			"runtime":          "workflow",
			"hosted_tool_type": toolType,
			"hosted_tool_risk": hostedRisk,
		},
	}
	return authorizeWorkflowStep(ctx, a.authorization, workflowAuthorizationRequest(
		ctx,
		resourceKind,
		name,
		types.ActionExecute,
		riskTier,
		authContext,
	))
}

type hitlHumanInputHandler struct {
	requester     planning.InterruptRequester
	authorization usecase.AuthorizationService
}

func (h hitlHumanInputHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (*core.HumanInputResult, error) {
	if h.requester == nil {
		return nil, fmt.Errorf("workflow hitl requester is not configured")
	}
	if err := authorizeWorkflowStep(ctx, h.authorization, workflowAuthorizationRequest(
		ctx,
		types.ResourceWorkflow,
		"human_input",
		types.ActionApprove,
		types.RiskMutating,
		map[string]any{
			"arguments": map[string]any{
				"input_type":         inputType,
				"options_count":      len(options),
				"prompt_bytes":       len(prompt),
				"prompt_fingerprint": workflowStringFingerprint(prompt),
			},
			"metadata": map[string]string{
				"runtime":       "workflow",
				"workflow_step": "human_input",
			},
		},
	)); err != nil {
		return nil, err
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
	tool          *hosted.CodeExecTool
	authorization usecase.AuthorizationService
	policy        workflowCodeExecutionPolicy
}

func (h hostedCodeHandler) Execute(ctx context.Context, input core.StepInput) (map[string]any, error) {
	if h.tool == nil {
		return nil, fmt.Errorf("workflow code tool is not configured")
	}

	req, policy, err := h.codeExecutionRequest(input)
	if err != nil {
		return nil, err
	}

	if err := authorizeWorkflowStep(ctx, h.authorization, workflowAuthorizationRequest(
		ctx,
		types.ResourceCodeExec,
		h.tool.Name(),
		types.ActionExecute,
		types.RiskExecution,
		h.authorizationContext(req, policy),
	)); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(map[string]any{
		"language":        req.Language,
		"code":            req.Code,
		"timeout_seconds": req.TimeoutSeconds,
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

func (h hostedCodeHandler) codeExecutionRequest(input core.StepInput) (workflowCodeExecutionRequest, workflowCodeExecutionPolicy, error) {
	policy := h.policy.normalized()
	language, _ := input.Data["language"].(string)
	if language == "" {
		language = "python"
	}
	code, _ := input.Data["code"].(string)
	if code == "" {
		return workflowCodeExecutionRequest{}, policy, fmt.Errorf("workflow code step requires input.code")
	}
	if len(code) > policy.MaxCodeBytes {
		return workflowCodeExecutionRequest{}, policy, fmt.Errorf("workflow code step input.code exceeds max size: %d > %d bytes", len(code), policy.MaxCodeBytes)
	}

	timeoutSeconds, err := workflowCodeTimeoutSeconds(input.Data["timeout_seconds"], policy.DefaultTimeout, policy.MaxTimeout)
	if err != nil {
		return workflowCodeExecutionRequest{}, policy, err
	}

	return workflowCodeExecutionRequest{
		Language:       language,
		Code:           code,
		TimeoutSeconds: timeoutSeconds,
	}, policy, nil
}

func (h hostedCodeHandler) authorizationContext(req workflowCodeExecutionRequest, policy workflowCodeExecutionPolicy) map[string]any {
	return map[string]any{
		"arguments": map[string]any{
			"language":            req.Language,
			"code_bytes":          len(req.Code),
			"code_fingerprint":    workflowStringFingerprint(req.Code),
			"timeout_seconds":     req.TimeoutSeconds,
			"max_code_bytes":      policy.MaxCodeBytes,
			"max_timeout_seconds": int(policy.MaxTimeout.Seconds()),
			"max_output_bytes":    policy.MaxOutputBytes,
			"allowed_languages":   append([]string(nil), policy.AllowedLanguageTags...),
		},
		"metadata": map[string]string{
			"runtime":          "workflow",
			"hosted_tool_type": string(h.tool.Type()),
			"hosted_tool_risk": "requires_approval",
		},
	}
}

func workflowCodeTimeoutSeconds(value any, defaultTimeout, maxTimeout time.Duration) (int, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = defaultWorkflowCodeTimeoutSeconds * time.Second
	}
	if maxTimeout <= 0 {
		maxTimeout = defaultTimeout
	}

	defaultSeconds := int(defaultTimeout.Seconds())
	maxSeconds := int(maxTimeout.Seconds())
	if defaultSeconds > maxSeconds {
		defaultSeconds = maxSeconds
	}
	if value == nil {
		return defaultSeconds, nil
	}

	seconds, err := workflowIntegerSeconds(value)
	if err != nil {
		return 0, fmt.Errorf("workflow code step timeout_seconds must be an integer: %w", err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("workflow code step timeout_seconds must be positive")
	}
	if seconds > maxSeconds {
		return 0, fmt.Errorf("workflow code step timeout_seconds exceeds max: %d > %d", seconds, maxSeconds)
	}
	return seconds, nil
}

func workflowIntegerSeconds(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		if v > workflowMaxInt || v < workflowMinInt {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case uint:
		if uint64(v) > uint64(workflowMaxInt) {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		if uint64(v) > uint64(workflowMaxInt) {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case uint64:
		if v > uint64(workflowMaxInt) {
			return 0, fmt.Errorf("value out of range")
		}
		return int(v), nil
	case float64:
		if math.Trunc(v) != v || v > float64(workflowMaxInt) || v < float64(workflowMinInt) {
			return 0, fmt.Errorf("value must be a whole number")
		}
		return int(v), nil
	case float32:
		f := float64(v)
		if math.Trunc(f) != f || f > float64(workflowMaxInt) || f < float64(workflowMinInt) {
			return 0, fmt.Errorf("value must be a whole number")
		}
		return int(v), nil
	case json.Number:
		i64, err := v.Int64()
		if err != nil {
			return 0, err
		}
		if i64 > workflowMaxInt || i64 < workflowMinInt {
			return 0, fmt.Errorf("value out of range")
		}
		return int(i64), nil
	default:
		return 0, fmt.Errorf("got %T", value)
	}
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

func authorizeWorkflowStep(ctx context.Context, service usecase.AuthorizationService, req types.AuthorizationRequest) error {
	if service == nil {
		return nil
	}
	decision, err := service.Authorize(ctx, req)
	if err != nil {
		return fmt.Errorf("authorize workflow %s %q: %w", req.ResourceKind, req.ResourceID, err)
	}
	if decision == nil {
		return fmt.Errorf("authorize workflow %s %q: empty decision", req.ResourceKind, req.ResourceID)
	}

	switch decision.Decision {
	case types.DecisionAllow:
		return nil
	case types.DecisionDeny:
		return workflowAuthorizationDecisionError("authorization denied", req, decision)
	case types.DecisionRequireApproval:
		return workflowAuthorizationDecisionError("authorization approval required", req, decision)
	default:
		return fmt.Errorf("authorize workflow %s %q: unknown decision %q", req.ResourceKind, req.ResourceID, decision.Decision)
	}
}

func workflowAuthorizationDecisionError(prefix string, req types.AuthorizationRequest, decision *types.AuthorizationDecision) error {
	if decision.ApprovalID != "" {
		return fmt.Errorf("%s for workflow %s %q (approval_id=%s): %s", prefix, req.ResourceKind, req.ResourceID, decision.ApprovalID, decision.Reason)
	}
	return fmt.Errorf("%s for workflow %s %q: %s", prefix, req.ResourceKind, req.ResourceID, decision.Reason)
}

func workflowAuthorizationRequest(
	ctx context.Context,
	resourceKind types.ResourceKind,
	resourceID string,
	action types.ActionKind,
	riskTier types.RiskTier,
	values map[string]any,
) types.AuthorizationRequest {
	authContext := cloneAnyMap(values)
	if authContext == nil {
		authContext = make(map[string]any, 8)
	}
	metadata := workflowAuthorizationMetadata(authContext)
	metadata["resource_kind"] = string(resourceKind)
	metadata["resource_id"] = resourceID
	metadata["action"] = string(action)
	metadata["risk_tier"] = string(riskTier)

	var principal types.Principal
	if existing, ok := types.PrincipalFromContext(ctx); ok {
		principal = existing
	}
	if traceID, ok := types.TraceID(ctx); ok {
		authContext["trace_id"] = traceID
		metadata["trace_id"] = traceID
	}
	if runID, ok := types.RunID(ctx); ok {
		authContext["run_id"] = runID
		authContext["session_id"] = runID
		metadata["run_id"] = runID
	}
	if agentID, ok := types.AgentID(ctx); ok {
		authContext["agent_id"] = agentID
		metadata["agent_id"] = agentID
		if principal.ID == "" {
			principal.Kind = types.PrincipalAgent
			principal.ID = agentID
		}
	}
	if userID, ok := types.UserID(ctx); ok {
		authContext["user_id"] = userID
		metadata["user_id"] = userID
		if principal.ID == "" {
			principal.Kind = types.PrincipalUser
			principal.ID = userID
		}
	}
	if roles, ok := types.Roles(ctx); ok {
		principal.Roles = append([]string(nil), roles...)
	}
	authContext["metadata"] = metadata

	return types.AuthorizationRequest{
		Principal:    principal,
		ResourceKind: resourceKind,
		ResourceID:   resourceID,
		Action:       action,
		RiskTier:     riskTier,
		Context:      authContext,
	}
}

func workflowAuthorizationMetadata(values map[string]any) map[string]string {
	out := map[string]string{"runtime": "workflow"}
	if values == nil {
		return out
	}
	switch metadata := values["metadata"].(type) {
	case map[string]string:
		for k, v := range metadata {
			out[k] = v
		}
	case map[string]any:
		for k, v := range metadata {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func workflowHostedToolAuthorizationShape(tool hosted.HostedTool, name string) (types.ResourceKind, types.RiskTier, string, string) {
	toolType := ""
	if tool != nil {
		toolType = string(tool.Type())
	}
	resourceKind := hosted.ClassifyHostedToolResourceKind(tool)
	if reporter, ok := tool.(interface{ AuthorizationResourceKind() types.ResourceKind }); ok {
		resourceKind = reporter.AuthorizationResourceKind()
	}
	riskTier := hosted.ClassifyHostedToolRiskTier(tool)
	if reporter, ok := tool.(interface{ AuthorizationRiskTier() types.RiskTier }); ok {
		riskTier = reporter.AuthorizationRiskTier()
	}
	return resourceKind,
		riskTier,
		toolType,
		hosted.ClassifyHostedToolPermissionRisk(tool)
}

func workflowArgumentsFromRaw(raw json.RawMessage) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil
	}
	return args
}

func workflowRawFingerprint(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func workflowStringFingerprint(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum)
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
