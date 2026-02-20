package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// approvalCounter is an atomic counter for generating unique approval IDs.
var approvalCounter int64

// HumanInLoopManager Human-in-the-Loop 管理器（生产级）
// 支持人工审批、反馈和干预
type HumanInLoopManager struct {
	approvalStore ApprovalStore
	eventBus      EventBus
	logger        *zap.Logger

	// 待审批请求
	pendingRequests map[string]*ApprovalRequest
	mu              sync.RWMutex
}

// ApprovalRequest 审批请求
type ApprovalRequest struct {
	ID          string                 `json:"id"`
	AgentID     string                 `json:"agent_id"`
	Type        ApprovalType           `json:"type"`
	Content     string                 `json:"content"`
	Context     map[string]any `json:"context"`
	Status      ApprovalStatus         `json:"status"`
	RequestedAt time.Time              `json:"requested_at"`
	RespondedAt time.Time              `json:"responded_at,omitempty"`
	Response    *ApprovalResponse      `json:"response,omitempty"`
	Timeout     time.Duration          `json:"timeout"`

	// 内部通道
	responseCh chan *ApprovalResponse
}

// ApprovalType 审批类型
type ApprovalType string

const (
	ApprovalTypeToolCall    ApprovalType = "tool_call"    // 工具调用审批
	ApprovalTypeOutput      ApprovalType = "output"       // 输出审批
	ApprovalTypeStateChange ApprovalType = "state_change" // 状态变更审批
	ApprovalTypeDataAccess  ApprovalType = "data_access"  // 数据访问审批
	ApprovalTypeCustom      ApprovalType = "custom"       // 自定义审批
)

// ApprovalStatus 审批状态
type ApprovalStatus string

const (
	ApprovalStatusPending   ApprovalStatus = "pending"
	ApprovalStatusApproved  ApprovalStatus = "approved"
	ApprovalStatusRejected  ApprovalStatus = "rejected"
	ApprovalStatusTimeout   ApprovalStatus = "timeout"
	ApprovalStatusCancelled ApprovalStatus = "cancelled"
)

// ApprovalResponse 审批响应
type ApprovalResponse struct {
	Approved bool                   `json:"approved"`
	Reason   string                 `json:"reason,omitempty"`
	Feedback string                 `json:"feedback,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ApprovalStore 审批存储接口
type ApprovalStore interface {
	Save(ctx context.Context, request *ApprovalRequest) error
	Load(ctx context.Context, requestID string) (*ApprovalRequest, error)
	List(ctx context.Context, agentID string, status ApprovalStatus, limit int) ([]*ApprovalRequest, error)
	Update(ctx context.Context, request *ApprovalRequest) error
}

// NewHumanInLoopManager 创建 Human-in-the-Loop 管理器
func NewHumanInLoopManager(store ApprovalStore, eventBus EventBus, logger *zap.Logger) *HumanInLoopManager {
	return &HumanInLoopManager{
		approvalStore:   store,
		eventBus:        eventBus,
		logger:          logger.With(zap.String("component", "human_in_loop")),
		pendingRequests: make(map[string]*ApprovalRequest),
	}
}

// RequestApproval 请求人工审批
func (m *HumanInLoopManager) RequestApproval(ctx context.Context, agentID string, approvalType ApprovalType, content string, timeout time.Duration) (*ApprovalResponse, error) {
	request := &ApprovalRequest{
		ID:          generateApprovalID(),
		AgentID:     agentID,
		Type:        approvalType,
		Content:     content,
		Status:      ApprovalStatusPending,
		RequestedAt: time.Now(),
		Timeout:     timeout,
		responseCh:  make(chan *ApprovalResponse, 1),
	}

	m.logger.Info("requesting approval",
		zap.String("request_id", request.ID),
		zap.String("agent_id", agentID),
		zap.String("type", string(approvalType)),
	)

	// 保存请求
	if err := m.approvalStore.Save(ctx, request); err != nil {
		return nil, fmt.Errorf("failed to save approval request: %w", err)
	}

	// 添加到待审批列表
	m.mu.Lock()
	m.pendingRequests[request.ID] = request
	m.mu.Unlock()

	// 发布审批请求事件
	if m.eventBus != nil {
		m.eventBus.Publish(&ApprovalRequestedEvent{
			RequestID:    request.ID,
			AgentID:      agentID,
			ApprovalType: approvalType,
			Content:      content,
			Timestamp_:   time.Now(),
		})
	}

	// 等待响应或超时
	select {
	case response := <-request.responseCh:
		return response, nil
	case <-time.After(timeout):
		m.mu.Lock()
		request.Status = ApprovalStatusTimeout
		delete(m.pendingRequests, request.ID)
		m.mu.Unlock()

		m.approvalStore.Update(ctx, request)

		m.logger.Warn("approval request timeout",
			zap.String("request_id", request.ID),
		)

		return &ApprovalResponse{
			Approved: false,
			Reason:   "timeout",
		}, fmt.Errorf("approval timeout")
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.pendingRequests, request.ID)
		m.mu.Unlock()
		return nil, ctx.Err()
	}
}

// RespondToApproval 响应审批请求
func (m *HumanInLoopManager) RespondToApproval(ctx context.Context, requestID string, response *ApprovalResponse) error {
	m.mu.Lock()
	request, ok := m.pendingRequests[requestID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("approval request not found: %s", requestID)
	}

	// Check that the request is still pending (could have timed out or been cancelled)
	if request.Status != ApprovalStatusPending {
		m.mu.Unlock()
		return fmt.Errorf("approval request %s is no longer pending (status: %s)", requestID, request.Status)
	}

	// Update request state while holding the lock
	request.Response = response
	request.RespondedAt = time.Now()
	if response.Approved {
		request.Status = ApprovalStatusApproved
	} else {
		request.Status = ApprovalStatusRejected
	}
	delete(m.pendingRequests, requestID)
	m.mu.Unlock()

	m.logger.Info("responding to approval",
		zap.String("request_id", requestID),
		zap.Bool("approved", response.Approved),
	)

	// 保存更新
	if err := m.approvalStore.Update(ctx, request); err != nil {
		return fmt.Errorf("failed to update approval request: %w", err)
	}

	// 发送响应 (channel send outside lock to avoid potential deadlock)
	select {
	case request.responseCh <- response:
	default:
		m.logger.Warn("response channel full or closed")
	}

	// 发布审批响应事件
	if m.eventBus != nil {
		m.eventBus.Publish(&ApprovalRespondedEvent{
			RequestID:  requestID,
			Approved:   response.Approved,
			Reason:     response.Reason,
			Timestamp_: time.Now(),
		})
	}

	return nil
}

// GetPendingRequests 获取待审批请求
func (m *HumanInLoopManager) GetPendingRequests(agentID string) []*ApprovalRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	requests := make([]*ApprovalRequest, 0)
	for _, req := range m.pendingRequests {
		if agentID == "" || req.AgentID == agentID {
			requests = append(requests, req)
		}
	}

	return requests
}

// CancelApproval 取消审批请求
func (m *HumanInLoopManager) CancelApproval(ctx context.Context, requestID string) error {
	m.mu.Lock()
	request, ok := m.pendingRequests[requestID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("approval request not found: %s", requestID)
	}

	// Only cancel if still pending
	if request.Status != ApprovalStatusPending {
		m.mu.Unlock()
		return fmt.Errorf("approval request %s is no longer pending (status: %s)", requestID, request.Status)
	}

	request.Status = ApprovalStatusCancelled
	delete(m.pendingRequests, requestID)
	m.mu.Unlock()

	// Send nil on the channel to unblock any waiter, using select+default to avoid
	// blocking if the channel is already full or nobody is listening.
	select {
	case request.responseCh <- nil:
	default:
	}

	m.logger.Info("approval request cancelled", zap.String("request_id", requestID))

	return nil
}

// generateApprovalID 生成审批 ID
func generateApprovalID() string {
	id := atomic.AddInt64(&approvalCounter, 1)
	return fmt.Sprintf("approval_%d_%d", time.Now().UnixNano(), id)
}

// ====== 审批策略 ======

// ApprovalPolicy 审批策略
type ApprovalPolicy interface {
	RequiresApproval(ctx context.Context, agentID string, action Action) bool
}

// Action 需要审批的动作
type Action struct {
	Type     string                 `json:"type"`
	Content  string                 `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

// DefaultApprovalPolicy 默认审批策略
type DefaultApprovalPolicy struct {
	// 需要审批的工具列表
	RequireApprovalTools []string

	// 需要审批的状态变更
	RequireApprovalStates []State

	// 总是需要审批
	AlwaysRequireApproval bool
}

// RequiresApproval 检查是否需要审批
func (p *DefaultApprovalPolicy) RequiresApproval(ctx context.Context, agentID string, action Action) bool {
	if p.AlwaysRequireApproval {
		return true
	}

	// 检查工具调用
	if action.Type == "tool_call" {
		toolName, ok := action.Metadata["tool_name"].(string)
		if ok {
			for _, t := range p.RequireApprovalTools {
				if t == toolName {
					return true
				}
			}
		}
	}

	// 检查状态变更
	if action.Type == "state_change" {
		state, ok := action.Metadata["to_state"].(State)
		if ok {
			for _, s := range p.RequireApprovalStates {
				if s == state {
					return true
				}
			}
		}
	}

	return false
}

// ====== 事件定义 ======

// ApprovalRequestedEvent 审批请求事件
type ApprovalRequestedEvent struct {
	RequestID    string
	AgentID      string
	ApprovalType ApprovalType
	Content      string
	Timestamp_   time.Time
}

func (e *ApprovalRequestedEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *ApprovalRequestedEvent) Type() EventType      { return EventApprovalRequested }

// ApprovalRespondedEvent 审批响应事件
type ApprovalRespondedEvent struct {
	RequestID  string
	Approved   bool
	Reason     string
	Timestamp_ time.Time
}

func (e *ApprovalRespondedEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *ApprovalRespondedEvent) Type() EventType      { return EventApprovalResponded }

// ====== 内存存储实现 ======

// InMemoryApprovalStore 内存审批存储
type InMemoryApprovalStore struct {
	requests map[string]*ApprovalRequest
	mu       sync.RWMutex
}

// NewInMemoryApprovalStore 创建内存审批存储
func NewInMemoryApprovalStore() *InMemoryApprovalStore {
	return &InMemoryApprovalStore{
		requests: make(map[string]*ApprovalRequest),
	}
}

func (s *InMemoryApprovalStore) Save(ctx context.Context, request *ApprovalRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[request.ID] = request
	return nil
}

func (s *InMemoryApprovalStore) Load(ctx context.Context, requestID string) (*ApprovalRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	request, ok := s.requests[requestID]
	if !ok {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	return request, nil
}

func (s *InMemoryApprovalStore) List(ctx context.Context, agentID string, status ApprovalStatus, limit int) ([]*ApprovalRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	requests := make([]*ApprovalRequest, 0)
	for _, req := range s.requests {
		if (agentID == "" || req.AgentID == agentID) && (status == "" || req.Status == status) {
			requests = append(requests, req)
			if len(requests) >= limit {
				break
			}
		}
	}

	return requests, nil
}

func (s *InMemoryApprovalStore) Update(ctx context.Context, request *ApprovalRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[request.ID] = request
	return nil
}
