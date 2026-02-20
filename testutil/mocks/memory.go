// MockMemoryManager 的测试模拟实现。
//
// 用于消息存储、检索与状态验证相关测试。
package mocks

import (
	"context"
	"sync"

	"github.com/BaSui01/agentflow/types"
)

// --- MockMemoryManager 结构 ---

// MockMemoryManager 是记忆管理器的模拟实现
type MockMemoryManager struct {
	mu sync.RWMutex

	// 消息存储
	messages []types.Message

	// 配置
	maxMessages int
	tokenLimit  int

	// 错误注入
	addErr    error
	getErr    error
	clearErr  error
	searchErr error

	// 调用记录
	addCalls    int
	getCalls    int
	clearCalls  int
	searchCalls int

	// 搜索结果
	searchResults []types.Message
}

// --- 构造函数和 Builder 方法 ---

// NewMockMemoryManager 创建新的 MockMemoryManager
func NewMockMemoryManager() *MockMemoryManager {
	return &MockMemoryManager{
		messages:      []types.Message{},
		maxMessages:   100,
		tokenLimit:    8000,
		searchResults: []types.Message{},
	}
}

// WithMaxMessages 设置最大消息数
func (m *MockMemoryManager) WithMaxMessages(max int) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxMessages = max
	return m
}

// WithTokenLimit 设置 Token 限制
func (m *MockMemoryManager) WithTokenLimit(limit int) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenLimit = limit
	return m
}

// WithMessages 预设消息
func (m *MockMemoryManager) WithMessages(messages []types.Message) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append([]types.Message{}, messages...)
	return m
}

// WithAddError 设置 Add 方法的错误
func (m *MockMemoryManager) WithAddError(err error) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addErr = err
	return m
}

// WithGetError 设置 Get 方法的错误
func (m *MockMemoryManager) WithGetError(err error) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getErr = err
	return m
}

// WithClearError 设置 Clear 方法的错误
func (m *MockMemoryManager) WithClearError(err error) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clearErr = err
	return m
}

// WithSearchResults 设置搜索结果
func (m *MockMemoryManager) WithSearchResults(results []types.Message) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchResults = append([]types.Message{}, results...)
	return m
}

// WithSearchError 设置 Search 方法的错误
func (m *MockMemoryManager) WithSearchError(err error) *MockMemoryManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.searchErr = err
	return m
}

// --- MemoryManager 接口实现 ---

// Add 添加消息到记忆
func (m *MockMemoryManager) Add(ctx context.Context, msg types.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addCalls++

	if m.addErr != nil {
		return m.addErr
	}

	m.messages = append(m.messages, msg)

	// 如果超过最大消息数，移除最早的消息
	if len(m.messages) > m.maxMessages {
		m.messages = m.messages[len(m.messages)-m.maxMessages:]
	}

	return nil
}

// AddBatch 批量添加消息
func (m *MockMemoryManager) AddBatch(ctx context.Context, msgs []types.Message) error {
	for _, msg := range msgs {
		if err := m.Add(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// GetAll 获取所有消息
func (m *MockMemoryManager) GetAll(ctx context.Context) ([]types.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.getCalls++

	if m.getErr != nil {
		return nil, m.getErr
	}

	return append([]types.Message{}, m.messages...), nil
}

// GetRecent 获取最近 N 条消息
func (m *MockMemoryManager) GetRecent(ctx context.Context, n int) ([]types.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.getCalls++

	if m.getErr != nil {
		return nil, m.getErr
	}

	if n >= len(m.messages) {
		return append([]types.Message{}, m.messages...), nil
	}

	return append([]types.Message{}, m.messages[len(m.messages)-n:]...), nil
}

// Clear 清空记忆
func (m *MockMemoryManager) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clearCalls++

	if m.clearErr != nil {
		return m.clearErr
	}

	m.messages = []types.Message{}
	return nil
}

// Search 搜索相关消息（向量搜索模拟）
func (m *MockMemoryManager) Search(ctx context.Context, query string, topK int) ([]types.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.searchCalls++

	if m.searchErr != nil {
		return nil, m.searchErr
	}

	// 返回预设的搜索结果
	if len(m.searchResults) > 0 {
		if topK >= len(m.searchResults) {
			return append([]types.Message{}, m.searchResults...), nil
		}
		return append([]types.Message{}, m.searchResults[:topK]...), nil
	}

	// 默认返回最近的消息
	return m.GetRecent(ctx, topK)
}

// Count 返回消息数量
func (m *MockMemoryManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages)
}

// --- 查询方法 ---

// GetAddCalls 获取 Add 调用次数
func (m *MockMemoryManager) GetAddCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.addCalls
}

// GetGetCalls 获取 Get 调用次数
func (m *MockMemoryManager) GetGetCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getCalls
}

// GetClearCalls 获取 Clear 调用次数
func (m *MockMemoryManager) GetClearCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clearCalls
}

// GetSearchCalls 获取 Search 调用次数
func (m *MockMemoryManager) GetSearchCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.searchCalls
}

// Reset 重置所有状态
func (m *MockMemoryManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = []types.Message{}
	m.addCalls = 0
	m.getCalls = 0
	m.clearCalls = 0
	m.searchCalls = 0
	m.addErr = nil
	m.getErr = nil
	m.clearErr = nil
	m.searchErr = nil
}

// --- 预设 MemoryManager 工厂 ---

// NewEmptyMemory 创建空的记忆管理器
func NewEmptyMemory() *MockMemoryManager {
	return NewMockMemoryManager()
}

// NewPrefilledMemory 创建预填充消息的记忆管理器
func NewPrefilledMemory(messages []types.Message) *MockMemoryManager {
	return NewMockMemoryManager().WithMessages(messages)
}

// NewLimitedMemory 创建有限制的记忆管理器
func NewLimitedMemory(maxMessages int) *MockMemoryManager {
	return NewMockMemoryManager().WithMaxMessages(maxMessages)
}

// NewErrorMemory 创建总是返回错误的记忆管理器
func NewErrorMemory(err error) *MockMemoryManager {
	return NewMockMemoryManager().
		WithAddError(err).
		WithGetError(err).
		WithClearError(err)
}
