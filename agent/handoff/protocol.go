package handoff

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 交接状态代表交接状态.
type HandoffStatus string

const (
	StatusPending    HandoffStatus = "pending"
	StatusAccepted   HandoffStatus = "accepted"
	StatusRejected   HandoffStatus = "rejected"
	StatusInProgress HandoffStatus = "in_progress"
	StatusCompleted  HandoffStatus = "completed"
	StatusFailed     HandoffStatus = "failed"
)

// Handoff代表着从一个代理人到另一个代理人的任务代表团.
type Handoff struct {
	ID          string         `json:"id"`
	FromAgentID string         `json:"from_agent_id"`
	ToAgentID   string         `json:"to_agent_id"`
	Task        Task           `json:"task"`
	Status      HandoffStatus  `json:"status"`
	Context     HandoffContext `json:"context"`
	Result      *HandoffResult `json:"result,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	AcceptedAt  *time.Time     `json:"accepted_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Timeout     time.Duration  `json:"timeout"`
	RetryCount  int            `json:"retry_count"`
	MaxRetries  int            `json:"max_retries"`

	mu sync.Mutex `json:"-"`
}

// 任务代表着正在移交的任务。
type Task struct {
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Input       any            `json:"input"`
	Priority    int            `json:"priority"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// HandoffContext为交接提供了上下文.
type HandoffContext struct {
	ConversationID string         `json:"conversation_id,omitempty"`
	Messages       []Message      `json:"messages,omitempty"`
	Variables      map[string]any `json:"variables,omitempty"`
	ParentHandoff  string         `json:"parent_handoff,omitempty"`
}

// 信件代表对话信息 。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// HandoffResult包含完成交接的结果.
type HandoffResult struct {
	Output   any    `json:"output"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"duration_ms"`
}

// Agent Captainable 描述一个代理能够做什么.
type AgentCapability struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TaskTypes   []string `json:"task_types"`
	Priority    int      `json:"priority"`
}

// 支持交接的代理商的交接代理接口.
type HandoffAgent interface {
	ID() string
	Capabilities() []AgentCapability
	CanHandle(task Task) bool
	AcceptHandoff(ctx context.Context, handoff *Handoff) error
	ExecuteHandoff(ctx context.Context, handoff *Handoff) (*HandoffResult, error)
}

// HandoffManager管理代理人的交割.
type HandoffManager struct {
	agents   map[string]HandoffAgent
	handoffs map[string]*Handoff
	pending  map[string]chan *HandoffResult
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewHandoffManager创建了新的交接管理器.
func NewHandoffManager(logger *zap.Logger) *HandoffManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HandoffManager{
		agents:   make(map[string]HandoffAgent),
		handoffs: make(map[string]*Handoff),
		pending:  make(map[string]chan *HandoffResult),
		logger:   logger.With(zap.String("component", "handoff_manager")),
	}
}

// 代理人登记代理人进行交割.
func (m *HandoffManager) RegisterAgent(agent HandoffAgent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agent.ID()] = agent
	m.logger.Info("registered agent", zap.String("id", agent.ID()))
}

// 未注册代理删除代理 。
func (m *HandoffManager) UnregisterAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, agentID)
}

// FindAgent 找到任务的最佳代理 。
func (m *HandoffManager) FindAgent(task Task) (HandoffAgent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var bestAgent HandoffAgent
	bestPriority := -1

	for _, agent := range m.agents {
		if agent.CanHandle(task) {
			for _, cap := range agent.Capabilities() {
				if cap.Priority > bestPriority {
					bestAgent = agent
					bestPriority = cap.Priority
				}
			}
		}
	}

	if bestAgent == nil {
		return nil, fmt.Errorf("no agent found for task type: %s", task.Type)
	}
	return bestAgent, nil
}

// Handoff会向另一个特工交接
func (m *HandoffManager) Handoff(ctx context.Context, opts HandoffOptions) (*Handoff, error) {
	// 未指定时查找目标代理
	var targetAgent HandoffAgent
	var err error

	if opts.ToAgentID != "" {
		m.mu.RLock()
		targetAgent = m.agents[opts.ToAgentID]
		m.mu.RUnlock()
		if targetAgent == nil {
			return nil, fmt.Errorf("agent not found: %s", opts.ToAgentID)
		}
	} else {
		targetAgent, err = m.FindAgent(opts.Task)
		if err != nil {
			return nil, err
		}
	}

	handoff := &Handoff{
		ID:          generateHandoffID(),
		FromAgentID: opts.FromAgentID,
		ToAgentID:   targetAgent.ID(),
		Task:        opts.Task,
		Status:      StatusPending,
		Context:     opts.Context,
		CreatedAt:   time.Now(),
		Timeout:     opts.Timeout,
		MaxRetries:  opts.MaxRetries,
	}

	if handoff.Timeout == 0 {
		handoff.Timeout = 5 * time.Minute
	}

	m.logger.Info("initiating handoff",
		zap.String("id", handoff.ID),
		zap.String("from", handoff.FromAgentID),
		zap.String("to", handoff.ToAgentID),
	)

	// 存储交接
	m.mu.Lock()
	m.handoffs[handoff.ID] = handoff
	resultCh := make(chan *HandoffResult, 1)
	m.pending[handoff.ID] = resultCh
	m.mu.Unlock()

	// 接受移交
	if err := targetAgent.AcceptHandoff(ctx, handoff); err != nil {
		handoff.Status = StatusRejected
		return handoff, fmt.Errorf("handoff rejected: %w", err)
	}

	now := time.Now()
	handoff.AcceptedAt = &now
	handoff.Status = StatusAccepted

	// 不等待则执行同步
	if !opts.Wait {
		go m.executeHandoff(ctx, targetAgent, handoff, resultCh)
		return handoff, nil
	}

	// 执行和等待
	go m.executeHandoff(ctx, targetAgent, handoff, resultCh)

	select {
	case result := <-resultCh:
		handoff.Result = result
		return handoff, nil
	case <-time.After(handoff.Timeout):
		handoff.mu.Lock()
		handoff.Status = StatusFailed
		handoff.mu.Unlock()
		return handoff, fmt.Errorf("handoff timeout")
	case <-ctx.Done():
		return handoff, ctx.Err()
	}
}

func (m *HandoffManager) executeHandoff(ctx context.Context, agent HandoffAgent, handoff *Handoff, resultCh chan *HandoffResult) {
	handoff.mu.Lock()
	handoff.Status = StatusInProgress
	handoff.mu.Unlock()
	start := time.Now()

	result, err := agent.ExecuteHandoff(ctx, handoff)
	if err != nil {
		result = &HandoffResult{Error: err.Error(), Duration: time.Since(start).Milliseconds()}
		handoff.mu.Lock()
		handoff.Status = StatusFailed
		handoff.mu.Unlock()
	} else {
		result.Duration = time.Since(start).Milliseconds()
		handoff.mu.Lock()
		handoff.Status = StatusCompleted
		handoff.mu.Unlock()
	}

	now := time.Now()
	handoff.mu.Lock()
	handoff.CompletedAt = &now
	handoff.Result = result
	handoff.mu.Unlock()

	m.logger.Info("handoff completed",
		zap.String("id", handoff.ID),
		zap.String("status", string(handoff.Status)),
		zap.Int64("duration_ms", result.Duration),
	)

	select {
	case resultCh <- result:
	default:
	}
}

// 找汉多夫拿回身份证
func (m *HandoffManager) GetHandoff(handoffID string) (*Handoff, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handoff, ok := m.handoffs[handoffID]
	if !ok {
		return nil, fmt.Errorf("handoff not found: %s", handoffID)
	}
	return handoff, nil
}

// 交接选项配置交接。
type HandoffOptions struct {
	FromAgentID string
	ToAgentID   string
	Task        Task
	Context     HandoffContext
	Timeout     time.Duration
	MaxRetries  int
	Wait        bool
}

func generateHandoffID() string {
	return fmt.Sprintf("hoff_%d", time.Now().UnixNano())
}
