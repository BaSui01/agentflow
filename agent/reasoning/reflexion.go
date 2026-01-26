// Package reasoning provides Reflexion pattern for self-improving reasoning.
package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"go.uber.org/zap"
)

// ReflexionConfig configures the Reflexion executor.
type ReflexionConfig struct {
	MaxTrials        int           `json:"max_trials"`
	SuccessThreshold float64       `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
	EnableMemory     bool          `json:"enable_memory"`
}

// DefaultReflexionConfig returns sensible defaults.
func DefaultReflexionConfig() ReflexionConfig {
	return ReflexionConfig{MaxTrials: 5, SuccessThreshold: 0.8, Timeout: 300 * time.Second, EnableMemory: true}
}

// Trial represents a single attempt at solving the task.
type Trial struct {
	Number     int         `json:"number"`
	Action     string      `json:"action"`
	Result     string      `json:"result"`
	Score      float64     `json:"score"`
	Reflection *Reflection `json:"reflection,omitempty"`
}

// Reflection represents feedback on a trial.
type Reflection struct {
	Analysis     string   `json:"analysis"`
	Mistakes     []string `json:"mistakes"`
	NextStrategy string   `json:"next_strategy"`
}

// ReflexionMemory stores past experiences.
type ReflexionMemory struct {
	mu      sync.RWMutex
	entries []MemoryEntry
}

// MemoryEntry represents a stored experience.
type MemoryEntry struct {
	Task       string      `json:"task"`
	Reflection *Reflection `json:"reflection"`
}

// ReflexionExecutor implements the Reflexion pattern.
type ReflexionExecutor struct {
	provider     llm.Provider
	toolExecutor tools.ToolExecutor
	toolSchemas  []llm.ToolSchema
	config       ReflexionConfig
	memory       *ReflexionMemory
	logger       *zap.Logger
}

// NewReflexionExecutor creates a new Reflexion executor.
func NewReflexionExecutor(provider llm.Provider, executor tools.ToolExecutor, schemas []llm.ToolSchema, config ReflexionConfig, logger *zap.Logger) *ReflexionExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReflexionExecutor{
		provider: provider, toolExecutor: executor, toolSchemas: schemas, config: config,
		memory: &ReflexionMemory{entries: make([]MemoryEntry, 0)}, logger: logger,
	}
}

func (r *ReflexionExecutor) Name() string { return "reflexion" }

// Execute runs the Reflexion loop.
func (r *ReflexionExecutor) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	result := &ReasoningResult{Pattern: r.Name(), Task: task, Steps: make([]ReasoningStep, 0), Metadata: make(map[string]any)}
	var trials []Trial
	var bestTrial *Trial

	for trialNum := 1; trialNum <= r.config.MaxTrials; trialNum++ {
		select {
		case <-ctx.Done():
			result.TotalLatency = time.Since(start)
			return result, nil
		default:
		}

		trial, tokens, _ := r.executeTrial(ctx, task, trialNum, trials)
		result.TotalTokens += tokens
		trials = append(trials, *trial)

		result.Steps = append(result.Steps, ReasoningStep{StepID: fmt.Sprintf("trial_%d", trialNum), Type: "action", Content: trial.Action, Score: trial.Score})

		if trial.Score >= r.config.SuccessThreshold {
			bestTrial = trial
			break
		}
		if bestTrial == nil || trial.Score > bestTrial.Score {
			bestTrial = trial
		}

		if trialNum < r.config.MaxTrials {
			reflection, reflectTokens, _ := r.generateReflection(ctx, task, trial)
			result.TotalTokens += reflectTokens
			trial.Reflection = reflection
			result.Steps = append(result.Steps, ReasoningStep{StepID: fmt.Sprintf("reflection_%d", trialNum), Type: "reflection", Content: reflection.Analysis})
		}
	}

	if bestTrial != nil {
		result.FinalAnswer = bestTrial.Result
	}
	result.TotalLatency = time.Since(start)
	return result, nil
}

func (r *ReflexionExecutor) executeTrial(ctx context.Context, task string, trialNum int, prevTrials []Trial) (*Trial, int, error) {
	trial := &Trial{Number: trialNum}

	// Use strings.Builder for efficient string concatenation
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task: %s\nTrial: %d\n", task, trialNum))
	if len(prevTrials) > 0 {
		sb.WriteString("Previous attempts:\n")
		for _, t := range prevTrials {
			sb.WriteString(fmt.Sprintf("- Trial %d (score: %.2f)\n", t.Number, t.Score))
			if t.Reflection != nil {
				sb.WriteString(fmt.Sprintf("  Lesson: %s\n", t.Reflection.NextStrategy))
			}
		}
	}
	sb.WriteString("\nProvide your best solution.")
	prompt := sb.String()

	resp, err := r.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o", Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}},
		Tools: r.toolSchemas, Temperature: 0.3, MaxTokens: 2000,
	})
	if err != nil {
		return trial, 0, err
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if len(choice.Message.ToolCalls) > 0 {
			results := r.toolExecutor.Execute(ctx, choice.Message.ToolCalls)
			for _, res := range results {
				trial.Result += string(res.Result)
			}
			trial.Action = "tool_calls"
		} else {
			trial.Action = choice.Message.Content
			trial.Result = choice.Message.Content
		}
	}

	trial.Score, _, _ = r.evaluateTrial(ctx, task, trial)
	return trial, resp.Usage.TotalTokens, nil
}

func (r *ReflexionExecutor) evaluateTrial(ctx context.Context, task string, trial *Trial) (float64, int, error) {
	prompt := fmt.Sprintf("Rate this response (0.0-1.0):\nTask: %s\nResponse: %s\nJSON: {\"score\": X}", task, trial.Result)
	resp, err := r.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o", Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}}, Temperature: 0.1, MaxTokens: 100,
	})
	if err != nil {
		return 0.5, 0, err
	}

	if len(resp.Choices) == 0 {
		return 0.5, resp.Usage.TotalTokens, nil
	}

	var eval struct {
		Score float64 `json:"score"`
	}
	jsonStr := extractJSONFromContent(resp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(jsonStr), &eval); err != nil {
		r.logger.Warn("failed to parse evaluation score", zap.Error(err), zap.String("content", jsonStr))
		return 0.5, resp.Usage.TotalTokens, nil
	}
	return eval.Score, resp.Usage.TotalTokens, nil
}

func (r *ReflexionExecutor) generateReflection(ctx context.Context, task string, trial *Trial) (*Reflection, int, error) {
	prompt := fmt.Sprintf("Analyze this attempt:\nTask: %s\nResult: %s\nScore: %.2f\nJSON: {\"analysis\": \"\", \"mistakes\": [], \"next_strategy\": \"\"}", task, trial.Result, trial.Score)
	resp, err := r.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o", Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}}, Temperature: 0.3, MaxTokens: 500,
	})
	if err != nil {
		return &Reflection{Analysis: "Error", NextStrategy: "Try again"}, 0, err
	}

	if len(resp.Choices) == 0 {
		return &Reflection{Analysis: "No response", NextStrategy: "Try again"}, resp.Usage.TotalTokens, nil
	}

	var reflection Reflection
	jsonStr := extractJSONFromContent(resp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(jsonStr), &reflection); err != nil {
		r.logger.Warn("failed to parse reflection", zap.Error(err), zap.String("content", jsonStr))
		return &Reflection{Analysis: resp.Choices[0].Message.Content, NextStrategy: "Try again"}, resp.Usage.TotalTokens, nil
	}
	return &reflection, resp.Usage.TotalTokens, nil
}

func extractJSONFromContent(s string) string {
	start, depth := -1, 0
	for i, c := range s {
		if c == '{' {
			if start == -1 {
				start = i
			}
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 && start != -1 {
				return s[start : i+1]
			}
		}
	}
	return s
}
