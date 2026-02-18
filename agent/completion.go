package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/tools"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// ChatCompletion 调用 LLM 完成对话
func (b *BaseAgent) ChatCompletion(ctx context.Context, messages []llm.Message) (*llm.ChatResponse, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 上下文工程：优化消息历史
	if b.contextEngineEnabled && b.contextManager != nil && len(messages) > 1 {
		query := ""
		// 提取用户查询（最后一条用户消息）
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := b.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			b.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			tokensBefore := b.contextManager.EstimateTokens(messages)
			tokensAfter := b.contextManager.EstimateTokens(optimized)
			if tokensBefore != tokensAfter {
				b.logger.Debug("context optimized",
					zap.Int("tokens_before", tokensBefore),
					zap.Int("tokens_after", tokensAfter))
			}
			messages = optimized
		}
	}

	model := b.config.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   b.config.MaxTokens,
		Temperature: b.config.Temperature,
	}

	// 按白名单过滤可用工具
	if b.toolManager != nil && len(b.config.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(b.toolManager.GetAllowedTools(b.config.ID), b.config.Tools)
	}
	if len(req.Tools) > 0 {
		// 确定实际用于工具调用的 provider
		effectiveToolProvider := b.provider
		if b.toolProvider != nil {
			effectiveToolProvider = b.toolProvider
		}
		if effectiveToolProvider != nil && !effectiveToolProvider.SupportsNativeFunctionCalling() {
			return nil, fmt.Errorf("provider %q does not support native function calling", effectiveToolProvider.Name())
		}
	}

	emit, streaming := runtimeStreamEmitterFromContext(ctx)
	if streaming {
		// 若存在可用工具：使用流式 ReAct 循环，并将 token/tool 事件发射给上游（RunStream/Workflow）。
		if len(req.Tools) > 0 && b.toolManager != nil {
			// 双模型模式：ReAct 循环优先使用 toolProvider（便宜），未设置时退化为主 provider
			reactProvider := b.provider
			if b.toolProvider != nil {
				reactProvider = b.toolProvider
			}
			executor := llmtools.NewReActExecutor(reactProvider, newToolManagerExecutor(b.toolManager, b.config.ID, b.config.Tools, b.bus), llmtools.ReActConfig{
				MaxIterations: b.maxReActIterations(),
				StopOnError:   false,
			}, b.logger)

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

		// 无工具：直接走 provider stream，并将 token 发射给上游，同时组装最终响应。
		streamCh, err := b.provider.Stream(ctx, req)
		if err != nil {
			return nil, err
		}
		var (
			assembled llm.Message
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

	// 若存在可用工具：执行 ReAct 循环（LLM -> Tool -> LLM），直到模型停止调用工具或达到最大迭代次数。
	if len(req.Tools) > 0 && b.toolManager != nil {
		// 双模型模式：ReAct 循环优先使用 toolProvider（便宜），未设置时退化为主 provider
		reactProvider := b.provider
		if b.toolProvider != nil {
			reactProvider = b.toolProvider
		}
		reactExecutor := llmtools.NewReActExecutor(reactProvider, newToolManagerExecutor(b.toolManager, b.config.ID, b.config.Tools, b.bus), llmtools.ReActConfig{
			MaxIterations: b.maxReActIterations(),
			StopOnError:   false,
		}, b.logger)

		resp, _, err := reactExecutor.Execute(ctx, req)
		return resp, err
	}

	return b.provider.Completion(ctx, req)
}

// StreamCompletion 流式调用 LLM
func (b *BaseAgent) StreamCompletion(ctx context.Context, messages []llm.Message) (<-chan llm.StreamChunk, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}

	// 上下文工程：优化消息历史
	if b.contextEngineEnabled && b.contextManager != nil && len(messages) > 1 {
		query := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := b.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			b.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			messages = optimized
		}
	}

	model := b.config.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   b.config.MaxTokens,
		Temperature: b.config.Temperature,
	}
	if b.toolManager != nil && len(b.config.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(b.toolManager.GetAllowedTools(b.config.ID), b.config.Tools)
	}
	if len(req.Tools) > 0 {
		// 确定实际用于工具调用的 provider（与 ChatCompletion 保持一致）
		effectiveToolProvider := b.provider
		if b.toolProvider != nil {
			effectiveToolProvider = b.toolProvider
		}
		if effectiveToolProvider != nil && !effectiveToolProvider.SupportsNativeFunctionCalling() {
			return nil, fmt.Errorf("provider %q does not support native function calling", effectiveToolProvider.Name())
		}
	}

	return b.provider.Stream(ctx, req)
}
