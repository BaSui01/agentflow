package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ChatConverter centralizes request/response conversion between API and LLM layers.
// Implementations are provided by the handlers layer.
type ChatConverter interface {
	ToLLMRequest(req *api.ChatRequest) *llm.ChatRequest
	ToAPIResponse(resp *llm.ChatResponse) *api.ChatResponse
}

// ChatService encapsulates chat routing and gateway invocation logic.
type ChatService interface {
	Complete(ctx context.Context, req *api.ChatRequest) (*ChatCompletionResult, *types.Error)
	Stream(ctx context.Context, req *api.ChatRequest) (<-chan llmcore.UnifiedChunk, *types.Error)
	SupportedRoutePolicies() []string
	DefaultRoutePolicy() string
}

// ChatCompletionResult captures completion output and execution metadata.
type ChatCompletionResult struct {
	Response *api.ChatResponse
	Raw      *llm.ChatResponse
	Duration time.Duration
}

const (
	defaultChatReActIterations = 10
	chatToolRuntimeAgentID     = "chat"
)

// DefaultChatService is the default ChatService implementation.
type DefaultChatService struct {
	gateway      llmcore.Gateway
	chatProvider llm.Provider
	toolManager  agent.ToolManager
	converter    ChatConverter
	logger       *zap.Logger
}

// NewDefaultChatService constructs a ChatService with gateway, provider, tool manager, and converter.
func NewDefaultChatService(
	gateway llmcore.Gateway,
	chatProvider llm.Provider,
	toolManager agent.ToolManager,
	converter ChatConverter,
	logger *zap.Logger,
) ChatService {
	if logger == nil {
		panic("usecase.ChatService: logger is required and cannot be nil")
	}
	return &DefaultChatService{
		gateway:      gateway,
		chatProvider: chatProvider,
		toolManager:  toolManager,
		converter:    converter,
		logger:       logger,
	}
}

func (s *DefaultChatService) Complete(ctx context.Context, req *api.ChatRequest) (*ChatCompletionResult, *types.Error) {
	unifiedReq, llmReq, err := s.buildUnifiedRequest(req)
	if err != nil {
		return nil, err
	}

	if llmReq.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, llmReq.Timeout)
		defer cancel()
	}

	reactReq, localToolsEnabled := s.buildLocalToolRequest(llmReq)
	start := time.Now()
	if localToolsEnabled {
		raw, reactErr := s.executeLocalReAct(ctx, reactReq)
		if reactErr != nil {
			return nil, reactErr
		}
		return &ChatCompletionResult{
			Response: s.converter.ToAPIResponse(raw),
			Raw:      raw,
			Duration: time.Since(start),
		}, nil
	}

	resp, invokeErr := s.gateway.Invoke(ctx, unifiedReq)
	if invokeErr != nil {
		return nil, toTypesChatError(invokeErr)
	}
	raw, ok := resp.Output.(*llm.ChatResponse)
	if !ok || raw == nil {
		return nil, types.NewInternalError("invalid chat gateway response")
	}
	return &ChatCompletionResult{
		Response: s.converter.ToAPIResponse(raw),
		Raw:      raw,
		Duration: time.Since(start),
	}, nil
}

func (s *DefaultChatService) Stream(ctx context.Context, req *api.ChatRequest) (<-chan llmcore.UnifiedChunk, *types.Error) {
	unifiedReq, llmReq, err := s.buildUnifiedRequest(req)
	if err != nil {
		return nil, err
	}

	reactReq, localToolsEnabled := s.buildLocalToolRequest(llmReq)
	if localToolsEnabled {
		return s.streamLocalReAct(ctx, reactReq)
	}

	stream, streamErr := s.gateway.Stream(ctx, unifiedReq)
	if streamErr != nil {
		return nil, toTypesChatError(streamErr)
	}
	return stream, nil
}

func (s *DefaultChatService) SupportedRoutePolicies() []string {
	return SupportedRoutePolicies()
}

func (s *DefaultChatService) DefaultRoutePolicy() string {
	return string(llmcore.RoutePolicyBalanced)
}

func (s *DefaultChatService) buildUnifiedRequest(req *api.ChatRequest) (*llmcore.UnifiedRequest, *llm.ChatRequest, *types.Error) {
	if req == nil {
		return nil, nil, types.NewInvalidRequestError("request is required")
	}

	provider, err := NormalizeProviderHint(req.Provider)
	if err != nil {
		return nil, nil, err
	}
	routePolicy, err := NormalizeRoutePolicy(req.RoutePolicy)
	if err != nil {
		return nil, nil, err
	}
	endpointMode, err := NormalizeEndpointMode(req.EndpointMode)
	if err != nil {
		return nil, nil, err
	}

	llmReq := s.converter.ToLLMRequest(req)
	llmReq.Metadata = ApplyChatRouteMetadata(llmReq.Metadata, provider, routePolicy, endpointMode)
	llmReq.Tags = NormalizeRouteTags(llmReq.Tags)

	return &llmcore.UnifiedRequest{
		Capability:   llmcore.CapabilityChat,
		ProviderHint: provider,
		ModelHint:    llmReq.Model,
		RoutePolicy:  routePolicy,
		TraceID:      llmReq.TraceID,
		Payload:      llmReq,
		Metadata:     llmReq.Metadata,
		Tags:         llmReq.Tags,
		Hints: llmcore.CapabilityHints{
			ChatProvider: provider,
		},
	}, llmReq, nil
}

func (s *DefaultChatService) buildLocalToolRequest(llmReq *llm.ChatRequest) (*llm.ChatRequest, bool) {
	if llmReq == nil || len(llmReq.Tools) == 0 || s.toolManager == nil || s.chatProvider == nil {
		return nil, false
	}

	allowedByRuntime := s.toolManager.GetAllowedTools(chatToolRuntimeAgentID)
	if len(allowedByRuntime) == 0 {
		return nil, false
	}

	allowed := make(map[string]types.ToolSchema, len(allowedByRuntime))
	for _, schema := range allowedByRuntime {
		name := strings.TrimSpace(schema.Name)
		if name == "" {
			continue
		}
		allowed[name] = schema
	}
	if len(allowed) == 0 {
		return nil, false
	}

	filtered := make([]types.ToolSchema, 0, len(llmReq.Tools))
	for _, schema := range llmReq.Tools {
		name := strings.TrimSpace(schema.Name)
		if name == "" {
			continue
		}
		if _, ok := allowed[name]; ok {
			filtered = append(filtered, schema)
		}
	}
	if len(filtered) == 0 {
		return nil, false
	}

	reactReq := *llmReq
	reactReq.Tools = filtered
	return &reactReq, true
}

func (s *DefaultChatService) executeLocalReAct(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, *types.Error) {
	executor := llmtools.NewReActExecutor(
		s.chatProvider,
		newChatToolManagerExecutor(s.toolManager, chatToolRuntimeAgentID, req.Tools),
		llmtools.ReActConfig{
			MaxIterations: defaultChatReActIterations,
			StopOnError:   false,
		},
		s.logger,
	)

	resp, _, err := executor.Execute(ctx, req)
	if err != nil {
		return nil, toTypesChatError(err)
	}
	if resp == nil {
		return nil, types.NewInternalError("empty chat response")
	}
	return resp, nil
}

func (s *DefaultChatService) streamLocalReAct(ctx context.Context, req *llm.ChatRequest) (<-chan llmcore.UnifiedChunk, *types.Error) {
	executor := llmtools.NewReActExecutor(
		s.chatProvider,
		newChatToolManagerExecutor(s.toolManager, chatToolRuntimeAgentID, req.Tools),
		llmtools.ReActConfig{
			MaxIterations: defaultChatReActIterations,
			StopOnError:   false,
		},
		s.logger,
	)

	events, err := executor.ExecuteStream(ctx, req)
	if err != nil {
		return nil, toTypesChatError(err)
	}

	stream := make(chan llmcore.UnifiedChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("streamLocalReAct relay panic recovered", zap.Any("panic", r))
			}
			close(stream)
		}()
		for event := range events {
			var chunk llmcore.UnifiedChunk
			switch event.Type {
			case llmtools.ReActEventLLMChunk:
				if event.Chunk == nil {
					continue
				}
				chunk = llmcore.UnifiedChunk{Output: event.Chunk}
			case llmtools.ReActEventError:
				chunk = llmcore.UnifiedChunk{
					Err: types.NewInternalError(event.Error),
				}
				select {
				case stream <- chunk:
				case <-ctx.Done():
				}
				return
			default:
				continue
			}
			select {
			case stream <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()

	return stream, nil
}

type chatToolManagerExecutor struct {
	mgr        agent.ToolManager
	agentID    string
	allowedSet map[string]struct{}
}

func newChatToolManagerExecutor(mgr agent.ToolManager, agentID string, tools []types.ToolSchema) chatToolManagerExecutor {
	allowedSet := make(map[string]struct{}, len(tools))
	for _, schema := range tools {
		name := strings.TrimSpace(schema.Name)
		if name == "" {
			continue
		}
		allowedSet[name] = struct{}{}
	}
	return chatToolManagerExecutor{
		mgr:        mgr,
		agentID:    agentID,
		allowedSet: allowedSet,
	}
}

func (e chatToolManagerExecutor) Execute(ctx context.Context, calls []types.ToolCall) []llmtools.ToolResult {
	if len(calls) == 0 {
		return nil
	}

	out := make([]llmtools.ToolResult, len(calls))
	if e.mgr == nil {
		for i, c := range calls {
			out[i] = llmtools.ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Error:      "tool manager not configured",
			}
		}
		return out
	}

	allowedCalls := make([]types.ToolCall, 0, len(calls))
	allowedIdx := make([]int, 0, len(calls))
	for i, c := range calls {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			out[i] = llmtools.ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Error:      "tool name is required",
			}
			continue
		}
		if _, ok := e.allowedSet[name]; !ok {
			out[i] = llmtools.ToolResult{
				ToolCallID: c.ID,
				Name:       c.Name,
				Error:      "tool not allowed",
			}
			continue
		}
		allowedCalls = append(allowedCalls, c)
		allowedIdx = append(allowedIdx, i)
	}

	if len(allowedCalls) == 0 {
		return out
	}

	executed := e.mgr.ExecuteForAgent(ctx, e.agentID, allowedCalls)
	for i, idx := range allowedIdx {
		if i < len(executed) {
			out[idx] = executed[i]
			continue
		}
		call := allowedCalls[i]
		out[idx] = llmtools.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Error:      "no tool result",
		}
	}

	return out
}

func (e chatToolManagerExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) llmtools.ToolResult {
	results := e.Execute(ctx, []types.ToolCall{call})
	if len(results) == 0 {
		return llmtools.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Error:      "no tool result",
		}
	}
	return results[0]
}

func toTypesChatError(err error) *types.Error {
	if err == nil {
		return nil
	}
	if typedErr, ok := err.(*types.Error); ok {
		return typedErr
	}
	return types.NewInternalError("chat operation failed").WithCause(err)
}
