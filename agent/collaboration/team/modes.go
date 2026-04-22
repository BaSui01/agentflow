package team

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent"
	planner "github.com/BaSui01/agentflow/agent/capabilities/planning"

	"go.uber.org/zap"
)

// =============================================================================
// SupervisorMode — 第一个 member 作为 supervisor，分解任务给 workers
// =============================================================================

type supervisorMode struct {
	logger *zap.Logger
}

func newSupervisorMode(logger *zap.Logger) *supervisorMode {
	return &supervisorMode{logger: logger.Named("supervisor")}
}

func (m *supervisorMode) Execute(ctx context.Context, members []agent.TeamMember, task string, config TeamConfig, opts agent.TeamOptions) (*agent.Output, error) {
	if len(members) < 2 {
		return nil, fmt.Errorf("supervisor mode requires at least 2 members (1 supervisor + 1 worker)")
	}

	supervisor := members[0]
	workers := members[1:]

	if config.EnablePlanner {
		return m.executeWithPlanner(ctx, supervisor, workers, task)
	}
	return m.executeSimple(ctx, supervisor, workers, task)
}

// executeWithPlanner uses TaskPlanner to decompose and dispatch tasks.
func (m *supervisorMode) executeWithPlanner(ctx context.Context, supervisor agent.TeamMember, workers []agent.TeamMember, task string) (*agent.Output, error) {
	planInput := &agent.Input{
		Content: fmt.Sprintf(
			"Original task:\n%s\n\nAvailable workers: %s\n\nCreate an ordered execution plan that can be distributed across the team. Focus on directly executable steps; the runtime will assign them to workers.",
			task,
			workerList(workers),
		),
	}
	planResult, err := supervisor.Agent.Plan(ctx, planInput)
	if err != nil || planResult == nil || len(planResult.Steps) == 0 {
		m.logger.Warn("supervisor planner unavailable, returning supervisor response directly",
			zap.Error(err),
		)
		supOutput, execErr := supervisor.Agent.Execute(ctx, &agent.Input{
			Content: fmt.Sprintf("You are a supervisor. Provide instructions for your workers to complete this task: %s", task),
		})
		if execErr != nil {
			if err != nil {
				return nil, fmt.Errorf("supervisor planning failed: %w", err)
			}
			return nil, fmt.Errorf("supervisor failed: %w", execErr)
		}
		return supOutput, nil
	}

	taskArgs := buildPlannerTasks(planResult, workers)
	if len(taskArgs) == 0 {
		return nil, fmt.Errorf("supervisor plan did not include executable steps")
	}

	// Step 2: Create plan and execute
	dispatcher := planner.NewDefaultDispatcher(planner.StrategyByRole, m.logger)
	tp := planner.NewTaskPlanner(dispatcher, m.logger)

	plan, err := tp.CreatePlan(ctx, planner.CreatePlanArgs{
		Title: fmt.Sprintf("Plan for: %s", truncateStr(task, 60)),
		Tasks: taskArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create plan: %w", err)
	}

	// Build executor map from workers
	executors := make(map[string]planner.Executor, len(workers))
	for _, w := range workers {
		executors[w.Role] = &agentExecutorAdapter{agent: w.Agent}
	}

	executor := planner.NewPlanExecutor(tp, dispatcher, len(workers), m.logger)
	result, err := executor.ExecuteWithAgents(ctx, plan.ID, executors)
	if err != nil {
		return nil, fmt.Errorf("plan execution failed: %w", err)
	}

	totalPlanTokens := planResultTokens(planResult)
	return &agent.Output{
		Content:    result.Content,
		TokensUsed: result.TokensUsed + totalPlanTokens,
		Cost:       result.Cost,
		Duration:   result.Duration,
		Metadata: map[string]any{
			"plan_id":        plan.ID,
			"mode":           "supervisor",
			"planning_steps": len(taskArgs),
			"plan_source":    "agent_plan",
		},
	}, nil
}

// executeSimple runs supervisor → workers sequentially without planner.
func (m *supervisorMode) executeSimple(ctx context.Context, supervisor agent.TeamMember, workers []agent.TeamMember, task string) (*agent.Output, error) {
	supOutput, err := supervisor.Agent.Execute(ctx, &agent.Input{
		Content: fmt.Sprintf("You are a supervisor. Provide instructions for your workers to complete this task: %s", task),
	})
	if err != nil {
		return nil, fmt.Errorf("supervisor failed: %w", err)
	}

	var (
		contents    []string
		totalTokens = supOutput.TokensUsed
		totalCost   = supOutput.Cost
	)

	for _, w := range workers {
		out, execErr := w.Agent.Execute(ctx, &agent.Input{
			Content: fmt.Sprintf("Instructions from supervisor:\n%s\n\nOriginal task: %s", supOutput.Content, task),
		})
		if execErr != nil {
			m.logger.Warn("worker failed", zap.String("role", w.Role), zap.Error(execErr))
			continue
		}
		contents = append(contents, fmt.Sprintf("[%s] %s", w.Role, out.Content))
		totalTokens += out.TokensUsed
		totalCost += out.Cost
	}

	return &agent.Output{
		Content:    strings.Join(contents, "\n\n"),
		TokensUsed: totalTokens,
		Cost:       totalCost,
		Metadata:   map[string]any{"mode": "supervisor"},
	}, nil
}

// =============================================================================
// RoundRobinMode — 成员轮流处理，每轮输出作为下一轮输入
// =============================================================================

type roundRobinMode struct {
	logger *zap.Logger
}

func newRoundRobinMode(logger *zap.Logger) *roundRobinMode {
	return &roundRobinMode{logger: logger.Named("round_robin")}
}

func (m *roundRobinMode) Execute(ctx context.Context, members []agent.TeamMember, task string, config TeamConfig, opts agent.TeamOptions) (*agent.Output, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("round_robin mode requires at least 1 member")
	}

	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = len(members)
	}

	var (
		history     []TurnRecord
		current     = task
		lastOutput  *agent.Output
		totalTokens int
		totalCost   float64
	)

	for round := 0; round < maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			break
		}

		member := members[round%len(members)]
		m.logger.Debug("round_robin turn",
			zap.Int("round", round+1),
			zap.String("agent", member.Role),
		)

		out, err := member.Agent.Execute(ctx, &agent.Input{Content: current})
		if err != nil {
			m.logger.Warn("agent failed in round_robin",
				zap.String("role", member.Role),
				zap.Int("round", round+1),
				zap.Error(err),
			)
			if lastOutput != nil {
				break
			}
			return nil, fmt.Errorf("round_robin: agent %s failed at round %d: %w", member.Role, round+1, err)
		}

		lastOutput = out
		totalTokens += out.TokensUsed
		totalCost += out.Cost
		current = out.Content

		history = append(history, TurnRecord{
			AgentID: member.Agent.ID(),
			Content: out.Content,
			Round:   round + 1,
		})

		if config.TerminationFunc != nil && config.TerminationFunc(history) {
			m.logger.Debug("termination condition met", zap.Int("round", round+1))
			break
		}
	}

	if lastOutput == nil {
		return nil, fmt.Errorf("round_robin completed without producing output")
	}

	lastOutput.TokensUsed = totalTokens
	lastOutput.Cost = totalCost
	if lastOutput.Metadata == nil {
		lastOutput.Metadata = make(map[string]any)
	}
	lastOutput.Metadata["mode"] = "round_robin"
	lastOutput.Metadata["rounds"] = len(history)
	return lastOutput, nil
}

// =============================================================================
// SelectorMode — LLM 选择下一个发言的 agent
// =============================================================================

type selectorMode struct {
	logger *zap.Logger
}

func newSelectorMode(logger *zap.Logger) *selectorMode {
	return &selectorMode{logger: logger.Named("selector")}
}

func (m *selectorMode) Execute(ctx context.Context, members []agent.TeamMember, task string, config TeamConfig, opts agent.TeamOptions) (*agent.Output, error) {
	if len(members) < 2 {
		return nil, fmt.Errorf("selector mode requires at least 2 members (1 selector + 1 worker)")
	}

	selector := members[0]
	candidates := members[1:]

	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}

	var (
		history     []TurnRecord
		lastOutput  *agent.Output
		totalTokens int
		totalCost   float64
	)

	for round := 0; round < maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			break
		}

		// Build selection prompt
		selectionPrompt := m.buildSelectionPrompt(config.SelectorPrompt, task, candidates, history)

		selOut, err := selector.Agent.Execute(ctx, &agent.Input{Content: selectionPrompt})
		if err != nil {
			m.logger.Warn("selector failed", zap.Int("round", round+1), zap.Error(err))
			break
		}
		totalTokens += selOut.TokensUsed
		totalCost += selOut.Cost

		// Match selected agent
		chosen := m.matchAgent(selOut.Content, candidates)
		if chosen == nil {
			m.logger.Debug("selector did not choose a valid agent, ending",
				zap.Int("round", round+1),
				zap.String("selector_output", truncateStr(selOut.Content, 100)),
			)
			// Use selector output as final if no agent matched
			if lastOutput == nil {
				lastOutput = selOut
			}
			break
		}

		m.logger.Debug("selector chose agent",
			zap.Int("round", round+1),
			zap.String("chosen", chosen.Role),
		)

		// Execute chosen agent
		agentInput := task
		if len(history) > 0 {
			agentInput = fmt.Sprintf("Previous discussion:\n%s\n\nOriginal task: %s",
				formatHistory(history), task)
		}

		out, err := chosen.Agent.Execute(ctx, &agent.Input{Content: agentInput})
		if err != nil {
			m.logger.Warn("chosen agent failed",
				zap.String("role", chosen.Role),
				zap.Error(err),
			)
			continue
		}

		lastOutput = out
		totalTokens += out.TokensUsed
		totalCost += out.Cost

		history = append(history, TurnRecord{
			AgentID: chosen.Agent.ID(),
			Content: out.Content,
			Round:   round + 1,
		})

		if config.TerminationFunc != nil && config.TerminationFunc(history) {
			break
		}
	}

	if lastOutput == nil {
		return nil, fmt.Errorf("selector mode completed without producing output")
	}

	lastOutput.TokensUsed = totalTokens
	lastOutput.Cost = totalCost
	if lastOutput.Metadata == nil {
		lastOutput.Metadata = make(map[string]any)
	}
	lastOutput.Metadata["mode"] = "selector"
	lastOutput.Metadata["rounds"] = len(history)
	return lastOutput, nil
}

func (m *selectorMode) buildSelectionPrompt(prefix, task string, candidates []agent.TeamMember, history []TurnRecord) string {
	var sb strings.Builder

	if prefix != "" {
		sb.WriteString(prefix)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("You are a selector agent. Choose the most appropriate agent to handle the next step.\n\n")
	}

	sb.WriteString("Available agents:\n")
	for _, c := range candidates {
		sb.WriteString(fmt.Sprintf("- %s (ID: %s)\n", c.Role, c.Agent.ID()))
	}

	sb.WriteString(fmt.Sprintf("\nTask: %s\n", task))

	if len(history) > 0 {
		sb.WriteString(fmt.Sprintf("\nConversation so far:\n%s\n", formatHistory(history)))
	}

	sb.WriteString("\nRespond with ONLY the role name of the agent you want to speak next. If the task is complete, respond with DONE.")
	return sb.String()
}

func (m *selectorMode) matchAgent(output string, candidates []agent.TeamMember) *agent.TeamMember {
	normalized := strings.TrimSpace(strings.ToLower(output))
	if normalized == "done" {
		return nil
	}
	for i := range candidates {
		if strings.Contains(normalized, strings.ToLower(candidates[i].Role)) {
			return &candidates[i]
		}
		if strings.Contains(normalized, strings.ToLower(candidates[i].Agent.ID())) {
			return &candidates[i]
		}
	}
	return nil
}

// =============================================================================
// SwarmMode — 自主协作，通过 HANDOFF 指令传递控制权
// =============================================================================

type swarmMode struct {
	logger *zap.Logger
}

func newSwarmMode(logger *zap.Logger) *swarmMode {
	return &swarmMode{logger: logger.Named("swarm")}
}

const handoffPrefix = "HANDOFF:"

func (m *swarmMode) Execute(ctx context.Context, members []agent.TeamMember, task string, config TeamConfig, opts agent.TeamOptions) (*agent.Output, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("swarm mode requires at least 1 member")
	}

	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}

	// Build member lookup
	memberMap := make(map[string]*agent.TeamMember, len(members))
	for i := range members {
		memberMap[strings.ToLower(members[i].Role)] = &members[i]
		memberMap[strings.ToLower(members[i].Agent.ID())] = &members[i]
	}

	current := &members[0]
	currentInput := task
	var (
		lastOutput  *agent.Output
		totalTokens int
		totalCost   float64
		history     []TurnRecord
	)

	for round := 0; round < maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			break
		}

		m.logger.Debug("swarm turn",
			zap.Int("round", round+1),
			zap.String("agent", current.Role),
		)

		prompt := currentInput
		if len(history) > 0 {
			prompt = fmt.Sprintf("Previous discussion:\n%s\n\nYour turn. Original task: %s\n\nCurrent input: %s\n\n"+
				"If you want to hand off to another agent, include HANDOFF:<agent_role> in your response.",
				formatHistory(history), task, currentInput)
		}

		out, err := current.Agent.Execute(ctx, &agent.Input{Content: prompt})
		if err != nil {
			m.logger.Warn("swarm agent failed",
				zap.String("role", current.Role),
				zap.Error(err),
			)
			if lastOutput != nil {
				break
			}
			return nil, fmt.Errorf("swarm: agent %s failed: %w", current.Role, err)
		}

		lastOutput = out
		totalTokens += out.TokensUsed
		totalCost += out.Cost

		history = append(history, TurnRecord{
			AgentID: current.Agent.ID(),
			Content: out.Content,
			Round:   round + 1,
		})

		if config.TerminationFunc != nil && config.TerminationFunc(history) {
			break
		}

		// Check for handoff
		nextAgent := extractHandoff(out.Content, memberMap)
		if nextAgent == nil {
			// No handoff — current agent is done
			break
		}

		m.logger.Debug("swarm handoff",
			zap.String("from", current.Role),
			zap.String("to", nextAgent.Role),
		)

		// Remove handoff directive from content for next agent
		currentInput = removeHandoff(out.Content)
		current = nextAgent
	}

	if lastOutput == nil {
		return nil, fmt.Errorf("swarm completed without producing output")
	}

	lastOutput.TokensUsed = totalTokens
	lastOutput.Cost = totalCost
	if lastOutput.Metadata == nil {
		lastOutput.Metadata = make(map[string]any)
	}
	lastOutput.Metadata["mode"] = "swarm"
	lastOutput.Metadata["rounds"] = len(history)
	return lastOutput, nil
}

// extractHandoff looks for "HANDOFF:<agent_name>" in the output.
func extractHandoff(content string, memberMap map[string]*agent.TeamMember) *agent.TeamMember {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), handoffPrefix) {
			// Also check within the line
			idx := strings.Index(strings.ToUpper(line), handoffPrefix)
			if idx < 0 {
				continue
			}
			line = line[idx:]
		}
		target := strings.TrimSpace(line[len(handoffPrefix):])
		target = strings.ToLower(target)
		if m, ok := memberMap[target]; ok {
			return m
		}
	}
	return nil
}

// removeHandoff strips HANDOFF directives from content.
func removeHandoff(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		upper := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(upper, handoffPrefix) {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// =============================================================================
// Helpers
// =============================================================================

// agentExecutorAdapter adapts agent.Agent to planner.Executor interface.
type agentExecutorAdapter struct {
	agent agent.Agent
}

func (a *agentExecutorAdapter) ID() string   { return a.agent.ID() }
func (a *agentExecutorAdapter) Name() string { return a.agent.Name() }

func (a *agentExecutorAdapter) Execute(ctx context.Context, content string, taskCtx map[string]any) (*planner.TaskOutput, error) {
	out, err := a.agent.Execute(ctx, &agent.Input{
		Content: content,
		Context: taskCtx,
	})
	if err != nil {
		return nil, err
	}
	return &planner.TaskOutput{
		Content:    out.Content,
		TokensUsed: out.TokensUsed,
		Cost:       out.Cost,
		Duration:   out.Duration,
		Metadata:   out.Metadata,
	}, nil
}

func workerList(workers []agent.TeamMember) string {
	names := make([]string, len(workers))
	for i, w := range workers {
		names[i] = w.Role
	}
	return strings.Join(names, ", ")
}

func formatHistory(history []TurnRecord) string {
	var sb strings.Builder
	for _, h := range history {
		sb.WriteString(fmt.Sprintf("[Round %d - %s]: %s\n", h.Round, h.AgentID, h.Content))
	}
	return sb.String()
}

func buildPlannerTasks(plan *agent.PlanResult, workers []agent.TeamMember) []planner.CreateTaskArgs {
	if plan == nil || len(plan.Steps) == 0 || len(workers) == 0 {
		return nil
	}

	tasks := make([]planner.CreateTaskArgs, 0, len(plan.Steps))
	var prevID string
	for i, step := range plan.Steps {
		description := strings.TrimSpace(step)
		if description == "" {
			continue
		}
		id := fmt.Sprintf("step_%d", i+1)
		assignTo := workers[i%len(workers)].Role
		task := planner.CreateTaskArgs{
			ID:          id,
			AssignTo:    assignTo,
			Title:       truncateStr(description, 48),
			Description: description,
			Priority:    len(plan.Steps) - i,
			Metadata: map[string]any{
				"plan_index": i,
			},
		}
		if prevID != "" {
			task.Dependencies = []string{prevID}
		}
		tasks = append(tasks, task)
		prevID = id
	}
	return tasks
}

func planResultTokens(plan *agent.PlanResult) int {
	if plan == nil || plan.Metadata == nil {
		return 0
	}
	switch value := plan.Metadata["tokens_used"].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
