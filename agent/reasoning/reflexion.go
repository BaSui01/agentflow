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

// ReflexionConfig 配置了 Reflexion 执行器.
type ReflexionConfig struct {
	MaxTrials        int           `json:"max_trials"`
	SuccessThreshold float64       `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
	EnableMemory     bool          `json:"enable_memory"`
}

// 默认 Reflexion Config 返回合理的默认值 。
func DefaultReflexionConfig() ReflexionConfig {
	return ReflexionConfig{MaxTrials: 5, SuccessThreshold: 0.8, Timeout: 300 * time.Second, EnableMemory: true}
}

// 审判是解决这项任务的一次尝试。
type Trial struct {
	Number     int         `json:"number"`
	Action     string      `json:"action"`
	Result     string      `json:"result"`
	Score      float64     `json:"score"`
	Reflection *Reflection `json:"reflection,omitempty"`
}

// 反思是对审判的反馈。
type Reflection struct {
	Analysis     string   `json:"analysis"`
	Mistakes     []string `json:"mistakes"`
	NextStrategy string   `json:"next_strategy"`
}

// 折射记忆存储过去的经验。
type ReflexionMemory struct {
	mu      sync.RWMutex
	entries []MemoryEntry
}

// 内存 Entry 代表存储的经验 。
type MemoryEntry struct {
	Task       string      `json:"task"`
	Reflection *Reflection `json:"reflection"`
}

// ReflexionExecutor执行Reflexion模式.
type ReflexionExecutor struct {
	provider     llm.Provider
	toolExecutor tools.ToolExecutor
	toolSchemas  []llm.ToolSchema
	config       ReflexionConfig
	memory       *ReflexionMemory
	logger       *zap.Logger
}

// 新ReflexionExecutor创建了新的Reflexion执行器.
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

// 执行运行折射回路.
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

		// BUG-4 FIX: 正确处理 executeTrial 错误，记录日志并在非 context 错误时继续尝试
		trial, tokens, trialErr := r.executeTrial(ctx, task, trialNum, trials)
		result.TotalTokens += tokens
		if trialErr != nil {
			r.logger.Error("trial execution failed",
				zap.Int("trial", trialNum), zap.Error(trialErr))
			// context 取消/超时时直接返回
			if ctx.Err() != nil {
				result.TotalLatency = time.Since(start)
				return result, trialErr
			}
			// 其他错误继续下一轮 trial
			continue
		}
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
			// BUG-4 FIX: 正确处理 generateReflection 错误，记录日志后继续（reflection 失败不阻塞流程）
			reflection, reflectTokens, reflectErr := r.generateReflection(ctx, task, trial)
			result.TotalTokens += reflectTokens
			if reflectErr != nil {
				r.logger.Warn("reflection generation failed",
					zap.Int("trial", trialNum), zap.Error(reflectErr))
			}
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

	// 使用字符串。 高效字符串连接的构建器
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
