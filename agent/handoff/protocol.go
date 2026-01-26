// Package handoff provides Agent Handoff protocol for task delegation between agents.
// Implements OpenAI SDK-style agent handoff mechanism.
package handoff

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// HandoffStatus represents the status of a handoff.
type HandoffStatus string

const (
	StatusPending    HandoffStatus = "pending"
	StatusAccepted   HandoffStatus = "accepted"
	StatusRejected   HandoffStatus = "rejected"
	StatusInProgress HandoffStatus = "in_progress"
	StatusCompleted  HandoffStatus = "completed"
	StatusFailed     HandoffStatus = "failed"
)

// Handoff represents a task delegation from one agent to another.
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
}

// Task represents the task being handed off.
type Task struct {
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Input       any            `json:"input"`
	Priority    int            `json:"priority"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// HandoffContext provides context for the handoff.
type HandoffContext struct {
	ConversationID string         `json:"conversation_id,omitempty"`
	Messages       []Message      `json:"messages,omitempty"`
	Variables      map[string]any `json:"variables,omitempty"`
	ParentHandoff  string         `json:"parent_handoff,omitempty"`
}

// Message represents a conversation message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// HandoffResult contains the result of a completed handoff.
type HandoffResult struct {
	Output   any    `json:"output"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"duration_ms"`
}

// AgentCapability describes what an agent can do.
type AgentCapability struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TaskTypes   []string `json:"task_types"`
	Priority    int      `json:"priority"`
}

// HandoffAgent interface for agents that support handoff.
type HandoffAgent interface {
	ID() string
	Capabilities() []AgentCapability
	CanHandle(task Task) bool
	AcceptHandoff(ctx context.Context, handoff *Handoff) error
	ExecuteHandoff(ctx context.Context, handoff *Handoff) (*HandoffResult, error)
}

// HandoffManager manages agent handoffs.
type HandoffManager struct {
	agents   map[string]HandoffAgent
	handoffs map[string]*Handoff
	pending  map[string]chan *HandoffResult
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewHandoffManager creates a new handoff manager.
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

// RegisterAgent registers an agent for handoffs.
func (m *HandoffManager) RegisterAgent(agent HandoffAgent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agent.ID()] = agent
	m.logger.Info("registered agent", zap.String("id", agent.ID()))
}

// UnregisterAgent removes an agent.
func (m *HandoffManager) UnregisterAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, agentID)
}

// FindAgent finds the best agent for a task.
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

// Handoff initiates a handoff to another agent.
func (m *HandoffManager) Handoff(ctx context.Context, opts HandoffOptions) (*Handoff, error) {
	// Find target agent if not specified
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

	// Store handoff
	m.mu.Lock()
	m.handoffs[handoff.ID] = handoff
	resultCh := make(chan *HandoffResult, 1)
	m.pending[handoff.ID] = resultCh
	m.mu.Unlock()

	// Accept handoff
	if err := targetAgent.AcceptHandoff(ctx, handoff); err != nil {
		handoff.Status = StatusRejected
		return handoff, fmt.Errorf("handoff rejected: %w", err)
	}

	now := time.Now()
	handoff.AcceptedAt = &now
	handoff.Status = StatusAccepted

	// Execute async if not waiting
	if !opts.Wait {
		go m.executeHandoff(ctx, targetAgent, handoff, resultCh)
		return handoff, nil
	}

	// Execute and wait
	go m.executeHandoff(ctx, targetAgent, handoff, resultCh)

	select {
	case result := <-resultCh:
		handoff.Result = result
		return handoff, nil
	case <-time.After(handoff.Timeout):
		handoff.Status = StatusFailed
		return handoff, fmt.Errorf("handoff timeout")
	case <-ctx.Done():
		return handoff, ctx.Err()
	}
}

func (m *HandoffManager) executeHandoff(ctx context.Context, agent HandoffAgent, handoff *Handoff, resultCh chan *HandoffResult) {
	handoff.Status = StatusInProgress
	start := time.Now()

	result, err := agent.ExecuteHandoff(ctx, handoff)
	if err != nil {
		result = &HandoffResult{Error: err.Error(), Duration: time.Since(start).Milliseconds()}
		handoff.Status = StatusFailed
	} else {
		result.Duration = time.Since(start).Milliseconds()
		handoff.Status = StatusCompleted
	}

	now := time.Now()
	handoff.CompletedAt = &now
	handoff.Result = result

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

// GetHandoff retrieves a handoff by ID.
func (m *HandoffManager) GetHandoff(handoffID string) (*Handoff, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handoff, ok := m.handoffs[handoffID]
	if !ok {
		return nil, fmt.Errorf("handoff not found: %s", handoffID)
	}
	return handoff, nil
}

// HandoffOptions configures a handoff.
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
