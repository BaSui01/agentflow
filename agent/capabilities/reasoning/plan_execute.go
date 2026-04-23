package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"go.uber.org/zap"
)

// ============================================================
// 计划和执行模式
// ============================================================

// PlanExecuteConfig 配置了 Plan- and-Execute 推理模式.
type PlanExecuteConfig struct {
	MaxPlanSteps      int           // Maximum steps in initial plan
	MaxReplanAttempts int           // Maximum replanning attempts on failure
	Timeout           time.Duration // Overall timeout
	AdaptivePlanning  bool          // Allow plan modification during execution
	Model             string        // LLM model to use for reasoning steps
}

// 默认 PlanExecuteConfig 返回合理的默认值 。
func DefaultPlanExecuteConfig() PlanExecuteConfig {
	return PlanExecuteConfig{
		MaxPlanSteps:      15,
		MaxReplanAttempts: 3,
		Timeout:           180 * time.Second,
		AdaptivePlanning:  true,
		Model:             "gpt-4o",
	}
}

// PlanAndExecute执行"计划与执行"推理模式.
// 与ReWOO不同,它可以根据中间结果来调整计划.
type PlanAndExecute struct {
	gateway      llmcore.Gateway
	toolExecutor tools.ToolExecutor
	toolSchemas  []types.ToolSchema
	config       PlanExecuteConfig
	logger       *zap.Logger
}

const (
	planStatusPlanning  = "planning"
	planStatusExecuting = "executing"
	planStatusReplan    = "replanning"
	planStatusCompleted = "completed"
	planStatusFailed    = "failed"

	stepStatusPending   = "pending"
	stepStatusRunning   = "running"
	stepStatusCompleted = "completed"
	stepStatusFailed    = "failed"
	stepStatusSkipped   = "skipped"
)

// NewPlanAndExecute创建了一个新的"计划"和"执行"推理器.
func NewPlanAndExecute(gateway llmcore.Gateway, executor tools.ToolExecutor, schemas []types.ToolSchema, config PlanExecuteConfig, logger *zap.Logger) *PlanAndExecute {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PlanAndExecute{
		gateway:      gateway,
		toolExecutor: executor,
		toolSchemas:  schemas,
		config:       config,
		logger:       logger,
	}
}

func (p *PlanAndExecute) Name() string { return "plan_and_execute" }

// 执行计划代表目前的计划状态.
type ExecutionPlan struct {
	Goal           string          `json:"goal"`
	Steps          []ExecutionStep `json:"steps"`
	CurrentStep    int             `json:"current_step"`
	CompletedSteps []string        `json:"completed_steps"`
	Status         string          `json:"status"` // planning, executing, replanning, completed, failed
}

// "执行步骤"代表了计划的一个步骤.
type ExecutionStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Tool        string `json:"tool,omitempty"`
	Arguments   string `json:"arguments,omitempty"`
	Status      string `json:"status"` // pending, running, completed, failed, skipped
	Result      string `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Execute运行"计划与执行"推理模式.
func (p *PlanAndExecute) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	result := &ReasoningResult{
		Pattern:  p.Name(),
		Task:     task,
		Metadata: make(map[string]any),
	}
	result.Metadata["plan_execute_replan_budget"] = p.config.MaxReplanAttempts
	result.Metadata["plan_execute_max_plan_steps"] = p.config.MaxPlanSteps
	result.Metadata["plan_execute_budget_scope"] = "strategy_internal"

	// 第一阶段:制定初步计划
	p.logger.Info("Plan-and-Execute: Creating initial plan")
	plan, planTokens, err := p.createPlan(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("planning failed: %w", err)
	}
	result.TotalTokens += planTokens
	result.Steps = append(result.Steps, ReasoningStep{
		StepID:     "initial_plan",
		Type:       "thought",
		Content:    fmt.Sprintf("Created plan with %d steps", len(plan.Steps)),
		TokensUsed: planTokens,
	})

	// 第二阶段:实施适应性再规划计划
	replanAttempts := 0
	internalStopCause := "completed"
	for plan.Status != planStatusCompleted && plan.Status != planStatusFailed {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// 执行下一步
		stepResult, stepTokens, err := p.executeStep(ctx, plan)
		result.TotalTokens += stepTokens

		if err != nil {
			p.logger.Warn("step execution failed", zap.Error(err))

			if p.config.AdaptivePlanning && replanAttempts < p.config.MaxReplanAttempts {
				// 试图重新规划
				p.logger.Info("attempting to replan", zap.Int("attempt", replanAttempts+1))
				newPlan, replanTokens, replanErr := p.replan(ctx, task, plan, err.Error())
				result.TotalTokens += replanTokens
				replanAttempts++

				if replanErr != nil {
					plan.Status = planStatusFailed
					internalStopCause = "plan_execute_replan_generation_failed"
					result.Steps = append(result.Steps, ReasoningStep{
						StepID:  "replan_failed",
						Type:    "backtrack",
						Content: fmt.Sprintf("Replanning failed: %s", replanErr.Error()),
					})
				} else {
					plan = newPlan
					result.Steps = append(result.Steps, ReasoningStep{
						StepID:     fmt.Sprintf("replan_%d", replanAttempts),
						Type:       "backtrack",
						Content:    fmt.Sprintf("Replanned with %d new steps", len(plan.Steps)-plan.CurrentStep),
						TokensUsed: replanTokens,
					})
				}
				continue
			}

			plan.Status = planStatusFailed
			if replanAttempts >= p.config.MaxReplanAttempts {
				internalStopCause = "plan_execute_replan_budget_exhausted"
			} else {
				internalStopCause = "plan_execute_execution_failed"
			}
			break
		}

		result.Steps = append(result.Steps, ReasoningStep{
			StepID:     stepResult.ID,
			Type:       "action",
			Content:    stepResult.Result,
			TokensUsed: stepTokens,
		})

		// 检查计划是否完成
		if plan.CurrentStep >= len(plan.Steps) {
			plan.Status = planStatusCompleted
		}
	}

	// 第3阶段:合成最后答案
	if plan.Status == planStatusCompleted {
		answer, synthTokens, err := p.synthesizeAnswer(ctx, task, plan)
		result.TotalTokens += synthTokens
		if err != nil {
			p.logger.Warn("synthesis failed", zap.Error(err))
			// 使用最后一个步骤结果作为答案
			if len(plan.Steps) > 0 {
				result.FinalAnswer = plan.Steps[len(plan.Steps)-1].Result
			}
		} else {
			result.FinalAnswer = answer
		}
		result.Confidence = 0.8
	} else {
		result.FinalAnswer = fmt.Sprintf("Plan execution failed after %d steps", plan.CurrentStep)
		result.Confidence = 0.2
	}

	result.TotalLatency = time.Since(start)
	result.Metadata["total_steps"] = len(plan.Steps)
	result.Metadata["completed_steps"] = plan.CurrentStep
	result.Metadata["replan_attempts"] = replanAttempts
	result.Metadata["final_status"] = plan.Status
	result.Metadata["internal_stop_cause"] = internalStopCause

	return result, nil
}

func (p *PlanAndExecute) createPlan(ctx context.Context, task string) (*ExecutionPlan, int, error) {
	// 构建工具描述
	var toolDescs []string
	for _, t := range p.toolSchemas {
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	prompt := fmt.Sprintf(`Plan the execution of this task.

Available tools:
%s

Task: %s

Use the %s tool to return the goal and ordered steps.

Rules:
- Use at most %d steps
- Keep each step concrete and executable
- Prefer tool calls when they materially help
- Do not answer with prose outside the tool call`, strings.Join(toolDescs, "\n"), task, submitExecutionPlanTool, p.config.MaxPlanSteps)

	resp, err := invokeChatGateway(ctx, p.gateway, newGatewayChatRequest(
		defaultModel(p.config.Model),
		[]types.Message{{Role: llmcore.RoleUser, Content: prompt}},
		func(req *llmcore.ChatRequest) {
			req.Tools = []types.ToolSchema{executionPlanToolSchema()}
			req.ToolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeRequired}
			req.ToolCallMode = llmcore.ToolCallModeNative
			req.Temperature = 0.3
			req.MaxTokens = 2000
		},
	))
	if err != nil {
		return nil, 0, err
	}

	choice, err := llmcore.FirstChoice(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("plan creation returned no choices: %w", err)
	}
	tokens := resp.Usage.TotalTokens

	plan, err := parseExecutionPlanToolCall(choice.Message)
	if err != nil {
		return nil, tokens, fmt.Errorf("plan creation did not return native tool call: %w", err)
	}

	plan.Status = planStatusExecuting
	plan.CurrentStep = 0
	for i := range plan.Steps {
		plan.Steps[i].Status = stepStatusPending
	}

	return &plan, tokens, nil
}

func (p *PlanAndExecute) executeStep(ctx context.Context, plan *ExecutionPlan) (*ExecutionStep, int, error) {
	if plan.CurrentStep >= len(plan.Steps) {
		return nil, 0, fmt.Errorf("no more steps to execute")
	}

	step := &plan.Steps[plan.CurrentStep]
	step.Status = stepStatusRunning
	tokens := 0

	p.logger.Debug("executing step",
		zap.String("id", step.ID),
		zap.String("tool", step.Tool),
		zap.String("description", step.Description))

	if step.Tool != "" {
		// 执行工具
		argsJSON, _ := json.Marshal(map[string]string{"input": step.Arguments})
		call := types.ToolCall{
			ID:        step.ID,
			Name:      step.Tool,
			Arguments: argsJSON,
		}

		results := p.toolExecutor.Execute(ctx, []types.ToolCall{call})
		if len(results) > 0 {
			if results[0].Error != "" {
				step.Status = stepStatusFailed
				step.Error = results[0].Error
				return step, tokens, fmt.Errorf("tool execution failed: %s", results[0].Error)
			}
			step.Result = string(results[0].Result)
		}
	} else {
		// 没有指定工具, 请使用 LLM 执行步骤
		result, stepTokens, err := p.executeLLMStep(ctx, plan, step)
		tokens += stepTokens
		if err != nil {
			step.Status = stepStatusFailed
			step.Error = err.Error()
			return step, tokens, err
		}
		step.Result = result
	}

	step.Status = stepStatusCompleted
	plan.CurrentStep++
	plan.CompletedSteps = append(plan.CompletedSteps, step.ID)

	return step, tokens, nil
}

func (p *PlanAndExecute) executeLLMStep(ctx context.Context, plan *ExecutionPlan, step *ExecutionStep) (string, int, error) {
	// 从已完成的步骤建立背景
	var context []string
	for i := 0; i < plan.CurrentStep; i++ {
		s := plan.Steps[i]
		if s.Status == stepStatusCompleted {
			context = append(context, fmt.Sprintf("Step %s: %s -> %s", s.ID, s.Description, truncate(s.Result, 200)))
		}
	}

	prompt := fmt.Sprintf(`Goal: %s

Completed steps:
%s

Current step: %s
Description: %s

Execute this step and provide the result.`, plan.Goal, strings.Join(context, "\n"), step.ID, step.Description)

	resp, err := invokeChatGateway(ctx, p.gateway, newGatewayChatRequest(
		defaultModel(p.config.Model),
		[]types.Message{{Role: llmcore.RoleUser, Content: prompt}},
		func(req *llmcore.ChatRequest) {
			req.Temperature = 0.5
			req.MaxTokens = 1000
		},
	))
	if err != nil {
		return "", 0, err
	}

	choice, err := llmcore.FirstChoice(resp)
	if err != nil {
		return "", 0, fmt.Errorf("LLM step returned no choices: %w", err)
	}

	return choice.Message.Content, resp.Usage.TotalTokens, nil
}

func (p *PlanAndExecute) replan(ctx context.Context, task string, currentPlan *ExecutionPlan, errorMsg string) (*ExecutionPlan, int, error) {
	// 从已完成的步骤建立背景
	var completedContext []string
	for i := 0; i < currentPlan.CurrentStep; i++ {
		s := currentPlan.Steps[i]
		completedContext = append(completedContext, fmt.Sprintf("- %s: %s (result: %s)",
			s.ID, s.Description, truncate(s.Result, 100)))
	}

	failedStep := currentPlan.Steps[currentPlan.CurrentStep]

	prompt := fmt.Sprintf(`The current execution plan failed. Continue from the latest useful state.

Original task: %s

Completed steps:
%s

Failed step: %s - %s
Error: %s

Use the %s tool to return the updated goal and only the remaining steps.

Rules:
- Do not repeat completed work
- Only include the remaining steps
- Do not answer with prose outside the tool call`, task, strings.Join(completedContext, "\n"), failedStep.ID, failedStep.Description, errorMsg, submitExecutionPlanTool)

	resp, err := invokeChatGateway(ctx, p.gateway, newGatewayChatRequest(
		defaultModel(p.config.Model),
		[]types.Message{{Role: llmcore.RoleUser, Content: prompt}},
		func(req *llmcore.ChatRequest) {
			req.Tools = []types.ToolSchema{executionPlanToolSchema()}
			req.ToolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeRequired}
			req.ToolCallMode = llmcore.ToolCallModeNative
			req.Temperature = 0.4
			req.MaxTokens = 1500
		},
	))
	if err != nil {
		return nil, 0, err
	}

	replanChoice, err := llmcore.FirstChoice(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("replan returned no choices: %w", err)
	}
	tokens := resp.Usage.TotalTokens

	newPlan, err := parseExecutionPlanToolCall(replanChoice.Message)
	if err != nil {
		return nil, tokens, fmt.Errorf("replan did not return native tool call: %w", err)
	}

	// 保留已完成的步骤
	newPlan.Steps = append(currentPlan.Steps[:currentPlan.CurrentStep], newPlan.Steps...)
	newPlan.CurrentStep = currentPlan.CurrentStep
	newPlan.CompletedSteps = currentPlan.CompletedSteps
	newPlan.Status = planStatusExecuting

	return &newPlan, tokens, nil
}

func (p *PlanAndExecute) synthesizeAnswer(ctx context.Context, task string, plan *ExecutionPlan) (string, int, error) {
	var stepResults []string
	for _, s := range plan.Steps {
		if s.Status == stepStatusCompleted {
			stepResults = append(stepResults, fmt.Sprintf("- %s: %s", s.Description, truncate(s.Result, 300)))
		}
	}

	prompt := fmt.Sprintf(`Task: %s

Execution results:
%s

Based on these results, provide a clear and complete final answer.`, task, strings.Join(stepResults, "\n"))

	resp, err := invokeChatGateway(ctx, p.gateway, newGatewayChatRequest(
		defaultModel(p.config.Model),
		[]types.Message{{Role: llmcore.RoleUser, Content: prompt}},
		func(req *llmcore.ChatRequest) {
			req.Temperature = 0.3
			req.MaxTokens = 1000
		},
	))
	if err != nil {
		return "", 0, err
	}

	synthChoice, err := llmcore.FirstChoice(resp)
	if err != nil {
		return "", 0, fmt.Errorf("synthesis returned no choices: %w", err)
	}

	return synthChoice.Message.Content, resp.Usage.TotalTokens, nil
}
