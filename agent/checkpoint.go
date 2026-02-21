package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Checkpoint Agent 执行检查点（基于 LangGraph 2026 标准）
type Checkpoint struct {
	ID        string                 `json:"id"`
	ThreadID  string                 `json:"thread_id"` // 会话线程 ID
	AgentID   string                 `json:"agent_id"`
	Version   int                    `json:"version"` // 版本号（线程内递增）
	State     State                  `json:"state"`
	Messages  []CheckpointMessage    `json:"messages"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	ParentID  string                 `json:"parent_id,omitempty"` // 父检查点 ID

	// ExecutionContext 工作流执行上下文
	ExecutionContext *ExecutionContext `json:"execution_context,omitempty"`
}

// CheckpointMessage 检查点消息
type CheckpointMessage struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	ToolCalls []CheckpointToolCall   `json:"tool_calls,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// CheckpointToolCall 工具调用记录
type CheckpointToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ExecutionContext 工作流执行上下文
type ExecutionContext struct {
	WorkflowID  string                 `json:"workflow_id,omitempty"`
	CurrentNode string                 `json:"current_node,omitempty"`
	NodeResults map[string]any `json:"node_results,omitempty"`
	Variables   map[string]any `json:"variables,omitempty"`
}

// CheckpointVersion 检查点版本元数据
type CheckpointVersion struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	State     State     `json:"state"`
	Summary   string    `json:"summary"`
}

// CheckpointStore 检查点存储接口（Agent 层）。
//
// 注意：项目中存在两个 CheckpointStore 接口，操作不同的检查点类型：
//   - agent.CheckpointStore（本接口）   — 操作 *agent.Checkpoint，含 List/DeleteThread/Rollback
//   - workflow.CheckpointStore          — 操作 *workflow.EnhancedCheckpoint，用于 DAG 工作流时间旅行
//
// 两者的检查点结构体字段不同（Agent 状态 vs DAG 节点结果），无法统一。
type CheckpointStore interface {
	// Save 保存检查点
	Save(ctx context.Context, checkpoint *Checkpoint) error

	// Load 加载检查点
	Load(ctx context.Context, checkpointID string) (*Checkpoint, error)

	// LoadLatest 加载线程最新检查点
	LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error)

	// List 列出线程的所有检查点
	List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error)

	// Delete 删除检查点
	Delete(ctx context.Context, checkpointID string) error

	// DeleteThread 删除整个线程
	DeleteThread(ctx context.Context, threadID string) error

	// LoadVersion 加载指定版本的检查点
	LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error)

	// ListVersions 列出线程的所有版本
	ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error)

	// Rollback 回滚到指定版本
	Rollback(ctx context.Context, threadID string, version int) error
}

// CheckpointManager 检查点管理器
type CheckpointManager struct {
	store  CheckpointStore
	logger *zap.Logger

	// 自动保存配置
	autoSaveEnabled  bool
	autoSaveInterval time.Duration
	autoSaveCancel   context.CancelFunc
	autoSaveMu       sync.Mutex
}

// NewCheckpointManager 创建检查点管理器
func NewCheckpointManager(store CheckpointStore, logger *zap.Logger) *CheckpointManager {
	return &CheckpointManager{
		store:           store,
		logger:          logger.With(zap.String("component", "checkpoint_manager")),
		autoSaveEnabled: false,
	}
}

// SaveCheckpoint 保存检查点
func (m *CheckpointManager) SaveCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint.ID == "" {
		checkpoint.ID = generateCheckpointID()
	}
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = time.Now()
	}

	m.logger.Debug("saving checkpoint",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.String("agent_id", checkpoint.AgentID),
	)

	if err := m.store.Save(ctx, checkpoint); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

// LoadCheckpoint 加载检查点
func (m *CheckpointManager) LoadCheckpoint(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	m.logger.Debug("loading checkpoint", zap.String("checkpoint_id", checkpointID))

	checkpoint, err := m.store.Load(ctx, checkpointID)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	return checkpoint, nil
}

// LoadLatestCheckpoint 加载最新检查点
func (m *CheckpointManager) LoadLatestCheckpoint(ctx context.Context, threadID string) (*Checkpoint, error) {
	m.logger.Debug("loading latest checkpoint", zap.String("thread_id", threadID))

	checkpoint, err := m.store.LoadLatest(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load latest checkpoint: %w", err)
	}

	return checkpoint, nil
}

// ResumeFromCheckpoint 从检查点恢复执行
func (m *CheckpointManager) ResumeFromCheckpoint(ctx context.Context, agent Agent, checkpointID string) error {
	checkpoint, err := m.LoadCheckpoint(ctx, checkpointID)
	if err != nil {
		return err
	}

	m.logger.Info("resuming from checkpoint",
		zap.String("checkpoint_id", checkpointID),
		zap.String("agent_id", checkpoint.AgentID),
		zap.String("state", string(checkpoint.State)),
	)

	// 验证 Agent ID
	if agent.ID() != checkpoint.AgentID {
		return fmt.Errorf("agent ID mismatch: expected %s, got %s", checkpoint.AgentID, agent.ID())
	}

	// 恢复状态（需要 Agent 支持状态恢复）
	if ba, ok := agent.(*BaseAgent); ok {
		if err := ba.Transition(ctx, checkpoint.State); err != nil {
			return fmt.Errorf("failed to restore state: %w", err)
		}
	}

	m.logger.Info("checkpoint restored successfully")

	return nil
}

// 启用自动保存以指定间隔自动保存检查点
func (m *CheckpointManager) EnableAutoSave(ctx context.Context, agent Agent, threadID string, interval time.Duration) error {
	m.autoSaveMu.Lock()
	defer m.autoSaveMu.Unlock()

	if m.autoSaveEnabled {
		return fmt.Errorf("auto-save already enabled")
	}

	if interval <= 0 {
		return fmt.Errorf("invalid interval: must be positive")
	}

	m.autoSaveInterval = interval
	m.autoSaveEnabled = true

	// 为自动保存 goroutine 创建可删除上下文
	autoSaveCtx, cancel := context.WithCancel(ctx)
	m.autoSaveCancel = cancel

	// 开始自动保存出轨
	go m.autoSaveLoop(autoSaveCtx, agent, threadID)

	m.logger.Info("auto-save enabled",
		zap.Duration("interval", interval),
		zap.String("thread_id", threadID),
	)

	return nil
}

// 禁用自动保存停止自动检查
func (m *CheckpointManager) DisableAutoSave() {
	m.autoSaveMu.Lock()
	defer m.autoSaveMu.Unlock()

	if !m.autoSaveEnabled {
		return
	}

	if m.autoSaveCancel != nil {
		m.autoSaveCancel()
		m.autoSaveCancel = nil
	}

	m.autoSaveEnabled = false

	m.logger.Info("auto-save disabled")
}

// 自动保存环路运行自动检查点保存环路
func (m *CheckpointManager) autoSaveLoop(ctx context.Context, agent Agent, threadID string) {
	ticker := time.NewTicker(m.autoSaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Debug("auto-save loop stopped")
			return
		case <-ticker.C:
			if err := m.CreateCheckpoint(ctx, agent, threadID); err != nil {
				m.logger.Error("auto-save failed", zap.Error(err))
			} else {
				m.logger.Debug("auto-save checkpoint created", zap.String("thread_id", threadID))
			}
		}
	}
}

// 创建检查点来抓取当前代理状态并将其保存为检查点
func (m *CheckpointManager) CreateCheckpoint(ctx context.Context, agent Agent, threadID string) error {
	m.logger.Debug("creating checkpoint",
		zap.String("agent_id", agent.ID()),
		zap.String("thread_id", threadID),
	)

	// 提取代理状态
	state := agent.State()
	messages := []CheckpointMessage{}
	var executionContext *ExecutionContext

	// 创建检查站
	checkpoint := &Checkpoint{
		ID:               generateCheckpointID(),
		ThreadID:         threadID,
		AgentID:          agent.ID(),
		State:            state,
		Messages:         messages,
		Metadata:         make(map[string]any),
		CreatedAt:        time.Now(),
		ExecutionContext: executionContext,
	}

	// 保存检查点
	if err := m.SaveCheckpoint(ctx, checkpoint); err != nil {
		return fmt.Errorf("failed to create checkpoint: %w", err)
	}

	m.logger.Info("checkpoint created",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", threadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Rollback ToVersion 将代理拖回特定检查点版本
func (m *CheckpointManager) RollbackToVersion(ctx context.Context, agent Agent, threadID string, version int) error {
	m.logger.Info("rolling back to version",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	// 装入目标版本
	checkpoint, err := m.store.LoadVersion(ctx, threadID, version)
	if err != nil {
		return fmt.Errorf("failed to load version %d: %w", version, err)
	}

	// 验证代理标识
	if agent.ID() != checkpoint.AgentID {
		return fmt.Errorf("agent ID mismatch: expected %s, got %s", checkpoint.AgentID, agent.ID())
	}

	// 恢复代理状态
	// 尝试在可用的情况下使用过渡方法( BaseAgent 和相容类型)
	type transitioner interface {
		Transition(ctx context.Context, newState State) error
	}
	
	if t, ok := agent.(transitioner); ok {
		if err := t.Transition(ctx, checkpoint.State); err != nil {
			return fmt.Errorf("failed to restore state: %w", err)
		}
	} else {
		m.logger.Warn("agent does not support Transition method, state restoration skipped",
			zap.String("agent_id", agent.ID()),
		)
	}

	// 执行存储回滚( 创建新检查点)
	if err := m.store.Rollback(ctx, threadID, version); err != nil {
		return fmt.Errorf("failed to rollback in store: %w", err)
	}

	m.logger.Info("rollback completed",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	return nil
}

// 比较Version 比较两个检查点版本并返回差异
func (m *CheckpointManager) CompareVersions(ctx context.Context, threadID string, version1, version2 int) (*CheckpointDiff, error) {
	m.logger.Debug("comparing versions",
		zap.String("thread_id", threadID),
		zap.Int("version1", version1),
		zap.Int("version2", version2),
	)

	// 装入两个版本
	cp1, err := m.store.LoadVersion(ctx, threadID, version1)
	if err != nil {
		return nil, fmt.Errorf("failed to load version %d: %w", version1, err)
	}

	cp2, err := m.store.LoadVersion(ctx, threadID, version2)
	if err != nil {
		return nil, fmt.Errorf("failed to load version %d: %w", version2, err)
	}

	// 生成 diff
	diff := &CheckpointDiff{
		ThreadID:     threadID,
		Version1:     version1,
		Version2:     version2,
		StateChanged: cp1.State != cp2.State,
		OldState:     cp1.State,
		NewState:     cp2.State,
		MessagesDiff: m.compareMessages(cp1.Messages, cp2.Messages),
		MetadataDiff: m.compareMetadata(cp1.Metadata, cp2.Metadata),
		TimeDiff:     cp2.CreatedAt.Sub(cp1.CreatedAt),
	}

	return diff, nil
}

// ListVersion 列出用于线索的所有检查点版本
func (m *CheckpointManager) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	m.logger.Debug("listing versions", zap.String("thread_id", threadID))

	versions, err := m.store.ListVersions(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	return versions, nil
}

// 比较Messages 比较两个消息切片并返回摘要
func (m *CheckpointManager) compareMessages(msgs1, msgs2 []CheckpointMessage) string {
	if len(msgs1) == len(msgs2) {
		return fmt.Sprintf("No change (%d messages)", len(msgs1))
	}
	return fmt.Sprintf("Changed from %d to %d messages", len(msgs1), len(msgs2))
}

// 比较Metadata 比较两个元数据地图并返回一个摘要
func (m *CheckpointManager) compareMetadata(meta1, meta2 map[string]any) string {
	added := 0
	removed := 0
	changed := 0

	// 检查添加和更改的密钥
	for k, v2 := range meta2 {
		if v1, exists := meta1[k]; !exists {
			added++
		} else if fmt.Sprintf("%v", v1) != fmt.Sprintf("%v", v2) {
			changed++
		}
	}

	// 检查已删除的密钥
	for k := range meta1 {
		if _, exists := meta2[k]; !exists {
			removed++
		}
	}

	if added == 0 && removed == 0 && changed == 0 {
		return "No changes"
	}

	return fmt.Sprintf("Added: %d, Removed: %d, Changed: %d", added, removed, changed)
}

// 检查站 Diff 代表两个检查站版本之间的差异
type CheckpointDiff struct {
	ThreadID     string        `json:"thread_id"`
	Version1     int           `json:"version1"`
	Version2     int           `json:"version2"`
	StateChanged bool          `json:"state_changed"`
	OldState     State         `json:"old_state"`
	NewState     State         `json:"new_state"`
	MessagesDiff string        `json:"messages_diff"`
	MetadataDiff string        `json:"metadata_diff"`
	TimeDiff     time.Duration `json:"time_diff"`
}

var checkpointIDCounter uint64

// generateCheckpointID 生成检查点 ID
func generateCheckpointID() string {
	// 使用纳秒时间戳 + 原子计数器确保唯一性
	counter := atomic.AddUint64(&checkpointIDCounter, 1)
	return fmt.Sprintf("ckpt_%d%d", time.Now().UnixNano(), counter)
}

// ====== Redis 实现 ======

// RedisCheckpointStore Redis 检查点存储
type RedisCheckpointStore struct {
	client RedisClient
	prefix string
	ttl    time.Duration
	logger *zap.Logger
}

// RedisClient Redis 客户端接口
type RedisClient interface {
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Keys(ctx context.Context, pattern string) ([]string, error)
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	ZRemRangeByScore(ctx context.Context, key string, min, max string) error
}

// NewRedisCheckpointStore 创建 Redis 检查点存储
func NewRedisCheckpointStore(client RedisClient, prefix string, ttl time.Duration, logger *zap.Logger) *RedisCheckpointStore {
	return &RedisCheckpointStore{
		client: client,
		prefix: prefix,
		ttl:    ttl,
		logger: logger.With(zap.String("store", "redis_checkpoint")),
	}
}

// Save 保存检查点
func (s *RedisCheckpointStore) Save(ctx context.Context, checkpoint *Checkpoint) error {
	// 如果版本号为0，自动分配版本号
	if checkpoint.Version == 0 {
		versions, err := s.ListVersions(ctx, checkpoint.ThreadID)
		if err == nil && len(versions) > 0 {
			maxVersion := 0
			for _, v := range versions {
				if v.Version > maxVersion {
					maxVersion = v.Version
				}
			}
			checkpoint.Version = maxVersion + 1
		} else {
			checkpoint.Version = 1
		}
	}

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// 保存检查点数据
	key := s.checkpointKey(checkpoint.ID)
	if err := s.client.Set(ctx, key, data, s.ttl); err != nil {
		return fmt.Errorf("save checkpoint data to redis: %w", err)
	}

	// 添加到线程索引（有序集合，按时间排序）
	threadKey := s.threadKey(checkpoint.ThreadID)
	score := float64(checkpoint.CreatedAt.Unix())
	if err := s.client.ZAdd(ctx, threadKey, score, checkpoint.ID); err != nil {
		return fmt.Errorf("add checkpoint to thread index: %w", err)
	}

	s.logger.Debug("checkpoint saved to redis",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Load 加载检查点
func (s *RedisCheckpointStore) Load(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	key := s.checkpointKey(checkpointID)
	data, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get checkpoint from redis: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// LoadLatest 加载最新检查点
func (s *RedisCheckpointStore) LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error) {
	threadKey := s.threadKey(threadID)

	// 获取最新的检查点 ID
	ids, err := s.client.ZRevRange(ctx, threadKey, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("get latest checkpoint ID: %w", err)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("no checkpoints found for thread: %s", threadID)
	}

	return s.Load(ctx, ids[0])
}

// List 列出检查点
func (s *RedisCheckpointStore) List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	threadKey := s.threadKey(threadID)

	// 获取检查点 ID 列表
	ids, err := s.client.ZRevRange(ctx, threadKey, 0, int64(limit-1))
	if err != nil {
		return nil, fmt.Errorf("list checkpoint IDs: %w", err)
	}

	checkpoints := make([]*Checkpoint, 0, len(ids))
	for _, id := range ids {
		checkpoint, err := s.Load(ctx, id)
		if err != nil {
			s.logger.Warn("failed to load checkpoint", zap.String("id", id), zap.Error(err))
			continue
		}
		checkpoints = append(checkpoints, checkpoint)
	}

	return checkpoints, nil
}

// Delete 删除检查点
func (s *RedisCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	key := s.checkpointKey(checkpointID)
	return s.client.Delete(ctx, key)
}

// DeleteThread 删除线程
func (s *RedisCheckpointStore) DeleteThread(ctx context.Context, threadID string) error {
	// 获取所有检查点
	checkpoints, err := s.List(ctx, threadID, 1000)
	if err != nil {
		return fmt.Errorf("list checkpoints for thread deletion: %w", err)
	}

	// 删除所有检查点
	for _, checkpoint := range checkpoints {
		if err := s.Delete(ctx, checkpoint.ID); err != nil {
			s.logger.Warn("failed to delete checkpoint", zap.String("id", checkpoint.ID), zap.Error(err))
		}
	}

	// 删除线程索引
	threadKey := s.threadKey(threadID)
	return s.client.Delete(ctx, threadKey)
}

func (s *RedisCheckpointStore) checkpointKey(id string) string {
	return fmt.Sprintf("%s:checkpoint:%s", s.prefix, id)
}

func (s *RedisCheckpointStore) threadKey(threadID string) string {
	return fmt.Sprintf("%s:thread:%s", s.prefix, threadID)
}

// LoadVersion 加载指定版本的检查点
func (s *RedisCheckpointStore) LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error) {
	versions, err := s.ListVersions(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("list versions for load: %w", err)
	}

	for _, v := range versions {
		if v.Version == version {
			return s.Load(ctx, v.ID)
		}
	}

	return nil, fmt.Errorf("version %d not found for thread %s", version, threadID)
}

// ListVersions 列出线程的所有版本
func (s *RedisCheckpointStore) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	checkpoints, err := s.List(ctx, threadID, 1000)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints for versions: %w", err)
	}

	versions := make([]CheckpointVersion, 0, len(checkpoints))
	for _, cp := range checkpoints {
		versions = append(versions, CheckpointVersion{
			Version:   cp.Version,
			ID:        cp.ID,
			CreatedAt: cp.CreatedAt,
			State:     cp.State,
			Summary:   fmt.Sprintf("Checkpoint at %s", cp.CreatedAt.Format(time.RFC3339)),
		})
	}

	return versions, nil
}

// Rollback 回滚到指定版本
func (s *RedisCheckpointStore) Rollback(ctx context.Context, threadID string, version int) error {
	checkpoint, err := s.LoadVersion(ctx, threadID, version)
	if err != nil {
		return fmt.Errorf("load version %d for rollback: %w", version, err)
	}

	// 创建新的检查点作为回滚点
	newCheckpoint := *checkpoint
	newCheckpoint.ID = generateCheckpointID()
	newCheckpoint.CreatedAt = time.Now()
	newCheckpoint.ParentID = checkpoint.ID

	// 获取当前最大版本号
	versions, err := s.ListVersions(ctx, threadID)
	if err != nil {
		return fmt.Errorf("list versions for rollback: %w", err)
	}

	maxVersion := 0
	for _, v := range versions {
		if v.Version > maxVersion {
			maxVersion = v.Version
		}
	}

	newCheckpoint.Version = maxVersion + 1

	if newCheckpoint.Metadata == nil {
		newCheckpoint.Metadata = make(map[string]any)
	}
	newCheckpoint.Metadata["rollback_from_version"] = version

	return s.Save(ctx, &newCheckpoint)
}

// ====== PostgreSQL 实现 ======

// PostgreSQLCheckpointStore PostgreSQL 检查点存储
type PostgreSQLCheckpointStore struct {
	db     PostgreSQLClient
	logger *zap.Logger
}

// PostgreSQLClient PostgreSQL 客户端接口
type PostgreSQLClient interface {
	Exec(ctx context.Context, query string, args ...any) error
	QueryRow(ctx context.Context, query string, args ...any) Row
	Query(ctx context.Context, query string, args ...any) (Rows, error)
}

// Row 数据库行接口
type Row interface {
	Scan(dest ...any) error
}

// Rows 数据库行集合接口
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
}

// NewPostgreSQLCheckpointStore 创建 PostgreSQL 检查点存储
func NewPostgreSQLCheckpointStore(db PostgreSQLClient, logger *zap.Logger) *PostgreSQLCheckpointStore {
	return &PostgreSQLCheckpointStore{
		db:     db,
		logger: logger.With(zap.String("store", "postgresql_checkpoint")),
	}
}

// Save 保存检查点
func (s *PostgreSQLCheckpointStore) Save(ctx context.Context, checkpoint *Checkpoint) error {
	// 如果版本号为0，自动分配版本号
	if checkpoint.Version == 0 {
		versions, err := s.ListVersions(ctx, checkpoint.ThreadID)
		if err == nil && len(versions) > 0 {
			maxVersion := 0
			for _, v := range versions {
				if v.Version > maxVersion {
					maxVersion = v.Version
				}
			}
			checkpoint.Version = maxVersion + 1
		} else {
			checkpoint.Version = 1
		}
	}

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	query := `
		INSERT INTO agent_checkpoints (id, thread_id, agent_id, version, state, data, created_at, parent_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			version = EXCLUDED.version,
			state = EXCLUDED.state,
			data = EXCLUDED.data
	`

	err = s.db.Exec(ctx, query,
		checkpoint.ID,
		checkpoint.ThreadID,
		checkpoint.AgentID,
		checkpoint.Version,
		checkpoint.State,
		data,
		checkpoint.CreatedAt,
		checkpoint.ParentID,
	)

	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	s.logger.Debug("checkpoint saved to postgresql",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Load 加载检查点
func (s *PostgreSQLCheckpointStore) Load(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	query := `SELECT data FROM agent_checkpoints WHERE id = $1`

	var data []byte
	row := s.db.QueryRow(ctx, query, checkpointID)
	if err := row.Scan(&data); err != nil {
		return nil, fmt.Errorf("checkpoint not found: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// LoadLatest 加载最新检查点
func (s *PostgreSQLCheckpointStore) LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error) {
	query := `
		SELECT data FROM agent_checkpoints
		WHERE thread_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var data []byte
	row := s.db.QueryRow(ctx, query, threadID)
	if err := row.Scan(&data); err != nil {
		return nil, fmt.Errorf("no checkpoints found: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// List 列出检查点
func (s *PostgreSQLCheckpointStore) List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	query := `
		SELECT data FROM agent_checkpoints
		WHERE thread_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, threadID, limit)
	if err != nil {
		return nil, fmt.Errorf("query checkpoints: %w", err)
	}
	defer rows.Close()

	checkpoints := make([]*Checkpoint, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			s.logger.Warn("failed to scan row", zap.Error(err))
			continue
		}

		var checkpoint Checkpoint
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			s.logger.Warn("failed to unmarshal checkpoint", zap.Error(err))
			continue
		}

		checkpoints = append(checkpoints, &checkpoint)
	}

	return checkpoints, nil
}

// Delete 删除检查点
func (s *PostgreSQLCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	query := `DELETE FROM agent_checkpoints WHERE id = $1`
	return s.db.Exec(ctx, query, checkpointID)
}

// DeleteThread 删除线程
func (s *PostgreSQLCheckpointStore) DeleteThread(ctx context.Context, threadID string) error {
	query := `DELETE FROM agent_checkpoints WHERE thread_id = $1`
	return s.db.Exec(ctx, query, threadID)
}

// LoadVersion 加载指定版本的检查点
func (s *PostgreSQLCheckpointStore) LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error) {
	query := `
		SELECT data FROM agent_checkpoints
		WHERE thread_id = $1 AND version = $2
		LIMIT 1
	`

	var data []byte
	row := s.db.QueryRow(ctx, query, threadID, version)
	if err := row.Scan(&data); err != nil {
		return nil, fmt.Errorf("version %d not found: %w", version, err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// ListVersions 列出线程的所有版本
func (s *PostgreSQLCheckpointStore) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	query := `
		SELECT id, version, created_at, state FROM agent_checkpoints
		WHERE thread_id = $1
		ORDER BY version ASC
	`

	rows, err := s.db.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("query checkpoint versions: %w", err)
	}
	defer rows.Close()

	versions := make([]CheckpointVersion, 0)
	for rows.Next() {
		var v CheckpointVersion
		var state string
		if err := rows.Scan(&v.ID, &v.Version, &v.CreatedAt, &state); err != nil {
			s.logger.Warn("failed to scan version row", zap.Error(err))
			continue
		}
		v.State = State(state)
		v.Summary = fmt.Sprintf("Checkpoint at %s", v.CreatedAt.Format(time.RFC3339))
		versions = append(versions, v)
	}

	return versions, nil
}

// Rollback 回滚到指定版本
func (s *PostgreSQLCheckpointStore) Rollback(ctx context.Context, threadID string, version int) error {
	checkpoint, err := s.LoadVersion(ctx, threadID, version)
	if err != nil {
		return fmt.Errorf("load version %d for rollback: %w", version, err)
	}

	// 创建新的检查点作为回滚点
	newCheckpoint := *checkpoint
	newCheckpoint.ID = generateCheckpointID()
	newCheckpoint.CreatedAt = time.Now()
	newCheckpoint.ParentID = checkpoint.ID

	// 获取当前最大版本号
	versions, err := s.ListVersions(ctx, threadID)
	if err != nil {
		return fmt.Errorf("list versions for rollback: %w", err)
	}

	maxVersion := 0
	for _, v := range versions {
		if v.Version > maxVersion {
			maxVersion = v.Version
		}
	}

	newCheckpoint.Version = maxVersion + 1

	if newCheckpoint.Metadata == nil {
		newCheckpoint.Metadata = make(map[string]any)
	}
	newCheckpoint.Metadata["rollback_from_version"] = version

	return s.Save(ctx, &newCheckpoint)
}

// ====== File-based 实现 ======

// FileCheckpointStore 文件检查点存储（用于本地开发和测试）
type FileCheckpointStore struct {
	basePath string
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewFileCheckpointStore 创建文件检查点存储
func NewFileCheckpointStore(basePath string, logger *zap.Logger) (*FileCheckpointStore, error) {
	// 创建基础目录
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &FileCheckpointStore{
		basePath: basePath,
		logger:   logger.With(zap.String("store", "file_checkpoint")),
	}, nil
}

// Save 保存检查点
func (s *FileCheckpointStore) Save(ctx context.Context, checkpoint *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveLocked(ctx, checkpoint)
}

// saveLocked 保存检查点的内部实现（调用方必须持有 s.mu 锁）
func (s *FileCheckpointStore) saveLocked(ctx context.Context, checkpoint *Checkpoint) error {
	// 如果版本号为0，自动分配版本号
	if checkpoint.Version == 0 {
		versions, err := s.listVersionsUnlocked(ctx, checkpoint.ThreadID)
		if err == nil && len(versions) > 0 {
			maxVersion := 0
			for _, v := range versions {
				if v.Version > maxVersion {
					maxVersion = v.Version
				}
			}
			checkpoint.Version = maxVersion + 1
		} else {
			checkpoint.Version = 1
		}
	}

	// 创建线程目录
	threadDir := s.threadDir(checkpoint.ThreadID)
	if err := os.MkdirAll(threadDir, 0755); err != nil {
		return fmt.Errorf("failed to create thread directory: %w", err)
	}

	checkpointsDir := filepath.Join(threadDir, "checkpoints")
	if err := os.MkdirAll(checkpointsDir, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoints directory: %w", err)
	}

	// 序列化检查点
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// 写入检查点文件
	checkpointFile := filepath.Join(checkpointsDir, fmt.Sprintf("%s.json", checkpoint.ID))
	if err := os.WriteFile(checkpointFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	// 更新版本索引
	if err := s.updateVersionsIndex(checkpoint.ThreadID, checkpoint); err != nil {
		return fmt.Errorf("failed to update versions index: %w", err)
	}

	// 更新 latest.txt
	latestFile := filepath.Join(threadDir, "latest.txt")
	if err := os.WriteFile(latestFile, []byte(checkpoint.ID), 0644); err != nil {
		return fmt.Errorf("failed to update latest file: %w", err)
	}

	s.logger.Debug("checkpoint saved to file",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Load 加载检查点
func (s *FileCheckpointStore) Load(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 搜索所有线程目录
	threadsDir := filepath.Join(s.basePath, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
		}
		return nil, fmt.Errorf("failed to read threads directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		checkpointFile := filepath.Join(threadsDir, entry.Name(), "checkpoints", fmt.Sprintf("%s.json", checkpointID))
		if _, err := os.Stat(checkpointFile); err == nil {
			return s.loadCheckpointFile(checkpointFile)
		}
	}

	return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
}

// LoadLatest 加载最新检查点
func (s *FileCheckpointStore) LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threadDir := s.threadDir(threadID)
	latestFile := filepath.Join(threadDir, "latest.txt")

	data, err := os.ReadFile(latestFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no checkpoints found for thread: %s", threadID)
		}
		return nil, fmt.Errorf("failed to read latest file: %w", err)
	}

	checkpointID := string(data)
	checkpointFile := filepath.Join(threadDir, "checkpoints", fmt.Sprintf("%s.json", checkpointID))

	return s.loadCheckpointFile(checkpointFile)
}

// List 列出检查点
func (s *FileCheckpointStore) List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	checkpointsDir := filepath.Join(s.threadDir(threadID), "checkpoints")
	entries, err := os.ReadDir(checkpointsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Checkpoint{}, nil
		}
		return nil, fmt.Errorf("failed to read checkpoints directory: %w", err)
	}

	// 加载所有检查点
	checkpoints := make([]*Checkpoint, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		checkpointFile := filepath.Join(checkpointsDir, entry.Name())
		checkpoint, err := s.loadCheckpointFile(checkpointFile)
		if err != nil {
			s.logger.Warn("failed to load checkpoint file",
				zap.String("file", checkpointFile),
				zap.Error(err))
			continue
		}

		checkpoints = append(checkpoints, checkpoint)
	}

	// 按创建时间降序排序
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].CreatedAt.After(checkpoints[j].CreatedAt)
	})

	// 限制返回数量
	if limit > 0 && len(checkpoints) > limit {
		checkpoints = checkpoints[:limit]
	}

	return checkpoints, nil
}

// Delete 删除检查点
func (s *FileCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 搜索所有线程目录
	threadsDir := filepath.Join(s.basePath, "threads")
	entries, err := os.ReadDir(threadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 已经不存在
		}
		return fmt.Errorf("failed to read threads directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		checkpointFile := filepath.Join(threadsDir, entry.Name(), "checkpoints", fmt.Sprintf("%s.json", checkpointID))
		if _, err := os.Stat(checkpointFile); err == nil {
			if err := os.Remove(checkpointFile); err != nil {
				return fmt.Errorf("failed to delete checkpoint file: %w", err)
			}

			s.logger.Debug("checkpoint deleted",
				zap.String("checkpoint_id", checkpointID),
				zap.String("thread_id", entry.Name()))

			return nil
		}
	}

	return nil // 检查点不存在，视为成功
}

// DeleteThread 删除线程
func (s *FileCheckpointStore) DeleteThread(ctx context.Context, threadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	threadDir := s.threadDir(threadID)
	if err := os.RemoveAll(threadDir); err != nil {
		if os.IsNotExist(err) {
			return nil // 已经不存在
		}
		return fmt.Errorf("failed to delete thread directory: %w", err)
	}

	s.logger.Debug("thread deleted", zap.String("thread_id", threadID))

	return nil
}

// LoadVersion 加载指定版本的检查点
func (s *FileCheckpointStore) LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions, err := s.listVersionsUnlocked(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}

	for _, v := range versions {
		if v.Version == version {
			checkpointFile := filepath.Join(s.threadDir(threadID), "checkpoints", fmt.Sprintf("%s.json", v.ID))
			return s.loadCheckpointFile(checkpointFile)
		}
	}

	return nil, fmt.Errorf("version %d not found for thread %s", version, threadID)
}

// ListVersions 列出线程的所有版本
func (s *FileCheckpointStore) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listVersionsUnlocked(ctx, threadID)
}

// Rollback 回滚到指定版本
func (s *FileCheckpointStore) Rollback(ctx context.Context, threadID string, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 加载指定版本的检查点
	versions, err := s.listVersionsUnlocked(ctx, threadID)
	if err != nil {
		return fmt.Errorf("list versions for rollback: %w", err)
	}

	var targetCheckpoint *Checkpoint
	for _, v := range versions {
		if v.Version == version {
			checkpointFile := filepath.Join(s.threadDir(threadID), "checkpoints", fmt.Sprintf("%s.json", v.ID))
			targetCheckpoint, err = s.loadCheckpointFile(checkpointFile)
			if err != nil {
				return fmt.Errorf("load checkpoint file for rollback: %w", err)
			}
			break
		}
	}

	if targetCheckpoint == nil {
		return fmt.Errorf("version %d not found for thread %s", version, threadID)
	}

	// 创建新的检查点作为回滚点
	newCheckpoint := *targetCheckpoint
	newCheckpoint.ID = generateCheckpointID()
	newCheckpoint.CreatedAt = time.Now()
	newCheckpoint.ParentID = targetCheckpoint.ID

	// 获取当前最大版本号
	maxVersion := 0
	for _, v := range versions {
		if v.Version > maxVersion {
			maxVersion = v.Version
		}
	}

	newCheckpoint.Version = maxVersion + 1

	if newCheckpoint.Metadata == nil {
		newCheckpoint.Metadata = make(map[string]any)
	}
	newCheckpoint.Metadata["rollback_from_version"] = version

	// 直接调用 saveLocked，避免释放锁后的竞态窗口
	return s.saveLocked(ctx, &newCheckpoint)
}

// 辅助方法

func (s *FileCheckpointStore) threadDir(threadID string) string {
	return filepath.Join(s.basePath, "threads", threadID)
}

func (s *FileCheckpointStore) loadCheckpointFile(filePath string) (*Checkpoint, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

func (s *FileCheckpointStore) updateVersionsIndex(threadID string, checkpoint *Checkpoint) error {
	versionsFile := filepath.Join(s.threadDir(threadID), "versions.json")

	// 读取现有版本索引
	var versions []CheckpointVersion
	if data, err := os.ReadFile(versionsFile); err == nil {
		if err := json.Unmarshal(data, &versions); err != nil {
			return fmt.Errorf("failed to unmarshal versions: %w", err)
		}
	}

	// 检查版本是否已存在
	found := false
	for i, v := range versions {
		if v.Version == checkpoint.Version {
			versions[i] = CheckpointVersion{
				Version:   checkpoint.Version,
				ID:        checkpoint.ID,
				CreatedAt: checkpoint.CreatedAt,
				State:     checkpoint.State,
				Summary:   fmt.Sprintf("Checkpoint at %s", checkpoint.CreatedAt.Format(time.RFC3339)),
			}
			found = true
			break
		}
	}

	if !found {
		versions = append(versions, CheckpointVersion{
			Version:   checkpoint.Version,
			ID:        checkpoint.ID,
			CreatedAt: checkpoint.CreatedAt,
			State:     checkpoint.State,
			Summary:   fmt.Sprintf("Checkpoint at %s", checkpoint.CreatedAt.Format(time.RFC3339)),
		})
	}

	// 按版本号排序
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})

	// 写入版本索引
	data, err := json.MarshalIndent(versions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal versions: %w", err)
	}

	if err := os.WriteFile(versionsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write versions file: %w", err)
	}

	return nil
}

func (s *FileCheckpointStore) listVersionsUnlocked(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	versionsFile := filepath.Join(s.threadDir(threadID), "versions.json")

	data, err := os.ReadFile(versionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []CheckpointVersion{}, nil
		}
		return nil, fmt.Errorf("failed to read versions file: %w", err)
	}

	var versions []CheckpointVersion
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal versions: %w", err)
	}

	return versions, nil
}
