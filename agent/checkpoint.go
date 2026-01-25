package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Checkpoint Agent 执行检查点（基于 LangGraph 2026 标准）
type Checkpoint struct {
	ID        string                 `json:"id"`
	ThreadID  string                 `json:"thread_id"`  // 会话线程 ID
	AgentID   string                 `json:"agent_id"`
	State     State                  `json:"state"`
	Messages  []CheckpointMessage    `json:"messages"`
	Metadata  map[string]interface{} `json:"metadata"`
	CreatedAt time.Time              `json:"created_at"`
	ParentID  string                 `json:"parent_id,omitempty"` // 父检查点 ID
}

// CheckpointMessage 检查点消息
type CheckpointMessage struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	ToolCalls []CheckpointToolCall   `json:"tool_calls,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// CheckpointToolCall 工具调用记录
type CheckpointToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// CheckpointStore 检查点存储接口
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
}

// CheckpointManager 检查点管理器
type CheckpointManager struct {
	store  CheckpointStore
	logger *zap.Logger
}

// NewCheckpointManager 创建检查点管理器
func NewCheckpointManager(store CheckpointStore, logger *zap.Logger) *CheckpointManager {
	return &CheckpointManager{
		store:  store,
		logger: logger.With(zap.String("component", "checkpoint_manager")),
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

// generateCheckpointID 生成检查点 ID
func generateCheckpointID() string {
	return fmt.Sprintf("ckpt_%d", time.Now().UnixNano())
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
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}
	
	// 保存检查点数据
	key := s.checkpointKey(checkpoint.ID)
	if err := s.client.Set(ctx, key, data, s.ttl); err != nil {
		return err
	}
	
	// 添加到线程索引（有序集合，按时间排序）
	threadKey := s.threadKey(checkpoint.ThreadID)
	score := float64(checkpoint.CreatedAt.Unix())
	if err := s.client.ZAdd(ctx, threadKey, score, checkpoint.ID); err != nil {
		return err
	}
	
	s.logger.Debug("checkpoint saved to redis",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
	)
	
	return nil
}

// Load 加载检查点
func (s *RedisCheckpointStore) Load(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	key := s.checkpointKey(checkpointID)
	data, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, err
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
		return nil, err
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
		return nil, err
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
		return err
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

// ====== PostgreSQL 实现 ======

// PostgreSQLCheckpointStore PostgreSQL 检查点存储
type PostgreSQLCheckpointStore struct {
	db     PostgreSQLClient
	logger *zap.Logger
}

// PostgreSQLClient PostgreSQL 客户端接口
type PostgreSQLClient interface {
	Exec(ctx context.Context, query string, args ...interface{}) error
	QueryRow(ctx context.Context, query string, args ...interface{}) Row
	Query(ctx context.Context, query string, args ...interface{}) (Rows, error)
}

// Row 数据库行接口
type Row interface {
	Scan(dest ...interface{}) error
}

// Rows 数据库行集合接口
type Rows interface {
	Next() bool
	Scan(dest ...interface{}) error
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
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}
	
	query := `
		INSERT INTO agent_checkpoints (id, thread_id, agent_id, state, data, created_at, parent_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			data = EXCLUDED.data
	`
	
	err = s.db.Exec(ctx, query,
		checkpoint.ID,
		checkpoint.ThreadID,
		checkpoint.AgentID,
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
		return nil, err
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
