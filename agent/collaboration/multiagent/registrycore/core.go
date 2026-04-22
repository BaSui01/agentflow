package registrycore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ExecutionResult packages the outcome of an asynchronous execution.
type ExecutionResult[TOutput any] struct {
	Output *TOutput
	Err    error
}

// ExecutionCallbacks controls how callers observe and persist execution state.
type ExecutionCallbacks[TExec any, TOutput any] struct {
	SetCompleted func(TExec, *TOutput)
	SetFailed    func(TExec, error)
	NotifyDone   func(TExec, ExecutionResult[TOutput])
}

func (c ExecutionCallbacks[TExec, TOutput]) setCompleted(exec TExec, output *TOutput) {
	if c.SetCompleted != nil {
		c.SetCompleted(exec, output)
	}
}

func (c ExecutionCallbacks[TExec, TOutput]) setFailed(exec TExec, err error) {
	if c.SetFailed != nil {
		c.SetFailed(exec, err)
	}
}

func (c ExecutionCallbacks[TExec, TOutput]) notifyDone(exec TExec, result ExecutionResult[TOutput]) {
	if c.NotifyDone != nil {
		c.NotifyDone(exec, result)
	}
}

// ExecutionRunner starts a background execution using caller-provided lifecycle hooks.
type ExecutionRunner[TInput any, TOutput any, TAgent any, TExec any] struct {
	Context      context.Context
	Agent        TAgent
	Input        *TInput
	Exec         TExec
	ExecutionID  string
	AgentID      string
	Logger       *zap.Logger
	TracerName   string
	SpanName     string
	PanicMessage string
	Execute      func(context.Context, TAgent, *TInput) (*TOutput, error)
	Callbacks    ExecutionCallbacks[TExec, TOutput]
}

// RunExecution executes the configured task in a background goroutine.
func RunExecution[TInput any, TOutput any, TAgent any, TExec any](cfg ExecutionRunner[TInput, TOutput, TAgent, TExec]) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	go func(ctx context.Context) {
		if cfg.TracerName != "" && cfg.SpanName != "" {
			var span trace.Span
			ctx, span = otel.Tracer(cfg.TracerName).Start(ctx, cfg.SpanName)
			defer span.End()
		}

		defer func() {
			if r := recover(); r != nil {
				panicErr := fmt.Errorf("%s: %v", cfg.PanicMessage, r)
				logger.Error(cfg.PanicMessage,
					zap.String("execution_id", cfg.ExecutionID),
					zap.String("agent_id", cfg.AgentID),
					zap.Any("recover", r),
					zap.Stack("stack"),
				)
				cfg.Callbacks.setFailed(cfg.Exec, panicErr)
				cfg.Callbacks.notifyDone(cfg.Exec, ExecutionResult[TOutput]{Err: panicErr})
			}
		}()

		select {
		case <-ctx.Done():
			cfg.Callbacks.setFailed(cfg.Exec, ctx.Err())
			cfg.Callbacks.notifyDone(cfg.Exec, ExecutionResult[TOutput]{Err: ctx.Err()})
			return
		default:
		}

		output, err := cfg.Execute(ctx, cfg.Agent, cfg.Input)
		if err != nil {
			cfg.Callbacks.setFailed(cfg.Exec, err)
		} else {
			cfg.Callbacks.setCompleted(cfg.Exec, output)
		}
		cfg.Callbacks.notifyDone(cfg.Exec, ExecutionResult[TOutput]{Output: output, Err: err})
	}(cfg.Context)
}

// GenerateExecutionID creates a distributed-unique execution ID.
func GenerateExecutionID() string {
	return "exec_" + uuid.New().String()
}

// ManagerConfig configures the generic subagent execution manager.
type ManagerConfig[TInput any, TOutput any, TAgent interface {
	ID() string
	Execute(context.Context, *TInput) (*TOutput, error)
}, TExec any, TStatus comparable] struct {
	Logger              *zap.Logger
	Component           string
	AutoCleanupInterval time.Duration
	AutoCleanupTTL      time.Duration
	NewExecutionID      func() string
	CloneInput          func(*TInput) *TInput
	PrepareContext      func(context.Context, string) context.Context
	NewExecution        func(string, string, *TInput) TExec
	Callbacks           ExecutionCallbacks[TExec, TOutput]
	GetStatus           func(TExec) TStatus
	GetEndTime          func(TExec) time.Time
	GetID               func(TExec) string
	CompletedStatuses   []TStatus
}

// SubagentManager is a generic execution tracker with cleanup support.
type SubagentManager[TInput any, TOutput any, TAgent interface {
	ID() string
	Execute(context.Context, *TInput) (*TOutput, error)
}, TExec any, TStatus comparable] struct {
	mu         sync.RWMutex
	executions map[string]TExec
	logger     *zap.Logger
	closeCh    chan struct{}
	cfg        ManagerConfig[TInput, TOutput, TAgent, TExec, TStatus]
}

// NewSubagentManager constructs a manager and starts its cleanup loop.
func NewSubagentManager[TInput any, TOutput any, TAgent interface {
	ID() string
	Execute(context.Context, *TInput) (*TOutput, error)
}, TExec any, TStatus comparable](cfg ManagerConfig[TInput, TOutput, TAgent, TExec, TStatus]) *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus] {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.Component == "" {
		cfg.Component = "subagent_manager"
	}
	if cfg.AutoCleanupInterval <= 0 {
		cfg.AutoCleanupInterval = 5 * time.Minute
	}
	if cfg.AutoCleanupTTL <= 0 {
		cfg.AutoCleanupTTL = 10 * time.Minute
	}
	if cfg.NewExecutionID == nil {
		cfg.NewExecutionID = GenerateExecutionID
	}

	m := &SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]{
		executions: make(map[string]TExec),
		logger:     logger.With(zap.String("component", cfg.Component)),
		closeCh:    make(chan struct{}),
		cfg:        cfg,
	}
	go m.autoCleanupLoop()
	return m
}

func (m *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]) autoCleanupLoop() {
	ticker := time.NewTicker(m.cfg.AutoCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cleaned := m.CleanupCompleted(m.cfg.AutoCleanupTTL)
			if cleaned > 0 {
				m.logger.Info("auto cleanup completed executions", zap.Int("count", cleaned))
			}
		case <-m.closeCh:
			return
		}
	}
}

// Close stops the background cleanup loop.
func (m *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]) Close() {
	if m == nil {
		return
	}
	select {
	case <-m.closeCh:
	default:
		close(m.closeCh)
	}
}

// SpawnSubagent clones input, stores the execution, and starts background work.
func (m *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]) SpawnSubagent(ctx context.Context, subagent TAgent, input *TInput) (TExec, error) {
	inputCopy := input
	if m.cfg.CloneInput != nil {
		inputCopy = m.cfg.CloneInput(input)
	}

	executionID := m.cfg.NewExecutionID()
	exec := m.cfg.NewExecution(executionID, subagent.ID(), inputCopy)

	m.mu.Lock()
	m.executions[m.cfg.GetID(exec)] = exec
	m.mu.Unlock()

	childCtx := ctx
	if m.cfg.PrepareContext != nil {
		childCtx = m.cfg.PrepareContext(ctx, executionID)
	}

	m.logger.Debug("spawning subagent",
		zap.String("execution_id", executionID),
		zap.String("subagent_id", subagent.ID()),
	)

	RunExecution(ExecutionRunner[TInput, TOutput, TAgent, TExec]{
		Context:      childCtx,
		Agent:        subagent,
		Input:        inputCopy,
		Exec:         exec,
		ExecutionID:  executionID,
		AgentID:      subagent.ID(),
		Logger:       m.logger,
		PanicMessage: "subagent execution panicked",
		Execute: func(execCtx context.Context, agent TAgent, execInput *TInput) (*TOutput, error) {
			return agent.Execute(execCtx, execInput)
		},
		Callbacks: m.cfg.Callbacks,
	})

	return exec, nil
}

// GetExecution fetches a tracked execution by ID.
func (m *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]) GetExecution(executionID string) (TExec, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	exec, ok := m.executions[executionID]
	if !ok {
		var zero TExec
		return zero, fmt.Errorf("execution not found: %s", executionID)
	}

	return exec, nil
}

// ListExecutions returns a snapshot of currently tracked executions.
func (m *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]) ListExecutions() []TExec {
	m.mu.RLock()
	defer m.mu.RUnlock()

	executions := make([]TExec, 0, len(m.executions))
	for _, exec := range m.executions {
		executions = append(executions, exec)
	}

	return executions
}

// CleanupCompleted removes completed executions older than the given threshold.
func (m *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]) CleanupCompleted(olderThan time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	cleaned := 0

	for id, exec := range m.executions {
		if !m.isCompleted(m.cfg.GetStatus(exec)) {
			continue
		}

		endTime := m.cfg.GetEndTime(exec)
		if olderThan <= 0 || endTime.Before(cutoff) || endTime.Equal(cutoff) {
			delete(m.executions, id)
			cleaned++
		}
	}

	m.logger.Debug("cleaned up completed executions", zap.Int("count", cleaned))
	return cleaned
}

func (m *SubagentManager[TInput, TOutput, TAgent, TExec, TStatus]) isCompleted(status TStatus) bool {
	for _, completed := range m.cfg.CompletedStatuses {
		if status == completed {
			return true
		}
	}
	return false
}

// ParallelExecutionConfig configures a spawn-and-wait fanout workflow.
type ParallelExecutionConfig[TInput any, TOutput any, TAgent any, TExec any] struct {
	Context                   context.Context
	Input                     *TInput
	Subagents                 []TAgent
	Spawn                     func(context.Context, TAgent, *TInput) (TExec, error)
	Wait                      func(TExec, context.Context) (*TOutput, error)
	OnSpawnError              func(TAgent, error)
	OnWaitError               func(TExec, error)
	OnSuccess                 func(TExec, *TOutput)
	IgnoreContextCancellation bool
}

// CollectParallelResults fans out subagent executions and compacts successful outputs.
func CollectParallelResults[TInput any, TOutput any, TAgent any, TExec any](cfg ParallelExecutionConfig[TInput, TOutput, TAgent, TExec]) ([]*TOutput, error) {
	type slot struct {
		exec  TExec
		valid bool
	}

	executions := make([]slot, len(cfg.Subagents))
	for i, subagent := range cfg.Subagents {
		exec, err := cfg.Spawn(cfg.Context, subagent, cfg.Input)
		if err != nil {
			if cfg.OnSpawnError != nil {
				cfg.OnSpawnError(subagent, err)
			}
			continue
		}
		executions[i] = slot{exec: exec, valid: true}
	}

	results := make([]*TOutput, len(executions))
	var wg sync.WaitGroup

	for i, execution := range executions {
		if !execution.valid {
			continue
		}

		wg.Add(1)
		go func(index int, exec TExec) {
			defer wg.Done()

			output, err := cfg.Wait(exec, cfg.Context)
			if err != nil {
				if cfg.Context != nil && cfg.Context.Err() != nil {
					return
				}
				if cfg.OnWaitError != nil {
					cfg.OnWaitError(exec, err)
				}
				return
			}

			results[index] = output
			if cfg.OnSuccess != nil {
				cfg.OnSuccess(exec, output)
			}
		}(i, execution.exec)
	}

	wg.Wait()

	if !cfg.IgnoreContextCancellation && cfg.Context != nil && cfg.Context.Err() != nil {
		return nil, cfg.Context.Err()
	}

	compacted := make([]*TOutput, 0, len(results))
	for _, result := range results {
		if result != nil {
			compacted = append(compacted, result)
		}
	}
	if len(compacted) == 0 {
		return nil, fmt.Errorf("all subagents failed")
	}

	return compacted, nil
}
