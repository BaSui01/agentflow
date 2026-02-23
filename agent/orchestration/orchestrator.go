package orchestration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

// Pattern identifies an orchestration pattern.
type Pattern string

const (
	PatternCollaboration Pattern = "collaboration"
	PatternCrew          Pattern = "crew"
	PatternHierarchical  Pattern = "hierarchical"
	PatternHandoff       Pattern = "handoff"
	PatternAuto          Pattern = "auto"
)

// OrchestratorConfig configures the orchestrator behavior.
type OrchestratorConfig struct {
	DefaultPattern    Pattern
	AutoSelectEnabled bool
	Timeout           time.Duration
	MaxAgents         int
}

// DefaultOrchestratorConfig returns sensible defaults.
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		DefaultPattern:    PatternAuto,
		AutoSelectEnabled: true,
		Timeout:           10 * time.Minute,
		MaxAgents:         20,
	}
}

// PatternExecutor is the unified interface for all orchestration patterns.
type PatternExecutor interface {
	Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error)
	Name() Pattern
	CanHandle(task *OrchestrationTask) bool
	Priority(task *OrchestrationTask) int // higher = better fit
}

// OrchestrationTask wraps agent.Input with orchestration metadata.
type OrchestrationTask struct {
	ID          string
	Description string
	Input       *agent.Input
	Agents      []agent.Agent
	Metadata    map[string]any
}

// OrchestrationResult wraps agent.Output with pattern info and metrics.
type OrchestrationResult struct {
	Pattern   Pattern
	Output    *agent.Output
	AgentUsed []string // agent IDs that participated
	Duration  time.Duration
	Metadata  map[string]any
}

// Orchestrator dynamically selects and executes orchestration patterns.
type Orchestrator struct {
	config   OrchestratorConfig
	patterns map[Pattern]PatternExecutor
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(config OrchestratorConfig, logger *zap.Logger) *Orchestrator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Orchestrator{
		config:   config,
		patterns: make(map[Pattern]PatternExecutor),
		logger:   logger.With(zap.String("component", "orchestrator")),
	}
}

// RegisterPattern registers a pattern executor.
func (o *Orchestrator) RegisterPattern(executor PatternExecutor) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.patterns[executor.Name()] = executor
	o.logger.Info("registered pattern", zap.String("pattern", string(executor.Name())))
}

// Execute runs the task using the configured or auto-selected pattern.
func (o *Orchestrator) Execute(ctx context.Context, task *OrchestrationTask) (*OrchestrationResult, error) {
	if task == nil {
		return nil, fmt.Errorf("orchestration task is nil")
	}
	if len(task.Agents) == 0 {
		return nil, fmt.Errorf("no agents provided")
	}
	if o.config.MaxAgents > 0 && len(task.Agents) > o.config.MaxAgents {
		return nil, fmt.Errorf("too many agents: %d exceeds max %d", len(task.Agents), o.config.MaxAgents)
	}

	// Apply timeout
	if o.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.config.Timeout)
		defer cancel()
	}

	// Select pattern
	pattern, err := o.SelectPattern(task)
	if err != nil {
		return nil, fmt.Errorf("pattern selection failed: %w", err)
	}

	o.mu.RLock()
	executor, ok := o.patterns[pattern]
	o.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no executor registered for pattern %q", pattern)
	}

	o.logger.Info("executing orchestration",
		zap.String("pattern", string(pattern)),
		zap.String("task_id", task.ID),
		zap.Int("agents", len(task.Agents)),
	)

	start := time.Now()
	result, err := executor.Execute(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("pattern %q execution failed: %w", pattern, err)
	}

	result.Pattern = pattern
	result.Duration = time.Since(start)

	return result, nil
}

// SelectPattern analyzes the task to pick the best pattern.
func (o *Orchestrator) SelectPattern(task *OrchestrationTask) (Pattern, error) {
	// Explicit pattern requested
	if o.config.DefaultPattern != PatternAuto {
		return o.config.DefaultPattern, nil
	}

	// Auto-select based on agent count and characteristics
	agentCount := len(task.Agents)

	switch {
	case agentCount == 1:
		return PatternHandoff, nil
	case agentCount == 2:
		return PatternCollaboration, nil
	default:
		// 3+ agents: check for hierarchy indicators
		if hasSupervisor(task.Agents) {
			return PatternHierarchical, nil
		}
		// Check if agents have distinct roles via metadata
		if hasDistinctRoles(task) {
			return PatternCrew, nil
		}
		return PatternCollaboration, nil
	}
}

// hasSupervisor checks if any agent has "supervisor" in its name or type.
func hasSupervisor(agents []agent.Agent) bool {
	for _, a := range agents {
		name := strings.ToLower(a.Name())
		agentType := strings.ToLower(string(a.Type()))
		if strings.Contains(name, "supervisor") || strings.Contains(agentType, "supervisor") {
			return true
		}
	}
	return false
}

// hasDistinctRoles checks if the task metadata indicates distinct roles.
func hasDistinctRoles(task *OrchestrationTask) bool {
	if task.Metadata == nil {
		return false
	}
	_, ok := task.Metadata["roles"]
	return ok
}
