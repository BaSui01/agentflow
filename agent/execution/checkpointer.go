// 软件包执行为代理状态提供了可插接的持久后端。
package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("checkpoint not found")
	ErrInvalidThread = errors.New("invalid thread id")
	ErrStorageFull   = errors.New("storage capacity exceeded")
)

// 检查点代表一个时间点的保存状态.
type Checkpoint struct {
	ID        string         `json:"id"`
	ThreadID  string         `json:"thread_id"`
	ParentID  string         `json:"parent_id,omitempty"`
	State     map[string]any `json:"state"`
	Metadata  Metadata       `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

// 元数据包含检查站元数据。
type Metadata struct {
	Step   int               `json:"step"`
	NodeID string            `json:"node_id,omitempty"`
	Tags   []string          `json:"tags,omitempty"`
	Custom map[string]string `json:"custom,omitempty"`
}

// 检查站定义了检查站存储后端的接口.
type Checkpointer interface {
	// 设取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取出取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取出取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取取
	Put(ctx context.Context, cp *Checkpoint) error

	// 以身份证取回检查站
	Get(ctx context.Context, threadID, checkpointID string) (*Checkpoint, error)

	// GetLatest 为线索检索最近的检查点 。
	GetLatest(ctx context.Context, threadID string) (*Checkpoint, error)

	// 按创建时间顺序列出返回检查点。
	List(ctx context.Context, threadID string, opts ListOptions) ([]*Checkpoint, error)

	// 删除一个检查站。
	Delete(ctx context.Context, threadID, checkpointID string) error

	// 删除 Thread 为线索删除所有检查点 。
	DeleteThread(ctx context.Context, threadID string) error
}

// 列表选项配置检查站列表 。
type ListOptions struct {
	Limit  int       `json:"limit"`
	Before time.Time `json:"before,omitempty"`
	After  time.Time `json:"after,omitempty"`
}

// 内存检查点器执行内存检查点存储.
type MemoryCheckpointer struct {
	threads map[string][]*Checkpoint
	mu      sync.RWMutex
	maxSize int
}

// 新记忆检查点创建了新的记忆检查点.
func NewMemoryCheckpointer(maxSize int) *MemoryCheckpointer {
	return &MemoryCheckpointer{
		threads: make(map[string][]*Checkpoint),
		maxSize: maxSize,
	}
}

func (m *MemoryCheckpointer) Put(ctx context.Context, cp *Checkpoint) error {
	if cp.ThreadID == "" {
		return ErrInvalidThread
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cp.CreatedAt = time.Now()
	m.threads[cp.ThreadID] = append(m.threads[cp.ThreadID], cp)

	// 每个线程执行最大尺寸
	if m.maxSize > 0 && len(m.threads[cp.ThreadID]) > m.maxSize {
		m.threads[cp.ThreadID] = m.threads[cp.ThreadID][1:]
	}

	return nil
}

func (m *MemoryCheckpointer) Get(ctx context.Context, threadID, checkpointID string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cps, ok := m.threads[threadID]
	if !ok {
		return nil, ErrNotFound
	}

	for _, cp := range cps {
		if cp.ID == checkpointID {
			return cp, nil
		}
	}

	return nil, ErrNotFound
}

func (m *MemoryCheckpointer) GetLatest(ctx context.Context, threadID string) (*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cps, ok := m.threads[threadID]
	if !ok || len(cps) == 0 {
		return nil, ErrNotFound
	}

	return cps[len(cps)-1], nil
}

func (m *MemoryCheckpointer) List(ctx context.Context, threadID string, opts ListOptions) ([]*Checkpoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cps, ok := m.threads[threadID]
	if !ok {
		return []*Checkpoint{}, nil
	}

	result := make([]*Checkpoint, 0, len(cps))
	for _, cp := range cps {
		if !opts.Before.IsZero() && cp.CreatedAt.After(opts.Before) {
			continue
		}
		if !opts.After.IsZero() && cp.CreatedAt.Before(opts.After) {
			continue
		}
		result = append(result, cp)
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
	}

	return result, nil
}

func (m *MemoryCheckpointer) Delete(ctx context.Context, threadID, checkpointID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cps, ok := m.threads[threadID]
	if !ok {
		return ErrNotFound
	}

	for i, cp := range cps {
		if cp.ID == checkpointID {
			m.threads[threadID] = append(cps[:i], cps[i+1:]...)
			return nil
		}
	}

	return ErrNotFound
}

func (m *MemoryCheckpointer) DeleteThread(ctx context.Context, threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.threads, threadID)
	return nil
}

// Serde定义了序列化/去序列化接口.
type Serde interface {
	Serialize(v any) ([]byte, error)
	Deserialize(data []byte, v any) error
}

// JSONSerde执行JSON系列化.
type JSONSerde struct{}

func (JSONSerde) Serialize(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONSerde) Deserialize(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// 检查器Config配置检查器行为.
type CheckpointerConfig struct {
	MaxCheckpointsPerThread int           `json:"max_checkpoints_per_thread"`
	TTL                     time.Duration `json:"ttl"`
	Serde                   Serde         `json:"-"`
}

// 默认检查指定符 Config 返回合理的默认值 。
func DefaultCheckpointerConfig() CheckpointerConfig {
	return CheckpointerConfig{
		MaxCheckpointsPerThread: 100,
		TTL:                     24 * time.Hour,
		Serde:                   JSONSerde{},
	}
}

// ThreadManager通过检查站管理多条线程.
type ThreadManager struct {
	checkpointer Checkpointer
	config       CheckpointerConfig
	mu           sync.RWMutex
}

// NewThreadManager 创建了新线程管理器.
func NewThreadManager(cp Checkpointer, config CheckpointerConfig) *ThreadManager {
	return &ThreadManager{
		checkpointer: cp,
		config:       config,
	}
}

// 保存状态保存为线程 。
func (tm *ThreadManager) SaveState(ctx context.Context, threadID string, state map[string]any, meta Metadata) (*Checkpoint, error) {
	cp := &Checkpoint{
		ID:       generateCheckpointID(),
		ThreadID: threadID,
		State:    state,
		Metadata: meta,
	}

	// 从最近的检查站获取父母身份
	if latest, err := tm.checkpointer.GetLatest(ctx, threadID); err == nil {
		cp.ParentID = latest.ID
	}

	if err := tm.checkpointer.Put(ctx, cp); err != nil {
		return nil, err
	}

	return cp, nil
}

// 装入状态为线程加载最新状态 。
func (tm *ThreadManager) LoadState(ctx context.Context, threadID string) (map[string]any, error) {
	cp, err := tm.checkpointer.GetLatest(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return cp.State, nil
}

// 倒滚回某个特定的检查站。
func (tm *ThreadManager) Rollback(ctx context.Context, threadID, checkpointID string) (*Checkpoint, error) {
	return tm.checkpointer.Get(ctx, threadID, checkpointID)
}

// GetHistory 返回检查点历史为一线程 。
func (tm *ThreadManager) GetHistory(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	return tm.checkpointer.List(ctx, threadID, ListOptions{Limit: limit})
}

func generateCheckpointID() string {
	return time.Now().Format("20060102150405.000000")
}
