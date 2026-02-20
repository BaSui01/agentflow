package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"go.uber.org/zap"
)

// ============================================================
// ReWOO( 无观测重振) 模式
// ============================================================

// ReWOOConfig配置了ReWOO推理模式.
type ReWOOConfig struct {
	MaxPlanSteps    int           // Maximum steps in the plan
	Timeout         time.Duration // Overall timeout
	ParallelWorkers int           // Number of parallel workers for independent steps
}

// 默认 ReWOOConfig 返回合理的默认值 。
func DefaultReWOOConfig() ReWOOConfig {
	return ReWOOConfig{
		MaxPlanSteps:    10,
		Timeout:         120 * time.Second,
		ParallelWorkers: 5,
	}
}

// ReWOO执行无观测理性模式.
// 它产生一个完整的计划前, 然后执行所有步骤,
// 最后从所有观测中合成答案。
type ReWOO struct {
	provider     llm.Provider
	toolExecutor tools.ToolExecutor
	toolSchemas  []llm.ToolSchema
	config       ReWOOConfig
	logger       *zap.Logger
}

// NewReWOO创建了新的ReWOO理性.
func NewReWOO(provider llm.Provider, executor tools.ToolExecutor, schemas []llm.ToolSchema, config ReWOOConfig, logger *zap.Logger) *ReWOO {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReWOO{
		provider:     provider,
		toolExecutor: executor,
		toolSchemas:  schemas,
		config:       config,
		logger:       logger,
	}
}

func (r *ReWOO) Name() string { return "rewoo" }

// PlanStep代表了ReWOO计划中的一步.
type PlanStep struct {
	ID           string   `json:"id"`           // e.g., #E1, #E2
	Tool         string   `json:"tool"`         // Tool name to call
	Arguments    string   `json:"arguments"`    // Arguments (may reference previous steps like #E1)
	Dependencies []string `json:"dependencies"` // IDs of steps this depends on
	Reasoning    string   `json:"reasoning"`    // Why this step is needed
}

// 执行运行了ReWOO推理模式.
func (r *ReWOO) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	result := &ReasoningResult{
		Pattern:  r.Name(),
		Task:     task,
		Metadata: make(map[string]any),
	}

	// 第一阶段:规划人员-制定完整的计划
	r.logger.Info("ReWOO Phase 1: Planning")
	plan, planTokens, err := r.generatePlan(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("planning failed: %w", err)
	}
	result.TotalTokens += planTokens
	result.Steps = append(result.Steps, ReasoningStep{
		StepID:     "plan",
		Type:       "thought",
		Content:    fmt.Sprintf("Generated plan with %d steps", len(plan)),
		TokensUsed: planTokens,
	})

	// 第2阶段:工人-执行所有步骤
	r.logger.Info("ReWOO Phase 2: Executing", zap.Int("steps", len(plan)))
	observations, execTokens := r.executeSteps(ctx, plan)
	result.TotalTokens += execTokens

	for id, obs := range observations {
		result.Steps = append(result.Steps, ReasoningStep{
			StepID:  id,
			Type:    "observation",
			Content: obs,
		})
	}

	// 第3阶段:解决方案 - 合成最终答案
	r.logger.Info("ReWOO Phase 3: Solving")
	answer, solveTokens, err := r.synthesize(ctx, task, plan, observations)
	if err != nil {
		return nil, fmt.Errorf("synthesis failed: %w", err)
	}
	result.TotalTokens += solveTokens
	result.FinalAnswer = answer
	result.TotalLatency = time.Since(start)

	result.Metadata["plan_steps"] = len(plan)
	result.Metadata["observations"] = len(observations)

	return result, nil
}

func (r *ReWOO) generatePlan(ctx context.Context, task string) ([]PlanStep, int, error) {
	// 构建工具描述
	var toolDescs []string
	for _, t := range r.toolSchemas {
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	prompt := fmt.Sprintf(`You are a planner. Given a task, create a step-by-step plan using available tools.
Each step should be in format: #E[n] = Tool[arguments]
You can reference previous step results using #E[n] in arguments.

Available tools:
%s

Task: %s

Create a plan (max %d steps). Output as JSON array:
[
  {"id": "#E1", "tool": "tool_name", "arguments": "arg string", "reasoning": "why needed"},
  {"id": "#E2", "tool": "tool_name", "arguments": "use #E1 result", "reasoning": "why needed"}
]`, strings.Join(toolDescs, "\n"), task, r.config.MaxPlanSteps)

	resp, err := r.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.2,
		MaxTokens:   2000,
	})
	if err != nil {
		return nil, 0, err
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("plan generation returned no choices: %w", err)
	}

	content := choice.Message.Content
	tokens := resp.Usage.TotalTokens

	// 从回应中提取 JSON
	content = extractJSON(content)

	var plan []PlanStep
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		r.logger.Warn("failed to parse plan JSON", zap.Error(err), zap.String("content", content))
		// 尝试手动分析
		plan = r.parsePlanManually(content)
	}

	// 构建依赖图
	for i := range plan {
		plan[i].Dependencies = r.extractDependencies(plan[i].Arguments)
	}

	return plan, tokens, nil
}

func (r *ReWOO) extractDependencies(args string) []string {
	re := regexp.MustCompile(`#E\d+`)
	matches := re.FindAllString(args, -1)
	seen := make(map[string]bool)
	var deps []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			deps = append(deps, m)
		}
	}
	return deps
}

func (r *ReWOO) parsePlanManually(content string) []PlanStep {
	var plan []PlanStep
	re := regexp.MustCompile(`#E(\d+)\s*=\s*(\w+)\[([^\]]*)\]`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) >= 4 {
			plan = append(plan, PlanStep{
				ID:        "#E" + m[1],
				Tool:      m[2],
				Arguments: m[3],
			})
		}
	}
	return plan
}

func (r *ReWOO) executeSteps(ctx context.Context, plan []PlanStep) (map[string]string, int) {
	observations := make(map[string]string)
	totalTokens := 0

	// 根据依赖关系构建执行命令
	executed := make(map[string]bool)

	for len(executed) < len(plan) {
		// 查找可以执行的步骤( 所有已满足的道克)
		var ready []PlanStep
		for _, step := range plan {
			if executed[step.ID] {
				continue
			}
			canExecute := true
			for _, dep := range step.Dependencies {
				if !executed[dep] {
					canExecute = false
					break
				}
			}
			if canExecute {
				ready = append(ready, step)
			}
		}

		if len(ready) == 0 {
			r.logger.Warn("no executable steps found, possible circular dependency")
			break
		}

		// 执行已准备好的步骤(可并行)
		for _, step := range ready {
			// 参数中的替代依赖性
			args := step.Arguments
			for dep, obs := range observations {
				args = strings.ReplaceAll(args, dep, obs)
			}

			// 执行工具
			result := r.executeTool(ctx, step.Tool, args)
			observations[step.ID] = result
			executed[step.ID] = true

			r.logger.Debug("executed step",
				zap.String("id", step.ID),
				zap.String("tool", step.Tool),
				zap.String("result_preview", truncate(result, 100)))
		}
	}

	return observations, totalTokens
}

func (r *ReWOO) executeTool(ctx context.Context, toolName, args string) string {
	// 构建工具调用
	argsJSON, _ := json.Marshal(map[string]string{"input": args})
	call := llm.ToolCall{
		ID:        fmt.Sprintf("rewoo_%d", time.Now().UnixNano()),
		Name:      toolName,
		Arguments: argsJSON,
	}

	results := r.toolExecutor.Execute(ctx, []llm.ToolCall{call})
	if len(results) > 0 {
		if results[0].Error != "" {
			return fmt.Sprintf("Error: %s", results[0].Error)
		}
		return string(results[0].Result)
	}
	return "No result"
}

func (r *ReWOO) synthesize(ctx context.Context, task string, plan []PlanStep, observations map[string]string) (string, int, error) {
	// 从计划和观察中构建环境
	var planSummary []string
	for _, step := range plan {
		obs := observations[step.ID]
		planSummary = append(planSummary, fmt.Sprintf("%s = %s[%s] -> %s",
			step.ID, step.Tool, step.Arguments, truncate(obs, 200)))
	}

	prompt := fmt.Sprintf(`You are a solver. Given a task and the results of a plan execution, provide the final answer.

Task: %s

Plan execution results:
%s

Based on these results, provide a clear and complete answer to the task.`, task, strings.Join(planSummary, "\n"))

	resp, err := r.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   1000,
	})
	if err != nil {
		return "", 0, err
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return "", 0, fmt.Errorf("synthesis returned no choices: %w", err)
	}

	return choice.Message.Content, resp.Usage.TotalTokens, nil
}

// 辅助功能

func extractJSON(s string) string {
	// 在响应中查找 JSON 阵列
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
