// Package tools provides audit logging for tool execution in enterprise AI Agent frameworks.
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

// AuditEventType represents the type of audit event.
type AuditEventType string

const (
	AuditEventToolCall       AuditEventType = "tool_call"
	AuditEventToolResult     AuditEventType = "tool_result"
	AuditEventPermissionCheck AuditEventType = "permission_check"
	AuditEventRateLimitHit   AuditEventType = "rate_limit_hit"
	AuditEventCostAlert      AuditEventType = "cost_alert"
)

// AuditEntry represents a single audit log entry.
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

// AuditLogger defines the interface for audit logging.
type AuditLogger interface {
	// Log records an audit entry.
	Log(ctx context.Context, entry *AuditEntry) error

	// LogAsync records an audit entry asynchronously.
	LogAsync(entry *AuditEntry)

	// Query retrieves audit entries based on filters.
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)

	// Close closes the audit logger and flushes pending entries.
	Close() error
}

// AuditFilter defines filters for querying audit entries.
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

// AuditBackend defines the interface for audit storage backends.
type AuditBackend interface {
	// Write writes an audit entry to the backend.
	Write(ctx context.Context, entry *AuditEntry) error

	// Query retrieves audit entries from the backend.
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)

	// Close closes the backend.
	Close() error
}

// DefaultAuditLogger is the default implementation of AuditLogger.
type DefaultAuditLogger struct {
	backends     []AuditBackend
	asyncQueue   chan *AuditEntry
	wg           sync.WaitGroup
	logger       *zap.Logger
	closed       bool
	closeMu      sync.Mutex
	idGenerator  func() string
}

// AuditLoggerConfig configures the audit logger.
type AuditLoggerConfig struct {
	Backends       []AuditBackend
	AsyncQueueSize int
	AsyncWorkers   int
	IDGenerator    func() string
}

// NewAuditLogger creates a new audit logger.
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

	// Start async workers
	for i := 0; i < cfg.AsyncWorkers; i++ {
		al.wg.Add(1)
		go al.asyncWorker()
	}

	return al
}

// asyncWorker processes audit entries asynchronously.
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

// writeToBackends writes an entry to all backends.
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

// Log records an audit entry synchronously.
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

// LogAsync records an audit entry asynchronously.
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
		// Successfully queued
	default:
		al.logger.Warn("audit queue full, dropping entry",
			zap.String("entry_id", entry.ID),
		)
	}
}

// Query retrieves audit entries based on filters.
func (al *DefaultAuditLogger) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	if len(al.backends) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}

	// Query from the first backend that supports querying
	return al.backends[0].Query(ctx, filter)
}

// Close closes the audit logger and flushes pending entries.
func (al *DefaultAuditLogger) Close() error {
	al.closeMu.Lock()
	if al.closed {
		al.closeMu.Unlock()
		return nil
	}
	al.closed = true
	al.closeMu.Unlock()

	// Close the async queue and wait for workers to finish
	close(al.asyncQueue)
	al.wg.Wait()

	// Close all backends
	var lastErr error
	for _, backend := range al.backends {
		if err := backend.Close(); err != nil {
			lastErr = err
		}
	}

	al.logger.Info("audit logger closed")
	return lastErr
}

// ====== Memory Backend ======

// MemoryAuditBackend stores audit entries in memory.
type MemoryAuditBackend struct {
	entries  []*AuditEntry
	maxSize  int
	mu       sync.RWMutex
}

// NewMemoryAuditBackend creates a new memory audit backend.
func NewMemoryAuditBackend(maxSize int) *MemoryAuditBackend {
	if maxSize <= 0 {
		maxSize = 100000
	}
	return &MemoryAuditBackend{
		entries: make([]*AuditEntry, 0),
		maxSize: maxSize,
	}
}

// Write writes an audit entry to memory.
func (m *MemoryAuditBackend) Write(ctx context.Context, entry *AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Evict oldest entries if at capacity
	if len(m.entries) >= m.maxSize {
		// Remove oldest 10%
		removeCount := m.maxSize / 10
		if removeCount < 1 {
			removeCount = 1
		}
		m.entries = m.entries[removeCount:]
	}

	m.entries = append(m.entries, entry)
	return nil
}

// Query retrieves audit entries from memory.
func (m *MemoryAuditBackend) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*AuditEntry

	for _, entry := range m.entries {
		if m.matchesFilter(entry, filter) {
			results = append(results, entry)
		}
	}

	// Apply offset and limit
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

// matchesFilter checks if an entry matches the filter.
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

// Close closes the memory backend.
func (m *MemoryAuditBackend) Close() error {
	return nil
}

// ====== File Backend ======

// FileAuditBackend stores audit entries in files.
type FileAuditBackend struct {
	dir         string
	currentFile *os.File
	currentDate string
	maxFileSize int64
	mu          sync.Mutex
	logger      *zap.Logger
}

// FileAuditBackendConfig configures the file audit backend.
type FileAuditBackendConfig struct {
	Directory   string
	MaxFileSize int64 // Max file size in bytes before rotation
}

// NewFileAuditBackend creates a new file audit backend.
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

	// Create directory if it doesn't exist
	if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create audit directory: %w", err)
	}

	return &FileAuditBackend{
		dir:         cfg.Directory,
		maxFileSize: cfg.MaxFileSize,
		logger:      logger.With(zap.String("component", "file_audit_backend")),
	}, nil
}

// Write writes an audit entry to a file.
func (f *FileAuditBackend) Write(ctx context.Context, entry *AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if we need to rotate the file
	currentDate := entry.Timestamp.Format("2006-01-02")
	if f.currentFile == nil || f.currentDate != currentDate {
		if err := f.rotateFile(currentDate); err != nil {
			return err
		}
	}

	// Check file size
	if f.currentFile != nil {
		info, err := f.currentFile.Stat()
		if err == nil && info.Size() >= f.maxFileSize {
			if err := f.rotateFile(currentDate); err != nil {
				return err
			}
		}
	}

	// Write entry as JSON line
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	if _, err := f.currentFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}

	return nil
}

// rotateFile rotates to a new file.
func (f *FileAuditBackend) rotateFile(date string) error {
	// Close current file
	if f.currentFile != nil {
		f.currentFile.Close()
	}

	// Create new file
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

// Query retrieves audit entries from files (limited implementation).
func (f *FileAuditBackend) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	// Note: This is a simplified implementation that only reads from the current file
	// A production implementation would need to read from multiple files based on the filter
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.currentFile == nil {
		return []*AuditEntry{}, nil
	}

	// For a full implementation, you would:
	// 1. List all files in the directory
	// 2. Filter files based on date range
	// 3. Read and parse each file
	// 4. Apply filters and return results

	return []*AuditEntry{}, fmt.Errorf("file query not fully implemented; use memory backend for queries")
}

// Close closes the file backend.
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

// ====== Database Backend Interface ======

// DatabaseAuditBackend defines the interface for database audit backends.
// Implementations can use PostgreSQL, MySQL, MongoDB, etc.
type DatabaseAuditBackend interface {
	AuditBackend
	// Migrate creates or updates the database schema.
	Migrate(ctx context.Context) error
}

// ====== Audit Middleware ======

// AuditMiddleware creates a middleware that logs tool executions.
func AuditMiddleware(auditLogger AuditLogger) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// Extract context information
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

			// Execute the tool
			start := time.Now()
			result, err := next(ctx, args)
			entry.Duration = time.Since(start)

			// Record result
			if err != nil {
				entry.Error = err.Error()
			} else {
				entry.Result = result
			}

			// Log asynchronously
			auditLogger.LogAsync(entry)

			return result, err
		}
	}
}

// ====== Helper Functions ======

var auditIDCounter uint64
var auditIDMu sync.Mutex

func generateAuditID() string {
	auditIDMu.Lock()
	defer auditIDMu.Unlock()
	auditIDCounter++
	return fmt.Sprintf("audit_%d_%d", time.Now().UnixNano(), auditIDCounter)
}

// LogToolCall is a convenience function to log a tool call.
func LogToolCall(auditLogger AuditLogger, agentID, userID, toolName string, args json.RawMessage) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventToolCall,
		AgentID:   agentID,
		UserID:    userID,
		ToolName:  toolName,
		Arguments: args,
	})
}

// LogToolResult is a convenience function to log a tool result.
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

// LogPermissionCheck is a convenience function to log a permission check.
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

// LogRateLimitHit is a convenience function to log a rate limit hit.
func LogRateLimitHit(auditLogger AuditLogger, agentID, userID, toolName, limitType string) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventRateLimitHit,
		AgentID:   agentID,
		UserID:    userID,
		ToolName:  toolName,
		Metadata:  map[string]string{"limit_type": limitType},
	})
}

// LogCostAlert is a convenience function to log a cost alert.
func LogCostAlert(auditLogger AuditLogger, agentID, userID string, cost float64, alertType string) {
	auditLogger.LogAsync(&AuditEntry{
		EventType: AuditEventCostAlert,
		AgentID:   agentID,
		UserID:    userID,
		Cost:      cost,
		Metadata:  map[string]string{"alert_type": alertType},
	})
}
