package handoff

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/types"
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
	ID              string         `json:"id"`
	FromAgentID     string         `json:"from_agent_id"`
	ToAgentID       string         `json:"to_agent_id"`
	ToolName        string         `json:"tool_name,omitempty"`
	ToolDescription string         `json:"tool_description,omitempty"`
	TransferMessage string         `json:"transfer_message,omitempty"`
	Task            Task           `json:"task"`
	Status          HandoffStatus  `json:"status"`
	Context         HandoffContext `json:"context"`
	Result          *HandoffResult `json:"result,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	AcceptedAt      *time.Time     `json:"accepted_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Timeout         time.Duration  `json:"timeout"`
	RetryCount      int            `json:"retry_count"`
	MaxRetries      int            `json:"max_retries"`

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
	ConversationID string          `json:"conversation_id,omitempty"`
	Messages       []types.Message `json:"messages,omitempty"`
	Variables      map[string]any  `json:"variables,omitempty"`
	ParentHandoff  string          `json:"parent_handoff,omitempty"`
}

// HandoffInputData 对齐官方 Agents SDK 的 handoff input filter 语义，
// 用于在交接发生时重写下一位 agent 看到的历史和新增消息。
type HandoffInputData struct {
	InputHistory  []types.Message `json:"input_history,omitempty"`
	PreHandoff    []types.Message `json:"pre_handoff,omitempty"`
	NewMessages   []types.Message `json:"new_messages,omitempty"`
	InputMessages []types.Message `json:"input_messages,omitempty"`
	Context       HandoffContext  `json:"context"`
}

// Clone returns a shallow-cloned HandoffInputData with deep-copied slices/maps.
func (d HandoffInputData) Clone() HandoffInputData {
	return HandoffInputData{
		InputHistory:  cloneMessages(d.InputHistory),
		PreHandoff:    cloneMessages(d.PreHandoff),
		NewMessages:   cloneMessages(d.NewMessages),
		InputMessages: cloneMessages(d.InputMessages),
		Context: HandoffContext{
			ConversationID: d.Context.ConversationID,
			Messages:       cloneMessages(d.Context.Messages),
			Variables:      cloneMap(d.Context.Variables),
			ParentHandoff:  d.Context.ParentHandoff,
		},
	}
}

type HandoffInputFilter func(ctx context.Context, data HandoffInputData) (HandoffInputData, error)
type HandoffHook func(ctx context.Context, handoff *Handoff) error
type HandoffEnabledFunc func(ctx context.Context, opts HandoffOptions) (bool, error)
type HandoffHistoryMapper func(history []types.Message) []types.Message

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

	if opts.EnableFunc != nil {
		enabled, enableErr := opts.EnableFunc(ctx, opts)
		if enableErr != nil {
			return nil, enableErr
		}
		if !enabled {
			return nil, fmt.Errorf("handoff disabled for target agent: %s", targetAgent.ID())
		}
	} else if opts.Enabled != nil && !*opts.Enabled {
		return nil, fmt.Errorf("handoff disabled for target agent: %s", targetAgent.ID())
	}

	toolName := strings.TrimSpace(opts.ToolNameOverride)
	if toolName == "" {
		toolName = defaultToolName(targetAgent.ID())
	}
	toolDescription := strings.TrimSpace(opts.ToolDescriptionOverride)
	if toolDescription == "" {
		toolDescription = defaultToolDescription(targetAgent.ID())
	}

	handoff := &Handoff{
		ID:              generateHandoffID(),
		FromAgentID:     opts.FromAgentID,
		ToAgentID:       targetAgent.ID(),
		ToolName:        toolName,
		ToolDescription: toolDescription,
		TransferMessage: defaultTransferMessage(targetAgent.ID()),
		Task:            opts.Task,
		Status:          StatusPending,
		Context:         opts.Context,
		CreatedAt:       time.Now(),
		Timeout:         opts.Timeout,
		MaxRetries:      opts.MaxRetries,
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
		m.finalizePending(handoff.ID, nil)
		return handoff, fmt.Errorf("handoff rejected: %w", err)
	}

	now := time.Now()
	handoff.AcceptedAt = &now
	handoff.Status = StatusAccepted

	if opts.OnHandoff != nil {
		if err := opts.OnHandoff(ctx, handoff); err != nil {
			handoff.Status = StatusFailed
			handoff.Result = &HandoffResult{Error: err.Error()}
			m.finalizePending(handoff.ID, handoff.Result)
			return handoff, fmt.Errorf("handoff hook failed: %w", err)
		}
	}

	inputData := HandoffInputData{
		InputHistory: cloneMessages(handoff.Context.Messages),
		Context: HandoffContext{
			ConversationID: handoff.Context.ConversationID,
			Messages:       cloneMessages(handoff.Context.Messages),
			Variables:      cloneMap(handoff.Context.Variables),
			ParentHandoff:  handoff.Context.ParentHandoff,
		},
	}
	if prompt, ok := handoff.Task.Input.(string); ok && strings.TrimSpace(prompt) != "" {
		inputData.NewMessages = []types.Message{{
			Role:    types.RoleUser,
			Content: prompt,
			Metadata: map[string]any{
				"handoff_id":   handoff.ID,
				"handoff_tool": handoff.ToolName,
			},
			Timestamp: time.Now(),
		}}
	}
	if opts.InputFilter != nil {
		filtered, err := opts.InputFilter(ctx, inputData.Clone())
		if err != nil {
			handoff.Status = StatusFailed
			handoff.Result = &HandoffResult{Error: err.Error()}
			m.finalizePending(handoff.ID, handoff.Result)
			return handoff, fmt.Errorf("handoff input filter failed: %w", err)
		}
		handoff.Context.Messages = composeNextMessages(filtered)
	} else if defaultNestHistoryValue(opts.NestHistory) {
		mapper := opts.HistoryMapper
		if mapper == nil {
			mapper = defaultHistoryMapper
		}
		inputData.InputHistory = mapper(inputData.InputHistory)
		handoff.Context.Messages = composeNextMessages(inputData)
	} else {
		handoff.Context.Messages = composeNextMessages(inputData)
	}

	// 不等待则执行同步
	if !opts.Wait {
		go m.executeHandoff(ctx, targetAgent, handoff, resultCh)
		return handoff, nil
	}

	// 执行和等待
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, handoff.Timeout)
	defer timeoutCancel()
	go m.executeHandoff(timeoutCtx, targetAgent, handoff, resultCh)

	select {
	case result := <-resultCh:
		handoff.Result = result
		m.cleanupPending(handoff.ID, resultCh)
		return handoff, nil
	case <-timeoutCtx.Done():
		handoff.mu.Lock()
		handoff.Status = StatusFailed
		handoff.mu.Unlock()
		m.cleanupPending(handoff.ID, resultCh)
		if ctx.Err() != nil {
			return handoff, ctx.Err()
		}
		return handoff, fmt.Errorf("handoff timeout")
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

	m.sendResult(resultCh, result)
	m.cleanupPending(handoff.ID, resultCh)
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
	FromAgentID             string
	ToAgentID               string
	Task                    Task
	Context                 HandoffContext
	Timeout                 time.Duration
	MaxRetries              int
	Wait                    bool
	OnHandoff               HandoffHook
	InputFilter             HandoffInputFilter
	Enabled                 *bool
	EnableFunc              HandoffEnabledFunc
	ToolNameOverride        string
	ToolDescriptionOverride string
	NestHistory             *bool
	HistoryMapper           HandoffHistoryMapper
}

func generateHandoffID() string {
	return fmt.Sprintf("hoff_%d", time.Now().UnixNano())
}

func defaultToolName(agentID string) string {
	s := strings.ToLower(strings.TrimSpace(agentID))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	if s == "" {
		s = "agent"
	}
	return "transfer_to_" + s
}

func defaultToolDescription(agentID string) string {
	if strings.TrimSpace(agentID) == "" {
		return "Handoff to another agent to handle the request."
	}
	return fmt.Sprintf("Handoff to the %s agent to handle the request.", agentID)
}

func defaultTransferMessage(agentID string) string {
	payload := map[string]string{"assistant": agentID}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"assistant":"%s"}`, agentID)
	}
	return string(raw)
}

func defaultNestHistoryValue(v *bool) bool {
	return v != nil && *v
}

func defaultHistoryMapper(history []types.Message) []types.Message {
	if len(history) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("<CONVERSATION HISTORY>\n")
	for _, msg := range history {
		b.WriteString(string(msg.Role))
		b.WriteString(": ")
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	b.WriteString("</CONVERSATION HISTORY>")
	return []types.Message{{
		Role:    types.RoleAssistant,
		Content: b.String(),
	}}
}

func composeNextMessages(data HandoffInputData) []types.Message {
	base := cloneMessages(data.InputHistory)
	next := data.InputMessages
	if len(next) == 0 {
		next = data.NewMessages
	}
	return append(base, cloneMessages(next)...)
}

func cloneMessages(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]types.Message, len(messages))
	copy(out, messages)
	return out
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (m *HandoffManager) finalizePending(handoffID string, result *HandoffResult) {
	m.mu.Lock()
	resultCh, exists := m.pending[handoffID]
	if exists {
		delete(m.pending, handoffID)
	}
	m.mu.Unlock()

	if !exists {
		return
	}
	if result != nil {
		m.sendResult(resultCh, result)
	}
	close(resultCh)
}

func (m *HandoffManager) cleanupPending(handoffID string, expected chan *HandoffResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	resultCh, exists := m.pending[handoffID]
	if !exists || resultCh != expected {
		return
	}
	delete(m.pending, handoffID)
	close(resultCh)
}

func (m *HandoffManager) sendResult(resultCh chan *HandoffResult, result *HandoffResult) {
	defer func() {
		// 有意拦截并吞掉 panic：向已关闭的 channel 发送会 panic，此处 recover 防止整个 handoff 流程崩溃。
		if r := recover(); r != nil {
			m.logger.Warn("sendResult recovered from panic (channel may be closed)", zap.Any("panic", r))
		}
	}()
	select {
	case resultCh <- result:
	default:
	}
}
