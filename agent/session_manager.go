package agent

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ExecutionSession 跟踪一个活跃的流式执行
type ExecutionSession struct {
	ID         string           `json:"id"`
	AgentID    string           `json:"agent_id"`
	SteeringCh *SteeringChannel `json:"-"`
	CreatedAt  time.Time        `json:"created_at"`
}

// Status 返回当前会话状态（单一状态源：SteeringChannel.IsClosed）
func (s *ExecutionSession) Status() string {
	if s.SteeringCh.IsClosed() {
		return "completed"
	}
	return "running"
}

// Complete 标记会话为已完成（关闭 steering channel）
func (s *ExecutionSession) Complete() {
	s.SteeringCh.Close()
}

// IsRunning 检查会话是否仍在运行
func (s *ExecutionSession) IsRunning() bool {
	return !s.SteeringCh.IsClosed()
}

// SessionManager 管理活跃的流式执行会话（内存 map + 自动过期清理）
type SessionManager struct {
	sessions sync.Map
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSessionManager 创建会话管理器并启动后台清理 goroutine
func NewSessionManager() *SessionManager {
	m := &SessionManager{
		stopCh: make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// Create 创建一个新的执行会话
func (m *SessionManager) Create(agentID string) *ExecutionSession {
	sess := &ExecutionSession{
		ID:         fmt.Sprintf("exec_%s", uuid.New().String()[:12]),
		AgentID:    agentID,
		SteeringCh: NewSteeringChannel(4),
		CreatedAt:  time.Now(),
	}
	m.sessions.Store(sess.ID, sess)
	return sess
}

// Get 根据 ID 获取会话
func (m *SessionManager) Get(id string) (*ExecutionSession, bool) {
	v, ok := m.sessions.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*ExecutionSession), true
}

// Remove 移除会话并关闭其 steering channel
func (m *SessionManager) Remove(id string) {
	if v, loaded := m.sessions.LoadAndDelete(id); loaded {
		v.(*ExecutionSession).Complete()
	}
}

// Cleanup 清理过期会话：已完成的超过 maxAge 清理，活跃的不强制终止
func (m *SessionManager) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	m.sessions.Range(func(key, value any) bool {
		sess := value.(*ExecutionSession)
		if sess.CreatedAt.Before(cutoff) && !sess.IsRunning() {
			m.sessions.Delete(key)
		}
		return true
	})
}

// Stop 停止后台清理 goroutine
func (m *SessionManager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

// cleanupLoop 每 60s 清理超过 30min 的过期会话
func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.Cleanup(30 * time.Minute)
		case <-m.stopCh:
			return
		}
	}
}
