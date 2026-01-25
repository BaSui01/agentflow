package context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrVersionConflict = errors.New("version conflict")
)

// SessionStore 会话存储接口
type SessionStore interface {
	// Get 获取会话
	Get(ctx context.Context, sessionID string) (*Session, error)
	// Save 保存会话（带乐观锁）
	Save(ctx context.Context, session *Session) error
	// Delete 删除会话
	Delete(ctx context.Context, sessionID string) error
	// AppendMessage 追加消息（原子操作）
	AppendMessage(ctx context.Context, sessionID string, msg Message) error
}

// Session 会话数据
type Session struct {
	ID            string         `json:"id"`
	SessionID     string         `json:"session_id"`
	TenantID      string         `json:"tenant_id,omitempty"`
	UserID        string         `json:"user_id,omitempty"`
	Messages      []Message      `json:"messages"`
	TokensUsed    int            `json:"tokens_used"`
	Model         string         `json:"model,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	LastMessageAt time.Time      `json:"last_message_at"`
	Version       int            `json:"version"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// RedisSessionStore Redis 会话存储（热存储）
type RedisSessionStore struct {
	rdb       *redis.Client
	keyPrefix string
	ttl       time.Duration
	logger    *zap.Logger
}

// NewRedisSessionStore 创建 Redis 会话存储
func NewRedisSessionStore(rdb *redis.Client, logger *zap.Logger) *RedisSessionStore {
	return &RedisSessionStore{
		rdb:       rdb,
		keyPrefix: "llm:session:",
		ttl:       24 * time.Hour,
		logger:    logger,
	}
}

func (s *RedisSessionStore) key(sessionID string) string {
	return s.keyPrefix + sessionID
}

func (s *RedisSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	data, err := s.rdb.Get(ctx, s.key(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &session, nil
}

func (s *RedisSessionStore) Save(ctx context.Context, session *Session) error {
	session.UpdatedAt = time.Now()
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	// 使用 Lua 脚本实现乐观锁
	script := redis.NewScript(`
		local key = KEYS[1]
		local data = ARGV[1]
		local expectedVersion = tonumber(ARGV[2])

		local current = redis.call('GET', key)
		if current then
			local session = cjson.decode(current)
			if session.version ~= expectedVersion then
				return -1  -- 版本冲突
			end
		end

		redis.call('SET', key, data, 'EX', ARGV[3])
		return 1
	`)

	result, err := script.Run(ctx, s.rdb, []string{s.key(session.SessionID)},
		data, session.Version-1, int(s.ttl.Seconds())).Int()
	if err != nil {
		return fmt.Errorf("redis script: %w", err)
	}
	if result == -1 {
		return ErrVersionConflict
	}
	return nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, sessionID string) error {
	return s.rdb.Del(ctx, s.key(sessionID)).Err()
}

func (s *RedisSessionStore) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
	// 使用 Lua 脚本原子追加
	script := redis.NewScript(`
		local key = KEYS[1]
		local msgData = ARGV[1]
		local ttl = tonumber(ARGV[2])

		local current = redis.call('GET', key)
		if not current then
			return -1  -- 会话不存在
		end

		local session = cjson.decode(current)
		table.insert(session.messages, cjson.decode(msgData))
		session.version = session.version + 1
		session.last_message_at = ARGV[3]
		session.updated_at = ARGV[3]

		redis.call('SET', key, cjson.encode(session), 'EX', ttl)
		return session.version
	`)

	msgData, _ := json.Marshal(msg)
	now := time.Now().Format(time.RFC3339)

	result, err := script.Run(ctx, s.rdb, []string{s.key(sessionID)},
		msgData, int(s.ttl.Seconds()), now).Int()
	if err != nil {
		return fmt.Errorf("redis append: %w", err)
	}
	if result == -1 {
		return ErrSessionNotFound
	}
	return nil
}

// HybridSessionStore 混合存储（Redis + DB）
type HybridSessionStore struct {
	redis     *RedisSessionStore
	db        DBSessionStore
	tokenizer Tokenizer
	logger    *zap.Logger
	// 异步落盘
	persistCh   chan *Session
	persistOnce sync.Once
}

// DBSessionStore 数据库会话存储接口
type DBSessionStore interface {
	Get(ctx context.Context, sessionID string) (*Session, error)
	Save(ctx context.Context, session *Session) error
	Delete(ctx context.Context, sessionID string) error
}

// NewHybridSessionStore 创建混合存储
func NewHybridSessionStore(
	rdb *redis.Client,
	db DBSessionStore,
	tokenizer Tokenizer,
	logger *zap.Logger,
) *HybridSessionStore {
	h := &HybridSessionStore{
		redis:     NewRedisSessionStore(rdb, logger),
		db:        db,
		tokenizer: tokenizer,
		logger:    logger,
		persistCh: make(chan *Session, 100),
	}
	return h
}

// StartPersistWorker 启动异步落盘 worker
func (h *HybridSessionStore) StartPersistWorker(ctx context.Context) {
	h.persistOnce.Do(func() {
		go h.persistWorker(ctx)
	})
}

func (h *HybridSessionStore) persistWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case session, ok := <-h.persistCh:
			if !ok {
				return // channel 已关闭
			}
			if err := h.db.Save(ctx, session); err != nil {
				h.logger.Error("persist session to db failed",
					zap.String("session_id", session.SessionID),
					zap.Error(err))
			} else {
				h.logger.Debug("session persisted to db",
					zap.String("session_id", session.SessionID))
			}
		}
	}
}

func (h *HybridSessionStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	// 1. 先查 Redis
	session, err := h.redis.Get(ctx, sessionID)
	if err == nil {
		return session, nil
	}
	if !errors.Is(err, ErrSessionNotFound) {
		h.logger.Warn("redis get failed, fallback to db", zap.Error(err))
	}

	// 2. 回源 DB
	session, err = h.db.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// 3. 回填 Redis
	if err := h.redis.Save(ctx, session); err != nil {
		h.logger.Warn("backfill redis failed", zap.Error(err))
	}

	return session, nil
}

func (h *HybridSessionStore) Save(ctx context.Context, session *Session) error {
	// 计算 token 使用量
	session.TokensUsed = h.tokenizer.CountMessagesTokens(session.Messages)
	session.Version++
	session.UpdatedAt = time.Now()

	// 1. 写 Redis
	if err := h.redis.Save(ctx, session); err != nil {
		return err
	}

	// 2. 异步落盘 DB
	select {
	case h.persistCh <- session:
	default:
		h.logger.Warn("persist channel full, dropping",
			zap.String("session_id", session.SessionID))
	}

	return nil
}

func (h *HybridSessionStore) Delete(ctx context.Context, sessionID string) error {
	// 同时删除 Redis 和 DB
	if err := h.redis.Delete(ctx, sessionID); err != nil {
		h.logger.Warn("redis delete failed", zap.Error(err))
	}
	return h.db.Delete(ctx, sessionID)
}

func (h *HybridSessionStore) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
	// 1. 原子追加到 Redis
	if err := h.redis.AppendMessage(ctx, sessionID, msg); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			// 会话不在 Redis，从 DB 加载
			session, dbErr := h.db.Get(ctx, sessionID)
			if dbErr != nil {
				return dbErr
			}
			session.Messages = append(session.Messages, msg)
			return h.Save(ctx, session)
		}
		return err
	}

	// 2. 获取更新后的会话，异步落盘
	session, err := h.redis.Get(ctx, sessionID)
	if err == nil {
		select {
		case h.persistCh <- session:
		default:
		}
	}

	return nil
}

// Close 关闭存储
func (h *HybridSessionStore) Close() {
	close(h.persistCh)
}

// SessionManager 会话管理器（集成上下文裁剪）
type SessionManager struct {
	store      SessionStore
	ctxManager ContextManager
	maxTokens  int
	strategy   PruneStrategy
	logger     *zap.Logger
}

// NewSessionManager 创建会话管理器
func NewSessionManager(
	store SessionStore,
	ctxManager ContextManager,
	maxTokens int,
	logger *zap.Logger,
) *SessionManager {
	return &SessionManager{
		store:      store,
		ctxManager: ctxManager,
		maxTokens:  maxTokens,
		strategy:   PruneOldest,
		logger:     logger,
	}
}

// GetOrCreate 获取或创建会话
func (m *SessionManager) GetOrCreate(ctx context.Context, sessionID, tenantID, userID string) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err == nil {
		return session, nil
	}
	if !errors.Is(err, ErrSessionNotFound) {
		return nil, err
	}

	// 创建新会话
	now := time.Now()
	session = &Session{
		SessionID: sessionID,
		TenantID:  tenantID,
		UserID:    userID,
		Messages:  []Message{},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := m.store.Save(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// AddMessage 添加消息（自动裁剪）
func (m *SessionManager) AddMessage(ctx context.Context, sessionID string, msg Message) error {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Messages = append(session.Messages, msg)
	session.LastMessageAt = time.Now()

	// 检查是否需要裁剪
	if m.ctxManager.EstimateTokens(session.Messages) > m.maxTokens {
		trimmed, err := m.ctxManager.PruneByStrategy(session.Messages, m.maxTokens, m.strategy)
		if err != nil {
			m.logger.Warn("prune messages failed", zap.Error(err))
		} else {
			m.logger.Info("messages pruned",
				zap.Int("before", len(session.Messages)),
				zap.Int("after", len(trimmed)))
			session.Messages = trimmed
		}
	}

	return m.store.Save(ctx, session)
}

// GetMessages 获取会话消息
func (m *SessionManager) GetMessages(ctx context.Context, sessionID string) ([]Message, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return session.Messages, nil
}

// ClearMessages 清空会话消息
func (m *SessionManager) ClearMessages(ctx context.Context, sessionID string) error {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	session.Messages = []Message{}
	session.TokensUsed = 0
	return m.store.Save(ctx, session)
}
