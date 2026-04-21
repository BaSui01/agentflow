package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AgentExecutor is a local interface for executing agent capabilities.
// Defined here to avoid importing agent/ (which would create a circular dep).
// The caller (e.g., agent/orchestration/) provides the concrete implementation.
type AgentExecutor interface {
	ExecuteCapability(ctx context.Context, agentID string, capability string, input any) (any, error)
}

// CompositionExecutor executes a composed agent plan from CompositionResult.
type CompositionExecutor struct {
	agentExecutor AgentExecutor
	logger        *zap.Logger
}

// NewCompositionExecutor creates a new CompositionExecutor.
func NewCompositionExecutor(executor AgentExecutor, logger *zap.Logger) *CompositionExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CompositionExecutor{
		agentExecutor: executor,
		logger:        logger.With(zap.String("component", "composition_executor")),
	}
}

// ExecutionResult contains the results of executing a composition.
type ExecutionResult struct {
	// Results maps capability name to its output.
	Results map[string]any
	// Errors maps capability name to its error (if any).
	Errors map[string]error
	// Duration is the total wall-clock time of the execution.
	Duration time.Duration
	// AgentsUsed lists the agent IDs that were actually invoked.
	AgentsUsed []string
	// Completed is true when all capabilities finished without error.
	Completed bool
}

// Execute runs agents according to the CompositionResult plan.
// It respects ExecutionOrder and Dependencies, running independent capabilities
// in parallel within each dependency level.
func (e *CompositionExecutor) Execute(ctx context.Context, result *CompositionResult, input any) (*ExecutionResult, error) {
	if result == nil {
		return nil, fmt.Errorf("composition result is nil")
	}
	if !result.Complete {
		return nil, fmt.Errorf("composition is incomplete: missing capabilities: %v", result.MissingCapabilities)
	}
	if e.agentExecutor == nil {
		return nil, fmt.Errorf("agent executor is nil")
	}

	start := time.Now()

	execResult := &ExecutionResult{
		Results: make(map[string]any),
		Errors:  make(map[string]error),
	}

	// Build reverse dependency map: capability → capabilities it depends on.
	// CompositionResult.Dependencies maps capability → its prerequisites.
	dependsOn := result.Dependencies

	// Track completed capabilities for dependency resolution.
	var completedMu sync.Mutex
	completed := make(map[string]bool)

	// Track which agents were used.
	agentSet := make(map[string]struct{})

	// Group capabilities into levels by dependency depth.
	levels := e.buildExecutionLevels(result.ExecutionOrder, dependsOn)

	e.logger.Info("starting composition execution",
		zap.Int("capabilities", len(result.ExecutionOrder)),
		zap.Int("levels", len(levels)),
	)

	// Execute each level sequentially; within a level, run in parallel.
	for levelIdx, level := range levels {
		// Check context before starting a new level.
		if ctx.Err() != nil {
			e.logger.Warn("context cancelled, stopping execution", zap.Error(ctx.Err()))
			break
		}

		// Filter out capabilities whose dependencies failed.
		runnable := make([]string, 0, len(level))
		for _, cap := range level {
			if e.depsMetOrFailed(cap, dependsOn, completed, execResult.Errors) {
				runnable = append(runnable, cap)
			}
		}

		if len(runnable) == 0 {
			continue
		}

		e.logger.Debug("executing level",
			zap.Int("level", levelIdx),
			zap.Strings("capabilities", runnable),
		)

		var wg sync.WaitGroup
		for _, capName := range runnable {
			wg.Add(1)
			go func(cap string) {
				defer wg.Done()

				agentID, ok := result.CapabilityMap[cap]
				if !ok {
					completedMu.Lock()
					execResult.Errors[cap] = fmt.Errorf("no agent mapped for capability %s", cap)
					completedMu.Unlock()
					return
				}

				// Build input: use original input plus any upstream results.
				capInput := e.buildCapabilityInput(cap, input, dependsOn, execResult.Results, &completedMu)

				out, err := e.agentExecutor.ExecuteCapability(ctx, agentID, cap, capInput)

				completedMu.Lock()
				if err != nil {
					execResult.Errors[cap] = err
					e.logger.Warn("capability execution failed",
						zap.String("capability", cap),
						zap.String("agent_id", agentID),
						zap.Error(err),
					)
				} else {
					execResult.Results[cap] = out
					completed[cap] = true
					agentSet[agentID] = struct{}{}
				}
				completedMu.Unlock()
			}(capName)
		}
		wg.Wait()
	}

	// Collect agents used.
	for id := range agentSet {
		execResult.AgentsUsed = append(execResult.AgentsUsed, id)
	}
	execResult.Duration = time.Since(start)
	execResult.Completed = len(execResult.Errors) == 0

	e.logger.Info("composition execution finished",
		zap.Duration("duration", execResult.Duration),
		zap.Bool("completed", execResult.Completed),
		zap.Int("successes", len(execResult.Results)),
		zap.Int("failures", len(execResult.Errors)),
	)

	return execResult, nil
}

// buildExecutionLevels groups capabilities into dependency levels.
// Level 0 has no dependencies; level N depends only on capabilities in levels < N.
func (e *CompositionExecutor) buildExecutionLevels(order []string, deps map[string][]string) [][]string {
	if len(order) == 0 {
		return nil
	}

	assigned := make(map[string]int) // capability → level index
	levels := make([][]string, 0)

	for _, cap := range order {
		level := 0
		if capDeps, ok := deps[cap]; ok {
			for _, d := range capDeps {
				if dl, found := assigned[d]; found {
					if dl+1 > level {
						level = dl + 1
					}
				}
			}
		}
		assigned[cap] = level

		// Grow levels slice if needed.
		for len(levels) <= level {
			levels = append(levels, nil)
		}
		levels[level] = append(levels[level], cap)
	}

	return levels
}

// depsMetOrFailed returns true if all dependencies of cap are either completed
// successfully or have failed (so we skip this cap rather than block forever).
// A capability is runnable only if all its deps completed without error.
func (e *CompositionExecutor) depsMetOrFailed(cap string, deps map[string][]string, completed map[string]bool, errors map[string]error) bool {
	capDeps, ok := deps[cap]
	if !ok || len(capDeps) == 0 {
		return true
	}
	for _, d := range capDeps {
		if !completed[d] {
			if _, failed := errors[d]; failed {
				// Dependency failed — skip this capability too.
				return false
			}
			// Dependency not yet done and not failed — shouldn't happen within
			// level-based execution, but guard against it.
			return false
		}
	}
	return true
}

// buildCapabilityInput constructs the input for a capability execution.
// It wraps the original input together with upstream dependency results.
func (e *CompositionExecutor) buildCapabilityInput(
	cap string,
	originalInput any,
	deps map[string][]string,
	results map[string]any,
	mu *sync.Mutex,
) map[string]any {
	capInput := map[string]any{
		"input": originalInput,
	}

	capDeps, ok := deps[cap]
	if !ok || len(capDeps) == 0 {
		return capInput
	}

	mu.Lock()
	upstream := make(map[string]any, len(capDeps))
	for _, d := range capDeps {
		if r, found := results[d]; found {
			upstream[d] = r
		}
	}
	mu.Unlock()

	if len(upstream) > 0 {
		capInput["upstream"] = upstream
	}
	return capInput
}
