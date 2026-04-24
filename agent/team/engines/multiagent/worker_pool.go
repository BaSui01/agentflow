package multiagent

import (
	"context"
	"sync"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/types"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// FailurePolicy controls how the pool handles sub-agent failures.
type FailurePolicy string

const (
	PolicyFailFast      FailurePolicy = "fail_fast"
	PolicyPartialResult FailurePolicy = "partial_result"
	PolicyRetryFailed   FailurePolicy = "retry_failed"
)

// WorkerPoolConfig configures the worker pool.
type WorkerPoolConfig struct {
	FailurePolicy FailurePolicy
	MaxRetries    int
	TaskTimeout   time.Duration
}

// DefaultWorkerPoolConfig returns production defaults (PartialResult).
func DefaultWorkerPoolConfig() WorkerPoolConfig {
	return WorkerPoolConfig{
		FailurePolicy: PolicyPartialResult,
		MaxRetries:    2,
		TaskTimeout:   5 * time.Minute,
	}
}

// WorkerTask is a unit of work dispatched to a sub-agent.
type WorkerTask struct {
	AgentID   string
	Agent     agent.Agent
	Input     *agent.Input
	Weight    float64  // used by WeightedMerge aggregation
	ToolScope []string // allowed tool names (empty = all tools)
}

// WorkerPool manages concurrent sub-agent execution.
type WorkerPool struct {
	config WorkerPoolConfig
	logger *zap.Logger
}

// NewWorkerPool creates a WorkerPool.
func NewWorkerPool(cfg WorkerPoolConfig, logger *zap.Logger) *WorkerPool {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WorkerPool{config: cfg, logger: logger}
}

// Execute dispatches all tasks concurrently and collects results according to FailurePolicy.
func (p *WorkerPool) Execute(ctx context.Context, tasks []WorkerTask) ([]WorkerResult, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	results := make([]WorkerResult, len(tasks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	failFastCtx, failFastCancel := context.WithCancel(ctx)
	defer failFastCancel()

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t WorkerTask) {
			defer wg.Done()
			r := p.executeOne(failFastCtx, t)
			mu.Lock()
			results[idx] = r
			if r.Err != nil && p.config.FailurePolicy == PolicyFailFast && firstErr == nil {
				firstErr = r.Err
				failFastCancel()
			}
			mu.Unlock()
		}(i, task)
	}

	wg.Wait()

	if p.config.FailurePolicy == PolicyFailFast && firstErr != nil {
		return results, firstErr
	}

	// RetryFailed: retry failed tasks up to MaxRetries
	if p.config.FailurePolicy == PolicyRetryFailed {
		for retry := 0; retry < p.config.MaxRetries; retry++ {
			anyFailed := false
			for i := range results {
				if results[i].Err == nil {
					continue
				}
				anyFailed = true
				p.logger.Info("retrying failed worker",
					zap.String("agent_id", tasks[i].AgentID),
					zap.Int("retry", retry+1),
				)
				r := p.executeOne(ctx, tasks[i])
				results[i] = r
			}
			if !anyFailed {
				break
			}
		}
	}

	return results, nil
}

func (p *WorkerPool) executeOne(ctx context.Context, task WorkerTask) WorkerResult {
	// Build isolated child context
	childCtx := ctx
	if parentRunID, ok := types.RunID(ctx); ok {
		childCtx = types.WithParentRunID(childCtx, parentRunID)
	}
	childRunID := "run_" + uuid.New().String()
	childCtx = types.WithRunID(childCtx, childRunID)

	// Trace context isolation: share trace_id, independent span_id
	childCtx = types.WithSpanID(childCtx, "span_"+uuid.New().String())
	childCtx = types.WithAgentID(childCtx, task.AgentID)

	if p.config.TaskTimeout > 0 {
		var cancel context.CancelFunc
		childCtx, cancel = context.WithTimeout(childCtx, p.config.TaskTimeout)
		defer cancel()
	}

	start := time.Now()
	output, err := task.Agent.Execute(childCtx, task.Input)
	dur := time.Since(start)

	r := WorkerResult{
		AgentID:  task.AgentID,
		Duration: dur,
		Weight:   task.Weight,
		Err:      err,
	}
	if output != nil {
		r.Content = output.Content
		r.TokensUsed = output.TokensUsed
		r.Cost = output.Cost
		r.FinishReason = output.FinishReason
		r.Metadata = output.Metadata
	}
	return r
}
