package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ExecutionState is the state of a long-running execution.
type ExecutionState string

const (
	ExecutionStateInitialized ExecutionState = "initialized"
	ExecutionStateRunning     ExecutionState = "running"
	ExecutionStatePaused      ExecutionState = "paused"
	ExecutionStateResuming    ExecutionState = "resuming"
	ExecutionStateCompleted   ExecutionState = "completed"
	ExecutionStateFailed      ExecutionState = "failed"
	ExecutionStateCancelled   ExecutionState = "cancelled"
)

// Execution is a long-running agent execution instance.
type Execution struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	State       ExecutionState        `json:"state"`
	Progress    float64               `json:"progress"`
	CurrentStep int                   `json:"current_step"`
	TotalSteps  int                   `json:"total_steps"`
	StepNames   []string              `json:"step_names,omitempty"`
	StartTime   time.Time             `json:"start_time"`
	LastUpdate  time.Time             `json:"last_update"`
	EndTime     *time.Time            `json:"end_time,omitempty"`
	Checkpoints []ExecutionCheckpoint `json:"checkpoints"`
	Metadata    map[string]any        `json:"metadata,omitempty"`
	Error       string                `json:"error,omitempty"`

	mu sync.Mutex `json:"-"` // protects concurrent ExecutionCheckpoint writes
}

// ExecutionCheckpoint is a recoverable ExecutionCheckpoint snapshot.
type ExecutionCheckpoint struct {
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

// NamedStep pairs a step function with a name for ExecutionCheckpoint-based resume.
type NamedStep struct {
	Name string
	Func StepFunc
}

// ExecutionEventType identifies the kind of execution event.
type ExecutionEventType string

const (
	ExecutionEventStepStarted   ExecutionEventType = "step_started"
	ExecutionEventStepCompleted ExecutionEventType = "step_completed"
	ExecutionEventStepFailed    ExecutionEventType = "step_failed"
	ExecutionEventCheckpointed  ExecutionEventType = "checkpointed"
	ExecutionEventPaused        ExecutionEventType = "paused"
	ExecutionEventResumed       ExecutionEventType = "resumed"
)

// ExecutionEvent is emitted during execution lifecycle transitions.
type ExecutionEvent struct {
	Type      ExecutionEventType
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

// WithCheckpointStore sets a custom ExecutionCheckpointStore on the Executor.
func WithCheckpointStore(store ExecutionCheckpointStore) ExecutorOption {
	return func(e *Executor) {
		e.ExecutionCheckpointStore = store
	}
}

// Executor manages long-running agent executions.
type Executor struct {
	config                   ExecutorConfig
	executions               map[string]*Execution
	steps                    map[string][]StepFunc
	namedSteps               map[string][]NamedStep
	pauseRequested           map[string]bool
	resumeCh                 map[string]chan struct{}
	registry                 *StepRegistry
	ExecutionCheckpointStore ExecutionCheckpointStore
	OnEvent                  func(ExecutionEvent)
	logger                   *zap.Logger
	mu                       sync.RWMutex
}

// NewExecutor creates a new long-running executor.
func NewExecutor(config ExecutorConfig, logger *zap.Logger, opts ...ExecutorOption) *Executor {
	if logger == nil {
		logger = zap.NewNop()
	}
	if err := os.MkdirAll(config.CheckpointDir, 0755); err != nil {
		logger.Warn("failed to create ExecutionCheckpoint directory",
			zap.String("dir", config.CheckpointDir),
			zap.Error(err),
		)
	}

	e := &Executor{
		config:         config,
		executions:     make(map[string]*Execution),
		steps:          make(map[string][]StepFunc),
		namedSteps:     make(map[string][]NamedStep),
		pauseRequested: make(map[string]bool),
		resumeCh:       make(map[string]chan struct{}),
		registry:       NewStepRegistry(),
		logger:         logger.With(zap.String("component", "longrunning")),
	}
	e.ExecutionCheckpointStore = NewFileCheckpointStore(config.CheckpointDir, logger)

	for _, opt := range opts {
		opt(e)
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
		State:       ExecutionStateInitialized,
		Progress:    0,
		CurrentStep: 0,
		TotalSteps:  len(steps),
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
		Checkpoints: make([]ExecutionCheckpoint, 0),
		Metadata:    make(map[string]any),
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.steps[exec.ID] = steps
	e.resumeCh[exec.ID] = make(chan struct{}, 1)
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
		State:       ExecutionStateInitialized,
		Progress:    0,
		CurrentStep: 0,
		TotalSteps:  len(steps),
		StepNames:   stepNames,
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
		Checkpoints: make([]ExecutionCheckpoint, 0),
		Metadata:    make(map[string]any),
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.steps[exec.ID] = stepFuncs
	e.namedSteps[exec.ID] = steps
	e.resumeCh[exec.ID] = make(chan struct{}, 1)
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
	exec.State = ExecutionStateRunning
	exec.LastUpdate = time.Now()
	exec.mu.Unlock()

	go e.runExecution(ctx, exec, steps, initialState)

	return nil
}

func (e *Executor) runExecution(ctx context.Context, exec *Execution, steps []StepFunc, state any) {
	defer e.cleanupExecutionChannels(exec.ID)

	checkpointTicker := time.NewTicker(e.config.CheckpointInterval)
	heartbeatTicker := time.NewTicker(e.config.HeartbeatInterval)
	defer checkpointTicker.Stop()
	defer heartbeatTicker.Stop()

	e.mu.RLock()
	resumeCh := e.resumeCh[exec.ID]
	e.mu.RUnlock()

	currentState := state

	for exec.CurrentStep < exec.TotalSteps {
		// Check context cancellation first.
		select {
		case <-ctx.Done():
			exec.mu.Lock()
			exec.State = ExecutionStateCancelled
			exec.mu.Unlock()
			e.saveCheckpoint(exec, currentState)
			return
		default:
		}

		// Drain tickers without blocking.
		e.drainTickers(exec, checkpointTicker, heartbeatTicker, currentState)

		if !e.waitWhilePaused(ctx, exec, resumeCh, currentState) {
			return
		}

		// Execute step with retry and exponential backoff.
		step := steps[exec.CurrentStep]
		e.emitEvent(ExecutionEvent{
			Type: ExecutionEventStepStarted, ExecID: exec.ID,
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
				Type: ExecutionEventStepFailed, ExecID: exec.ID,
				Step: exec.CurrentStep, Timestamp: time.Now(), Error: err, State: currentState,
			})
			if retry < e.config.MaxRetries {
				backoff := retryBackoffDuration(retry)
				timer := time.NewTimer(backoff)
				select {
				case <-timer.C:
				case <-ctx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					exec.mu.Lock()
					exec.State = ExecutionStateCancelled
					exec.mu.Unlock()
					e.saveCheckpoint(exec, currentState)
					return
				}
			}
		}

		if err != nil {
			exec.mu.Lock()
			exec.State = ExecutionStateFailed
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
			Type: ExecutionEventStepCompleted, ExecID: exec.ID,
			Step: exec.CurrentStep - 1, Timestamp: time.Now(), State: currentState,
		})

		e.logger.Debug("step completed",
			zap.String("exec_id", exec.ID),
			zap.Int("step", exec.CurrentStep),
			zap.Float64("progress", exec.Progress),
		)
	}

	exec.mu.Lock()
	exec.State = ExecutionStateCompleted
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

func retryBackoffDuration(retry int) time.Duration {
	if retry <= 0 {
		return time.Second
	}

	shift := retry
	if shift > 30 {
		shift = 30
	}

	backoff := time.Duration(1<<uint(shift)) * time.Second
	return time.Duration(math.Min(float64(backoff), float64(30*time.Second)))
}

func (e *Executor) cleanupExecutionChannels(execID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pauseRequested, execID)
	delete(e.resumeCh, execID)
	delete(e.steps, execID)
	delete(e.namedSteps, execID)
}

func (e *Executor) waitWhilePaused(ctx context.Context, exec *Execution, resumeCh <-chan struct{}, state any) bool {
	e.mu.Lock()
	exec.mu.Lock()
	if e.pauseRequested[exec.ID] {
		delete(e.pauseRequested, exec.ID)
		if exec.State == ExecutionStateRunning || exec.State == ExecutionStateResuming {
			exec.State = ExecutionStatePaused
			exec.LastUpdate = time.Now()
		}
	}
	paused := exec.State == ExecutionStatePaused
	exec.mu.Unlock()
	e.mu.Unlock()
	if !paused {
		return true
	}

	e.saveCheckpoint(exec, state)
	e.emitEvent(ExecutionEvent{
		Type:      ExecutionEventPaused,
		ExecID:    exec.ID,
		Step:      exec.CurrentStep,
		Timestamp: time.Now(),
		State:     state,
	})

	for {
		select {
		case <-resumeCh:
			exec.mu.Lock()
			current := exec.State
			switch current {
			case ExecutionStatePaused:
				exec.mu.Unlock()
				continue
			case ExecutionStateRunning, ExecutionStateResuming:
				exec.State = ExecutionStateRunning
				exec.LastUpdate = time.Now()
				exec.mu.Unlock()
				e.emitEvent(ExecutionEvent{
					Type:      ExecutionEventResumed,
					ExecID:    exec.ID,
					Step:      exec.CurrentStep,
					Timestamp: time.Now(),
					State:     state,
				})
				return true
			default:
				exec.mu.Unlock()
				return false
			}
		case <-ctx.Done():
			exec.mu.Lock()
			exec.State = ExecutionStateCancelled
			exec.mu.Unlock()
			e.saveCheckpoint(exec, state)
			return false
		}
	}
}

// saveCheckpoint persists execution state via the ExecutionCheckpointStore.
func (e *Executor) saveCheckpoint(exec *Execution, state any) {
	exec.mu.Lock()
	ExecutionCheckpoint := ExecutionCheckpoint{
		ID:        generateID("cp"),
		Step:      exec.CurrentStep,
		State:     state,
		Timestamp: time.Now(),
	}
	exec.Checkpoints = append(exec.Checkpoints, ExecutionCheckpoint)
	exec.mu.Unlock()

	e.emitEvent(ExecutionEvent{
		Type: ExecutionEventCheckpointed, ExecID: exec.ID,
		Step: ExecutionCheckpoint.Step, Timestamp: ExecutionCheckpoint.Timestamp, State: state,
	})

	if e.ExecutionCheckpointStore == nil {
		return
	}
	if err := e.ExecutionCheckpointStore.SaveCheckpoint(context.Background(), exec); err != nil {
		e.logger.Error("failed to save ExecutionCheckpoint", zap.Error(err))
	}
}

// Pause signals a running execution to pause.
func (e *Executor) Pause(execID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	exec, ok := e.executions[execID]

	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	exec.mu.Lock()
	state := exec.State
	switch state {
	case ExecutionStateRunning:
		e.pauseRequested[execID] = true
		exec.LastUpdate = time.Now()
	case ExecutionStatePaused:
		exec.mu.Unlock()
		e.logger.Info("execution pause already pending", zap.String("exec_id", execID))
		return nil
	default:
		exec.mu.Unlock()
		return fmt.Errorf("execution not running: %s", state)
	}
	exec.mu.Unlock()

	e.logger.Info("execution pause signaled", zap.String("exec_id", execID))
	return nil
}

// Resume signals a paused execution to continue.
func (e *Executor) Resume(execID string) error {
	e.mu.Lock()

	exec, ok := e.executions[execID]
	resumeCh, hasCh := e.resumeCh[execID]

	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("execution not found: %s", execID)
	}

	exec.mu.Lock()
	state := exec.State
	switch state {
	case ExecutionStatePaused:
		exec.State = ExecutionStateRunning
		exec.LastUpdate = time.Now()
	case ExecutionStateRunning:
		delete(e.pauseRequested, execID)
		exec.mu.Unlock()
		e.mu.Unlock()
		e.logger.Info("execution resume already pending", zap.String("exec_id", execID))
		return nil
	case ExecutionStateResuming:
		delete(e.pauseRequested, execID)
		exec.mu.Unlock()
		e.mu.Unlock()
		e.logger.Info("execution resume already pending", zap.String("exec_id", execID))
		return nil
	default:
		exec.mu.Unlock()
		e.mu.Unlock()
		return fmt.Errorf("execution not paused: %s", state)
	}
	exec.mu.Unlock()
	e.mu.Unlock()

	if hasCh {
		signalExecutionControl(resumeCh)
	}

	e.logger.Info("execution resume signaled", zap.String("exec_id", execID))
	return nil
}

// LoadExecution loads an execution from the ExecutionCheckpoint store.
func (e *Executor) LoadExecution(execID string) (*Execution, error) {
	if e.ExecutionCheckpointStore == nil {
		return nil, fmt.Errorf("ExecutionCheckpoint store not configured")
	}
	exec, err := e.ExecutionCheckpointStore.LoadCheckpoint(context.Background(), execID)
	if err != nil {
		return nil, fmt.Errorf("loading ExecutionCheckpoint: %w", err)
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.resumeCh[exec.ID] = make(chan struct{}, 1)
	e.mu.Unlock()

	return exec, nil
}

// ResumeExecution resumes a loaded execution from its last ExecutionCheckpoint using the step registry.
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
	exec.State = ExecutionStateResuming
	exec.LastUpdate = time.Now()
	exec.mu.Unlock()

	e.logger.Info("resuming execution from ExecutionCheckpoint",
		zap.String("exec_id", execID),
		zap.Int("from_step", exec.CurrentStep),
		zap.Int("total_steps", exec.TotalSteps),
	)

	exec.mu.Lock()
	exec.State = ExecutionStateRunning
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
	if e.ExecutionCheckpointStore == nil {
		return 0, nil
	}
	execs, err := e.ExecutionCheckpointStore.ListCheckpoints(ctx)
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
		e.resumeCh[exec.ID] = make(chan struct{}, 1)
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
	case ExecutionStateRunning, ExecutionStatePaused, ExecutionStateResuming:
		return true
	default:
		return false
	}
}

// drainTickers processes any pending ExecutionCheckpoint/heartbeat ticks.
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

func signalExecutionControl(ch chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case ch <- struct{}{}:
		return true
	default:
		return false
	}
}
