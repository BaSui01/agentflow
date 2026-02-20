package workflow

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent"
)

// ============================================================
// Agent-Workflow Adapters
// These adapters bridge the gap between Agent and Workflow systems.
// ============================================================

// AgentExecutor defines the interface for agent execution.
// This allows workflow to use agents without direct dependency on agent package.
type AgentExecutor interface {
	// Execute executes the agent with the given input.
	Execute(ctx context.Context, input any) (any, error)
	// ID returns the agent's unique identifier.
	ID() string
	// Name returns the agent's name.
	Name() string
}

// AgentStep wraps an AgentExecutor as a workflow Step.
// This allows agents to be used as steps in workflow chains.
type AgentStep struct {
	agent        AgentExecutor
	name         string
	inputMapper  func(any) (any, error)
	outputMapper func(any) (any, error)
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
func WithInputMapper(mapper func(any) (any, error)) AgentStepOption {
	return func(s *AgentStep) {
		s.inputMapper = mapper
	}
}

// WithOutputMapper sets a function to transform output after agent execution.
func WithOutputMapper(mapper func(any) (any, error)) AgentStepOption {
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
func (s *AgentStep) Execute(ctx context.Context, input any) (any, error) {
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
	selector func(ctx context.Context, input any, agents map[string]AgentExecutor) (AgentExecutor, error)
}

// NewAgentRouter creates a new AgentRouter.
func NewAgentRouter(selector func(ctx context.Context, input any, agents map[string]AgentExecutor) (AgentExecutor, error)) *AgentRouter {
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
func (r *AgentRouter) Execute(ctx context.Context, input any) (any, error) {
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
	agents []AgentExecutor
	merger func(results []any) (any, error)
	name   string
}

// NewParallelAgentStep creates a step that executes agents in parallel.
func NewParallelAgentStep(agents []AgentExecutor, merger func([]any) (any, error)) *ParallelAgentStep {
	return &ParallelAgentStep{
		agents: agents,
		merger: merger,
		name:   "parallel_agents",
	}
}

// Execute runs all agents in parallel and merges results.
func (p *ParallelAgentStep) Execute(ctx context.Context, input any) (any, error) {
	if len(p.agents) == 0 {
		return nil, fmt.Errorf("no agents configured")
	}

	type result struct {
		index  int
		output any
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
	results := make([]any, len(p.agents))
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
		check func(ctx context.Context, input any) bool
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
func (c *ConditionalAgentStep) When(check func(ctx context.Context, input any) bool, agent AgentExecutor) *ConditionalAgentStep {
	c.conditions = append(c.conditions, struct {
		check func(ctx context.Context, input any) bool
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
func (c *ConditionalAgentStep) Execute(ctx context.Context, input any) (any, error) {
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

// ============================================================
// Agent Adapter — bridges external Agent implementations to AgentExecutor
// ============================================================

// AgentInterface is a minimal agent contract defined in the workflow package
// to avoid importing the agent package (which would cause circular imports).
// The agent.Agent interface uses (ctx, *Input) -> (*Output, error), but
// this interface uses plain string I/O for simplicity. Callers can use
// AgentAdapterOption functions to customize input/output conversion.
type AgentInterface interface {
	// Execute runs the agent with a string prompt and returns a string response.
	Execute(ctx context.Context, input string) (string, error)
	// ID returns the agent's unique identifier.
	ID() string
	// Name returns the agent's display name.
	Name() string
}

// AgentAdapter adapts an AgentInterface to the AgentExecutor interface,
// allowing any agent implementation to be used in workflow steps.
type AgentAdapter struct {
	agent        AgentInterface
	inputMapper  func(any) (string, error)
	outputMapper func(string) (any, error)
}

// AgentAdapterOption configures an AgentAdapter.
type AgentAdapterOption func(*AgentAdapter)

// WithAgentInputMapper sets a custom function to convert workflow input to agent string input.
func WithAgentInputMapper(mapper func(any) (string, error)) AgentAdapterOption {
	return func(a *AgentAdapter) {
		a.inputMapper = mapper
	}
}

// WithAgentOutputMapper sets a custom function to convert agent string output to workflow output.
func WithAgentOutputMapper(mapper func(string) (any, error)) AgentAdapterOption {
	return func(a *AgentAdapter) {
		a.outputMapper = mapper
	}
}

// NewAgentAdapter creates an AgentAdapter that implements AgentExecutor.
func NewAgentAdapter(agent AgentInterface, opts ...AgentAdapterOption) *AgentAdapter {
	a := &AgentAdapter{agent: agent}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Execute implements AgentExecutor. It converts the workflow input to a string,
// calls the agent, and converts the string output back.
func (a *AgentAdapter) Execute(ctx context.Context, input any) (any, error) {
	// Convert input to string
	var strInput string
	if a.inputMapper != nil {
		var err error
		strInput, err = a.inputMapper(input)
		if err != nil {
			return nil, fmt.Errorf("AgentAdapter: input mapping failed: %w", err)
		}
	} else {
		// Default: use fmt.Sprintf for conversion
		strInput = fmt.Sprintf("%v", input)
	}

	// Execute agent
	strOutput, err := a.agent.Execute(ctx, strInput)
	if err != nil {
		return nil, fmt.Errorf("AgentAdapter: agent execution failed: %w", err)
	}

	// Convert output
	if a.outputMapper != nil {
		return a.outputMapper(strOutput)
	}
	return strOutput, nil
}

// ID implements AgentExecutor.
func (a *AgentAdapter) ID() string {
	return a.agent.ID()
}

// Name implements AgentExecutor.
func (a *AgentAdapter) Name() string {
	return a.agent.Name()
}

// ============================================================
// NativeAgentAdapter — bridges agent.Agent to AgentExecutor
// ============================================================

// NativeAgentAdapter adapts an agent.Agent (with *Input/*Output signatures)
// to the AgentExecutor interface used by workflow steps.
//
// Input conversion (any -> *agent.Input):
//   - *agent.Input: passed through directly
//   - string: wrapped as Input.Content
//   - map[string]any: Content extracted from "content" key, rest goes to Context
//   - other types: converted to string via fmt.Sprintf and set as Content
//
// Output: returns the *agent.Output directly (callers can type-assert).
type NativeAgentAdapter struct {
	agent agent.Agent
}

// NewNativeAgentAdapter creates an adapter that bridges agent.Agent to AgentExecutor.
func NewNativeAgentAdapter(a agent.Agent) *NativeAgentAdapter {
	return &NativeAgentAdapter{agent: a}
}

// Execute implements AgentExecutor. It converts the workflow input to *agent.Input,
// calls agent.Execute, and returns the *agent.Output.
func (n *NativeAgentAdapter) Execute(ctx context.Context, input any) (any, error) {
	agentInput, err := toAgentInput(input)
	if err != nil {
		return nil, fmt.Errorf("NativeAgentAdapter: input conversion failed: %w", err)
	}

	output, err := n.agent.Execute(ctx, agentInput)
	if err != nil {
		return nil, fmt.Errorf("NativeAgentAdapter: agent execution failed: %w", err)
	}

	return output, nil
}

// ID implements AgentExecutor.
func (n *NativeAgentAdapter) ID() string {
	return n.agent.ID()
}

// Name implements AgentExecutor.
func (n *NativeAgentAdapter) Name() string {
	return n.agent.Name()
}

// toAgentInput converts a workflow any value to *agent.Input.
func toAgentInput(input any) (*agent.Input, error) {
	if input == nil {
		return &agent.Input{}, nil
	}

	switch v := input.(type) {
	case *agent.Input:
		return v, nil
	case string:
		return &agent.Input{Content: v}, nil
	case map[string]any:
		inp := &agent.Input{}
		if content, ok := v["content"].(string); ok {
			inp.Content = content
		}
		if traceID, ok := v["trace_id"].(string); ok {
			inp.TraceID = traceID
		}
		if tenantID, ok := v["tenant_id"].(string); ok {
			inp.TenantID = tenantID
		}
		if userID, ok := v["user_id"].(string); ok {
			inp.UserID = userID
		}
		if channelID, ok := v["channel_id"].(string); ok {
			inp.ChannelID = channelID
		}
		if ctxMap, ok := v["context"].(map[string]any); ok {
			inp.Context = ctxMap
		}
		if vars, ok := v["variables"].(map[string]string); ok {
			inp.Variables = vars
		}
		return inp, nil
	default:
		return &agent.Input{Content: fmt.Sprintf("%v", v)}, nil
	}
}
