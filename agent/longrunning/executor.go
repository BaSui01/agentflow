package longrunning

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ExecutionState is the state of a long-running execution.
type ExecutionState string

const (
	StateInitialized ExecutionState = "initialized"
	StateRunning     ExecutionState = "running"
	StatePaused      ExecutionState = "paused"
	StateResuming    ExecutionState = "resuming"
	StateCompleted   ExecutionState = "completed"
	StateFailed      ExecutionState = "failed"
	StateCancelled   ExecutionState = "cancelled"
)

// Execution is a long-running agent execution instance.
type Execution struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	State       ExecutionState `json:"state"`
	Progress    float64        `json:"progress"`
	CurrentStep int            `json:"current_step"`
	TotalSteps  int            `json:"total_steps"`
	StepNames   []string       `json:"step_names,omitempty"`
	StartTime   time.Time      `json:"start_time"`
	LastUpdate  time.Time      `json:"last_update"`
	EndTime     *time.Time     `json:"end_time,omitempty"`
	Checkpoints []Checkpoint   `json:"checkpoints"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Error       string         `json:"error,omitempty"`

	mu sync.Mutex `json:"-"` // protects concurrent checkpoint writes
}

// Checkpoint is a recoverable checkpoint snapshot.
type Checkpoint struct {
	ID        string         `json:"id"`
	Step      int            `json:"step"`
	State     any            `json:"state"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ExecutorConfig configures the long-running executor.
type ExecutorConfig struct {
	CheckpointInterval time.Duration `json:"checkpoint_interval"`
	CheckpointDir      string        `json:"checkpoint_dir"`
	MaxRetries         int           `json:"max_retries"`
	HeartbeatInterval  time.Duration `json:"heartbeat_interval"`
	AutoResume         bool          `json:"auto_resume"`
}

// DefaultExecutorConfig returns the default configuration.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		CheckpointInterval: 5 * time.Minute,
		CheckpointDir:      "./checkpoints",
		MaxRetries:         3,
		HeartbeatInterval:  30 * time.Second,
		AutoResume:         true,
	}
}

// StepFunc represents a single step in a long-running execution.
type StepFunc func(ctx context.Context, state any) (any, error)

// NamedStep pairs a step function with a name for checkpoint-based resume.
type NamedStep struct {
	Name string
	Func StepFunc
}

// EventType identifies the kind of execution event.
type EventType string

const (
	EventStepStarted   EventType = "step_started"
	EventStepCompleted EventType = "step_completed"
	EventStepFailed    EventType = "step_failed"
	EventCheckpointed  EventType = "checkpointed"
	EventPaused        EventType = "paused"
	EventResumed       EventType = "resumed"
)

// ExecutionEvent is emitted during execution lifecycle transitions.
type ExecutionEvent struct {
	Type      EventType
	ExecID    string
	Step      int
	Timestamp time.Time
	Error     error
	State     any
}

// StepRegistry allows named step registration for resume capability.
type StepRegistry struct {
	steps map[string]StepFunc
	mu    sync.RWMutex
}

// NewStepRegistry creates a new step registry.
func NewStepRegistry() *StepRegistry {
	return &StepRegistry{
		steps: make(map[string]StepFunc),
	}
}

// Register adds a named step function to the registry.
func (r *StepRegistry) Register(name string, fn StepFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps[name] = fn
}

// Get retrieves a step function by name.
func (r *StepRegistry) Get(name string) (StepFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.steps[name]
	return fn, ok
}

// ExecutorOption configures the Executor.
type ExecutorOption func(*Executor)

// WithCheckpointStore sets a custom CheckpointStore on the Executor.
func WithCheckpointStore(store CheckpointStore) ExecutorOption {
	return func(e *Executor) {
		e.checkpointStore = store
	}
}

// Executor manages long-running agent executions.
type Executor struct {
	config          ExecutorConfig
	executions      map[string]*Execution
	steps           map[string][]StepFunc
	namedSteps      map[string][]NamedStep
	pauseCh         map[string]chan struct{}
	resumeCh        map[string]chan struct{}
	registry        *StepRegistry
	checkpointStore CheckpointStore
	OnEvent         func(ExecutionEvent)
	logger          *zap.Logger
	mu              sync.RWMutex
}

// NewExecutor creates a new long-running executor.
func NewExecutor(config ExecutorConfig, logger *zap.Logger, opts ...ExecutorOption) *Executor {
	if logger == nil {
		logger = zap.NewNop()
	}
	os.MkdirAll(config.CheckpointDir, 0755)

	e := &Executor{
		config:     config,
		executions: make(map[string]*Execution),
		steps:      make(map[string][]StepFunc),
		namedSteps: make(map[string][]NamedStep),
		pauseCh:    make(map[string]chan struct{}),
		resumeCh:   make(map[string]chan struct{}),
		registry:   NewStepRegistry(),
		logger:     logger.With(zap.String("component", "longrunning")),
	}

	for _, opt := range opts {
		opt(e)
	}

	// Default to filesystem-based checkpoint store if none provided.
	if e.checkpointStore == nil {
		e.checkpointStore = NewFileCheckpointStore(config.CheckpointDir, e.logger)
	}

	return e
}

// Registry returns the executor's step registry.
func (e *Executor) Registry() *StepRegistry {
	return e.registry
}

// generateID produces a unique execution ID using crypto/rand.
func generateID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails (extremely unlikely).
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// emitEvent sends an event to the OnEvent callback if set.
func (e *Executor) emitEvent(evt ExecutionEvent) {
	if e.OnEvent != nil {
		e.OnEvent(evt)
	}
}

// CreateExecution creates a new long-running execution with anonymous steps.
func (e *Executor) CreateExecution(name string, steps []StepFunc) *Execution {
	exec := &Execution{
		ID:          generateID("exec"),
		Name:        name,
		State:       StateInitialized,
		Progress:    0,
		CurrentStep: 0,
		TotalSteps:  len(steps),
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
		Checkpoints: make([]Checkpoint, 0),
		Metadata:    make(map[string]any),
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.steps[exec.ID] = steps
	e.pauseCh[exec.ID] = make(chan struct{}, 1)
	e.resumeCh[exec.ID] = make(chan struct{})
	e.mu.Unlock()

	e.logger.Info("execution created",
		zap.String("id", exec.ID),
		zap.String("name", name),
		zap.Int("steps", len(steps)),
	)

	return exec
}

// CreateNamedExecution creates a new execution with named steps for resume capability.
func (e *Executor) CreateNamedExecution(name string, steps []NamedStep) *Execution {
	stepNames := make([]string, len(steps))
	stepFuncs := make([]StepFunc, len(steps))
	for i, s := range steps {
		stepNames[i] = s.Name
		stepFuncs[i] = s.Func
		e.registry.Register(s.Name, s.Func)
	}

	exec := &Execution{
		ID:          generateID("exec"),
		Name:        name,
		State:       StateInitialized,
		Progress:    0,
		CurrentStep: 0,
		TotalSteps:  len(steps),
		StepNames:   stepNames,
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
		Checkpoints: make([]Checkpoint, 0),
		Metadata:    make(map[string]any),
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.steps[exec.ID] = stepFuncs
	e.namedSteps[exec.ID] = steps
	e.pauseCh[exec.ID] = make(chan struct{}, 1)
	e.resumeCh[exec.ID] = make(chan struct{})
	e.mu.Unlock()

	e.logger.Info("named execution created",
		zap.String("id", exec.ID),
		zap.String("name", name),
		zap.Int("steps", len(steps)),
		zap.Strings("step_names", stepNames),
	)

	return exec
}

// Start begins a long-running execution.
func (e *Executor) Start(ctx context.Context, execID string, initialState any) error {
	e.mu.RLock()
	exec, ok := e.executions[execID]
	steps := e.steps[execID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	exec.mu.Lock()
	exec.State = StateRunning
	exec.LastUpdate = time.Now()
	exec.mu.Unlock()

	go e.runExecution(ctx, exec, steps, initialState)

	return nil
}

func (e *Executor) runExecution(ctx context.Context, exec *Execution, steps []StepFunc, state any) {
	checkpointTicker := time.NewTicker(e.config.CheckpointInterval)
	heartbeatTicker := time.NewTicker(e.config.HeartbeatInterval)
	defer checkpointTicker.Stop()
	defer heartbeatTicker.Stop()

	e.mu.RLock()
	pauseCh := e.pauseCh[exec.ID]
	resumeCh := e.resumeCh[exec.ID]
	e.mu.RUnlock()

	currentState := state

	for exec.CurrentStep < exec.TotalSteps {
		// Check context cancellation first.
		select {
		case <-ctx.Done():
			exec.mu.Lock()
			exec.State = StateCancelled
			exec.mu.Unlock()
			e.saveCheckpoint(exec, currentState)
			return
		default:
		}

		// Drain tickers without blocking.
		e.drainTickers(exec, checkpointTicker, heartbeatTicker, currentState)

		// Check for pause signal (channel-based, not busy-wait).
		select {
		case <-pauseCh:
			exec.mu.Lock()
			exec.State = StatePaused
			exec.mu.Unlock()
			e.saveCheckpoint(exec, currentState)
			e.emitEvent(ExecutionEvent{
				Type: EventPaused, ExecID: exec.ID,
				Step: exec.CurrentStep, Timestamp: time.Now(), State: currentState,
			})
			// Block until resume signal or context cancellation.
			select {
			case <-resumeCh:
				// Allocate new channels for next pause/resume cycle.
				e.mu.Lock()
				e.pauseCh[exec.ID] = make(chan struct{}, 1)
				e.resumeCh[exec.ID] = make(chan struct{})
				pauseCh = e.pauseCh[exec.ID]
				resumeCh = e.resumeCh[exec.ID]
				e.mu.Unlock()

				exec.mu.Lock()
				exec.State = StateRunning
				exec.mu.Unlock()
				e.emitEvent(ExecutionEvent{
					Type: EventResumed, ExecID: exec.ID,
					Step: exec.CurrentStep, Timestamp: time.Now(), State: currentState,
				})
			case <-ctx.Done():
				exec.mu.Lock()
				exec.State = StateCancelled
				exec.mu.Unlock()
				e.saveCheckpoint(exec, currentState)
				return
			}
		default:
		}

		// Execute step with retry and exponential backoff.
		step := steps[exec.CurrentStep]
		e.emitEvent(ExecutionEvent{
			Type: EventStepStarted, ExecID: exec.ID,
			Step: exec.CurrentStep, Timestamp: time.Now(), State: currentState,
		})

		var err error
		var result any

		for retry := 0; retry <= e.config.MaxRetries; retry++ {
			result, err = step(ctx, currentState)
			if err == nil {
				break
			}
			e.logger.Warn("step failed, retrying",
				zap.String("exec_id", exec.ID),
				zap.Int("step", exec.CurrentStep),
				zap.Int("retry", retry),
				zap.Error(err),
			)
			e.emitEvent(ExecutionEvent{
				Type: EventStepFailed, ExecID: exec.ID,
				Step: exec.CurrentStep, Timestamp: time.Now(), Error: err, State: currentState,
			})
			if retry < e.config.MaxRetries {
				backoff := time.Duration(1<<uint(retry)) * time.Second
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					exec.mu.Lock()
					exec.State = StateCancelled
					exec.mu.Unlock()
					e.saveCheckpoint(exec, currentState)
					return
				}
			}
		}

		if err != nil {
			exec.mu.Lock()
			exec.State = StateFailed
			exec.Error = err.Error()
			now := time.Now()
			exec.EndTime = &now
			exec.mu.Unlock()
			e.saveCheckpoint(exec, currentState)
			e.logger.Error("execution failed",
				zap.String("exec_id", exec.ID),
				zap.Int("step", exec.CurrentStep),
				zap.Error(err),
			)
			return
		}

		currentState = result
		exec.mu.Lock()
		exec.CurrentStep++
		exec.Progress = float64(exec.CurrentStep) / float64(exec.TotalSteps) * 100
		exec.LastUpdate = time.Now()
		exec.mu.Unlock()

		e.emitEvent(ExecutionEvent{
			Type: EventStepCompleted, ExecID: exec.ID,
			Step: exec.CurrentStep - 1, Timestamp: time.Now(), State: currentState,
		})

		e.logger.Debug("step completed",
			zap.String("exec_id", exec.ID),
			zap.Int("step", exec.CurrentStep),
			zap.Float64("progress", exec.Progress),
		)
	}

	exec.mu.Lock()
	exec.State = StateCompleted
	exec.Progress = 100
	now := time.Now()
	exec.EndTime = &now
	exec.mu.Unlock()
	e.saveCheckpoint(exec, currentState)

	e.logger.Info("execution completed",
		zap.String("exec_id", exec.ID),
		zap.Duration("duration", exec.EndTime.Sub(exec.StartTime)),
	)
}

// saveCheckpoint persists execution state via the CheckpointStore.
func (e *Executor) saveCheckpoint(exec *Execution, state any) {
	exec.mu.Lock()
	checkpoint := Checkpoint{
		ID:        generateID("cp"),
		Step:      exec.CurrentStep,
		State:     state,
		Timestamp: time.Now(),
	}
	exec.Checkpoints = append(exec.Checkpoints, checkpoint)
	exec.mu.Unlock()

	e.emitEvent(ExecutionEvent{
		Type: EventCheckpointed, ExecID: exec.ID,
		Step: checkpoint.Step, Timestamp: checkpoint.Timestamp, State: state,
	})

	if err := e.checkpointStore.SaveCheckpoint(context.Background(), exec); err != nil {
		e.logger.Error("failed to save checkpoint", zap.Error(err))
	}
}

// Pause signals a running execution to pause.
func (e *Executor) Pause(execID string) error {
	e.mu.RLock()
	exec, ok := e.executions[execID]
	pauseCh, hasCh := e.pauseCh[execID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	exec.mu.Lock()
	state := exec.State
	exec.mu.Unlock()

	if state != StateRunning {
		return fmt.Errorf("execution not running: %s", state)
	}

	if hasCh {
		select {
		case pauseCh <- struct{}{}:
		default:
			// Already signaled.
		}
	}

	e.logger.Info("execution pause signaled", zap.String("exec_id", execID))
	return nil
}

// Resume signals a paused execution to continue.
func (e *Executor) Resume(execID string) error {
	e.mu.RLock()
	exec, ok := e.executions[execID]
	resumeCh, hasCh := e.resumeCh[execID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	exec.mu.Lock()
	state := exec.State
	exec.mu.Unlock()

	if state != StatePaused {
		return fmt.Errorf("execution not paused: %s", state)
	}

	if hasCh {
		// Close the channel to unblock the waiting goroutine.
		select {
		case <-resumeCh:
			// Already closed.
		default:
			close(resumeCh)
		}
	}

	e.logger.Info("execution resume signaled", zap.String("exec_id", execID))
	return nil
}

// LoadExecution loads an execution from the checkpoint store.
func (e *Executor) LoadExecution(execID string) (*Execution, error) {
	exec, err := e.checkpointStore.LoadCheckpoint(context.Background(), execID)
	if err != nil {
		return nil, fmt.Errorf("loading checkpoint: %w", err)
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.pauseCh[exec.ID] = make(chan struct{}, 1)
	e.resumeCh[exec.ID] = make(chan struct{})
	e.mu.Unlock()

	return exec, nil
}

// ResumeExecution resumes a loaded execution from its last checkpoint using the step registry.
func (e *Executor) ResumeExecution(ctx context.Context, execID string, lastState any) error {
	e.mu.RLock()
	exec, ok := e.executions[execID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	if len(exec.StepNames) == 0 {
		return fmt.Errorf("execution %s has no step names, cannot resume by name", execID)
	}

	// Rebuild step functions from registry.
	steps := make([]StepFunc, len(exec.StepNames))
	for i, name := range exec.StepNames {
		fn, found := e.registry.Get(name)
		if !found {
			return fmt.Errorf("step %q not found in registry", name)
		}
		steps[i] = fn
	}

	e.mu.Lock()
	e.steps[execID] = steps
	e.mu.Unlock()

	exec.mu.Lock()
	exec.State = StateResuming
	exec.LastUpdate = time.Now()
	exec.mu.Unlock()

	e.logger.Info("resuming execution from checkpoint",
		zap.String("exec_id", execID),
		zap.Int("from_step", exec.CurrentStep),
		zap.Int("total_steps", exec.TotalSteps),
	)

	exec.mu.Lock()
	exec.State = StateRunning
	exec.mu.Unlock()

	go e.runExecution(ctx, exec, steps, lastState)

	return nil
}

// GetExecution retrieves an execution by ID.
func (e *Executor) GetExecution(execID string) (*Execution, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	exec, ok := e.executions[execID]
	return exec, ok
}

// ListExecutions returns all tracked executions.
func (e *Executor) ListExecutions() []*Execution {
	e.mu.RLock()
	defer e.mu.RUnlock()
	execs := make([]*Execution, 0, len(e.executions))
	for _, exec := range e.executions {
		execs = append(execs, exec)
	}
	return execs
}

// AutoResumeAll loads all checkpoints from the store and resumes any that are
// resumable (running, paused, or resuming) and have named steps registered in
// the step registry.
func (e *Executor) AutoResumeAll(ctx context.Context) (int, error) {
	execs, err := e.checkpointStore.ListCheckpoints(ctx)
	if err != nil {
		return 0, fmt.Errorf("listing checkpoints: %w", err)
	}

	resumed := 0
	for _, exec := range execs {
		if !isResumableState(exec.State) {
			continue
		}
		if len(exec.StepNames) == 0 {
			e.logger.Warn("skipping execution without step names",
				zap.String("exec_id", exec.ID))
			continue
		}

		// Check all steps are registered.
		allFound := true
		for _, name := range exec.StepNames {
			if _, ok := e.registry.Get(name); !ok {
				e.logger.Warn("skipping execution with unregistered step",
					zap.String("exec_id", exec.ID), zap.String("step", name))
				allFound = false
				break
			}
		}
		if !allFound {
			continue
		}

		// Register the execution in the executor.
		e.mu.Lock()
		e.executions[exec.ID] = exec
		e.pauseCh[exec.ID] = make(chan struct{}, 1)
		e.resumeCh[exec.ID] = make(chan struct{})
		e.mu.Unlock()

		// Determine last state from checkpoints.
		var lastState any
		if len(exec.Checkpoints) > 0 {
			lastState = exec.Checkpoints[len(exec.Checkpoints)-1].State
		}

		if err := e.ResumeExecution(ctx, exec.ID, lastState); err != nil {
			e.logger.Error("failed to auto-resume execution",
				zap.String("exec_id", exec.ID), zap.Error(err))
			continue
		}
		resumed++
	}

	return resumed, nil
}

// isResumableState returns true if the execution state can be resumed.
func isResumableState(state ExecutionState) bool {
	switch state {
	case StateRunning, StatePaused, StateResuming:
		return true
	default:
		return false
	}
}

// drainTickers processes any pending checkpoint/heartbeat ticks.
func (e *Executor) drainTickers(exec *Execution, cpTicker, hbTicker *time.Ticker, state any) {
	for {
		select {
		case <-cpTicker.C:
			e.saveCheckpoint(exec, state)
		case <-hbTicker.C:
			exec.mu.Lock()
			exec.LastUpdate = time.Now()
			exec.mu.Unlock()
		default:
			return
		}
	}
}
