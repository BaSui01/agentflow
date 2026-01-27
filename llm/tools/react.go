package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ReActConfig defines ReAct loop configuration.
type ReActConfig struct {
	MaxIterations int  // Maximum iterations (prevents infinite loops)
	StopOnError   bool // Stop on tool execution error
}

// ReActExecutor implements the ReAct (Reasoning and Acting) loop.
// Automatically handles "LLM -> Tool -> LLM" multi-turn conversations.
type ReActExecutor struct {
	provider     llm.Provider
	toolExecutor ToolExecutor
	logger       *zap.Logger
	config       ReActConfig
}

// NewReActExecutor creates a ReAct executor.
func NewReActExecutor(provider llm.Provider, toolExecutor ToolExecutor, config ReActConfig, logger *zap.Logger) *ReActExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.MaxIterations == 0 {
		config.MaxIterations = 10
	}
	return &ReActExecutor{
		provider:     provider,
		toolExecutor: toolExecutor,
		logger:       logger,
		config:       config,
	}
}

// Execute runs the ReAct loop, returning final response and all steps.
func (r *ReActExecutor) Execute(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, []ReActStep, error) {
	steps := make([]ReActStep, 0)
	messages := append([]llm.Message{}, req.Messages...)

	for i := 0; i < r.config.MaxIterations; i++ {
		r.logger.Debug("ReAct iteration", zap.Int("iteration", i+1))

		callReq := *req
		callReq.Messages = messages
		resp, err := r.provider.Completion(ctx, &callReq)
		if err != nil {
			return nil, steps, fmt.Errorf("LLM call failed at iteration %d: %w", i+1, err)
		}

		if len(resp.Choices) == 0 {
			return resp, steps, fmt.Errorf("no choices in LLM response")
		}

		choice := resp.Choices[0]
		toolCalls := choice.Message.ToolCalls

		step := ReActStep{
			StepNumber: i + 1,
			Thought:    choice.Message.Content,
			Timestamp:  fmt.Sprintf("%d", i+1),
			TokensUsed: resp.Usage.TotalTokens,
		}

		if len(toolCalls) == 0 {
			r.logger.Info("ReAct completed", zap.Int("iterations", i+1), zap.String("finish_reason", choice.FinishReason))
			steps = append(steps, step)
			return resp, steps, nil
		}

		r.logger.Info("executing tools", zap.Int("count", len(toolCalls)))
		step.Actions = toolCalls
		toolResults := r.toolExecutor.Execute(ctx, toolCalls)
		step.Observations = toolResults

		hasError := false
		for _, result := range toolResults {
			if result.Error != "" {
				hasError = true
				r.logger.Warn("tool execution failed", zap.String("tool", result.Name), zap.String("error", result.Error))
			}
		}

		if hasError && r.config.StopOnError {
			steps = append(steps, step)
			return resp, steps, fmt.Errorf("tool execution failed, stopping ReAct loop")
		}

		messages = append(messages, choice.Message)
		for _, result := range toolResults {
			messages = append(messages, result.ToMessage())
		}
		steps = append(steps, step)
	}

	r.logger.Warn("ReAct max iterations reached", zap.Int("max", r.config.MaxIterations))
	return nil, steps, fmt.Errorf("max iterations reached (%d)", r.config.MaxIterations)
}

// ExecuteWithTrace executes ReAct loop and returns full trace.
func (r *ReActExecutor) ExecuteWithTrace(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, *ReActTrace, error) {
	resp, steps, err := r.Execute(ctx, req)

	trace := &ReActTrace{
		TraceID:    fmt.Sprintf("react-%d", len(steps)),
		Steps:      steps,
		TotalSteps: len(steps),
		Success:    err == nil,
	}

	for _, step := range steps {
		trace.TotalTokens += step.TokensUsed
	}

	if resp != nil && len(resp.Choices) > 0 {
		trace.FinalAnswer = resp.Choices[0].Message.Content
	}

	if err != nil {
		trace.ErrorMessage = err.Error()
	}

	return resp, trace, err
}

// ReActStep represents one step in the ReAct loop (Thought → Action → Observation).
type ReActStep struct {
	StepNumber   int            `json:"step_number"`
	Thought      string         `json:"thought,omitempty"`
	Actions      []llm.ToolCall `json:"actions,omitempty"`
	Observations []ToolResult   `json:"observations,omitempty"`
	Timestamp    string         `json:"timestamp"`
	TokensUsed   int            `json:"tokens_used,omitempty"`
}

// ReActTrace represents the complete ReAct execution trace.
type ReActTrace struct {
	TraceID      string      `json:"trace_id"`
	Steps        []ReActStep `json:"steps"`
	TotalTokens  int         `json:"total_tokens"`
	TotalSteps   int         `json:"total_steps"`
	Success      bool        `json:"success"`
	FinalAnswer  string      `json:"final_answer,omitempty"`
	ErrorMessage string      `json:"error_message,omitempty"`
}

// LLMCallInfo records LLM call details (kept for backward compatibility).
type LLMCallInfo struct {
	Request  llm.ChatRequest  `json:"request"`
	Response llm.ChatResponse `json:"response"`
}

// ExecuteStream executes streaming ReAct loop.
func (r *ReActExecutor) ExecuteStream(ctx context.Context, req *llm.ChatRequest) (<-chan ReActStreamEvent, error) {
	eventCh := make(chan ReActStreamEvent)

	go func() {
		defer close(eventCh)
		messages := append([]llm.Message{}, req.Messages...)

		for i := 0; i < r.config.MaxIterations; i++ {
			select {
			case <-ctx.Done():
				eventCh <- ReActStreamEvent{Type: "error", Error: fmt.Sprintf("context cancelled: %v", ctx.Err())}
				return
			default:
			}

			eventCh <- ReActStreamEvent{Type: "iteration_start", Iteration: i + 1}

			callReq := *req
			callReq.Messages = messages

			streamCh, err := r.provider.Stream(ctx, &callReq)
			if err != nil {
				eventCh <- ReActStreamEvent{Type: "error", Error: fmt.Sprintf("LLM stream failed: %s", err.Error())}
				return
			}

			var (
				assembledMessage llm.Message
				toolCallOrder    []string
				toolCallByID     map[string]*struct {
					id           string
					name         string
					argsFinal    json.RawMessage
					argsBuilding strings.Builder
				}
				lastChunkID, lastProvider, lastModel, lastFinishReason string
				lastUsage                                              *llm.ChatUsage
			)

			for chunk := range streamCh {
				select {
				case <-ctx.Done():
					eventCh <- ReActStreamEvent{Type: "error", Error: fmt.Sprintf("context cancelled: %v", ctx.Err())}
					return
				default:
				}

				eventCh <- ReActStreamEvent{Type: "llm_chunk", Chunk: &chunk}

				if chunk.Err != nil {
					eventCh <- ReActStreamEvent{Type: "error", Error: chunk.Err.Error()}
					return
				}

				if chunk.ID != "" {
					lastChunkID = chunk.ID
				}
				if chunk.Provider != "" {
					lastProvider = chunk.Provider
				}
				if chunk.Model != "" {
					lastModel = chunk.Model
				}
				if chunk.Usage != nil {
					lastUsage = chunk.Usage
				}
				if chunk.FinishReason != "" {
					lastFinishReason = chunk.FinishReason
				}

				if chunk.Delta.Content != "" {
					assembledMessage.Content += chunk.Delta.Content
				}
				if len(chunk.Delta.ToolCalls) > 0 {
					if toolCallByID == nil {
						toolCallByID = make(map[string]*struct {
							id           string
							name         string
							argsFinal    json.RawMessage
							argsBuilding strings.Builder
						})
					}
					for _, tc := range chunk.Delta.ToolCalls {
						id := strings.TrimSpace(tc.ID)
						if id == "" {
							id = fmt.Sprintf("call_%d_%d", i+1, len(toolCallOrder)+1)
						}
						acc := toolCallByID[id]
						if acc == nil {
							acc = &struct {
								id           string
								name         string
								argsFinal    json.RawMessage
								argsBuilding strings.Builder
							}{id: id}
							toolCallByID[id] = acc
							toolCallOrder = append(toolCallOrder, id)
						}
						if strings.TrimSpace(tc.Name) != "" {
							acc.name = strings.TrimSpace(tc.Name)
						}
						if len(tc.Arguments) == 0 || len(acc.argsFinal) > 0 {
							continue
						}
						var argSegStr string
						if err := json.Unmarshal(tc.Arguments, &argSegStr); err == nil {
							acc.argsBuilding.WriteString(argSegStr)
							continue
						}
						if json.Valid(tc.Arguments) {
							acc.argsFinal = append([]byte(nil), tc.Arguments...)
							continue
						}
						acc.argsBuilding.WriteString(string(tc.Arguments))
					}
				}
			}

			assembledMessage.Role = llm.RoleAssistant
			nativeToolCalls := make([]llm.ToolCall, 0, len(toolCallOrder))
			for _, id := range toolCallOrder {
				acc := toolCallByID[id]
				if acc == nil {
					continue
				}
				args := json.RawMessage(nil)
				if len(acc.argsFinal) > 0 {
					args = acc.argsFinal
				} else {
					raw := strings.TrimSpace(acc.argsBuilding.String())
					if raw != "" {
						if !json.Valid([]byte(raw)) {
							eventCh <- ReActStreamEvent{Type: "error", Error: fmt.Sprintf("invalid tool call arguments (id=%s tool=%s): %s", acc.id, acc.name, raw)}
							return
						}
						args = json.RawMessage(raw)
					}
				}
				nativeToolCalls = append(nativeToolCalls, llm.ToolCall{ID: acc.id, Name: acc.name, Arguments: args})
			}
			assembledMessage.ToolCalls = nativeToolCalls

			if len(assembledMessage.ToolCalls) == 0 {
				final := &llm.ChatResponse{
					ID: lastChunkID, Provider: lastProvider, Model: lastModel,
					Choices: []llm.ChatChoice{{Index: 0, FinishReason: lastFinishReason, Message: assembledMessage}},
				}
				if lastUsage != nil {
					final.Usage = *lastUsage
				}
				eventCh <- ReActStreamEvent{Type: "completed", Iteration: i + 1, FinalResponse: final}
				return
			}

			eventCh <- ReActStreamEvent{Type: "tools_start", ToolCalls: assembledMessage.ToolCalls}
			toolResults := r.toolExecutor.Execute(ctx, assembledMessage.ToolCalls)
			eventCh <- ReActStreamEvent{Type: "tools_end", ToolResults: toolResults}

			messages = append(messages, assembledMessage)
			for _, result := range toolResults {
				toolMessage := result.ToMessage()
				if result.Error != "" && r.config.StopOnError {
					eventCh <- ReActStreamEvent{Type: "error", Error: fmt.Sprintf("tool execution failed: %s", result.Error)}
					return
				}
				messages = append(messages, toolMessage)
			}
		}

		eventCh <- ReActStreamEvent{Type: "error", Error: fmt.Sprintf("max iterations reached (%d)", r.config.MaxIterations)}
	}()

	return eventCh, nil
}

// ReActStreamEvent represents a streaming ReAct loop event.
type ReActStreamEvent struct {
	Type          string            `json:"type"`
	Iteration     int               `json:"iteration,omitempty"`
	Chunk         *llm.StreamChunk  `json:"chunk,omitempty"`
	ToolCalls     []llm.ToolCall    `json:"tool_calls,omitempty"`
	ToolResults   []ToolResult      `json:"tool_results,omitempty"`
	FinalResponse *llm.ChatResponse `json:"final_response,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// ToMessage converts ToolResult to LLM Message.
func (tr ToolResult) ToMessage() llm.Message {
	msg := llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: tr.ToolCallID,
		Name:       tr.Name,
	}
	if tr.Error != "" {
		msg.Content = fmt.Sprintf("Error: %s", tr.Error)
	} else {
		msg.Content = string(tr.Result)
	}
	return msg
}

// ToJSON serializes ToolResult to JSON.
func (tr ToolResult) ToJSON() (json.RawMessage, error) {
	return json.Marshal(tr)
}
