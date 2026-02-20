// 软件包工具为企业AI代理框架中的工具执行提供了审计记录.
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

// AuditEventType代表审计事件的类型.
type AuditEventType string

const (
	AuditEventToolCall       AuditEventType = "tool_call"
	AuditEventToolResult     AuditEventType = "tool_result"
	AuditEventPermissionCheck AuditEventType = "permission_check"
	AuditEventRateLimitHit   AuditEventType = "rate_limit_hit"
	AuditEventCostAlert      AuditEventType = "cost_alert"
)

// Auditry代表单一的审计记录条目.
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

// AuditLogger定义了审计日志的接口.
type AuditLogger interface {
	// 日志记录了一个审计条目。
	Log(ctx context.Context, entry *AuditEntry) error

	// LogAsync同步记录审计条目.
	LogAsync(entry *AuditEntry)

	// 查询根据过滤器检索审计条目。
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)

	// 关闭审计日志并冲出待录条目 。
	Close() error
}

// AuditFilter定义了查询审计条目的过滤器.
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

// 审计后端定义了审计存储后端的接口.
type AuditBackend interface {
	// 写入后端的审计条目 。
	Write(ctx context.Context, entry *AuditEntry) error

	// 查询从后端检索审计条目.
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)

	// 关闭后端 。
	Close() error
}

// 默认AuditLogger是AuditLogger的默认执行.
type DefaultAuditLogger struct {
	backends     []AuditBackend
	asyncQueue   chan *AuditEntry
	wg           sync.WaitGroup
	logger       *zap.Logger
	closed       bool
	closeMu      sync.Mutex
	idGenerator  func() string
}

// 审计LoggerConfig 配置审计日志 。
type AuditLoggerConfig struct {
	Backends       []AuditBackend
	AsyncQueueSize int
	AsyncWorkers   int
	IDGenerator    func() string
}

// NewAuditLogger创建了新的审计日志.
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

	// 启动同步工作
	for i := 0; i < cfg.AsyncWorkers; i++ {
		al.wg.Add(1)
		go al.asyncWorker()
	}

	return al
}

// ayncWorker 同步处理审计条目。
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

// 写入 ToBackends 为所有后端写出一个条目 。
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

// 日志同步记录审计条目。
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

// LogAsync同步记录审计条目.
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

// 查询根据过滤器检索审计条目。
func (al *DefaultAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	if len(al.backends) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}

	// 从第一个支持查询的后端查询
	return al.backends[0].Query(ctx, filter)
}

// 关闭审计日志并冲出待录条目 。
func (al *DefaultAuditLogger) Close() error {
	al.closeMu.Lock()
	if al.closed {
		al.closeMu.Unlock()
		return nil
	}
	al.closed = true
	al.closeMu.Unlock()

	// 关闭同步队列并等待工人完成
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

// QQ 内存后端QQ

// MemoryAuditBackend将审计条目存储于内存.
type MemoryAuditBackend struct {
	entries  []*AuditEntry
	maxSize  int
	mu       sync.RWMutex
}

// New Memory AuditBackend创建了新的内存审计后端.
func NewMemoryAuditBackend(maxSize int) *MemoryAuditBackend {
	if maxSize <= 0 {
		maxSize = 100000
	}
	return &MemoryAuditBackend{
		entries: make([]*AuditEntry, 0),
		maxSize: maxSize,
	}
}

// 写一个审计条目到内存 。
func (m *MemoryAuditBackend) Write(ctx context.Context, entry *AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果具备能力, 将保留最老条目
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

// 查询从内存中检索审计条目 。
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

// 匹配过滤器。
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

// 关闭内存后端 。
func (m *MemoryAuditBackend) Close() error {
	return nil
}

// QQ 文件后端 QQ

// FileAuditBackend将审计条目存储在文件中.
type FileAuditBackend struct {
	dir         string
	currentFile *os.File
	currentDate string
	maxFileSize int64
	mu          sync.Mutex
	logger      *zap.Logger
}

// FileAuditBackendConfig配置文件审计后端.
type FileAuditBackendConfig struct {
	Directory   string
	MaxFileSize int64 // Max file size in bytes before rotation
}

// NewFileAuditBackend创建了一个新的文件审计后端.
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

// 写入文件的审计条目 。
func (f *FileAuditBackend) Write(ctx context.Context, entry *AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 检查是否需要旋转文件
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

// 旋转文件旋转为新文件 。
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

// 查询从文件中检索审计条目(执行有限).
func (f *FileAuditBackend) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	// 说明: 这是一个只读取当前文件的简化执行
	// 生产实施需要根据过滤器从多个文件中读取
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.currentFile == nil {
		return []*AuditEntry{}, nil
	}

	// 为了充分执行,你们将:
	// 1. 列出目录中的所有文件
	// 2. 基于日期范围的过滤文件
	// 3. 阅读并分析每个文件
	// 4. 应用过滤并返回结果

	return []*AuditEntry{}, fmt.Errorf("file query not fully implemented; use memory backend for queries")
}

// 关闭文件后端 。
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

// QQ 数据库后端接口QQ

// DatabaseAuditBackend定义了数据库审计后端的接口.
// 执行可以使用PostgreSQL,MySQL,MongoDB等.
type DatabaseAuditBackend interface {
	AuditBackend
	// 迁移创建或更新数据库计划。
	Migrate(ctx context.Context) error
}

// 审计中间软件

// AuditleMiddleware创建了一个记录工具执行的中间软件.
func AuditMiddleware(auditLogger AuditLogger) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// 摘录上下文信息
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

			// 同步日志
			auditLogger.LogAsync(entry)

			return result, err
		}
	}
}

// 帮助函数

var auditIDCounter uint64
var auditIDMu sync.Mutex

func generateAuditID() string {
	auditIDMu.Lock()
	defer auditIDMu.Unlock()
	auditIDCounter++
	return fmt.Sprintf("audit_%d_%d", time.Now().UnixNano(), auditIDCounter)
}

// LogToolCall是登录工具调用的一种便利功能.
func LogToolCall(auditLogger AuditLogger, agentID, userID, toolName string, args json.RawMessage) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventToolCall,
		AgentID:   agentID,
		UserID:    userID,
		ToolName:  toolName,
		Arguments: args,
	})
}

// LogToolResult是记录工具结果的便利功能.
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

// LogPermissionCheck是登录权限检查的便利功能.
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

// LogRateLimitHit是记录速率限制命中的便利功能.
func LogRateLimitHit(auditLogger AuditLogger, agentID, userID, toolName, limitType string) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventRateLimitHit,
		AgentID:   agentID,
		UserID:    userID,
		ToolName:  toolName,
		Metadata:  map[string]string{"limit_type": limitType},
	})
}

// LogCostAlert是登录成本提示的便利功能.
func LogCostAlert(auditLogger AuditLogger, agentID, userID string, cost float64, alertType string) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventCostAlert,
		AgentID:   agentID,
		UserID:    userID,
		Cost:      cost,
		Metadata:  map[string]string{"alert_type": alertType},
	})
}
