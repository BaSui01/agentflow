package workflow

import (
	"context"
	"fmt"
)

// ============================================================
// Agent-Workflow Adapters
// These adapters bridge the gap between Agent and Workflow systems.
// ============================================================

// AgentExecutor defines the interface for agent execution.
// This allows workflow to use agents without direct dependency on agent package.
type AgentExecutor interface {
	// Execute executes the agent with the given input.
	Execute(ctx context.Context, input interface{}) (interface{}, error)
	// ID returns the agent's unique identifier.
	ID() string
	// Name returns the agent's name.
	Name() string
}

// AgentStep wraps an AgentExecutor as a workflow Step.
// This allows agents to be used as steps in workflow chains.
type AgentStep struct {
	agent       AgentExecutor
	name        string
	inputMapper func(interface{}) (interface{}, error)
	outputMapper func(interface{}) (interface{}, error)
}

// AgentStepOption configures an AgentStep.
type AgentStepOption func(*AgentStep)

// WithStepName sets a custom name for the step.
func WithStepName(name string) AgentStepOption {
	return func(s *AgentStep) {
		s.name = name
	}
}

// WithInputMapper sets a function to transform input before agent execution.
func WithInputMapper(mapper func(interface{}) (interface{}, error)) AgentStepOption {
	return func(s *AgentStep) {
		s.inputMapper = mapper
	}
}

// WithOutputMapper sets a function to transform output after agent execution.
func WithOutputMapper(mapper func(interface{}) (interface{}, error)) AgentStepOption {
	return func(s *AgentStep) {
		s.outputMapper = mapper
	}
}

// NewAgentStep creates a new AgentStep from an AgentExecutor.
func NewAgentStep(agent AgentExecutor, opts ...AgentStepOption) *AgentStep {
	s := &AgentStep{
		agent: agent,
		name:  fmt.Sprintf("agent:%s", agent.Name()),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Execute implements the Step interface.
func (s *AgentStep) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	// Apply input mapper if configured
	if s.inputMapper != nil {
		var err error
		input, err = s.inputMapper(input)
		if err != nil {
			return nil, fmt.Errorf("input mapping failed: %w", err)
		}
	}

	// Execute the agent
	output, err := s.agent.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	// Apply output mapper if configured
	if s.outputMapper != nil {
		output, err = s.outputMapper(output)
		if err != nil {
			return nil, fmt.Errorf("output mapping failed: %w", err)
		}
	}

	return output, nil
}

// Name implements the Step interface.
func (s *AgentStep) Name() string {
	return s.name
}

// AgentID returns the underlying agent's ID.
func (s *AgentStep) AgentID() string {
	return s.agent.ID()
}

// ============================================================
// Multi-Agent Workflow Support
// ============================================================

// AgentRouter routes tasks to appropriate agents based on criteria.
type AgentRouter struct {
	agents   map[string]AgentExecutor
	selector func(ctx context.Context, input interface{}, agents map[string]AgentExecutor) (AgentExecutor, error)
}

// NewAgentRouter creates a new AgentRouter.
func NewAgentRouter(selector func(ctx context.Context, input interface{}, agents map[string]AgentExecutor) (AgentExecutor, error)) *AgentRouter {
	return &AgentRouter{
		agents:   make(map[string]AgentExecutor),
		selector: selector,
	}
}

// RegisterAgent registers an agent with the router.
func (r *AgentRouter) RegisterAgent(agent AgentExecutor) {
	r.agents[agent.ID()] = agent
}

// Execute implements the Step interface by routing to the appropriate agent.
func (r *AgentRouter) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	if r.selector == nil {
		return nil, fmt.Errorf("no agent selector configured")
	}

	agent, err := r.selector(ctx, input, r.agents)
	if err != nil {
		return nil, fmt.Errorf("agent selection failed: %w", err)
	}

	if agent == nil {
		return nil, fmt.Errorf("no suitable agent found")
	}

	return agent.Execute(ctx, input)
}

// Name implements the Step interface.
func (r *AgentRouter) Name() string {
	return "agent_router"
}

// ============================================================
// Parallel Agent Execution
// ============================================================

// ParallelAgentStep executes multiple agents in parallel.
type ParallelAgentStep struct {
	agents   []AgentExecutor
	merger   func(results []interface{}) (interface{}, error)
	name     string
}

// NewParallelAgentStep creates a step that executes agents in parallel.
func NewParallelAgentStep(agents []AgentExecutor, merger func([]interface{}) (interface{}, error)) *ParallelAgentStep {
	return &ParallelAgentStep{
		agents: agents,
		merger: merger,
		name:   "parallel_agents",
	}
}

// Execute runs all agents in parallel and merges results.
func (p *ParallelAgentStep) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	if len(p.agents) == 0 {
		return nil, fmt.Errorf("no agents configured")
	}

	type result struct {
		index  int
		output interface{}
		err    error
	}

	resultCh := make(chan result, len(p.agents))

	// Execute all agents in parallel
	for i, agent := range p.agents {
		go func(idx int, a AgentExecutor) {
			output, err := a.Execute(ctx, input)
			resultCh <- result{index: idx, output: output, err: err}
		}(i, agent)
	}

	// Collect results
	results := make([]interface{}, len(p.agents))
	var firstErr error
	for i := 0; i < len(p.agents); i++ {
		r := <-resultCh
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		results[r.index] = r.output
	}

	if firstErr != nil {
		return nil, fmt.Errorf("parallel execution failed: %w", firstErr)
	}

	// Merge results
	if p.merger != nil {
		return p.merger(results)
	}

	return results, nil
}

// Name implements the Step interface.
func (p *ParallelAgentStep) Name() string {
	return p.name
}

// ============================================================
// Conditional Agent Execution
// ============================================================

// ConditionalAgentStep executes different agents based on conditions.
type ConditionalAgentStep struct {
	conditions []struct {
		check func(ctx context.Context, input interface{}) bool
		agent AgentExecutor
	}
	defaultAgent AgentExecutor
	name         string
}

// NewConditionalAgentStep creates a conditional agent step.
func NewConditionalAgentStep() *ConditionalAgentStep {
	return &ConditionalAgentStep{
		name: "conditional_agent",
	}
}

// When adds a condition-agent pair.
func (c *ConditionalAgentStep) When(check func(ctx context.Context, input interface{}) bool, agent AgentExecutor) *ConditionalAgentStep {
	c.conditions = append(c.conditions, struct {
		check func(ctx context.Context, input interface{}) bool
		agent AgentExecutor
	}{check: check, agent: agent})
	return c
}

// Default sets the default agent when no conditions match.
func (c *ConditionalAgentStep) Default(agent AgentExecutor) *ConditionalAgentStep {
	c.defaultAgent = agent
	return c
}

// Execute implements the Step interface.
func (c *ConditionalAgentStep) Execute(ctx context.Context, input interface{}) (interface{}, error) {
	for _, cond := range c.conditions {
		if cond.check(ctx, input) {
			return cond.agent.Execute(ctx, input)
		}
	}

	if c.defaultAgent != nil {
		return c.defaultAgent.Execute(ctx, input)
	}

	return nil, fmt.Errorf("no matching condition and no default agent")
}

// Name implements the Step interface.
func (c *ConditionalAgentStep) Name() string {
	return c.name
}
