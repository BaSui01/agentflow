package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ChatConverter centralizes request/response conversion between API and LLM layers.
// Implementations are provided by the handlers layer.
type ChatConverter interface {
	ToLLMRequest(req *ChatRequest) *llmcore.ChatRequest
	ToChatResponse(resp *llmcore.ChatResponse) *ChatResponse
}

// ChatService encapsulates chat routing and gateway invocation logic.
type ChatService interface {
	Complete(ctx context.Context, req *ChatRequest) (*ChatCompletionResult, *types.Error)
	Stream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamEvent, *types.Error)
	SupportedRoutePolicies() []string
	DefaultRoutePolicy() string
}

// ChatCompletionResult captures completion output and execution metadata.
type ChatCompletionResult struct {
	Response *ChatResponse
	Raw      *llmcore.ChatResponse
	Duration time.Duration
}

// ChatRuntime captures the hot-swappable runtime dependencies used by ChatService.
type ChatRuntime struct {
	Gateway      llmcore.Gateway
	ChatProvider llmcore.Provider
	ToolManager  agent.ToolManager
}

const (
	defaultChatReActIterations = 10
	chatToolRuntimeAgentID     = "chat"
)

// DefaultChatService is the default ChatService implementation.
type DefaultChatService struct {
	runtimeRef RuntimeRef[ChatRuntime]
	converter  ChatConverter
	logger     *zap.Logger
}

// NewDefaultChatService constructs a ChatService backed by a hot-swappable runtime holder.
func NewDefaultChatService(
	runtime ChatRuntime,
	converter ChatConverter,
	logger *zap.Logger,
) (ChatService, error) {
	if logger == nil {
		return nil, fmt.Errorf("usecase.ChatService: logger is required and cannot be nil")
	}
	return &DefaultChatService{
		runtimeRef: NewAtomicRuntimeRef(runtime),
		converter:  converter,
		logger:     logger,
	}, nil
}

// UpdateRuntime swaps the service runtime in place.
func (s *DefaultChatService) UpdateRuntime(runtime ChatRuntime) {
	if s == nil {
		return
	}
	if s.runtimeRef == nil {
		s.runtimeRef = NewAtomicRuntimeRef(runtime)
		return
	}
	s.runtimeRef.Store(runtime)
}

func (s *DefaultChatService) runtime() ChatRuntime {
	if s == nil || s.runtimeRef == nil {
		return ChatRuntime{}
	}
	return s.runtimeRef.Load()
}

func (s *DefaultChatService) Complete(ctx context.Context, req *ChatRequest) (*ChatCompletionResult, *types.Error) {
	unifiedReq, llmReq, err := s.buildUnifiedRequest(req)
	if err != nil {
		return nil, err
	}
	runtime := s.runtime()
	if runtime.Gateway == nil {
		return nil, types.NewServiceUnavailableError("chat runtime is not configured")
	}

	if llmReq.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, llmReq.Timeout)
		defer cancel()
	}

	reactReq, localToolsEnabled := s.buildLocalToolRequest(runtime, llmReq)
	start := time.Now()
	if localToolsEnabled {
		raw, reactErr := s.executeLocalReAct(ctx, runtime, reactReq)
		if reactErr != nil {
			return nil, reactErr
		}
		return &ChatCompletionResult{
			Response: s.converter.ToChatResponse(raw),
			Raw:      raw,
			Duration: time.Since(start),
		}, nil
	}

	resp, invokeErr := runtime.Gateway.Invoke(ctx, unifiedReq)
	if invokeErr != nil {
		return nil, toTypesChatError(invokeErr)
	}
	raw, ok := resp.Output.(*llmcore.ChatResponse)
	if !ok || raw == nil {
		return nil, types.NewInternalError("invalid chat gateway response")
	}
	return &ChatCompletionResult{
		Response: s.converter.ToChatResponse(raw),
		Raw:      raw,
		Duration: time.Since(start),
	}, nil
}

func (s *DefaultChatService) Stream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamEvent, *types.Error) {
	unifiedReq, llmReq, err := s.buildUnifiedRequest(req)
	if err != nil {
		return nil, err
	}
	runtime := s.runtime()
	if runtime.Gateway == nil {
		return nil, types.NewServiceUnavailableError("chat runtime is not configured")
	}

	reactReq, localToolsEnabled := s.buildLocalToolRequest(runtime, llmReq)
	if localToolsEnabled {
		return s.streamLocalReAct(ctx, runtime, reactReq)
	}

	stream, streamErr := runtime.Gateway.Stream(ctx, unifiedReq)
	if streamErr != nil {
		return nil, toTypesChatError(streamErr)
	}
	return relayUnifiedChatStream(ctx, stream, s.logger), nil
}

func (s *DefaultChatService) SupportedRoutePolicies() []string {
	return SupportedRoutePolicies()
}

func (s *DefaultChatService) DefaultRoutePolicy() string {
	return string(llmcore.RoutePolicyBalanced)
}

func (s *DefaultChatService) buildUnifiedRequest(req *ChatRequest) (*llmcore.UnifiedRequest, *llmcore.ChatRequest, *types.Error) {
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

func (s *DefaultChatService) buildLocalToolRequest(runtime ChatRuntime, llmReq *llmcore.ChatRequest) (*llmcore.ChatRequest, bool) {
	if llmReq == nil || len(llmReq.Tools) == 0 || runtime.ToolManager == nil || runtime.ChatProvider == nil {
		return nil, false
	}

	allowedByRuntime := runtime.ToolManager.GetAllowedTools(chatToolRuntimeAgentID)
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

func (s *DefaultChatService) executeLocalReAct(ctx context.Context, runtime ChatRuntime, req *llmcore.ChatRequest) (*llmcore.ChatResponse, *types.Error) {
	executor := llmtools.NewReActExecutor(
		runtime.ChatProvider,
		newChatToolManagerExecutor(runtime.ToolManager, chatToolRuntimeAgentID, req.Tools),
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

func (s *DefaultChatService) streamLocalReAct(ctx context.Context, runtime ChatRuntime, req *llmcore.ChatRequest) (<-chan ChatStreamEvent, *types.Error) {
	executor := llmtools.NewReActExecutor(
		runtime.ChatProvider,
		newChatToolManagerExecutor(runtime.ToolManager, chatToolRuntimeAgentID, req.Tools),
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

	stream := make(chan ChatStreamEvent)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("streamLocalReAct relay panic recovered", zap.Any("panic", r))
			}
			close(stream)
		}()
		for event := range events {
			var chunk ChatStreamEvent
			switch event.Type {
			case llmtools.ReActEventLLMChunk:
				if event.Chunk == nil {
					continue
				}
				chunk = ChatStreamEvent{Chunk: convertLLMStreamChunkToUsecase(event.Chunk)}
			case llmtools.ReActEventError:
				chunk = ChatStreamEvent{Err: types.NewInternalError(event.Error)}
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

func relayUnifiedChatStream(ctx context.Context, src <-chan llmcore.UnifiedChunk, logger *zap.Logger) <-chan ChatStreamEvent {
	out := make(chan ChatStreamEvent)
	go func() {
		defer func() {
			if r := recover(); r != nil && logger != nil {
				logger.Error("relayUnifiedChatStream panic recovered", zap.Any("panic", r))
			}
			close(out)
		}()
		for chunk := range src {
			event := convertUnifiedChatChunkToUsecase(chunk)
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func convertUnifiedChatChunkToUsecase(chunk llmcore.UnifiedChunk) ChatStreamEvent {
	if chunk.Err != nil {
		return ChatStreamEvent{Err: chunk.Err}
	}
	streamChunk, ok := chunk.Output.(*llmcore.StreamChunk)
	if !ok || streamChunk == nil {
		return ChatStreamEvent{Err: types.NewInternalError("invalid chat stream chunk payload")}
	}
	return ChatStreamEvent{Chunk: convertLLMStreamChunkToUsecase(streamChunk)}
}

func convertLLMStreamChunkToUsecase(chunk *llmcore.StreamChunk) *ChatStreamChunk {
	if chunk == nil {
		return nil
	}
	return &ChatStreamChunk{
		ID:           chunk.ID,
		Provider:     chunk.Provider,
		Model:        chunk.Model,
		Index:        chunk.Index,
		Delta:        messageFromTypes(chunk.Delta),
		FinishReason: chunk.FinishReason,
		Usage:        chatUsageFromLLM(chunk.Usage),
	}
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
