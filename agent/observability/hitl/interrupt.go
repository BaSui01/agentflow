package hitl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// 中断Type定义了工作流程中断的类型.
type InterruptType string

const (
	InterruptTypeApproval   InterruptType = "approval"
	InterruptTypeInput      InterruptType = "input"
	InterruptTypeReview     InterruptType = "review"
	InterruptTypeBreakpoint InterruptType = "breakpoint"
	InterruptTypeError      InterruptType = "error"
)

// 中断状态代表中断状态.
type InterruptStatus string

const (
	InterruptStatusPending  InterruptStatus = "pending"
	InterruptStatusResolved InterruptStatus = "resolved"
	InterruptStatusRejected InterruptStatus = "rejected"
	InterruptStatusTimeout  InterruptStatus = "timeout"
	InterruptStatusCanceled InterruptStatus = "canceled"
)

// 中断代表工作流程中断点.
type Interrupt struct {
	ID           string          `json:"id"`
	WorkflowID   string          `json:"workflow_id"`
	NodeID       string          `json:"node_id"`
	Type         InterruptType   `json:"type"`
	Status       InterruptStatus `json:"status"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Data         any             `json:"data,omitempty"`
	Options      []Option        `json:"options,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	Response     *Response       `json:"response,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	ResolvedAt   *time.Time      `json:"resolved_at,omitempty"`
	Timeout      time.Duration   `json:"timeout"`
	CheckpointID string          `json:"checkpoint_id,omitempty"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
}

// 备选办法是可选择的核准中断的备选办法。
type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default,omitempty"`
}

// 反应代表了人类对中断的反应。
type Response struct {
	OptionID  string         `json:"option_id,omitempty"`
	Input     any            `json:"input,omitempty"`
	Comment   string         `json:"comment,omitempty"`
	Approved  bool           `json:"approved"`
	Timestamp time.Time      `json:"timestamp"`
	UserID    string         `json:"user_id,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// InterruptStore定义了中断的存储接口.
//
// TODO(concurrency): Persistence implementations (e.g. PostgreSQL) should support WithTransaction
// for atomic Save+Update flows. CreateInterrupt does Save then later Update; ResolveInterrupt does Update.
// InMemoryInterruptStore does not need transactions.
type InterruptStore interface {
	Save(ctx context.Context, interrupt *Interrupt) error
	Load(ctx context.Context, interruptID string) (*Interrupt, error)
	List(ctx context.Context, workflowID string, status InterruptStatus) ([]*Interrupt, error)
	Update(ctx context.Context, interrupt *Interrupt) error
}

// 中断汉德勒处理中断事件.
type InterruptHandler func(ctx context.Context, interrupt *Interrupt) error

// 中断管理者管理工作流程中断 。
type InterruptManager struct {
	store    InterruptStore
	logger   *zap.Logger
	handlers map[InterruptType][]InterruptHandler
	named    map[InterruptType]map[string]struct{}
	pending  map[string]*pendingInterrupt
	mu       sync.RWMutex
}

type pendingInterrupt struct {
	interrupt   *Interrupt
	responseCh  chan *Response
	cancelFn    context.CancelFunc
	timeoutCtx  context.Context
	resolveOnce sync.Once
}

// 新干扰管理器创建了新的中断管理器 。
func NewInterruptManager(store InterruptStore, logger *zap.Logger) *InterruptManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InterruptManager{
		store:    store,
		logger:   logger.With(zap.String("component", "interrupt_manager")),
		handlers: make(map[InterruptType][]InterruptHandler),
		named:    make(map[InterruptType]map[string]struct{}),
		pending:  make(map[string]*pendingInterrupt),
	}
}

// 登记 Handler 为中断类型登记处理器 。
func (m *InterruptManager) RegisterHandler(interruptType InterruptType, handler InterruptHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[interruptType] = append(m.handlers[interruptType], handler)
}

// RegisterNamedHandler registers a handler only once for the given interrupt
// type and stable name. It returns true when a new handler was added.
func (m *InterruptManager) RegisterNamedHandler(
	interruptType InterruptType,
	name string,
	handler InterruptHandler,
) bool {
	if strings.TrimSpace(name) == "" {
		m.RegisterHandler(interruptType, handler)
		return true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.named[interruptType] == nil {
		m.named[interruptType] = make(map[string]struct{})
	}
	if _, exists := m.named[interruptType][name]; exists {
		return false
	}

	m.handlers[interruptType] = append(m.handlers[interruptType], handler)
	m.named[interruptType][name] = struct{}{}
	return true
}

// 创建中断创建并等待中断解决 。
func (m *InterruptManager) CreateInterrupt(ctx context.Context, opts InterruptOptions) (*Response, error) {
	pending, err := m.createPendingInterrupt(ctx, opts, true)
	if err != nil {
		return nil, err
	}

	// 等待回应
	select {
	case response := <-pending.responseCh:
		return response, nil
	case <-ctx.Done():
		_ = m.CancelInterrupt(context.Background(), pending.interrupt.ID)
		return nil, ctx.Err()
	case <-pending.timeoutCtx.Done():
		m.handleTimeout(context.Background(), pending.interrupt)
		return nil, fmt.Errorf("interrupt timeout: %s", pending.interrupt.ID)
	}
}

// CreatePendingInterrupt creates an interrupt and registers it as pending without blocking for a response.
func (m *InterruptManager) CreatePendingInterrupt(ctx context.Context, opts InterruptOptions) (*Interrupt, error) {
	pending, err := m.createPendingInterrupt(ctx, opts, false)
	if err != nil {
		return nil, err
	}
	return pending.interrupt, nil
}

func (m *InterruptManager) createPendingInterrupt(
	ctx context.Context,
	opts InterruptOptions,
	bindToParent bool,
) (*pendingInterrupt, error) {
	interrupt := &Interrupt{
		ID:          generateInterruptID(),
		WorkflowID:  opts.WorkflowID,
		NodeID:      opts.NodeID,
		Type:        opts.Type,
		Status:      InterruptStatusPending,
		Title:       opts.Title,
		Description: opts.Description,
		Data:        opts.Data,
		Options:     opts.Options,
		InputSchema: opts.InputSchema,
		CreatedAt:   time.Now(),
		Timeout:     opts.Timeout,
		Metadata:    opts.Metadata,
	}

	if interrupt.Timeout == 0 {
		interrupt.Timeout = 24 * time.Hour
	}

	m.logger.Info("creating interrupt",
		zap.String("id", interrupt.ID),
		zap.String("workflow_id", interrupt.WorkflowID),
		zap.String("type", string(interrupt.Type)),
	)

	if err := m.store.Save(ctx, interrupt); err != nil {
		return nil, fmt.Errorf("failed to save interrupt: %w", err)
	}

	timeoutParent := context.Background()
	if bindToParent {
		timeoutParent = ctx
	}
	interruptCtx, cancel := context.WithTimeout(timeoutParent, interrupt.Timeout)
	pending := &pendingInterrupt{
		interrupt:  interrupt,
		responseCh: make(chan *Response, 1),
		cancelFn:   cancel,
		timeoutCtx: interruptCtx,
	}

	m.mu.Lock()
	m.pending[interrupt.ID] = pending
	m.mu.Unlock()

	// 通知处理者（必须在 pending 注册后，避免处理器提前 Resolve 产生 not found）
	m.notifyHandlers(ctx, interrupt)

	if !bindToParent {
		go func(waitCtx context.Context, interrupt *Interrupt) {
			<-waitCtx.Done()
			if waitCtx.Err() != context.DeadlineExceeded {
				return
			}
			m.handleTimeout(context.Background(), interrupt)
		}(interruptCtx, interrupt)
	}

	return pending, nil
}

// 解析中断解决待决中断 。
func (m *InterruptManager) ResolveInterrupt(ctx context.Context, interruptID string, response *Response) error {
	m.mu.Lock()
	pending, ok := m.pending[interruptID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("interrupt not found or already resolved: %s", interruptID)
	}
	delete(m.pending, interruptID)
	m.mu.Unlock()

	interrupt := pending.interrupt
	interrupt.Response = response
	interrupt.Status = InterruptStatusResolved
	if response.Approved {
		interrupt.Status = InterruptStatusResolved
	} else {
		interrupt.Status = InterruptStatusRejected
	}
	now := time.Now()
	interrupt.ResolvedAt = &now
	response.Timestamp = now

	m.logger.Info("resolving interrupt",
		zap.String("id", interruptID),
		zap.Bool("approved", response.Approved),
	)

	if err := m.store.Update(ctx, interrupt); err != nil {
		return fmt.Errorf("failed to update interrupt: %w", err)
	}

	// 发送对等待goroutine的响应
	pending.resolveOnce.Do(func() {
		select {
		case pending.responseCh <- response:
		default:
		}
		pending.cancelFn()
	})
	return nil
}

// 取消中断取消待决中断 。
func (m *InterruptManager) CancelInterrupt(ctx context.Context, interruptID string) error {
	m.mu.Lock()
	pending, ok := m.pending[interruptID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("interrupt not found: %s", interruptID)
	}
	delete(m.pending, interruptID)
	m.mu.Unlock()

	pending.interrupt.Status = InterruptStatusCanceled
	now := time.Now()
	pending.interrupt.ResolvedAt = &now

	if err := m.store.Update(ctx, pending.interrupt); err != nil {
		return err
	}

	pending.resolveOnce.Do(func() {
		pending.cancelFn()
		close(pending.responseCh)
	})

	m.logger.Info("interrupt canceled", zap.String("id", interruptID))
	return nil
}

// 获得待定 中断返回工作流程中所有待处理中断 。
func (m *InterruptManager) GetPendingInterrupts(workflowID string) []*Interrupt {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*Interrupt
	for _, p := range m.pending {
		if workflowID == "" || p.interrupt.WorkflowID == workflowID {
			results = append(results, p.interrupt)
		}
	}
	return results
}

// GetInterrupt loads a single interrupt from the backing store.
func (m *InterruptManager) GetInterrupt(ctx context.Context, interruptID string) (*Interrupt, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("interrupt store is not configured")
	}
	return m.store.Load(ctx, interruptID)
}

// ListInterrupts lists interrupts from the backing store filtered by workflow and status.
func (m *InterruptManager) ListInterrupts(
	ctx context.Context,
	workflowID string,
	status InterruptStatus,
) ([]*Interrupt, error) {
	if m == nil || m.store == nil {
		return nil, fmt.Errorf("interrupt store is not configured")
	}
	return m.store.List(ctx, workflowID, status)
}

func (m *InterruptManager) notifyHandlers(ctx context.Context, interrupt *Interrupt) {
	m.mu.RLock()
	handlers := m.handlers[interrupt.Type]
	m.mu.RUnlock()

	for _, handler := range handlers {
		go func(h InterruptHandler) {
			if err := h(ctx, interrupt); err != nil {
				m.logger.Error("handler error", zap.Error(err))
			}
		}(handler)
	}
}

func (m *InterruptManager) handleTimeout(ctx context.Context, interrupt *Interrupt) {
	interrupt.Status = InterruptStatusTimeout
	now := time.Now()
	interrupt.ResolvedAt = &now

	m.mu.Lock()
	delete(m.pending, interrupt.ID)
	m.mu.Unlock()

	if err := m.store.Update(ctx, interrupt); err != nil {
		m.logger.Error("failed to persist timeout interrupt", zap.Error(err), zap.String("id", interrupt.ID))
	}
	m.logger.Warn("interrupt timeout", zap.String("id", interrupt.ID))
}

// 中断选项配置中断创建 。
type InterruptOptions struct {
	WorkflowID   string
	NodeID       string
	Type         InterruptType
	Title        string
	Description  string
	Data         any
	Options      []Option
	InputSchema  json.RawMessage
	Timeout      time.Duration
	CheckpointID string
	Metadata     map[string]any
}

func generateInterruptID() string {
	return "int_" + uuid.New().String()
}

// 在MemoryInterruptStore中为中断提供内存.
type InMemoryInterruptStore struct {
	interrupts map[string]*Interrupt
	mu         sync.RWMutex
}

// New InMemory InterruptStore 创建了新的内存中断商店.
func NewInMemoryInterruptStore() *InMemoryInterruptStore {
	return &InMemoryInterruptStore{
		interrupts: make(map[string]*Interrupt),
	}
}

func (s *InMemoryInterruptStore) Save(ctx context.Context, interrupt *Interrupt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupts[interrupt.ID] = interrupt
	return nil
}

func (s *InMemoryInterruptStore) Load(ctx context.Context, interruptID string) (*Interrupt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	interrupt, ok := s.interrupts[interruptID]
	if !ok {
		return nil, fmt.Errorf("interrupt not found: %s", interruptID)
	}
	return interrupt, nil
}

func (s *InMemoryInterruptStore) List(ctx context.Context, workflowID string, status InterruptStatus) ([]*Interrupt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Interrupt
	for _, interrupt := range s.interrupts {
		if (workflowID == "" || interrupt.WorkflowID == workflowID) &&
			(status == "" || interrupt.Status == status) {
			results = append(results, interrupt)
		}
	}
	return results, nil
}

func (s *InMemoryInterruptStore) Update(ctx context.Context, interrupt *Interrupt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interrupts[interrupt.ID] = interrupt
	return nil
}
