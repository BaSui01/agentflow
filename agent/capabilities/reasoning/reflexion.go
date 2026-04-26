package reasoning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"go.uber.org/zap"
)

// ReflexionConfig 配置了 Reflexion 执行器.
type ReflexionConfig struct {
	MaxTrials        int           `json:"max_trials"`
	SuccessThreshold float64       `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
	EnableMemory     bool          `json:"enable_memory"`
	Model            string        `json:"model,omitempty"`
}

// 默认 Reflexion Config 返回合理的默认值 。
func DefaultReflexionConfig() ReflexionConfig {
	return ReflexionConfig{MaxTrials: 5, SuccessThreshold: 0.8, Timeout: 300 * time.Second, EnableMemory: true, Model: "gpt-4o"}
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

type reflexionScore struct {
	Score float64 `json:"score"`
}

// 折射记忆存储过去的经验。
type ReflexionMemory struct {
	entries []MemoryEntry
}

// 内存 Entry 代表存储的经验 。
type MemoryEntry struct {
	Task       string      `json:"task"`
	Reflection *Reflection `json:"reflection"`
}

// ReflexionExecutor执行Reflexion模式.
type ReflexionExecutor struct {
	gateway      llmcore.Gateway
	toolExecutor tools.ToolExecutor
	toolSchemas  []types.ToolSchema
	config       ReflexionConfig
	memory       *ReflexionMemory
	logger       *zap.Logger
}

// 新ReflexionExecutor创建了新的Reflexion执行器.
func NewReflexionExecutor(gateway llmcore.Gateway, executor tools.ToolExecutor, schemas []types.ToolSchema, config ReflexionConfig, logger *zap.Logger) *ReflexionExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReflexionExecutor{
		gateway: gateway, toolExecutor: executor, toolSchemas: schemas, config: config,
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
	result.Metadata["reflexion_trial_budget"] = r.config.MaxTrials
	result.Metadata["reflexion_success_threshold"] = r.config.SuccessThreshold
	result.Metadata["reflexion_budget_scope"] = "strategy_internal"
	result.Metadata["internal_stop_cause"] = "completed"
	var trials []Trial
	var bestTrial *Trial

	for trialNum := 1; trialNum <= r.config.MaxTrials; trialNum++ {
		select {
		case <-ctx.Done():
			result.TotalLatency = time.Since(start)
			result.Metadata["internal_stop_cause"] = "reflexion_timeout"
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
	if bestTrial != nil {
		result.Confidence = bestTrial.Score
	}
	if bestTrial != nil && bestTrial.Score < r.config.SuccessThreshold {
		result.Metadata["internal_stop_cause"] = "reflexion_trial_budget_exhausted"
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

	resp, err := invokeChatGateway(ctx, r.gateway, newGatewayChatRequest(
		defaultModel(r.config.Model),
		[]types.Message{{Role: llmcore.RoleUser, Content: prompt}},
		func(req *llmcore.ChatRequest) {
			req.Tools = append([]types.ToolSchema(nil), r.toolSchemas...)
			req.Temperature = 0.3
			req.MaxTokens = 2000
		},
	))
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

	var evalErr error
	trial.Score, _, evalErr = r.evaluateTrial(ctx, task, trial)
	if evalErr != nil {
		r.logger.Warn("reflexion trial evaluation failed", zap.Error(evalErr))
	}
	return trial, resp.Usage.TotalTokens, nil
}

func (r *ReflexionExecutor) evaluateTrial(ctx context.Context, task string, trial *Trial) (float64, int, error) {
	prompt := fmt.Sprintf("Rate this response on a 0.0-1.0 scale.\nTask: %s\nResponse: %s", task, trial.Result)
	parseResult, err := generateStructured[reflexionScore](ctx, r.gateway, newGatewayChatRequest(
		defaultModel(r.config.Model),
		[]types.Message{{Role: llmcore.RoleUser, Content: prompt}},
		func(req *llmcore.ChatRequest) {
			req.Temperature = 0.1
			req.MaxTokens = 100
		},
	))
	if err != nil {
		return 0.5, 0, err
	}
	return parseResult.Value.Score, structuredTokens(parseResult), nil
}

func (r *ReflexionExecutor) generateReflection(ctx context.Context, task string, trial *Trial) (*Reflection, int, error) {
	prompt := fmt.Sprintf("Analyze this attempt.\nTask: %s\nResult: %s\nScore: %.2f", task, trial.Result, trial.Score)
	parseResult, err := generateStructured[Reflection](ctx, r.gateway, newGatewayChatRequest(
		defaultModel(r.config.Model),
		[]types.Message{{Role: llmcore.RoleUser, Content: prompt}},
		func(req *llmcore.ChatRequest) {
			req.Temperature = 0.3
			req.MaxTokens = 500
		},
	))
	if err != nil {
		return &Reflection{Analysis: "Error", NextStrategy: "Try again"}, 0, err
	}
	return parseResult.Value, structuredTokens(parseResult), nil
}
