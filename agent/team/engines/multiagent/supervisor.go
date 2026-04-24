package multiagent

import (
	"context"
	"fmt"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// TaskSplitter splits a supervisor input into sub-tasks for workers.
type TaskSplitter interface {
	Split(ctx context.Context, input *agent.Input) ([]WorkerTask, error)
}

// SupervisorConfig configures the Supervisor.
type SupervisorConfig struct {
	AggregationStrategy AggregationStrategy
	FailurePolicy       FailurePolicy
	MaxRetries          int
	TaskTimeout         time.Duration
}

// DefaultSupervisorConfig returns production defaults.
func DefaultSupervisorConfig() SupervisorConfig {
	return SupervisorConfig{
		AggregationStrategy: StrategyMergeAll,
		FailurePolicy:       PolicyPartialResult,
		MaxRetries:          2,
		TaskTimeout:         5 * time.Minute,
	}
}

// Supervisor orchestrates task splitting, parallel worker execution, and result aggregation.
type Supervisor struct {
	splitter   TaskSplitter
	pool       *WorkerPool
	aggregator *Aggregator
	logger     *zap.Logger
}

// NewSupervisor creates a Supervisor with the given configuration.
func NewSupervisor(splitter TaskSplitter, cfg SupervisorConfig, logger *zap.Logger) *Supervisor {
	if logger == nil {
		logger = zap.NewNop()
	}
	poolCfg := WorkerPoolConfig{
		FailurePolicy: cfg.FailurePolicy,
		MaxRetries:    cfg.MaxRetries,
		TaskTimeout:   cfg.TaskTimeout,
	}
	return &Supervisor{
		splitter:   splitter,
		pool:       NewWorkerPool(poolCfg, logger),
		aggregator: NewAggregator(cfg.AggregationStrategy),
		logger:     logger.With(zap.String("component", "supervisor")),
	}
}

// Run executes the full supervisor pipeline: split -> dispatch -> aggregate.
func (s *Supervisor) Run(ctx context.Context, input *agent.Input) (*AggregatedResult, error) {
	start := time.Now()

	// Ensure run_id is set on the supervisor context
	if _, ok := types.RunID(ctx); !ok {
		ctx = types.WithRunID(ctx, fmt.Sprintf("sup_%d", start.UnixNano()))
	}

	s.logger.Info("supervisor starting",
		zap.String("trace_id", input.TraceID),
	)

	// 1. Split
	tasks, err := s.splitter.Split(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("task split failed: %w", err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("task splitter returned zero tasks")
	}

	s.logger.Info("tasks split",
		zap.Int("count", len(tasks)),
	)

	// 2. Dispatch to worker pool
	results, err := s.pool.Execute(ctx, tasks)
	if err != nil {
		return nil, fmt.Errorf("worker pool execution failed: %w", err)
	}

	// 3. Aggregate
	agg, err := s.aggregator.Aggregate(results)
	if err != nil {
		return nil, fmt.Errorf("aggregation failed: %w", err)
	}

	agg.Duration = time.Since(start)
	s.logger.Info("supervisor completed",
		zap.Int("source_count", agg.SourceCount),
		zap.Int("failed_count", agg.FailedCount),
		zap.Duration("duration", agg.Duration),
	)

	return agg, nil
}

// StaticSplitter is a simple TaskSplitter that dispatches the same input to a fixed set of agents.
type StaticSplitter struct {
	Agents  []agent.Agent
	Weights map[string]float64 // agent_id -> weight (optional)
}

// Split returns one WorkerTask per agent, all sharing the same input.
func (s *StaticSplitter) Split(_ context.Context, input *agent.Input) ([]WorkerTask, error) {
	tasks := make([]WorkerTask, len(s.Agents))
	for i, ag := range s.Agents {
		w := 1.0
		if s.Weights != nil {
			if ww, ok := s.Weights[ag.ID()]; ok {
				w = ww
			}
		}
		tasks[i] = WorkerTask{
			AgentID: ag.ID(),
			Agent:   ag,
			Input:   input,
			Weight:  w,
		}
	}
	return tasks, nil
}
