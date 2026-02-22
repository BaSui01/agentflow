package hierarchical

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

// HierarchicalAgent 层次化 Agent
// 实现 Supervisor-Worker 模式
type HierarchicalAgent struct {
	*agent.BaseAgent

	// 层次结构
	supervisor  agent.Agent      // 监督者
	workers     []agent.Agent    // 工作者
	coordinator *TaskCoordinator // 任务协调器

	// 配置
	config HierarchicalConfig

	logger *zap.Logger
}

// HierarchicalConfig 层次化配置
type HierarchicalConfig struct {
	MaxWorkers        int           `json:"max_workers"`         // 最大工作者数量
	TaskTimeout       time.Duration `json:"task_timeout"`        // 任务超时
	EnableRetry       bool          `json:"enable_retry"`        // 启用重试
	MaxRetries        int           `json:"max_retries"`         // 最大重试次数
	WorkerSelection   string        `json:"worker_selection"`    // 工作者选择策略
	EnableLoadBalance bool          `json:"enable_load_balance"` // 启用负载均衡
}

// DefaultHierarchicalConfig 默认配置
func DefaultHierarchicalConfig() HierarchicalConfig {
	return HierarchicalConfig{
		MaxWorkers:        5,
		TaskTimeout:       5 * time.Minute,
		EnableRetry:       true,
		MaxRetries:        3,
		WorkerSelection:   "round_robin",
		EnableLoadBalance: true,
	}
}

// TaskCoordinator 任务协调器
type TaskCoordinator struct {
	// 任务队列
	taskQueue chan *Task

	// 工作者池
	workers []agent.Agent

	// 工作者状态
	workerStatus map[string]*WorkerStatus
	statusMu     sync.RWMutex

	// 任务分配策略
	strategy AssignmentStrategy

	// 配置
	config HierarchicalConfig

	logger *zap.Logger
}

// Task 任务
type Task struct {
	ID           string
	Type         string
	Input        *agent.Input
	Priority     int
	Deadline     time.Time
	Dependencies []string
	Metadata     map[string]any

	// 执行状态
	Status      TaskStatus
	AssignedTo  string
	StartedAt   time.Time
	CompletedAt time.Time
	Result      *agent.Output
	Error       error
	RetryCount  int
}

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusAssigned  TaskStatus = "assigned"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// WorkerStatus 工作者状态
type WorkerStatus struct {
	AgentID        string
	Status         string // idle, busy, error
	CurrentTask    *Task
	CompletedTasks int
	FailedTasks    int
	AvgDuration    time.Duration
	LastActive     time.Time
	Load           float64 // 0-1
}

// AssignmentStrategy 任务分配策略
type AssignmentStrategy interface {
	// 选择工作者
	SelectWorker(ctx context.Context, task *Task, workers []agent.Agent, status map[string]*WorkerStatus) (agent.Agent, error)
}

// NewHierarchicalAgent 创建层次化 Agent
func NewHierarchicalAgent(
	base *agent.BaseAgent,
	supervisor agent.Agent,
	workers []agent.Agent,
	config HierarchicalConfig,
	logger *zap.Logger,
) *HierarchicalAgent {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	coordinator := NewTaskCoordinator(workers, config, logger)

	return &HierarchicalAgent{
		BaseAgent:   base,
		supervisor:  supervisor,
		workers:     workers,
		coordinator: coordinator,
		config:      config,
		logger:      logger.With(zap.String("component", "hierarchical_agent")),
	}
}

// Execute 执行任务（层次化）
func (h *HierarchicalAgent) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	h.logger.Info("hierarchical execution started",
		zap.String("trace_id", input.TraceID),
	)

	// 1. Supervisor 分解任务
	subtasks, err := h.decomposeTask(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("task decomposition failed: %w", err)
	}

	h.logger.Info("task decomposed",
		zap.Int("subtasks", len(subtasks)),
	)

	// 2. 分配任务给 Workers
	results := make([]*agent.Output, len(subtasks))
	errors := make([]error, len(subtasks))

	var wg sync.WaitGroup
	for i, subtask := range subtasks {
		wg.Add(1)
		go func(idx int, task *Task) {
			defer wg.Done()

			result, err := h.coordinator.ExecuteTask(ctx, task)
			results[idx] = result
			errors[idx] = err
		}(i, subtask)
	}

	wg.Wait()

	// 3. 检查错误
	for i, err := range errors {
		if err != nil {
			h.logger.Error("subtask failed",
				zap.Int("subtask_index", i),
				zap.Error(err),
			)
			return nil, fmt.Errorf("subtask %d failed: %w", i, err)
		}
	}

	// 4. Supervisor 聚合结果
	finalOutput, err := h.aggregateResults(ctx, input, results)
	if err != nil {
		return nil, fmt.Errorf("result aggregation failed: %w", err)
	}

	h.logger.Info("hierarchical execution completed",
		zap.String("trace_id", input.TraceID),
	)

	return finalOutput, nil
}

// decomposeTask 分解任务
func (h *HierarchicalAgent) decomposeTask(ctx context.Context, input *agent.Input) ([]*Task, error) {
	// 使用 Supervisor 分解任务
	decompositionPrompt := fmt.Sprintf(`请将以下任务分解为多个子任务：

任务：%s

要求：
1. 每个子任务应该独立且可并行执行
2. 子任务之间的依赖关系要明确
3. 输出格式：JSON 数组，每个元素包含 type, description, priority

示例输出：
[
  {"type": "research", "description": "收集相关资料", "priority": 1},
  {"type": "analysis", "description": "分析数据", "priority": 2}
]`, input.Content)

	supervisorInput := &agent.Input{
		TraceID: input.TraceID,
		Content: decompositionPrompt,
	}

	output, err := h.supervisor.Execute(ctx, supervisorInput)
	if err != nil {
		return nil, err
	}

	// 解析子任务（简化实现）
	subtasks := h.parseSubtasks(output.Content, input)

	return subtasks, nil
}

// subtaskJSON is the JSON structure for parsed subtasks from LLM output.
type subtaskJSON struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

// parseSubtasks parses subtasks from LLM output content.
// It tries: 1) direct JSON array parse, 2) extract ```json block, 3) fallback to single task.
func (h *HierarchicalAgent) parseSubtasks(content string, originalInput *agent.Input) []*Task {
	// Attempt 1: direct JSON array parse
	if tasks := h.tryParseSubtaskJSON(content, originalInput); tasks != nil {
		return tasks
	}

	// Attempt 2: extract JSON from ```json ... ``` code block
	if idx := strings.Index(content, "```json"); idx != -1 {
		start := idx + len("```json")
		if end := strings.Index(content[start:], "```"); end != -1 {
			jsonBlock := strings.TrimSpace(content[start : start+end])
			if tasks := h.tryParseSubtaskJSON(jsonBlock, originalInput); tasks != nil {
				return tasks
			}
		}
	}

	// Attempt 3: extract JSON from ``` ... ``` code block (no language tag)
	if idx := strings.Index(content, "```"); idx != -1 {
		start := idx + len("```")
		// Skip language tag if present on same line
		if nlIdx := strings.Index(content[start:], "\n"); nlIdx != -1 {
			start = start + nlIdx + 1
		}
		if end := strings.Index(content[start:], "```"); end != -1 {
			jsonBlock := strings.TrimSpace(content[start : start+end])
			if tasks := h.tryParseSubtaskJSON(jsonBlock, originalInput); tasks != nil {
				return tasks
			}
		}
	}

	// Fallback: create a single task with the original input
	return []*Task{
		{
			ID:       fmt.Sprintf("%s-subtask-1", originalInput.TraceID),
			Type:     "subtask",
			Priority: 1,
			Input: &agent.Input{
				TraceID: originalInput.TraceID,
				Content: originalInput.Content,
			},
			Status: TaskStatusPending,
		},
	}
}

// tryParseSubtaskJSON attempts to parse a JSON array of subtasks.
// Returns nil if parsing fails.
func (h *HierarchicalAgent) tryParseSubtaskJSON(raw string, originalInput *agent.Input) []*Task {
	var parsed []subtaskJSON
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	if len(parsed) == 0 {
		return nil
	}

	tasks := make([]*Task, 0, len(parsed))
	for i, st := range parsed {
		taskType := st.Type
		if taskType == "" {
			taskType = "subtask"
		}
		desc := st.Description
		if desc == "" {
			desc = originalInput.Content
		}
		tasks = append(tasks, &Task{
			ID:       fmt.Sprintf("%s-subtask-%d", originalInput.TraceID, i+1),
			Type:     taskType,
			Priority: st.Priority,
			Input: &agent.Input{
				TraceID: originalInput.TraceID,
				Content: desc,
			},
			Status: TaskStatusPending,
		})
	}
	return tasks
}

// aggregateResults 聚合结果
func (h *HierarchicalAgent) aggregateResults(ctx context.Context, input *agent.Input, results []*agent.Output) (*agent.Output, error) {
	// 使用 Supervisor 聚合结果
	aggregationPrompt := fmt.Sprintf(`请聚合以下子任务的结果：

原始任务：%s

子任务结果：`, input.Content)

	for i, result := range results {
		aggregationPrompt += fmt.Sprintf("\n%d. %s", i+1, result.Content)
	}

	aggregationPrompt += "\n\n请提供综合的最终结果。"

	supervisorInput := &agent.Input{
		TraceID: input.TraceID,
		Content: aggregationPrompt,
	}

	return h.supervisor.Execute(ctx, supervisorInput)
}

// NewTaskCoordinator 创建任务协调器
func NewTaskCoordinator(workers []agent.Agent, config HierarchicalConfig, logger *zap.Logger) *TaskCoordinator {
	coordinator := &TaskCoordinator{
		taskQueue:    make(chan *Task, 100),
		workers:      workers,
		workerStatus: make(map[string]*WorkerStatus),
		config:       config,
		logger:       logger.With(zap.String("component", "task_coordinator")),
	}

	// 初始化工作者状态
	for _, worker := range workers {
		coordinator.workerStatus[worker.ID()] = &WorkerStatus{
			AgentID:    worker.ID(),
			Status:     "idle",
			LastActive: time.Now(),
			Load:       0.0,
		}
	}

	// 设置分配策略
	switch config.WorkerSelection {
	case "round_robin":
		coordinator.strategy = &RoundRobinStrategy{}
	case "least_loaded":
		coordinator.strategy = &LeastLoadedStrategy{}
	case "random":
		coordinator.strategy = &RandomStrategy{}
	default:
		coordinator.strategy = &RoundRobinStrategy{}
	}

	return coordinator
}

// ExecuteTask 执行任务
func (c *TaskCoordinator) ExecuteTask(ctx context.Context, task *Task) (*agent.Output, error) {
	c.logger.Debug("executing task",
		zap.String("task_id", task.ID),
		zap.String("type", task.Type),
	)

	// 1. 选择工作者
	worker, err := c.strategy.SelectWorker(ctx, task, c.workers, c.workerStatus)
	if err != nil {
		return nil, fmt.Errorf("worker selection failed: %w", err)
	}

	// 2. 更新任务状态
	task.Status = TaskStatusAssigned
	task.AssignedTo = worker.ID()
	task.StartedAt = time.Now()

	// 3. 更新工作者状态
	c.updateWorkerStatus(worker.ID(), "busy", task)

	// 4. 执行任务（带超时和重试）
	var output *agent.Output
	var execErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Info("retrying task",
				zap.String("task_id", task.ID),
				zap.Int("attempt", attempt),
			)
			task.RetryCount++
		}

		// 创建带超时的上下文
		execCtx, cancel := context.WithTimeout(ctx, c.config.TaskTimeout)

		output, execErr = worker.Execute(execCtx, task.Input)
		cancel()

		if execErr == nil {
			break
		}

		if !c.config.EnableRetry || attempt >= c.config.MaxRetries {
			break
		}

		// 等待后重试
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}

	// 5. 更新任务状态
	task.CompletedAt = time.Now()
	if execErr != nil {
		task.Status = TaskStatusFailed
		task.Error = execErr
		c.updateWorkerStatus(worker.ID(), "idle", nil)
		c.incrementWorkerFailures(worker.ID())
		return nil, execErr
	}

	task.Status = TaskStatusCompleted
	task.Result = output

	// 6. 更新工作者状态
	c.updateWorkerStatus(worker.ID(), "idle", nil)
	c.incrementWorkerCompletions(worker.ID(), task.CompletedAt.Sub(task.StartedAt))

	c.logger.Debug("task completed",
		zap.String("task_id", task.ID),
		zap.String("worker_id", worker.ID()),
		zap.Duration("duration", task.CompletedAt.Sub(task.StartedAt)),
	)

	return output, nil
}

// updateWorkerStatus 更新工作者状态
func (c *TaskCoordinator) updateWorkerStatus(workerID string, status string, task *Task) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()

	if ws, ok := c.workerStatus[workerID]; ok {
		ws.Status = status
		ws.CurrentTask = task
		ws.LastActive = time.Now()

		// 更新负载
		if status == "busy" {
			ws.Load = 1.0
		} else {
			ws.Load = 0.0
		}
	}
}

// incrementWorkerCompletions 增加完成计数
func (c *TaskCoordinator) incrementWorkerCompletions(workerID string, duration time.Duration) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()

	if ws, ok := c.workerStatus[workerID]; ok {
		ws.CompletedTasks++

		// 更新平均持续时间
		if ws.AvgDuration == 0 {
			ws.AvgDuration = duration
		} else {
			ws.AvgDuration = (ws.AvgDuration*time.Duration(ws.CompletedTasks-1) + duration) / time.Duration(ws.CompletedTasks)
		}
	}
}

// incrementWorkerFailures 增加失败计数
func (c *TaskCoordinator) incrementWorkerFailures(workerID string) {
	c.statusMu.Lock()
	defer c.statusMu.Unlock()

	if ws, ok := c.workerStatus[workerID]; ok {
		ws.FailedTasks++
	}
}

// GetWorkerStatus 获取工作者状态
func (c *TaskCoordinator) GetWorkerStatus() map[string]*WorkerStatus {
	c.statusMu.RLock()
	defer c.statusMu.RUnlock()

	// 返回副本
	status := make(map[string]*WorkerStatus)
	for k, v := range c.workerStatus {
		status[k] = v
	}
	return status
}

// RoundRobinStrategy 轮询策略
type RoundRobinStrategy struct {
	current int
	mu      sync.Mutex
}

func (s *RoundRobinStrategy) SelectWorker(ctx context.Context, task *Task, workers []agent.Agent, status map[string]*WorkerStatus) (agent.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	// 找到空闲的工作者
	for i := 0; i < len(workers); i++ {
		idx := (s.current + i) % len(workers)
		worker := workers[idx]

		if ws, ok := status[worker.ID()]; ok && ws.Status == "idle" {
			s.current = (idx + 1) % len(workers)
			return worker, nil
		}
	}

	// 如果没有空闲的，返回第一个
	worker := workers[s.current]
	s.current = (s.current + 1) % len(workers)
	return worker, nil
}

// LeastLoadedStrategy 最少负载策略
type LeastLoadedStrategy struct{}

func (s *LeastLoadedStrategy) SelectWorker(ctx context.Context, task *Task, workers []agent.Agent, status map[string]*WorkerStatus) (agent.Agent, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	var bestWorker agent.Agent
	minLoad := 2.0 // 大于最大负载

	for _, worker := range workers {
		if ws, ok := status[worker.ID()]; ok {
			if ws.Load < minLoad {
				minLoad = ws.Load
				bestWorker = worker
			}
		}
	}

	if bestWorker == nil {
		return workers[0], nil
	}

	return bestWorker, nil
}

// RandomStrategy 随机策略
type RandomStrategy struct{}

func (s *RandomStrategy) SelectWorker(ctx context.Context, task *Task, workers []agent.Agent, status map[string]*WorkerStatus) (agent.Agent, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available")
	}

	// 简化实现：返回第一个空闲的
	for _, worker := range workers {
		if ws, ok := status[worker.ID()]; ok && ws.Status == "idle" {
			return worker, nil
		}
	}

	return workers[0], nil
}
