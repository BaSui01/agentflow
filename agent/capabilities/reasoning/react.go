package reasoning

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"go.uber.org/zap"
)

// ReActConfig configures the ReAct reasoning pattern.
type ReActConfig struct {
	MaxIterations int           // Maximum iterations (prevents infinite loops)
	StopOnError   bool          // Stop on tool execution error
	Timeout       time.Duration // Overall timeout
	Model         string        // LLM model to use
}

// DefaultReActConfig returns sensible defaults.
func DefaultReActConfig() ReActConfig {
	return ReActConfig{
		MaxIterations: 10,
		StopOnError:   false,
		Timeout:       120 * time.Second,
		Model:         "gpt-4o",
	}
}

// ReAct implements the ReasoningPattern interface for the ReAct loop.
// It performs Thought -> Action -> Observation cycles through the gateway.
type ReAct struct {
	gateway      llmcore.Gateway
	toolExecutor tools.ToolExecutor
	toolSchemas  []types.ToolSchema
	config       ReActConfig
	logger       *zap.Logger
}

// NewReAct creates a new ReAct reasoning pattern.
func NewReAct(gateway llmcore.Gateway, executor tools.ToolExecutor, schemas []types.ToolSchema, config ReActConfig, logger *zap.Logger) *ReAct {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReAct{
		gateway:      gateway,
		toolExecutor: executor,
		toolSchemas:  schemas,
		config:       config,
		logger:       logger,
	}
}

func (r *ReAct) Name() string { return "react" }

// Execute runs the ReAct reasoning loop.
func (r *ReAct) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	result := &ReasoningResult{
		Pattern:  r.Name(),
		Task:     task,
		Metadata: make(map[string]any),
	}

	messages := []types.Message{
		{Role: llmcore.RoleSystem, Content: "You are a helpful assistant that can use tools to solve tasks. Think step by step."},
		{Role: llmcore.RoleUser, Content: task},
	}

	var totalUsage llmcore.ChatUsage
	var prevPromptTokens int

	for i := 0; i < r.config.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			result.TotalLatency = time.Since(start)
			result.Metadata["stop_reason"] = "context_cancelled"
			return result, fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		r.logger.Debug("ReAct iteration", zap.Int("iteration", i+1))

		resp, err := invokeChatGateway(ctx, r.gateway, newGatewayChatRequest(
			defaultModel(r.config.Model),
			messages,
			func(req *llmcore.ChatRequest) {
				if len(r.toolSchemas) > 0 {
					req.Tools = r.toolSchemas
				}
				req.ToolCallMode = llmcore.ToolCallModeNative
			},
		))
		if err != nil {
			result.TotalLatency = time.Since(start)
			return result, fmt.Errorf("LLM call failed at iteration %d: %w", i+1, err)
		}

		promptDelta := resp.Usage.PromptTokens - prevPromptTokens
		if promptDelta < 0 {
			promptDelta = resp.Usage.PromptTokens
		}
		stepTokens := promptDelta + resp.Usage.CompletionTokens
		prevPromptTokens = resp.Usage.PromptTokens
		totalUsage.PromptTokens += promptDelta
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += stepTokens
		result.TotalTokens += stepTokens

		if len(resp.Choices) == 0 {
			result.TotalLatency = time.Since(start)
			return result, fmt.Errorf("no choices in LLM response")
		}

		choice := resp.Choices[0]
		toolCalls := choice.Message.ToolCalls

		step := ReasoningStep{
			StepID:     fmt.Sprintf("react_%d", i+1),
			Type:       "thought",
			Content:    choice.Message.Content,
			TokensUsed: stepTokens,
			Duration:   time.Since(start),
		}

		if len(toolCalls) == 0 {
			r.logger.Info("ReAct completed", zap.Int("iterations", i+1))
			result.FinalAnswer = choice.Message.Content
			result.Confidence = 0.9
			result.Steps = append(result.Steps, step)
			result.TotalLatency = time.Since(start)
			result.Metadata["iterations"] = i + 1
			result.Metadata["stop_reason"] = "natural_completion"
			return result, nil
		}

		r.logger.Info("executing tools", zap.Int("count", len(toolCalls)))
		step.Type = "action"
		toolResults := r.toolExecutor.Execute(ctx, toolCalls)

		obsContent := ""
		hasError := false
		for _, tr := range toolResults {
			if tr.IsError() {
				hasError = true
				obsContent += fmt.Sprintf("Tool %s error: %s\n", tr.Name, tr.Error)
			} else {
				obsContent += fmt.Sprintf("Tool %s result: %s\n", tr.Name, string(tr.Result))
			}
		}

		result.Steps = append(result.Steps, step)
		result.Steps = append(result.Steps, ReasoningStep{
			StepID:  fmt.Sprintf("react_%d_obs", i+1),
			Type:    "observation",
			Content: obsContent,
		})

		if hasError && r.config.StopOnError {
			result.TotalLatency = time.Since(start)
			result.Metadata["iterations"] = i + 1
			result.Metadata["stop_reason"] = "tool_error"
			return result, fmt.Errorf("tool execution failed, stopping ReAct loop")
		}

		messages = append(messages, choice.Message)
		for _, tr := range toolResults {
			messages = append(messages, tr.ToMessage())
		}
	}

	r.logger.Warn("ReAct max iterations reached", zap.Int("max", r.config.MaxIterations))
	result.TotalLatency = time.Since(start)
	result.Metadata["iterations"] = r.config.MaxIterations
	result.Metadata["stop_reason"] = "max_iterations"
	return result, fmt.Errorf("max iterations reached (%d)", r.config.MaxIterations)
}
