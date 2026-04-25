package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"github.com/BaSui01/agentflow/types"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const toolApprovalWorkflowID = "tool_approval"

const (
	toolApprovalScopeRequest   = "request"
	toolApprovalScopeAgentTool = "agent_tool"
	toolApprovalScopeTool      = "tool"
)

func ToolApprovalWorkflowID() string {
	return toolApprovalWorkflowID
}

type ToolApprovalConfig struct {
	Backend           string
	GrantTTL          time.Duration
	Scope             string
	PersistPath       string
	RedisPrefix       string
	HistoryMaxEntries int
	GrantStore        ToolApprovalGrantStore
	HistoryStore      ToolApprovalHistoryStore
}

type ToolApprovalGrant struct {
	Fingerprint string    `json:"fingerprint"`
	ApprovalID  string    `json:"approval_id"`
	Scope       string    `json:"scope"`
	ToolName    string    `json:"tool_name"`
	AgentID     string    `json:"agent_id,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type ToolApprovalGrantStore interface {
	Get(ctx context.Context, key string) (*ToolApprovalGrant, error)
	Put(ctx context.Context, grant *ToolApprovalGrant) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context) ([]*ToolApprovalGrant, error)
	CleanupExpired(ctx context.Context, now time.Time) (int, error)
}

type ToolApprovalHistoryEntry struct {
	EventType       string    `json:"event_type"`
	ApprovalID      string    `json:"approval_id,omitempty"`
	Fingerprint     string    `json:"fingerprint,omitempty"`
	ToolName        string    `json:"tool_name,omitempty"`
	AgentID         string    `json:"agent_id,omitempty"`
	PrincipalID     string    `json:"principal_id,omitempty"`
	UserID          string    `json:"user_id,omitempty"`
	RunID           string    `json:"run_id,omitempty"`
	TraceID         string    `json:"trace_id,omitempty"`
	ResourceKind    string    `json:"resource_kind,omitempty"`
	ResourceID      string    `json:"resource_id,omitempty"`
	Action          string    `json:"action,omitempty"`
	RiskTier        string    `json:"risk_tier,omitempty"`
	Decision        string    `json:"decision,omitempty"`
	Status          string    `json:"status,omitempty"`
	Scope           string    `json:"scope,omitempty"`
	Comment         string    `json:"comment,omitempty"`
	ArgsFingerprint string    `json:"args_fingerprint,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
}

type ToolApprovalHistoryStore interface {
	Append(ctx context.Context, entry *ToolApprovalHistoryEntry) error
	List(ctx context.Context, limit int) ([]*ToolApprovalHistoryEntry, error)
}

type memoryToolApprovalGrantStore struct {
	mu     sync.Mutex
	grants map[string]*ToolApprovalGrant
}

type memoryToolApprovalHistoryStore struct {
	mu      sync.Mutex
	maxSize int
	entries []*ToolApprovalHistoryEntry
}

func NewMemoryToolApprovalHistoryStore(maxSize int) ToolApprovalHistoryStore {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &memoryToolApprovalHistoryStore{
		maxSize: maxSize,
		entries: make([]*ToolApprovalHistoryEntry, 0, maxSize),
	}
}

func (s *memoryToolApprovalHistoryStore) Append(_ context.Context, entry *ToolApprovalHistoryEntry) error {
	if entry == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := *entry
	s.entries = append([]*ToolApprovalHistoryEntry{&cloned}, s.entries...)
	if len(s.entries) > s.maxSize {
		s.entries = s.entries[:s.maxSize]
	}
	return nil
}

func (s *memoryToolApprovalHistoryStore) List(_ context.Context, limit int) ([]*ToolApprovalHistoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.entries) {
		limit = len(s.entries)
	}
	out := make([]*ToolApprovalHistoryEntry, 0, limit)
	for i := 0; i < limit; i++ {
		cloned := *s.entries[i]
		out = append(out, &cloned)
	}
	return out, nil
}

func NewMemoryToolApprovalGrantStore() ToolApprovalGrantStore {
	return &memoryToolApprovalGrantStore{
		grants: make(map[string]*ToolApprovalGrant),
	}
}

func (s *memoryToolApprovalGrantStore) Get(_ context.Context, key string) (*ToolApprovalGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.grants[key]
	if !ok || grant == nil {
		return nil, nil
	}
	if time.Now().After(grant.ExpiresAt) {
		delete(s.grants, key)
		return nil, nil
	}
	cloned := *grant
	return &cloned, nil
}

func (s *memoryToolApprovalGrantStore) Put(_ context.Context, grant *ToolApprovalGrant) error {
	if grant == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := *grant
	s.grants[grant.Fingerprint] = &cloned
	return nil
}

func (s *memoryToolApprovalGrantStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.grants, key)
	return nil
}

func (s *memoryToolApprovalGrantStore) List(_ context.Context) ([]*ToolApprovalGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := make([]*ToolApprovalGrant, 0, len(s.grants))
	for key, grant := range s.grants {
		if grant == nil || now.After(grant.ExpiresAt) {
			delete(s.grants, key)
			continue
		}
		cloned := *grant
		out = append(out, &cloned)
	}
	return out, nil
}

func (s *memoryToolApprovalGrantStore) CleanupExpired(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for key, grant := range s.grants {
		if grant == nil || now.After(grant.ExpiresAt) {
			delete(s.grants, key)
			removed++
		}
	}
	return removed, nil
}

type fileToolApprovalGrantStore struct {
	path   string
	logger *zap.Logger
	mu     sync.Mutex
}

type fileToolApprovalHistoryStore struct {
	path    string
	maxSize int
	mu      sync.Mutex
}

func NewFileToolApprovalHistoryStore(path string, maxSize int) ToolApprovalHistoryStore {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &fileToolApprovalHistoryStore{
		path:    strings.TrimSpace(path),
		maxSize: maxSize,
	}
}

func (s *fileToolApprovalHistoryStore) Append(_ context.Context, entry *ToolApprovalHistoryEntry) error {
	if entry == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	cloned := *entry
	entries = append([]*ToolApprovalHistoryEntry{&cloned}, entries...)
	if len(entries) > s.maxSize {
		entries = entries[:s.maxSize]
	}
	return s.saveLocked(entries)
}

func (s *fileToolApprovalHistoryStore) List(_ context.Context, limit int) ([]*ToolApprovalHistoryEntry, error) {
	if s.path == "" {
		return []*ToolApprovalHistoryEntry{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	out := make([]*ToolApprovalHistoryEntry, 0, limit)
	for i := 0; i < limit; i++ {
		cloned := *entries[i]
		out = append(out, &cloned)
	}
	return out, nil
}

func (s *fileToolApprovalHistoryStore) loadLocked() ([]*ToolApprovalHistoryEntry, error) {
	if s.path == "" {
		return []*ToolApprovalHistoryEntry{}, nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []*ToolApprovalHistoryEntry{}, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return []*ToolApprovalHistoryEntry{}, nil
	}
	var entries []*ToolApprovalHistoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *fileToolApprovalHistoryStore) saveLocked(entries []*ToolApprovalHistoryEntry) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func NewFileToolApprovalGrantStore(path string, logger *zap.Logger) ToolApprovalGrantStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &fileToolApprovalGrantStore{
		path:   strings.TrimSpace(path),
		logger: logger.With(zap.String("component", "tool_approval_grant_store")),
	}
}

func (s *fileToolApprovalGrantStore) Get(_ context.Context, key string) (*ToolApprovalGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grants, changed, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	grant, ok := grants[key]
	if !ok || grant == nil {
		if changed {
			if err := s.saveLocked(grants); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if time.Now().After(grant.ExpiresAt) {
		delete(grants, key)
		if err := s.saveLocked(grants); err != nil {
			return nil, err
		}
		return nil, nil
	}
	cloned := *grant
	if changed {
		if err := s.saveLocked(grants); err != nil {
			return nil, err
		}
	}
	return &cloned, nil
}

func (s *fileToolApprovalGrantStore) Put(_ context.Context, grant *ToolApprovalGrant) error {
	if grant == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	grants, _, err := s.loadLocked()
	if err != nil {
		return err
	}
	cloned := *grant
	grants[grant.Fingerprint] = &cloned
	return s.saveLocked(grants)
}

func (s *fileToolApprovalGrantStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	grants, _, err := s.loadLocked()
	if err != nil {
		return err
	}
	delete(grants, key)
	return s.saveLocked(grants)
}

func (s *fileToolApprovalGrantStore) List(_ context.Context) ([]*ToolApprovalGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grants, changed, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	out := make([]*ToolApprovalGrant, 0, len(grants))
	for _, grant := range grants {
		if grant == nil {
			continue
		}
		cloned := *grant
		out = append(out, &cloned)
	}
	if changed {
		if err := s.saveLocked(grants); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *fileToolApprovalGrantStore) CleanupExpired(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grants, _, err := s.loadLocked()
	if err != nil {
		return 0, err
	}
	removed := 0
	for key, grant := range grants {
		if grant == nil || now.After(grant.ExpiresAt) {
			delete(grants, key)
			removed++
		}
	}
	if removed == 0 {
		return 0, nil
	}
	return removed, s.saveLocked(grants)
}

func (s *fileToolApprovalGrantStore) loadLocked() (map[string]*ToolApprovalGrant, bool, error) {
	grants := make(map[string]*ToolApprovalGrant)
	if s.path == "" {
		return grants, false, nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return grants, false, nil
		}
		return nil, false, err
	}
	if len(raw) == 0 {
		return grants, false, nil
	}
	if err := json.Unmarshal(raw, &grants); err != nil {
		return nil, false, err
	}
	now := time.Now()
	changed := false
	for key, grant := range grants {
		if grant == nil || now.After(grant.ExpiresAt) {
			delete(grants, key)
			changed = true
		}
	}
	return grants, changed, nil
}

func (s *fileToolApprovalGrantStore) saveLocked(grants map[string]*ToolApprovalGrant) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(grants, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func defaultToolApprovalGrantStore(config ToolApprovalConfig, logger *zap.Logger) ToolApprovalGrantStore {
	if config.GrantStore != nil {
		return config.GrantStore
	}
	switch strings.ToLower(strings.TrimSpace(config.Backend)) {
	case "memory":
		return NewMemoryToolApprovalGrantStore()
	case "file":
		if strings.TrimSpace(config.PersistPath) != "" {
			return NewFileToolApprovalGrantStore(config.PersistPath, logger)
		}
	case "redis":
		// Redis backend must be injected via GrantStore by bootstrap because it owns the shared redis client lifecycle.
		return NewMemoryToolApprovalGrantStore()
	}
	if strings.TrimSpace(config.PersistPath) != "" {
		return NewFileToolApprovalGrantStore(config.PersistPath, logger)
	}
	return NewMemoryToolApprovalGrantStore()
}

func defaultToolApprovalHistoryStore(config ToolApprovalConfig) ToolApprovalHistoryStore {
	if config.HistoryStore != nil {
		return config.HistoryStore
	}
	maxEntries := config.HistoryMaxEntries
	if maxEntries <= 0 {
		maxEntries = 200
	}
	switch strings.ToLower(strings.TrimSpace(config.Backend)) {
	case "memory":
		return NewMemoryToolApprovalHistoryStore(maxEntries)
	case "file":
		if strings.TrimSpace(config.PersistPath) != "" {
			return NewFileToolApprovalHistoryStore(config.PersistPath+".history", maxEntries)
		}
	}
	return NewMemoryToolApprovalHistoryStore(maxEntries)
}

type redisToolApprovalGrantStore struct {
	client    *redis.Client
	keyPrefix string
	logger    *zap.Logger
}

type redisToolApprovalHistoryStore struct {
	client  *redis.Client
	key     string
	maxSize int64
}

func NewRedisToolApprovalHistoryStore(client *redis.Client, keyPrefix string, maxSize int) ToolApprovalHistoryStore {
	if maxSize <= 0 {
		maxSize = 200
	}
	prefix := strings.TrimSpace(keyPrefix)
	if prefix == "" {
		prefix = "agentflow:tool_approval"
	}
	return &redisToolApprovalHistoryStore{
		client:  client,
		key:     strings.TrimSuffix(prefix, ":") + ":history",
		maxSize: int64(maxSize),
	}
}

func (s *redisToolApprovalHistoryStore) Append(ctx context.Context, entry *ToolApprovalHistoryEntry) error {
	if s == nil || s.client == nil || entry == nil {
		return nil
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	pipe := s.client.TxPipeline()
	pipe.LPush(ctx, s.key, raw)
	pipe.LTrim(ctx, s.key, 0, s.maxSize-1)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *redisToolApprovalHistoryStore) List(ctx context.Context, limit int) ([]*ToolApprovalHistoryEntry, error) {
	if s == nil || s.client == nil {
		return []*ToolApprovalHistoryEntry{}, nil
	}
	if limit <= 0 {
		limit = int(s.maxSize)
	}
	rawItems, err := s.client.LRange(ctx, s.key, 0, int64(limit-1)).Result()
	if err == redis.Nil {
		return []*ToolApprovalHistoryEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]*ToolApprovalHistoryEntry, 0, len(rawItems))
	for _, raw := range rawItems {
		var entry ToolApprovalHistoryEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, err
		}
		cloned := entry
		out = append(out, &cloned)
	}
	return out, nil
}

func NewRedisToolApprovalGrantStore(client *redis.Client, keyPrefix string, logger *zap.Logger) ToolApprovalGrantStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	prefix := strings.TrimSpace(keyPrefix)
	if prefix == "" {
		prefix = "agentflow:tool_approval"
	}
	return &redisToolApprovalGrantStore{
		client:    client,
		keyPrefix: strings.TrimSuffix(prefix, ":"),
		logger:    logger.With(zap.String("component", "tool_approval_grant_store"), zap.String("backend", "redis")),
	}
}

func (s *redisToolApprovalGrantStore) Get(ctx context.Context, key string) (*ToolApprovalGrant, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, s.keyFor(key)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var grant ToolApprovalGrant
	if err := json.Unmarshal(raw, &grant); err != nil {
		return nil, err
	}
	if time.Now().After(grant.ExpiresAt) {
		_ = s.client.Del(ctx, s.keyFor(key)).Err()
		return nil, nil
	}
	return &grant, nil
}

func (s *redisToolApprovalGrantStore) Put(ctx context.Context, grant *ToolApprovalGrant) error {
	if s == nil || s.client == nil || grant == nil {
		return nil
	}
	raw, err := json.Marshal(grant)
	if err != nil {
		return err
	}
	ttl := time.Until(grant.ExpiresAt)
	if ttl <= 0 {
		ttl = time.Second
	}
	return s.client.Set(ctx, s.keyFor(grant.Fingerprint), raw, ttl).Err()
}

func (s *redisToolApprovalGrantStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Del(ctx, s.keyFor(key)).Err()
}

func (s *redisToolApprovalGrantStore) keyFor(key string) string {
	return s.keyPrefix + ":" + key
}

func (s *redisToolApprovalGrantStore) List(ctx context.Context) ([]*ToolApprovalGrant, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	keys, err := s.client.Keys(ctx, s.keyPrefix+":*").Result()
	if err != nil {
		return nil, err
	}
	out := make([]*ToolApprovalGrant, 0, len(keys))
	now := time.Now()
	for _, key := range keys {
		raw, getErr := s.client.Get(ctx, key).Bytes()
		if getErr == redis.Nil {
			continue
		}
		if getErr != nil {
			return nil, getErr
		}
		var grant ToolApprovalGrant
		if err := json.Unmarshal(raw, &grant); err != nil {
			return nil, err
		}
		if now.After(grant.ExpiresAt) {
			_ = s.client.Del(ctx, key).Err()
			continue
		}
		cloned := grant
		out = append(out, &cloned)
	}
	return out, nil
}

func (s *redisToolApprovalGrantStore) CleanupExpired(ctx context.Context, now time.Time) (int, error) {
	if s == nil || s.client == nil {
		return 0, nil
	}
	keys, err := s.client.Keys(ctx, s.keyPrefix+":*").Result()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, key := range keys {
		raw, getErr := s.client.Get(ctx, key).Bytes()
		if getErr == redis.Nil {
			continue
		}
		if getErr != nil {
			return removed, getErr
		}
		var grant ToolApprovalGrant
		if err := json.Unmarshal(raw, &grant); err != nil {
			return removed, err
		}
		if now.After(grant.ExpiresAt) {
			if err := s.client.Del(ctx, key).Err(); err != nil {
				return removed, err
			}
			removed++
		}
	}
	return removed, nil
}

type toolApprovalHandler struct {
	manager *hitl.InterruptManager
	logger  *zap.Logger
	config  ToolApprovalConfig
	store   ToolApprovalGrantStore
	history ToolApprovalHistoryStore

	mu      sync.Mutex
	pending map[string]string
}

type toolAuthorizationApprovalBackend struct {
	handler *toolApprovalHandler
}

func newToolApprovalHandler(
	manager *hitl.InterruptManager,
	config ToolApprovalConfig,
	logger *zap.Logger,
) llmtools.ApprovalHandler {
	if manager == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.GrantTTL <= 0 {
		config.GrantTTL = 15 * time.Minute
	}
	config.Scope = normalizeToolApprovalScope(config.Scope)

	return &toolApprovalHandler{
		manager: manager,
		logger:  logger.With(zap.String("component", "tool_approval_handler")),
		config:  config,
		store:   defaultToolApprovalGrantStore(config, logger),
		history: defaultToolApprovalHistoryStore(config),
		pending: make(map[string]string),
	}
}

func newToolAuthorizationApprovalBackend(handler llmtools.ApprovalHandler) usecase.ApprovalBackend {
	typed, ok := handler.(*toolApprovalHandler)
	if !ok || typed == nil {
		return nil
	}
	return &toolAuthorizationApprovalBackend{handler: typed}
}

func (b *toolAuthorizationApprovalBackend) RequestApproval(
	ctx context.Context,
	req types.AuthorizationRequest,
	preliminary *types.AuthorizationDecision,
) (*types.AuthorizationDecision, error) {
	if b == nil || b.handler == nil {
		return nil, fmt.Errorf("tool approval backend is not configured")
	}
	permCtx := authorizationRequestPermissionContext(req)
	rule := authorizationApprovalRule(preliminary)
	approvalID, err := b.handler.RequestApproval(ctx, permCtx, rule)
	if err != nil {
		return nil, err
	}
	return b.approvalDecision(ctx, approvalID, preliminary)
}

func (b *toolAuthorizationApprovalBackend) CheckApproval(
	ctx context.Context,
	approvalID string,
) (*types.AuthorizationDecision, error) {
	return b.approvalDecision(ctx, approvalID, nil)
}

func (b *toolAuthorizationApprovalBackend) Revoke(ctx context.Context, fingerprint string) error {
	if b == nil || b.handler == nil || b.handler.store == nil {
		return nil
	}
	key := strings.TrimSpace(fingerprint)
	key = strings.TrimPrefix(key, grantApprovalIDPrefix)
	if key == "" {
		return nil
	}
	return b.handler.store.Delete(ctx, key)
}

func (b *toolAuthorizationApprovalBackend) approvalDecision(
	ctx context.Context,
	approvalID string,
	preliminary *types.AuthorizationDecision,
) (*types.AuthorizationDecision, error) {
	normalizedID := strings.TrimSpace(approvalID)
	if normalizedID == "" {
		return &types.AuthorizationDecision{
			Decision: types.DecisionRequireApproval,
			Reason:   approvalDecisionReason(preliminary, "approval required"),
			PolicyID: approvalDecisionPolicyID(preliminary),
			Scope:    approvalDecisionScope(preliminary),
		}, nil
	}
	approved, err := b.handler.CheckApprovalStatus(ctx, normalizedID)
	if err != nil {
		return nil, err
	}
	if approved {
		return &types.AuthorizationDecision{
			Decision:   types.DecisionAllow,
			Reason:     "approval granted: " + normalizedID,
			PolicyID:   approvalDecisionPolicyID(preliminary),
			ApprovalID: normalizedID,
			Scope:      approvalDecisionScope(preliminary),
		}, nil
	}
	return &types.AuthorizationDecision{
		Decision:   types.DecisionRequireApproval,
		Reason:     approvalDecisionReason(preliminary, "approval pending"),
		PolicyID:   approvalDecisionPolicyID(preliminary),
		ApprovalID: normalizedID,
		Scope:      approvalDecisionScope(preliminary),
	}, nil
}

func authorizationRequestPermissionContext(req types.AuthorizationRequest) *llmtools.PermissionContext {
	return &llmtools.PermissionContext{
		AgentID:   stringValue(req.Context, "agent_id"),
		UserID:    authorizationRequestUserID(req),
		Roles:     append([]string(nil), req.Principal.Roles...),
		ToolName:  strings.TrimSpace(req.ResourceID),
		Arguments: anyMapValue(req.Context, "arguments"),
		Metadata:  stringMapValue(req.Context, "metadata"),
		RequestAt: time.Now(),
		TraceID:   stringValue(req.Context, "trace_id"),
		SessionID: stringValue(req.Context, "session_id"),
	}
}

func authorizationRequestUserID(req types.AuthorizationRequest) string {
	if userID := stringValue(req.Context, "user_id"); userID != "" {
		return userID
	}
	if req.Principal.Kind == types.PrincipalUser {
		return strings.TrimSpace(req.Principal.ID)
	}
	return ""
}

func authorizationApprovalRule(decision *types.AuthorizationDecision) *llmtools.PermissionRule {
	if decision == nil {
		return nil
	}
	return &llmtools.PermissionRule{
		ID:       strings.TrimSpace(decision.PolicyID),
		Name:     strings.TrimSpace(decision.PolicyID),
		Decision: llmtools.PermissionRequireApproval,
	}
}

func approvalDecisionReason(decision *types.AuthorizationDecision, fallback string) string {
	if decision == nil || strings.TrimSpace(decision.Reason) == "" {
		return fallback
	}
	return strings.TrimSpace(decision.Reason)
}

func approvalDecisionPolicyID(decision *types.AuthorizationDecision) string {
	if decision == nil {
		return ""
	}
	return strings.TrimSpace(decision.PolicyID)
}

func approvalDecisionScope(decision *types.AuthorizationDecision) string {
	if decision == nil {
		return ""
	}
	return strings.TrimSpace(decision.Scope)
}

func (h *toolApprovalHandler) RequestApproval(
	ctx context.Context,
	permCtx *llmtools.PermissionContext,
	rule *llmtools.PermissionRule,
) (string, error) {
	if h == nil || h.manager == nil {
		return "", fmt.Errorf("tool approval manager is not configured")
	}
	if permCtx == nil {
		return "", fmt.Errorf("permission context is required")
	}

	key := approvalFingerprint(permCtx, rule, h.config.Scope)
	options := []hitl.Option{
		{ID: "approve", Label: "Approve", IsDefault: true},
		{ID: "reject", Label: "Reject"},
	}

	toolName := strings.TrimSpace(permCtx.ToolName)
	if toolName == "" {
		toolName = "tool"
	}

	h.mu.Lock()
	if existingID := h.lookupExistingApprovalLocked(ctx, key); existingID != "" {
		h.mu.Unlock()
		return existingID, nil
	}

	interrupt, err := h.manager.CreatePendingInterrupt(ctx, hitl.InterruptOptions{
		WorkflowID:  toolApprovalWorkflowID,
		NodeID:      toolName,
		Type:        hitl.InterruptTypeApproval,
		Title:       fmt.Sprintf("Tool approval required: %s", toolName),
		Description: buildToolApprovalDescription(permCtx, rule),
		Data: map[string]any{
			"tool_name":            permCtx.ToolName,
			"agent_id":             permCtx.AgentID,
			"user_id":              permCtx.UserID,
			"trace_id":             permCtx.TraceID,
			"session_id":           permCtx.SessionID,
			"arguments":            permCtx.Arguments,
			"rule_id":              ruleID(rule),
			"approval_fingerprint": key,
			"approval_scope":       h.config.Scope,
			"approval_grant_ttl":   h.config.GrantTTL.String(),
		},
		Options: options,
		Timeout: 30 * time.Minute,
		Metadata: map[string]any{
			"tool_name":            permCtx.ToolName,
			"agent_id":             permCtx.AgentID,
			"user_id":              permCtx.UserID,
			"trace_id":             permCtx.TraceID,
			"session_id":           permCtx.SessionID,
			"approval_fingerprint": key,
			"approval_scope":       h.config.Scope,
			"approval_grant_ttl":   h.config.GrantTTL.String(),
		},
	})
	if err != nil {
		h.mu.Unlock()
		return "", err
	}
	if interrupt == nil {
		h.mu.Unlock()
		return "", fmt.Errorf("approval interrupt is nil")
	}
	h.pending[key] = strings.TrimSpace(interrupt.ID)
	h.mu.Unlock()

	h.appendHistory(context.Background(), &ToolApprovalHistoryEntry{
		EventType:   "approval_requested",
		ApprovalID:  interrupt.ID,
		Fingerprint: key,
		ToolName:    toolName,
		AgentID:     strings.TrimSpace(permCtx.AgentID),
		Status:      string(hitl.InterruptStatusPending),
		Scope:       h.config.Scope,
		Timestamp:   time.Now().UTC(),
	})
	return interrupt.ID, nil
}

func (h *toolApprovalHandler) CheckApprovalStatus(ctx context.Context, approvalID string) (bool, error) {
	if h == nil || h.manager == nil {
		return false, fmt.Errorf("tool approval manager is not configured")
	}
	if strings.HasPrefix(approvalID, grantApprovalIDPrefix) {
		return h.checkPersistedGrant(ctx, strings.TrimPrefix(approvalID, grantApprovalIDPrefix))
	}

	interrupt, err := h.manager.GetInterrupt(ctx, approvalID)
	if err != nil {
		return false, err
	}
	if interrupt == nil || interrupt.Response == nil || !interrupt.Response.Approved {
		return false, nil
	}
	key := metadataStringAny(interrupt.Metadata, "approval_fingerprint")
	if key == "" {
		return false, nil
	}
	if err := h.ensureGrantStored(ctx, key, interrupt); err != nil {
		return false, err
	}
	return h.checkPersistedGrant(ctx, key)
}

const grantApprovalIDPrefix = "grant:"

func (h *toolApprovalHandler) lookupExistingApproval(ctx context.Context, key string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lookupExistingApprovalLocked(ctx, key)
}

func (h *toolApprovalHandler) lookupExistingApprovalLocked(ctx context.Context, key string) string {
	interruptID := strings.TrimSpace(h.pending[key])
	if interruptID != "" {
		interrupt, err := h.manager.GetInterrupt(ctx, interruptID)
		if err == nil && interrupt != nil {
			switch interrupt.Status {
			case hitl.InterruptStatusPending:
				return interruptID
			case hitl.InterruptStatusResolved:
				if interrupt.Response != nil && interrupt.Response.Approved {
					if err := h.ensureGrantStored(ctx, key, interrupt); err == nil {
						delete(h.pending, key)
						if ok, _ := h.checkPersistedGrant(ctx, key); ok {
							return grantApprovalIDPrefix + key
						}
					}
				}
			}
		}
		delete(h.pending, key)
	}

	ok, err := h.checkPersistedGrant(ctx, key)
	if err != nil || !ok {
		return ""
	}
	return grantApprovalIDPrefix + key
}

func (h *toolApprovalHandler) rememberPending(key, interruptID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending[key] = strings.TrimSpace(interruptID)
}

func (h *toolApprovalHandler) forgetPending(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.pending, key)
}

func (h *toolApprovalHandler) checkPersistedGrant(ctx context.Context, key string) (bool, error) {
	if h.store == nil {
		return false, nil
	}
	grant, err := h.store.Get(ctx, key)
	if err != nil {
		return false, err
	}
	if grant == nil {
		return false, nil
	}
	if time.Now().After(grant.ExpiresAt) {
		_ = h.store.Delete(ctx, key)
		return false, nil
	}
	return true, nil
}

func (h *toolApprovalHandler) ensureGrantStored(ctx context.Context, key string, interrupt *hitl.Interrupt) error {
	if h.store == nil || interrupt == nil || interrupt.Response == nil || !interrupt.Response.Approved {
		return nil
	}
	grant := &ToolApprovalGrant{
		Fingerprint: key,
		ApprovalID:  grantApprovalIDPrefix + key,
		Scope:       normalizeToolApprovalScope(metadataStringAny(interrupt.Metadata, "approval_scope")),
		ToolName:    metadataStringAny(interrupt.Metadata, "tool_name"),
		AgentID:     metadataStringAny(interrupt.Metadata, "agent_id"),
	}
	base := time.Now()
	if interrupt.ResolvedAt != nil && !interrupt.ResolvedAt.IsZero() {
		base = *interrupt.ResolvedAt
	}
	grant.ExpiresAt = base.Add(h.config.GrantTTL)
	if err := h.store.Put(ctx, grant); err != nil {
		return err
	}
	h.appendHistory(context.Background(), &ToolApprovalHistoryEntry{
		EventType:   "approval_granted",
		ApprovalID:  interrupt.ID,
		Fingerprint: key,
		ToolName:    grant.ToolName,
		AgentID:     grant.AgentID,
		Status:      string(hitl.InterruptStatusResolved),
		Scope:       grant.Scope,
		Timestamp:   time.Now().UTC(),
	})
	return nil
}

func buildToolApprovalDescription(
	permCtx *llmtools.PermissionContext,
	rule *llmtools.PermissionRule,
) string {
	parts := []string{
		fmt.Sprintf("Tool %q requested elevated access.", strings.TrimSpace(permCtx.ToolName)),
	}
	if permCtx.AgentID != "" {
		parts = append(parts, fmt.Sprintf("Agent: %s.", permCtx.AgentID))
	}
	if risk := metadataValue(permCtx, "hosted_tool_risk"); risk != "" {
		parts = append(parts, fmt.Sprintf("Risk tier: %s.", risk))
	}
	if rule != nil && strings.TrimSpace(rule.Name) != "" {
		parts = append(parts, fmt.Sprintf("Matched rule: %s.", strings.TrimSpace(rule.Name)))
	}
	return strings.Join(parts, " ")
}

func metadataValue(permCtx *llmtools.PermissionContext, key string) string {
	if permCtx == nil || permCtx.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(permCtx.Metadata[key])
}

func metadataStringAny(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func ruleID(rule *llmtools.PermissionRule) string {
	if rule == nil {
		return ""
	}
	return strings.TrimSpace(rule.ID)
}

func approvalFingerprint(
	permCtx *llmtools.PermissionContext,
	rule *llmtools.PermissionRule,
	scope string,
) string {
	normalizedScope := normalizeToolApprovalScope(scope)
	payload := map[string]any{
		"scope":     normalizedScope,
		"tool_name": strings.TrimSpace(permCtx.ToolName),
		"rule_id":   ruleID(rule),
		"risk":      metadataValue(permCtx, "hosted_tool_risk"),
		"tool_type": metadataValue(permCtx, "hosted_tool_type"),
	}
	switch normalizedScope {
	case toolApprovalScopeRequest:
		payload["agent_id"] = strings.TrimSpace(permCtx.AgentID)
		payload["args_hash"] = approvalArgumentsHash(permCtx.Arguments)
	case toolApprovalScopeAgentTool:
		payload["agent_id"] = strings.TrimSpace(permCtx.AgentID)
	case toolApprovalScopeTool:
	default:
		payload["agent_id"] = strings.TrimSpace(permCtx.AgentID)
		payload["args_hash"] = approvalArgumentsHash(permCtx.Arguments)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func approvalArgumentsHash(arguments map[string]any) string {
	if len(arguments) == 0 {
		return ""
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalizeToolApprovalScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case toolApprovalScopeRequest:
		return toolApprovalScopeRequest
	case toolApprovalScopeAgentTool:
		return toolApprovalScopeAgentTool
	case toolApprovalScopeTool:
		return toolApprovalScopeTool
	default:
		return toolApprovalScopeRequest
	}
}

func (h *toolApprovalHandler) appendHistory(ctx context.Context, entry *ToolApprovalHistoryEntry) {
	if h == nil || h.history == nil || entry == nil {
		return
	}
	_ = h.history.Append(ctx, entry)
}

func BuildToolApprovalGrantStore(
	cfg *config.Config,
	logger *zap.Logger,
) (*redis.Client, ToolApprovalGrantStore, error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("config is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	switch strings.ToLower(strings.TrimSpace(cfg.HostedTools.Approval.Backend)) {
	case "memory":
		return nil, NewMemoryToolApprovalGrantStore(), nil
	case "file":
		return nil, NewFileToolApprovalGrantStore(cfg.HostedTools.Approval.PersistPath, logger), nil
	case "redis":
		client, err := newToolApprovalRedisClient(cfg, logger)
		if err != nil {
			return nil, nil, err
		}
		return client, NewRedisToolApprovalGrantStore(client, cfg.HostedTools.Approval.RedisPrefix, logger), nil
	default:
		return nil, nil, fmt.Errorf("unsupported tool approval backend: %s", cfg.HostedTools.Approval.Backend)
	}
}

func BuildToolApprovalHistoryStore(
	cfg *config.Config,
	redisClient *redis.Client,
) (ToolApprovalHistoryStore, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	maxEntries := cfg.HostedTools.Approval.HistoryMaxEntries
	if maxEntries <= 0 {
		maxEntries = 200
	}
	switch strings.ToLower(strings.TrimSpace(cfg.HostedTools.Approval.Backend)) {
	case "memory":
		return NewMemoryToolApprovalHistoryStore(maxEntries), nil
	case "file":
		return NewFileToolApprovalHistoryStore(cfg.HostedTools.Approval.PersistPath+".history", maxEntries), nil
	case "redis":
		if redisClient == nil {
			return nil, fmt.Errorf("redis client is required when hosted_tools.approval.backend=redis")
		}
		return NewRedisToolApprovalHistoryStore(redisClient, cfg.HostedTools.Approval.RedisPrefix, maxEntries), nil
	default:
		return nil, fmt.Errorf("unsupported tool approval backend: %s", cfg.HostedTools.Approval.Backend)
	}
}

func newToolApprovalRedisClient(cfg *config.Config, logger *zap.Logger) (*redis.Client, error) {
	addr := strings.TrimSpace(cfg.Redis.Addr)
	if addr == "" {
		return nil, fmt.Errorf("redis address is required when hosted_tools.approval.backend=redis")
	}

	var (
		opts *redis.Options
		err  error
	)

	if strings.HasPrefix(addr, "redis://") || strings.HasPrefix(addr, "rediss://") {
		parsed, parseErr := url.Parse(addr)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid redis url: %w", parseErr)
		}
		scheme := strings.ToLower(parsed.Scheme)
		host := parsed.Hostname()
		if scheme == "redis" && !isLoopbackHost(host) {
			return nil, fmt.Errorf("insecure redis:// is only allowed for loopback hosts, use rediss:// for %q", host)
		}
		opts, err = redis.ParseURL(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid redis url: %w", err)
		}
		if cfg.Redis.Password != "" && opts.Password == "" {
			opts.Password = cfg.Redis.Password
		}
		if cfg.Redis.DB != 0 && opts.DB == 0 {
			opts.DB = cfg.Redis.DB
		}
		if cfg.Redis.PoolSize > 0 {
			opts.PoolSize = cfg.Redis.PoolSize
		}
		if cfg.Redis.MinIdleConns > 0 {
			opts.MinIdleConns = cfg.Redis.MinIdleConns
		}
		if scheme == "rediss" && opts.TLSConfig == nil {
			opts.TLSConfig = tlsutil.DefaultTLSConfig()
		}
		if scheme == "redis" && isLoopbackHost(host) {
			logger.Warn("using insecure redis:// for loopback host in tool approval store", zap.String("host", host))
		}
	} else {
		host := hostFromRedisAddr(addr)
		if !isLoopbackHost(host) {
			return nil, fmt.Errorf("non-loopback redis address %q requires rediss:// scheme", host)
		}
		opts = &redis.Options{
			Addr:         addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
		}
		logger.Warn("using insecure plaintext redis connection for loopback host in tool approval store", zap.String("host", host))
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	return client, nil
}

func hostFromRedisAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(strings.Trim(host, "[]"))
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

type toolApprovalRuntime struct {
	manager *hitl.InterruptManager
	store   ToolApprovalGrantStore
	history ToolApprovalHistoryStore
	config  ToolApprovalConfig
}

type authorizationAuditHistoryRuntime struct {
	history ToolApprovalHistoryStore
}

func (r *toolApprovalRuntime) GetInterrupt(ctx context.Context, interruptID string) (*hitl.Interrupt, error) {
	return r.manager.GetInterrupt(ctx, interruptID)
}

func (r *toolApprovalRuntime) ListInterrupts(ctx context.Context, workflowID string, status hitl.InterruptStatus) ([]*hitl.Interrupt, error) {
	return r.manager.ListInterrupts(ctx, workflowID, status)
}

func (r *toolApprovalRuntime) ResolveInterrupt(ctx context.Context, interruptID string, response *hitl.Response) error {
	interrupt, err := r.manager.GetInterrupt(ctx, interruptID)
	if err != nil {
		return err
	}
	if err := r.manager.ResolveInterrupt(ctx, interruptID, response); err != nil {
		return err
	}
	if interrupt != nil && r.history != nil {
		status := string(hitl.InterruptStatusRejected)
		if response != nil && response.Approved {
			status = string(hitl.InterruptStatusResolved)
		}
		_ = r.history.Append(context.Background(), &ToolApprovalHistoryEntry{
			EventType:   "approval_resolved",
			ApprovalID:  interruptID,
			Fingerprint: metadataStringAny(interrupt.Metadata, "approval_fingerprint"),
			ToolName:    metadataStringAny(interrupt.Metadata, "tool_name"),
			AgentID:     metadataStringAny(interrupt.Metadata, "agent_id"),
			Status:      status,
			Scope:       metadataStringAny(interrupt.Metadata, "approval_scope"),
			Comment:     strings.TrimSpace(response.Comment),
			Timestamp:   time.Now().UTC(),
		})
	}
	return nil
}

func (r *toolApprovalRuntime) GrantStats(ctx context.Context) (*usecase.ToolApprovalStats, error) {
	count := 0
	if r.store != nil {
		grants, err := r.store.List(ctx)
		if err != nil {
			return nil, err
		}
		count = len(grants)
	}
	return &usecase.ToolApprovalStats{
		Backend:          strings.ToLower(strings.TrimSpace(r.config.Backend)),
		Scope:            normalizeToolApprovalScope(r.config.Scope),
		GrantTTL:         r.config.GrantTTL.String(),
		ActiveGrantCount: count,
	}, nil
}

func (r *toolApprovalRuntime) CleanupExpiredGrants(ctx context.Context) (int, error) {
	if r.store == nil {
		return 0, nil
	}
	removed, err := r.store.CleanupExpired(ctx, time.Now())
	if err != nil {
		return 0, err
	}
	if removed > 0 && r.history != nil {
		_ = r.history.Append(context.Background(), &ToolApprovalHistoryEntry{
			EventType: "grant_cleanup",
			Comment:   fmt.Sprintf("removed %d expired grants", removed),
			Timestamp: time.Now().UTC(),
		})
	}
	return removed, nil
}

func (r *toolApprovalRuntime) ListGrants(ctx context.Context) ([]*usecase.ToolApprovalGrantView, error) {
	if r.store == nil {
		return []*usecase.ToolApprovalGrantView{}, nil
	}
	grants, err := r.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*usecase.ToolApprovalGrantView, 0, len(grants))
	for _, grant := range grants {
		if grant == nil {
			continue
		}
		out = append(out, &usecase.ToolApprovalGrantView{
			Fingerprint: grant.Fingerprint,
			ApprovalID:  grant.ApprovalID,
			Scope:       grant.Scope,
			ToolName:    grant.ToolName,
			AgentID:     grant.AgentID,
			ExpiresAt:   grant.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (r *toolApprovalRuntime) RevokeGrant(ctx context.Context, fingerprint string) error {
	if r.store == nil {
		return nil
	}
	key := strings.TrimSpace(fingerprint)
	if err := r.store.Delete(ctx, key); err != nil {
		return err
	}
	if r.history != nil {
		_ = r.history.Append(context.Background(), &ToolApprovalHistoryEntry{
			EventType:   "grant_revoked",
			Fingerprint: key,
			Timestamp:   time.Now().UTC(),
		})
	}
	return nil
}

func (r *toolApprovalRuntime) ListHistory(ctx context.Context, limit int) ([]*usecase.ToolApprovalHistoryEntry, error) {
	return listToolApprovalHistory(ctx, r.history, limit)
}

func (r *authorizationAuditHistoryRuntime) ListHistory(ctx context.Context, limit int) ([]*usecase.ToolApprovalHistoryEntry, error) {
	return listToolApprovalHistory(ctx, r.history, limit)
}

func listToolApprovalHistory(
	ctx context.Context,
	history ToolApprovalHistoryStore,
	limit int,
) ([]*usecase.ToolApprovalHistoryEntry, error) {
	if history == nil {
		return []*usecase.ToolApprovalHistoryEntry{}, nil
	}
	rows, err := history.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*usecase.ToolApprovalHistoryEntry, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		cloned := *row
		out = append(out, &usecase.ToolApprovalHistoryEntry{
			EventType:       cloned.EventType,
			ApprovalID:      cloned.ApprovalID,
			Fingerprint:     cloned.Fingerprint,
			ToolName:        cloned.ToolName,
			AgentID:         cloned.AgentID,
			PrincipalID:     cloned.PrincipalID,
			UserID:          cloned.UserID,
			RunID:           cloned.RunID,
			TraceID:         cloned.TraceID,
			ResourceKind:    cloned.ResourceKind,
			ResourceID:      cloned.ResourceID,
			Action:          cloned.Action,
			RiskTier:        cloned.RiskTier,
			Decision:        cloned.Decision,
			Status:          cloned.Status,
			Scope:           cloned.Scope,
			Comment:         cloned.Comment,
			ArgsFingerprint: cloned.ArgsFingerprint,
			Timestamp:       cloned.Timestamp.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}
