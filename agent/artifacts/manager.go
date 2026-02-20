package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 工艺品Type定义了文物的类型.
type ArtifactType string

const (
	ArtifactTypeFile   ArtifactType = "file"
	ArtifactTypeData   ArtifactType = "data"
	ArtifactTypeImage  ArtifactType = "image"
	ArtifactTypeCode   ArtifactType = "code"
	ArtifactTypeOutput ArtifactType = "output"
	ArtifactTypeModel  ArtifactType = "model"
)

// 人工活性状态代表文物的生命周期状态。
type ArtifactStatus string

const (
	StatusPending   ArtifactStatus = "pending"
	StatusUploading ArtifactStatus = "uploading"
	StatusReady     ArtifactStatus = "ready"
	StatusArchived  ArtifactStatus = "archived"
	StatusDeleted   ArtifactStatus = "deleted"
)

// 人工活性代表系统中的受管文物.
type Artifact struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Type        ArtifactType   `json:"type"`
	Status      ArtifactStatus `json:"status"`
	MimeType    string         `json:"mime_type,omitempty"`
	Size        int64          `json:"size"`
	Checksum    string         `json:"checksum"`
	StoragePath string         `json:"storage_path"`
	URL         string         `json:"url,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	CreatedBy   string         `json:"created_by"`
	SessionID   string         `json:"session_id,omitempty"`
	ParentID    string         `json:"parent_id,omitempty"`
	Version     int            `json:"version"`
}

// ArtifactStore定义了文物的存储界面.
type ArtifactStore interface {
	Save(ctx context.Context, artifact *Artifact, data io.Reader) error
	Load(ctx context.Context, artifactID string) (*Artifact, io.ReadCloser, error)
	GetMetadata(ctx context.Context, artifactID string) (*Artifact, error)
	Delete(ctx context.Context, artifactID string) error
	List(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	Archive(ctx context.Context, artifactID string) error
}

// ArtifactQuery定义了列出文物的查询参数.
type ArtifactQuery struct {
	SessionID string         `json:"session_id,omitempty"`
	Type      ArtifactType   `json:"type,omitempty"`
	Status    ArtifactStatus `json:"status,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	CreatedBy string         `json:"created_by,omitempty"`
	Limit     int            `json:"limit,omitempty"`
	Offset    int            `json:"offset,omitempty"`
}

// 经理处理文物生命周期管理.
type Manager struct {
	store     ArtifactStore
	logger    *zap.Logger
	basePath  string
	maxSize   int64
	ttl       time.Duration
	cleanupMu sync.Mutex
	artifacts map[string]*Artifact
	mu        sync.RWMutex
}

// Manager Config 配置了文物管理器 。
type ManagerConfig struct {
	BasePath   string        `json:"base_path"`
	MaxSize    int64         `json:"max_size"`
	DefaultTTL time.Duration `json:"default_ttl"`
}

// 默认管理器 Config 返回合理的默认值 。
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		BasePath:   "./artifacts",
		MaxSize:    100 * 1024 * 1024, // 100MB
		DefaultTTL: 24 * time.Hour,
	}
}

// NewManager创建了新的文物管理器.
func NewManager(config ManagerConfig, store ArtifactStore, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		store:     store,
		logger:    logger.With(zap.String("component", "artifact_manager")),
		basePath:  config.BasePath,
		maxSize:   config.MaxSize,
		ttl:       config.DefaultTTL,
		artifacts: make(map[string]*Artifact),
	}
}

// 从数据创建出新的文物 。
func (m *Manager) Create(ctx context.Context, name string, artifactType ArtifactType, data io.Reader, opts ...CreateOption) (*Artifact, error) {
	options := &createOptions{}
	for _, opt := range opts {
		opt(options)
	}

	artifact := &Artifact{
		ID:        generateArtifactID(),
		Name:      name,
		Type:      artifactType,
		Status:    StatusPending,
		Metadata:  options.metadata,
		Tags:      options.tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		CreatedBy: options.createdBy,
		SessionID: options.sessionID,
		Version:   1,
	}

	if options.mimeType != "" {
		artifact.MimeType = options.mimeType
	}

	if options.ttl > 0 {
		expiresAt := time.Now().Add(options.ttl)
		artifact.ExpiresAt = &expiresAt
	} else if m.ttl > 0 {
		expiresAt := time.Now().Add(m.ttl)
		artifact.ExpiresAt = &expiresAt
	}

	m.logger.Info("creating artifact",
		zap.String("id", artifact.ID),
		zap.String("name", name),
		zap.String("type", string(artifactType)),
	)

	artifact.Status = StatusUploading

	if err := m.store.Save(ctx, artifact, data); err != nil {
		artifact.Status = StatusDeleted
		return nil, fmt.Errorf("failed to save artifact: %w", err)
	}

	artifact.Status = StatusReady
	artifact.UpdatedAt = time.Now()

	m.mu.Lock()
	m.artifacts[artifact.ID] = artifact
	m.mu.Unlock()

	m.logger.Info("artifact created",
		zap.String("id", artifact.ID),
		zap.Int64("size", artifact.Size),
	)

	return artifact, nil
}

// 凭身份证取回文物
func (m *Manager) Get(ctx context.Context, artifactID string) (*Artifact, io.ReadCloser, error) {
	return m.store.Load(ctx, artifactID)
}

// GetMetadata检索没有数据的文物元数据.
func (m *Manager) GetMetadata(ctx context.Context, artifactID string) (*Artifact, error) {
	m.mu.RLock()
	if artifact, ok := m.artifacts[artifactID]; ok {
		m.mu.RUnlock()
		return artifact, nil
	}
	m.mu.RUnlock()

	return m.store.GetMetadata(ctx, artifactID)
}

// 删除一个文物。
func (m *Manager) Delete(ctx context.Context, artifactID string) error {
	m.logger.Info("deleting artifact", zap.String("id", artifactID))

	if err := m.store.Delete(ctx, artifactID); err != nil {
		return err
	}

	m.mu.Lock()
	delete(m.artifacts, artifactID)
	m.mu.Unlock()

	return nil
}

// 列表列出符合查询的文物.
func (m *Manager) List(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return m.store.List(ctx, query)
}

// 档案馆收藏了一件文物
func (m *Manager) Archive(ctx context.Context, artifactID string) error {
	m.logger.Info("archiving artifact", zap.String("id", artifactID))
	return m.store.Archive(ctx, artifactID)
}

// CreateVersion 创建了新版本的已有文物.
func (m *Manager) CreateVersion(ctx context.Context, parentID string, data io.Reader) (*Artifact, error) {
	parent, err := m.GetMetadata(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("parent artifact not found: %w", err)
	}

	return m.Create(ctx, parent.Name, parent.Type, data,
		WithMetadata(parent.Metadata),
		WithTags(parent.Tags...),
		WithSessionID(parent.SessionID),
		WithCreatedBy(parent.CreatedBy),
		withParentID(parentID),
		withVersion(parent.Version+1),
	)
}

// 清理清除过期文物.
func (m *Manager) Cleanup(ctx context.Context) (int, error) {
	m.cleanupMu.Lock()
	defer m.cleanupMu.Unlock()

	m.logger.Info("starting artifact cleanup")

	artifacts, err := m.store.List(ctx, ArtifactQuery{Status: StatusReady})
	if err != nil {
		return 0, err
	}

	now := time.Now()
	deleted := 0

	for _, artifact := range artifacts {
		if artifact.ExpiresAt != nil && artifact.ExpiresAt.Before(now) {
			if err := m.Delete(ctx, artifact.ID); err != nil {
				m.logger.Warn("failed to delete expired artifact",
					zap.String("id", artifact.ID),
					zap.Error(err),
				)
				continue
			}
			deleted++
		}
	}

	m.logger.Info("artifact cleanup completed", zap.Int("deleted", deleted))
	return deleted, nil
}

// 创建可选配置文物创建 。
type CreateOption func(*createOptions)

type createOptions struct {
	metadata  map[string]any
	tags      []string
	mimeType  string
	ttl       time.Duration
	createdBy string
	sessionID string
	parentID  string
	version   int
}

func WithMetadata(metadata map[string]any) CreateOption {
	return func(o *createOptions) { o.metadata = metadata }
}

func WithTags(tags ...string) CreateOption {
	return func(o *createOptions) { o.tags = tags }
}

func WithMimeType(mimeType string) CreateOption {
	return func(o *createOptions) { o.mimeType = mimeType }
}

func WithTTL(ttl time.Duration) CreateOption {
	return func(o *createOptions) { o.ttl = ttl }
}

func WithCreatedBy(createdBy string) CreateOption {
	return func(o *createOptions) { o.createdBy = createdBy }
}

func WithSessionID(sessionID string) CreateOption {
	return func(o *createOptions) { o.sessionID = sessionID }
}

func withParentID(parentID string) CreateOption {
	return func(o *createOptions) { o.parentID = parentID }
}

func withVersion(version int) CreateOption {
	return func(o *createOptions) { o.version = version }
}

func generateArtifactID() string {
	return fmt.Sprintf("art_%d", time.Now().UnixNano())
}

func computeChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
