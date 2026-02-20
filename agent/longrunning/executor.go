// 包长跑为长跑代理任务(日-等)提供支持.
package longrunning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 处决国是长期处决状态。
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

// 处决是长期执行的代理人处决。
type Execution struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	State       ExecutionState `json:"state"`
	Progress    float64        `json:"progress"`
	CurrentStep int            `json:"current_step"`
	TotalSteps  int            `json:"total_steps"`
	StartTime   time.Time      `json:"start_time"`
	LastUpdate  time.Time      `json:"last_update"`
	EndTime     *time.Time     `json:"end_time,omitempty"`
	Checkpoints []Checkpoint   `json:"checkpoints"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// 检查站是一个可恢复的检查站。
type Checkpoint struct {
	ID        string         `json:"id"`
	Step      int            `json:"step"`
	State     any            `json:"state"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// 执行器Config 配置长期执行器。
type ExecutorConfig struct {
	CheckpointInterval time.Duration `json:"checkpoint_interval"`
	CheckpointDir      string        `json:"checkpoint_dir"`
	MaxRetries         int           `json:"max_retries"`
	HeartbeatInterval  time.Duration `json:"heartbeat_interval"`
	AutoResume         bool          `json:"auto_resume"`
}

// 默认执行config 返回默认配置 。
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		CheckpointInterval: 5 * time.Minute,
		CheckpointDir:      "./checkpoints",
		MaxRetries:         3,
		HeartbeatInterval:  30 * time.Second,
		AutoResume:         true,
	}
}

// StepFunc代表了长跑行刑的单步.
type StepFunc func(ctx context.Context, state any) (any, error)

// 执行官管理长期代理处决。
type Executor struct {
	config     ExecutorConfig
	executions map[string]*Execution
	steps      map[string][]StepFunc
	logger     *zap.Logger
	mu         sync.RWMutex
}

// NewExecuters 创建一个新的长期执行器 。
func NewExecutor(config ExecutorConfig, logger *zap.Logger) *Executor {
	if logger == nil {
		logger = zap.NewNop()
	}
	os.MkdirAll(config.CheckpointDir, 0755)

	return &Executor{
		config:     config,
		executions: make(map[string]*Execution),
		steps:      make(map[string][]StepFunc),
		logger:     logger.With(zap.String("component", "longrunning")),
	}
}

// CreateExecution 创建了新的长期执行.
func (e *Executor) CreateExecution(name string, steps []StepFunc) *Execution {
	exec := &Execution{
		ID:          fmt.Sprintf("exec_%d", time.Now().UnixNano()),
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
	e.mu.Unlock()

	e.logger.Info("execution created",
		zap.String("id", exec.ID),
		zap.String("name", name),
		zap.Int("steps", len(steps)),
	)

	return exec
}

// 开始长期执行
func (e *Executor) Start(ctx context.Context, execID string, initialState any) error {
	e.mu.RLock()
	exec, ok := e.executions[execID]
	steps := e.steps[execID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	exec.State = StateRunning
	exec.LastUpdate = time.Now()

	go e.runExecution(ctx, exec, steps, initialState)

	return nil
}

func (e *Executor) runExecution(ctx context.Context, exec *Execution, steps []StepFunc, state any) {
	checkpointTicker := time.NewTicker(e.config.CheckpointInterval)
	heartbeatTicker := time.NewTicker(e.config.HeartbeatInterval)
	defer checkpointTicker.Stop()
	defer heartbeatTicker.Stop()

	currentState := state

	for exec.CurrentStep < exec.TotalSteps {
		select {
		case <-ctx.Done():
			exec.State = StateCancelled
			e.saveCheckpoint(exec, currentState)
			return
		case <-checkpointTicker.C:
			e.saveCheckpoint(exec, currentState)
		case <-heartbeatTicker.C:
			exec.LastUpdate = time.Now()
		default:
		}

		// 检查是否暂停
		if exec.State == StatePaused {
			e.saveCheckpoint(exec, currentState)
			time.Sleep(time.Second)
			continue
		}

		// 重复执行步骤
		step := steps[exec.CurrentStep]
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
			time.Sleep(time.Duration(retry+1) * time.Second)
		}

		if err != nil {
			exec.State = StateFailed
			exec.Error = err.Error()
			now := time.Now()
			exec.EndTime = &now
			e.saveCheckpoint(exec, currentState)
			e.logger.Error("execution failed",
				zap.String("exec_id", exec.ID),
				zap.Int("step", exec.CurrentStep),
				zap.Error(err),
			)
			return
		}

		currentState = result
		exec.CurrentStep++
		exec.Progress = float64(exec.CurrentStep) / float64(exec.TotalSteps) * 100
		exec.LastUpdate = time.Now()

		e.logger.Debug("step completed",
			zap.String("exec_id", exec.ID),
			zap.Int("step", exec.CurrentStep),
			zap.Float64("progress", exec.Progress),
		)
	}

	exec.State = StateCompleted
	exec.Progress = 100
	now := time.Now()
	exec.EndTime = &now
	e.saveCheckpoint(exec, currentState)

	e.logger.Info("execution completed",
		zap.String("exec_id", exec.ID),
		zap.Duration("duration", exec.EndTime.Sub(exec.StartTime)),
	)
}

func (e *Executor) saveCheckpoint(exec *Execution, state any) {
	checkpoint := Checkpoint{
		ID:        fmt.Sprintf("cp_%d", time.Now().UnixNano()),
		Step:      exec.CurrentStep,
		State:     state,
		Timestamp: time.Now(),
	}

	exec.Checkpoints = append(exec.Checkpoints, checkpoint)

	// 坚持到磁盘
	data, err := json.Marshal(exec)
	if err != nil {
		e.logger.Error("failed to marshal checkpoint", zap.Error(err))
		return
	}

	path := fmt.Sprintf("%s/%s.json", e.config.CheckpointDir, exec.ID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		e.logger.Error("failed to save checkpoint", zap.Error(err))
	}
}

// 暂停执行 。
func (e *Executor) Pause(execID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	exec, ok := e.executions[execID]
	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	if exec.State != StateRunning {
		return fmt.Errorf("execution not running: %s", exec.State)
	}

	exec.State = StatePaused
	e.logger.Info("execution paused", zap.String("exec_id", execID))
	return nil
}

// 恢复暂停执行 。
func (e *Executor) Resume(execID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	exec, ok := e.executions[execID]
	if !ok {
		return fmt.Errorf("execution not found: %s", execID)
	}

	if exec.State != StatePaused {
		return fmt.Errorf("execution not paused: %s", exec.State)
	}

	exec.State = StateRunning
	e.logger.Info("execution resumed", zap.String("exec_id", execID))
	return nil
}

// LoadExecution从检查站装入一个执行器.
func (e *Executor) LoadExecution(execID string) (*Execution, error) {
	path := fmt.Sprintf("%s/%s.json", e.config.CheckpointDir, execID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var exec Execution
	if err := json.Unmarshal(data, &exec); err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.executions[exec.ID] = &exec
	e.mu.Unlock()

	return &exec, nil
}

// GetExecution 以 ID 检索执行 。
func (e *Executor) GetExecution(execID string) (*Execution, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	exec, ok := e.executions[execID]
	return exec, ok
}

// ListExecutions 返回全部处决 。
func (e *Executor) ListExecutions() []*Execution {
	e.mu.RLock()
	defer e.mu.RUnlock()
	execs := make([]*Execution, 0, len(e.executions))
	for _, exec := range e.executions {
		execs = append(execs, exec)
	}
	return execs
}
