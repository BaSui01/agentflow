package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// LLMEngine encapsulates all LLM-related fields and methods extracted from BaseAgent.
type LLMEngine struct {
	provider       llm.Provider
	toolProvider   llm.Provider // optional; falls back to provider when nil
	config         LLMEngineConfig
	contextManager ContextManager
	contextEnabled bool
	toolManager    ToolManager
	bus            EventBus
	logger         *zap.Logger
}

// LLMEngineConfig holds the configuration subset relevant to LLM interactions.
type LLMEngineConfig struct {
	Model              string
	MaxTokens          int
	Temperature        float32
	Tools              []string
	MaxReActIterations int
	AgentID            string
}

// NewLLMEngine creates a new LLMEngine.
func NewLLMEngine(
	provider llm.Provider,
	toolProvider llm.Provider,
	cfg LLMEngineConfig,
	toolManager ToolManager,
	bus EventBus,
	logger *zap.Logger,
) *LLMEngine {
	return &LLMEngine{
		provider:     provider,
		toolProvider: toolProvider,
		config:       cfg,
		toolManager:  toolManager,
		bus:          bus,
		logger:       logger,
	}
}

// Provider returns the primary LLM provider.
func (e *LLMEngine) Provider() llm.Provider { return e.provider }

// ToolProvider returns the tool-call-specific LLM provider (may be nil).
func (e *LLMEngine) ToolProvider() llm.Provider { return e.toolProvider }

// SetToolProvider sets the tool-call-specific LLM provider.
func (e *LLMEngine) SetToolProvider(p llm.Provider) { e.toolProvider = p }

// SetContextManager sets the context manager for message optimization.
func (e *LLMEngine) SetContextManager(cm ContextManager) {
	e.contextManager = cm
	e.contextEnabled = cm != nil
	if cm != nil {
		e.logger.Info("context manager enabled")
	}
}

// ContextEngineEnabled returns whether context engineering is enabled.
func (e *LLMEngine) ContextEngineEnabled() bool { return e.contextEnabled }

// MaxReActIterations returns the effective max ReAct iterations (default 10).
func (e *LLMEngine) MaxReActIterations() int {
	if e.config.MaxReActIterations > 0 {
		return e.config.MaxReActIterations
	}
	return 10
}

// ChatCompletion calls the LLM to complete a conversation.
func (e *LLMEngine) ChatCompletion(ctx context.Context, messages []types.Message) (*llm.ChatResponse, error) {
	if e.provider == nil {
		return nil, ErrProviderNotSet
	}

	// Context engineering: optimize message history
	if e.contextEnabled && e.contextManager != nil && len(messages) > 1 {
		query := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := e.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			e.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			tokensBefore := e.contextManager.EstimateTokens(messages)
			tokensAfter := e.contextManager.EstimateTokens(optimized)
			if tokensBefore != tokensAfter {
				e.logger.Debug("context optimized",
					zap.Int("tokens_before", tokensBefore),
					zap.Int("tokens_after", tokensAfter))
			}
			messages = optimized
		}
	}

	model := e.config.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   e.config.MaxTokens,
		Temperature: e.config.Temperature,
	}

	// Runtime config override
	rc := GetRunConfig(ctx)
	if rc != nil {
		rc.ApplyToRequest(req, Config{
			Model:       e.config.Model,
			MaxTokens:   e.config.MaxTokens,
			Temperature: e.config.Temperature,
		})
	}

	// Filter available tools by whitelist
	if e.toolManager != nil && len(e.config.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(e.toolManager.GetAllowedTools(e.config.AgentID), e.config.Tools)
	}
	if len(req.Tools) > 0 {
		effectiveToolProvider := e.provider
		if e.toolProvider != nil {
			effectiveToolProvider = e.toolProvider
		}
		if effectiveToolProvider != nil && !effectiveToolProvider.SupportsNativeFunctionCalling() {
			return nil, fmt.Errorf("provider %q does not support native function calling", effectiveToolProvider.Name())
		}
	}

	effectiveReActIterations := rc.EffectiveMaxReActIterations(e.MaxReActIterations())

	emit, streaming := runtimeStreamEmitterFromContext(ctx)
	if streaming {
		return e.chatCompletionStreaming(ctx, req, effectiveReActIterations, emit)
	}

	// Non-streaming with tools: ReAct loop
	if len(req.Tools) > 0 && e.toolManager != nil {
		reactProvider := e.provider
		if e.toolProvider != nil {
			reactProvider = e.toolProvider
		}
		reactExecutor := llmtools.NewReActExecutor(reactProvider, newToolManagerExecutor(e.toolManager, e.config.AgentID, e.config.Tools, e.bus), llmtools.ReActConfig{
			MaxIterations: effectiveReActIterations,
			StopOnError:   false,
		}, e.logger)

		resp, _, err := reactExecutor.Execute(ctx, req)
		return resp, err
	}

	return e.provider.Completion(ctx, req)
}

// chatCompletionStreaming handles the streaming path of ChatCompletion.
func (e *LLMEngine) chatCompletionStreaming(ctx context.Context, req *llm.ChatRequest, maxIter int, emit func(RuntimeStreamEvent)) (*llm.ChatResponse, error) {
	if len(req.Tools) > 0 && e.toolManager != nil {
		reactProvider := e.provider
		if e.toolProvider != nil {
			reactProvider = e.toolProvider
		}
		executor := llmtools.NewReActExecutor(reactProvider, newToolManagerExecutor(e.toolManager, e.config.AgentID, e.config.Tools, e.bus), llmtools.ReActConfig{
			MaxIterations: maxIter,
			StopOnError:   false,
		}, e.logger)

		evCh, err := executor.ExecuteStream(ctx, req)
		if err != nil {
			return nil, err
		}
		var final *llm.ChatResponse
		for ev := range evCh {
			switch ev.Type {
			case "llm_chunk":
				if ev.Chunk != nil && ev.Chunk.Delta.Content != "" {
					emit(RuntimeStreamEvent{
						Type:      RuntimeStreamToken,
						Timestamp: time.Now(),
						Token:     ev.Chunk.Delta.Content,
						Delta:     ev.Chunk.Delta.Content,
					})
				}
			case "tools_start":
				for _, call := range ev.ToolCalls {
					emit(RuntimeStreamEvent{
						Type:      RuntimeStreamToolCall,
						Timestamp: time.Now(),
						ToolCall: &RuntimeToolCall{
							ID:        call.ID,
							Name:      call.Name,
							Arguments: append(json.RawMessage(nil), call.Arguments...),
						},
					})
				}
			case "tools_end":
				for _, tr := range ev.ToolResults {
					emit(RuntimeStreamEvent{
						Type:      RuntimeStreamToolResult,
						Timestamp: time.Now(),
						ToolResult: &RuntimeToolResult{
							ToolCallID: tr.ToolCallID,
							Name:       tr.Name,
							Result:     append(json.RawMessage(nil), tr.Result...),
							Error:      tr.Error,
							Duration:   tr.Duration,
						},
					})
				}
			case "tool_progress":
				emit(RuntimeStreamEvent{
					Type:       RuntimeStreamToolProgress,
					Timestamp:  time.Now(),
					ToolCallID: ev.ToolCallID,
					ToolName:   ev.ToolName,
					Data:       ev.ProgressData,
				})
			case "completed":
				final = ev.FinalResponse
			case "error":
				return nil, fmt.Errorf("%s", ev.Error)
			}
		}
		if final == nil {
			return nil, fmt.Errorf("no final response")
		}
		return final, nil
	}

	// No tools: direct provider stream
	streamCh, err := e.provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	var (
		assembled types.Message
		lastID    string
		lastProv  string
		lastModel string
		lastUsage *llm.ChatUsage
		lastFR    string
	)
	for chunk := range streamCh {
		if chunk.Err != nil {
			return nil, chunk.Err
		}
		if chunk.ID != "" {
			lastID = chunk.ID
		}
		if chunk.Provider != "" {
			lastProv = chunk.Provider
		}
		if chunk.Model != "" {
			lastModel = chunk.Model
		}
		if chunk.Usage != nil {
			lastUsage = chunk.Usage
		}
		if chunk.FinishReason != "" {
			lastFR = chunk.FinishReason
		}
		if chunk.Delta.Content != "" {
			emit(RuntimeStreamEvent{
				Type:      RuntimeStreamToken,
				Timestamp: time.Now(),
				Token:     chunk.Delta.Content,
				Delta:     chunk.Delta.Content,
			})
			assembled.Content += chunk.Delta.Content
		}
	}
	assembled.Role = llm.RoleAssistant
	resp := &llm.ChatResponse{
		ID:       lastID,
		Provider: lastProv,
		Model:    lastModel,
		Choices: []llm.ChatChoice{{
			Index:        0,
			FinishReason: lastFR,
			Message:      assembled,
		}},
	}
	if lastUsage != nil {
		resp.Usage = *lastUsage
	}
	return resp, nil
}

// StreamCompletion streams LLM responses.
func (e *LLMEngine) StreamCompletion(ctx context.Context, messages []types.Message) (<-chan llm.StreamChunk, error) {
	if e.provider == nil {
		return nil, ErrProviderNotSet
	}

	// Context engineering: optimize message history
	if e.contextEnabled && e.contextManager != nil && len(messages) > 1 {
		query := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := e.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			e.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			messages = optimized
		}
	}

	model := e.config.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   e.config.MaxTokens,
		Temperature: e.config.Temperature,
	}

	if rc := GetRunConfig(ctx); rc != nil {
		rc.ApplyToRequest(req, Config{
			Model:       e.config.Model,
			MaxTokens:   e.config.MaxTokens,
			Temperature: e.config.Temperature,
		})
	}

	if e.toolManager != nil && len(e.config.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(e.toolManager.GetAllowedTools(e.config.AgentID), e.config.Tools)
	}
	if len(req.Tools) > 0 {
		effectiveToolProvider := e.provider
		if e.toolProvider != nil {
			effectiveToolProvider = e.toolProvider
		}
		if effectiveToolProvider != nil && !effectiveToolProvider.SupportsNativeFunctionCalling() {
			return nil, fmt.Errorf("provider %q does not support native function calling", effectiveToolProvider.Name())
		}
	}

	return e.provider.Stream(ctx, req)
}

