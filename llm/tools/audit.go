package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AuditEventType 表示审计事件的类型.
type AuditEventType string

const (
	AuditEventToolCall       AuditEventType = "tool_call"
	AuditEventToolResult     AuditEventType = "tool_result"
	AuditEventPermissionCheck AuditEventType = "permission_check"
	AuditEventRateLimitHit   AuditEventType = "rate_limit_hit"
	AuditEventCostAlert      AuditEventType = "cost_alert"
)

// AuditEntry 表示单条审计记录.
type AuditEntry struct {
	ID            string            `json:"id"`
	Timestamp     time.Time         `json:"timestamp"`
	EventType     AuditEventType    `json:"event_type"`
	AgentID       string            `json:"agent_id"`
	UserID        string            `json:"user_id"`
	SessionID     string            `json:"session_id,omitempty"`
	TraceID       string            `json:"trace_id,omitempty"`
	ToolName      string            `json:"tool_name"`
	Arguments     json.RawMessage   `json:"arguments,omitempty"`
	Result        json.RawMessage   `json:"result,omitempty"`
	Error         string            `json:"error,omitempty"`
	Duration      time.Duration     `json:"duration,omitempty"`
	Decision      string            `json:"decision,omitempty"` // For permission checks
	Cost          float64           `json:"cost,omitempty"`     // For cost tracking
	Metadata      map[string]string `json:"metadata,omitempty"`
	RequestIP     string            `json:"request_ip,omitempty"`
}

// AuditLogger 定义审计日志的接口.
type AuditLogger interface {
	// Log 同步记录一条审计条目。
	Log(ctx context.Context, entry *AuditEntry) error

	// LogAsync 异步记录审计条目.
	LogAsync(entry *AuditEntry)

	// Query 根据过滤器检索审计条目。
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)

	// Close 关闭审计日志并刷新待写入条目。
	Close() error
}

// AuditFilter 定义查询审计条目的过滤器.
type AuditFilter struct {
	AgentID    string         `json:"agent_id,omitempty"`
	UserID     string         `json:"user_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	EventType  AuditEventType `json:"event_type,omitempty"`
	StartTime  *time.Time     `json:"start_time,omitempty"`
	EndTime    *time.Time     `json:"end_time,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	Limit      int            `json:"limit,omitempty"`
	Offset     int            `json:"offset,omitempty"`
}

// AuditBackend 定义审计存储后端的接口.
type AuditBackend interface {
	// Write 将审计条目写入后端。
	Write(ctx context.Context, entry *AuditEntry) error

	// Query 从后端检索审计条目.
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)

	// Close 关闭后端。
	Close() error
}

// DefaultAuditLogger 是 AuditLogger 的默认实现.
type DefaultAuditLogger struct {
	backends     []AuditBackend
	asyncQueue   chan *AuditEntry
	wg           sync.WaitGroup
	logger       *zap.Logger
	closed       bool
	closeMu      sync.Mutex
	idGenerator  func() string
}

// AuditLoggerConfig 配置审计日志。
type AuditLoggerConfig struct {
	Backends       []AuditBackend
	AsyncQueueSize int
	AsyncWorkers   int
	IDGenerator    func() string
}

// NewAuditLogger 创建新的审计日志.
func NewAuditLogger(cfg *AuditLoggerConfig, logger *zap.Logger) *DefaultAuditLogger {
	if logger == nil {
		logger = zap.NewNop()
	}

	if cfg.AsyncQueueSize == 0 {
		cfg.AsyncQueueSize = 10000
	}
	if cfg.AsyncWorkers == 0 {
		cfg.AsyncWorkers = 4
	}
	if cfg.IDGenerator == nil {
		cfg.IDGenerator = generateAuditID
	}

	al := &DefaultAuditLogger{
		backends:    cfg.Backends,
		asyncQueue:  make(chan *AuditEntry, cfg.AsyncQueueSize),
		logger:      logger.With(zap.String("component", "audit_logger")),
		idGenerator: cfg.IDGenerator,
	}

	// 启动异步工作协程
	for i := 0; i < cfg.AsyncWorkers; i++ {
		al.wg.Add(1)
		go al.asyncWorker()
	}

	return al
}

// asyncWorker 异步处理审计条目。
func (al *DefaultAuditLogger) asyncWorker() {
	defer al.wg.Done()

	for entry := range al.asyncQueue {
		if err := al.writeToBackends(context.Background(), entry); err != nil {
			al.logger.Error("failed to write audit entry",
				zap.String("entry_id", entry.ID),
				zap.Error(err),
			)
		}
	}
}

// writeToBackends 将条目写入所有后端。
func (al *DefaultAuditLogger) writeToBackends(ctx context.Context, entry *AuditEntry) error {
	var lastErr error
	for _, backend := range al.backends {
		if err := backend.Write(ctx, entry); err != nil {
			al.logger.Error("backend write failed",
				zap.String("entry_id", entry.ID),
				zap.Error(err),
			)
			lastErr = err
		}
	}
	return lastErr
}

// Log 同步记录审计条目。
func (al *DefaultAuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	al.closeMu.Lock()
	if al.closed {
		al.closeMu.Unlock()
		return fmt.Errorf("audit logger is closed")
	}
	al.closeMu.Unlock()

	if entry.ID == "" {
		entry.ID = al.idGenerator()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	return al.writeToBackends(ctx, entry)
}

// LogAsync 异步记录审计条目.
func (al *DefaultAuditLogger) LogAsync(entry *AuditEntry) {
	al.closeMu.Lock()
	if al.closed {
		al.closeMu.Unlock()
		al.logger.Warn("audit logger is closed, dropping entry")
		return
	}
	al.closeMu.Unlock()

	if entry.ID == "" {
		entry.ID = al.idGenerator()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	select {
	case al.asyncQueue <- entry:
		// 成功排队
	default:
		al.logger.Warn("audit queue full, dropping entry",
			zap.String("entry_id", entry.ID),
		)
	}
}

// Query 根据过滤器检索审计条目。
func (al *DefaultAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	if len(al.backends) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}

	// 从第一个支持查询的后端查询
	return al.backends[0].Query(ctx, filter)
}

// Close 关闭审计日志并刷新待写入条目。
func (al *DefaultAuditLogger) Close() error {
	al.closeMu.Lock()
	if al.closed {
		al.closeMu.Unlock()
		return nil
	}
	al.closed = true
	al.closeMu.Unlock()

	// 关闭异步队列并等待工作协程完成
	close(al.asyncQueue)
	al.wg.Wait()

	// 关闭所有后端
	var lastErr error
	for _, backend := range al.backends {
		if err := backend.Close(); err != nil {
			lastErr = err
		}
	}

	al.logger.Info("audit logger closed")
	return lastErr
}

// ====== 内存后端 ======

// MemoryAuditBackend 将审计条目存储在内存中.
type MemoryAuditBackend struct {
	entries  []*AuditEntry
	maxSize  int
	mu       sync.RWMutex
}

// NewMemoryAuditBackend 创建新的内存审计后端.
func NewMemoryAuditBackend(maxSize int) *MemoryAuditBackend {
	if maxSize <= 0 {
		maxSize = 100000
	}
	return &MemoryAuditBackend{
		entries: make([]*AuditEntry, 0),
		maxSize: maxSize,
	}
}

// Write 将审计条目写入内存。
func (m *MemoryAuditBackend) Write(ctx context.Context, entry *AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果达到容量上限，淘汰最旧的条目
	if len(m.entries) >= m.maxSize {
		// 删除最老的10%
		removeCount := m.maxSize / 10
		if removeCount < 1 {
			removeCount = 1
		}
		m.entries = m.entries[removeCount:]
	}

	m.entries = append(m.entries, entry)
	return nil
}

// Query 从内存中检索审计条目。
func (m *MemoryAuditBackend) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*AuditEntry

	for _, entry := range m.entries {
		if m.matchesFilter(entry, filter) {
			results = append(results, entry)
		}
	}

	// 应用偏移和限制
	if filter.Offset > 0 {
		if filter.Offset >= len(results) {
			return []*AuditEntry{}, nil
		}
		results = results[filter.Offset:]
	}

	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results, nil
}

// matchesFilter 检查条目是否匹配过滤器。
func (m *MemoryAuditBackend) matchesFilter(entry *AuditEntry, filter *AuditFilter) bool {
	if filter.AgentID != "" && entry.AgentID != filter.AgentID {
		return false
	}
	if filter.UserID != "" && entry.UserID != filter.UserID {
		return false
	}
	if filter.ToolName != "" && entry.ToolName != filter.ToolName {
		return false
	}
	if filter.EventType != "" && entry.EventType != filter.EventType {
		return false
	}
	if filter.SessionID != "" && entry.SessionID != filter.SessionID {
		return false
	}
	if filter.TraceID != "" && entry.TraceID != filter.TraceID {
		return false
	}
	if filter.StartTime != nil && entry.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && entry.Timestamp.After(*filter.EndTime) {
		return false
	}
	return true
}

// Close 关闭内存后端。
func (m *MemoryAuditBackend) Close() error {
	return nil
}

// ====== 文件后端 ======

// FileAuditBackend 将审计条目存储在文件中.
type FileAuditBackend struct {
	dir         string
	currentFile *os.File
	currentDate string
	maxFileSize int64
	mu          sync.Mutex
	logger      *zap.Logger
}

// FileAuditBackendConfig 配置文件审计后端.
type FileAuditBackendConfig struct {
	Directory   string
	MaxFileSize int64 // Max file size in bytes before rotation
}

// NewFileAuditBackend 创建新的文件审计后端.
func NewFileAuditBackend(cfg *FileAuditBackendConfig, logger *zap.Logger) (*FileAuditBackend, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	if cfg.Directory == "" {
		cfg.Directory = "./audit_logs"
	}
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 100 * 1024 * 1024 // 100MB
	}

	// 如果目录不存在, 则创建目录
	if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create audit directory: %w", err)
	}

	return &FileAuditBackend{
		dir:         cfg.Directory,
		maxFileSize: cfg.MaxFileSize,
		logger:      logger.With(zap.String("component", "file_audit_backend")),
	}, nil
}

// Write 将审计条目写入文件。
func (f *FileAuditBackend) Write(ctx context.Context, entry *AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 检查是否需要轮转文件
	currentDate := entry.Timestamp.Format("2006-01-02")
	if f.currentFile == nil || f.currentDate != currentDate {
		if err := f.rotateFile(currentDate); err != nil {
			return err
		}
	}

	// 检查文件大小
	if f.currentFile != nil {
		info, err := f.currentFile.Stat()
		if err == nil && info.Size() >= f.maxFileSize {
			if err := f.rotateFile(currentDate); err != nil {
				return err
			}
		}
	}

	// 将条目写入 JSON 行
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	if _, err := f.currentFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}

	return nil
}

// rotateFile 轮转到新文件。
func (f *FileAuditBackend) rotateFile(date string) error {
	// 关闭当前文件
	if f.currentFile != nil {
		f.currentFile.Close()
	}

	// 创建新文件
	filename := filepath.Join(f.dir, fmt.Sprintf("audit_%s_%d.jsonl", date, time.Now().UnixNano()))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create audit file: %w", err)
	}

	f.currentFile = file
	f.currentDate = date
	f.logger.Info("rotated audit file", zap.String("filename", filename))

	return nil
}

// Query 从文件中检索审计条目（功能有限）.
func (f *FileAuditBackend) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	// 注意: 这是一个仅读取当前文件的简化实现
	// 生产环境需要根据过滤器从多个文件中读取
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.currentFile == nil {
		return []*AuditEntry{}, nil
	}

	// 完整实现需要:
	// 1. 列出目录中的所有文件
	// 2. 根据日期范围过滤文件
	// 3. 读取并解析每个文件
	// 4. 应用过滤器并返回结果

	return []*AuditEntry{}, fmt.Errorf("file query not fully implemented; use memory backend for queries")
}

// Close 关闭文件后端。
func (f *FileAuditBackend) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.currentFile != nil {
		err := f.currentFile.Close()
		f.currentFile = nil
		return err
	}
	return nil
}

// ====== 数据库后端接口 ======

// DatabaseAuditBackend 定义数据库审计后端的接口.
// 实现可以使用 PostgreSQL、MySQL、MongoDB 等.
type DatabaseAuditBackend interface {
	AuditBackend
	// Migrate 创建或更新数据库表结构。
	Migrate(ctx context.Context) error
}

// ====== 审计中间件 ======

// AuditMiddleware 创建记录工具执行的中间件.
func AuditMiddleware(auditLogger AuditLogger) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// 提取上下文信息
			permCtx, _ := GetPermissionContext(ctx)

			entry := &AuditEntry{
				Timestamp: time.Now(),
				EventType: AuditEventToolCall,
				Arguments: args,
			}

			if permCtx != nil {
				entry.AgentID = permCtx.AgentID
				entry.UserID = permCtx.UserID
				entry.SessionID = permCtx.SessionID
				entry.TraceID = permCtx.TraceID
				entry.ToolName = permCtx.ToolName
				entry.RequestIP = permCtx.RequestIP
				entry.Metadata = permCtx.Metadata
			}

			// 执行工具
			start := time.Now()
			result, err := next(ctx, args)
			entry.Duration = time.Since(start)

			// 记录结果
			if err != nil {
				entry.Error = err.Error()
			} else {
				entry.Result = result
			}

			// 异步记录日志
			auditLogger.LogAsync(entry)

			return result, err
		}
	}
}

// ====== 辅助函数 ======

var auditIDCounter uint64
var auditIDMu sync.Mutex

func generateAuditID() string {
	auditIDMu.Lock()
	defer auditIDMu.Unlock()
	auditIDCounter++
	return fmt.Sprintf("audit_%d_%d", time.Now().UnixNano(), auditIDCounter)
}

// LogToolCall 是记录工具调用的便捷函数.
func LogToolCall(auditLogger AuditLogger, agentID, userID, toolName string, args json.RawMessage) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventToolCall,
		AgentID:   agentID,
		UserID:    userID,
		ToolName:  toolName,
		Arguments: args,
	})
}

// LogToolResult 是记录工具结果的便捷函数.
func LogToolResult(auditLogger AuditLogger, agentID, userID, toolName string, result json.RawMessage, err error, duration time.Duration) {
	entry := &AuditEntry{
		EventType: AuditEventToolResult,
		AgentID:   agentID,
		UserID:    userID,
		ToolName:  toolName,
		Result:    result,
		Duration:  duration,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	auditLogger.LogAsync(entry)
}

// LogPermissionCheck 是记录权限检查的便捷函数.
func LogPermissionCheck(auditLogger AuditLogger, permCtx *PermissionContext, decision PermissionDecision, reason string) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventPermissionCheck,
		AgentID:   permCtx.AgentID,
		UserID:    permCtx.UserID,
		SessionID: permCtx.SessionID,
		TraceID:   permCtx.TraceID,
		ToolName:  permCtx.ToolName,
		Decision:  string(decision),
		Metadata:  map[string]string{"reason": reason},
		RequestIP: permCtx.RequestIP,
	})
}

// LogRateLimitHit 是记录速率限制触发的便捷函数.
func LogRateLimitHit(auditLogger AuditLogger, agentID, userID, toolName, limitType string) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventRateLimitHit,
		AgentID:   agentID,
		UserID:    userID,
		ToolName:  toolName,
		Metadata:  map[string]string{"limit_type": limitType},
	})
}

// LogCostAlert 是记录成本告警的便捷函数.
func LogCostAlert(auditLogger AuditLogger, agentID, userID string, cost float64, alertType string) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventCostAlert,
		AgentID:   agentID,
		UserID:    userID,
		Cost:      cost,
		Metadata:  map[string]string{"alert_type": alertType},
	})
}
