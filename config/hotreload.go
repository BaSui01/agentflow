// =============================================================================
// AgentFlow Configuration Hot Reload Manager
// =============================================================================
// Manages configuration hot reloading with support for:
// - Partial configuration updates (no restart required)
// - Full configuration updates (restart required)
// - Change callbacks and notifications
// - Configuration validation before applying
// - Audit logging for configuration changes
// =============================================================================
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// Hot Reload Types
// =============================================================================

// HotReloadManager manages configuration hot reloading
type HotReloadManager struct {
	mu sync.RWMutex

	// Current configuration
	config     *Config
	configPath string

	// File watcher
	watcher *FileWatcher

	// Callbacks
	changeCallbacks []ChangeCallback
	reloadCallbacks []ReloadCallback

	// Change log
	changeLog []ConfigChange

	// Logger
	logger *zap.Logger

	// Running state
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// ChangeCallback is called when configuration changes
type ChangeCallback func(change ConfigChange)

// ReloadCallback is called after configuration is reloaded
type ReloadCallback func(oldConfig, newConfig *Config)

// ConfigChange represents a configuration change
type ConfigChange struct {
	// Timestamp of the change
	Timestamp time.Time `json:"timestamp"`

	// Source of the change (file, api, env)
	Source string `json:"source"`

	// Path to the changed field (e.g., "Server.HTTPPort")
	Path string `json:"path"`

	// OldValue before the change (may be redacted for sensitive fields)
	OldValue interface{} `json:"old_value,omitempty"`

	// NewValue after the change (may be redacted for sensitive fields)
	NewValue interface{} `json:"new_value,omitempty"`

	// RequiresRestart indicates if restart is needed for this change
	RequiresRestart bool `json:"requires_restart"`

	// Applied indicates if the change was applied
	Applied bool `json:"applied"`

	// Error if the change failed
	Error string `json:"error,omitempty"`
}

// HotReloadableField defines which fields can be hot reloaded
type HotReloadableField struct {
	// Path is the field path (e.g., "Log.Level")
	Path string

	// Description of the field
	Description string

	// RequiresRestart indicates if changing this field requires restart
	RequiresRestart bool

	// Sensitive indicates if the field contains sensitive data
	Sensitive bool

	// Validator is an optional validation function
	Validator func(value interface{}) error
}

// =============================================================================
// Hot Reloadable Fields Registry
// =============================================================================

// hotReloadableFields defines which configuration fields can be hot reloaded
var hotReloadableFields = map[string]HotReloadableField{
	// Log configuration - can be hot reloaded
	"Log.Level": {
		Path:            "Log.Level",
		Description:     "Log level (debug, info, warn, error)",
		RequiresRestart: false,
		Sensitive:       false,
	},
	"Log.Format": {
		Path:            "Log.Format",
		Description:     "Log format (json, console)",
		RequiresRestart: false,
		Sensitive:       false,
	},

	// Agent configuration - can be hot reloaded
	"Agent.MaxIterations": {
		Path:            "Agent.MaxIterations",
		Description:     "Maximum agent iterations",
		RequiresRestart: false,
		Sensitive:       false,
	},
	"Agent.Temperature": {
		Path:            "Agent.Temperature",
		Description:     "LLM temperature parameter",
		RequiresRestart: false,
		Sensitive:       false,
	},
	"Agent.MaxTokens": {
		Path:            "Agent.MaxTokens",
		Description:     "Maximum tokens for LLM",
		RequiresRestart: false,
		Sensitive:       false,
	},
	"Agent.Timeout": {
		Path:            "Agent.Timeout",
		Description:     "Agent execution timeout",
		RequiresRestart: false,
		Sensitive:       false,
	},
	"Agent.StreamEnabled": {
		Path:            "Agent.StreamEnabled",
		Description:     "Enable streaming responses",
		RequiresRestart: false,
		Sensitive:       false,
	},

	// LLM configuration - can be hot reloaded
	"LLM.MaxRetries": {
		Path:            "LLM.MaxRetries",
		Description:     "Maximum LLM request retries",
		RequiresRestart: false,
		Sensitive:       false,
	},
	"LLM.Timeout": {
		Path:            "LLM.Timeout",
		Description:     "LLM request timeout",
		RequiresRestart: false,
		Sensitive:       false,
	},

	// Telemetry configuration - can be hot reloaded
	"Telemetry.Enabled": {
		Path:            "Telemetry.Enabled",
		Description:     "Enable telemetry",
		RequiresRestart: false,
		Sensitive:       false,
	},
	"Telemetry.SampleRate": {
		Path:            "Telemetry.SampleRate",
		Description:     "Telemetry sample rate",
		RequiresRestart: false,
		Sensitive:       false,
	},

	// Server configuration - requires restart
	"Server.HTTPPort": {
		Path:            "Server.HTTPPort",
		Description:     "HTTP server port",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Server.GRPCPort": {
		Path:            "Server.GRPCPort",
		Description:     "gRPC server port",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Server.MetricsPort": {
		Path:            "Server.MetricsPort",
		Description:     "Metrics server port",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Server.ReadTimeout": {
		Path:            "Server.ReadTimeout",
		Description:     "HTTP read timeout",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Server.WriteTimeout": {
		Path:            "Server.WriteTimeout",
		Description:     "HTTP write timeout",
		RequiresRestart: true,
		Sensitive:       false,
	},

	// Database configuration - requires restart
	"Database.Host": {
		Path:            "Database.Host",
		Description:     "Database host",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Database.Port": {
		Path:            "Database.Port",
		Description:     "Database port",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Database.Password": {
		Path:            "Database.Password",
		Description:     "Database password",
		RequiresRestart: true,
		Sensitive:       true,
	},

	// Redis configuration - requires restart
	"Redis.Addr": {
		Path:            "Redis.Addr",
		Description:     "Redis address",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Redis.Password": {
		Path:            "Redis.Password",
		Description:     "Redis password",
		RequiresRestart: true,
		Sensitive:       true,
	},

	// LLM API Key - requires restart
	"LLM.APIKey": {
		Path:            "LLM.APIKey",
		Description:     "LLM API key",
		RequiresRestart: true,
		Sensitive:       true,
	},

	// Qdrant configuration - requires restart
	"Qdrant.Host": {
		Path:            "Qdrant.Host",
		Description:     "Qdrant host",
		RequiresRestart: true,
		Sensitive:       false,
	},
	"Qdrant.APIKey": {
		Path:            "Qdrant.APIKey",
		Description:     "Qdrant API key",
		RequiresRestart: true,
		Sensitive:       true,
	},
}

// =============================================================================
// Hot Reload Manager Options
// =============================================================================

// HotReloadOption configures the HotReloadManager
type HotReloadOption func(*HotReloadManager)

// WithHotReloadLogger sets the logger
func WithHotReloadLogger(logger *zap.Logger) HotReloadOption {
	return func(m *HotReloadManager) {
		m.logger = logger
	}
}

// WithConfigPath sets the configuration file path
func WithConfigPath(path string) HotReloadOption {
	return func(m *HotReloadManager) {
		m.configPath = path
	}
}

// =============================================================================
// Hot Reload Manager Implementation
// =============================================================================

// NewHotReloadManager creates a new hot reload manager
func NewHotReloadManager(config *Config, opts ...HotReloadOption) *HotReloadManager {
	m := &HotReloadManager{
		config:          config,
		changeCallbacks: make([]ChangeCallback, 0),
		reloadCallbacks: make([]ReloadCallback, 0),
		changeLog:       make([]ConfigChange, 0, 100),
		logger:          zap.NewNop(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Start starts the hot reload manager
func (m *HotReloadManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("hot reload manager already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// Start file watcher if config path is set
	if m.configPath != "" {
		watcher, err := NewFileWatcher(
			[]string{m.configPath},
			WithWatcherLogger(m.logger),
			WithDebounceDelay(500*time.Millisecond),
		)
		if err != nil {
			return fmt.Errorf("failed to create file watcher: %w", err)
		}

		watcher.OnChange(m.handleFileChange)

		if err := watcher.Start(m.ctx); err != nil {
			return fmt.Errorf("failed to start file watcher: %w", err)
		}

		m.watcher = watcher
	}

	m.running = true
	m.logger.Info("Hot reload manager started",
		zap.String("config_path", m.configPath))

	return nil
}

// Stop stops the hot reload manager
func (m *HotReloadManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	if m.watcher != nil {
		if err := m.watcher.Stop(); err != nil {
			m.logger.Error("Failed to stop file watcher", zap.Error(err))
		}
	}

	m.running = false
	m.logger.Info("Hot reload manager stopped")

	return nil
}

// handleFileChange handles file change events
func (m *HotReloadManager) handleFileChange(event FileEvent) {
	m.logger.Info("Configuration file changed",
		zap.String("path", event.Path),
		zap.String("op", event.Op.String()))

	if event.Op == FileOpWrite || event.Op == FileOpCreate {
		if err := m.ReloadFromFile(); err != nil {
			m.logger.Error("Failed to reload configuration", zap.Error(err))
		}
	}
}

// ReloadFromFile reloads configuration from the file
func (m *HotReloadManager) ReloadFromFile() error {
	if m.configPath == "" {
		return fmt.Errorf("no config path set")
	}

	// Load new configuration
	loader := NewLoader().WithConfigPath(m.configPath)
	newConfig, err := loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate new configuration
	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Apply changes
	return m.ApplyConfig(newConfig, "file")
}

// ApplyConfig applies a new configuration
func (m *HotReloadManager) ApplyConfig(newConfig *Config, source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldConfig := m.config
	changes := m.detectChanges(oldConfig, newConfig)

	var requiresRestart bool
	var appliedChanges []ConfigChange

	for _, change := range changes {
		change.Source = source
		change.Timestamp = time.Now()

		// Check if this field can be hot reloaded
		field, known := hotReloadableFields[change.Path]
		if known {
			change.RequiresRestart = field.RequiresRestart
			if field.Sensitive {
				change.OldValue = "[REDACTED]"
				change.NewValue = "[REDACTED]"
			}
		} else {
			// Unknown fields require restart by default
			change.RequiresRestart = true
		}

		if change.RequiresRestart {
			requiresRestart = true
		}

		change.Applied = true
		appliedChanges = append(appliedChanges, change)

		// Log the change
		m.logChange(change)
	}

	// Update configuration
	m.config = newConfig

	// Add to change log
	m.changeLog = append(m.changeLog, appliedChanges...)

	// Trim change log if too large
	if len(m.changeLog) > 1000 {
		m.changeLog = m.changeLog[len(m.changeLog)-1000:]
	}

	// Notify callbacks
	for _, cb := range m.changeCallbacks {
		for _, change := range appliedChanges {
			cb(change)
		}
	}

	for _, cb := range m.reloadCallbacks {
		cb(oldConfig, newConfig)
	}

	if requiresRestart {
		m.logger.Warn("Some configuration changes require restart to take effect")
	}

	m.logger.Info("Configuration reloaded",
		zap.Int("changes", len(appliedChanges)),
		zap.Bool("requires_restart", requiresRestart))

	return nil
}

// detectChanges detects changes between old and new configuration
func (m *HotReloadManager) detectChanges(oldConfig, newConfig *Config) []ConfigChange {
	var changes []ConfigChange

	oldVal := reflect.ValueOf(oldConfig).Elem()
	newVal := reflect.ValueOf(newConfig).Elem()

	m.compareStructs("", oldVal, newVal, &changes)

	return changes
}

// compareStructs recursively compares struct fields
func (m *HotReloadManager) compareStructs(prefix string, oldVal, newVal reflect.Value, changes *[]ConfigChange) {
	if oldVal.Kind() != reflect.Struct || newVal.Kind() != reflect.Struct {
		return
	}

	t := oldVal.Type()
	for i := 0; i < oldVal.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldPath := field.Name
		if prefix != "" {
			fieldPath = prefix + "." + field.Name
		}

		oldField := oldVal.Field(i)
		newField := newVal.Field(i)

		if oldField.Kind() == reflect.Struct {
			m.compareStructs(fieldPath, oldField, newField, changes)
		} else {
			if !reflect.DeepEqual(oldField.Interface(), newField.Interface()) {
				*changes = append(*changes, ConfigChange{
					Path:     fieldPath,
					OldValue: oldField.Interface(),
					NewValue: newField.Interface(),
				})
			}
		}
	}
}

// logChange logs a configuration change
func (m *HotReloadManager) logChange(change ConfigChange) {
	fields := []zap.Field{
		zap.String("path", change.Path),
		zap.String("source", change.Source),
		zap.Bool("requires_restart", change.RequiresRestart),
	}

	// Only log values if not sensitive
	field, known := hotReloadableFields[change.Path]
	if !known || !field.Sensitive {
		fields = append(fields,
			zap.Any("old_value", change.OldValue),
			zap.Any("new_value", change.NewValue),
		)
	}

	m.logger.Info("Configuration changed", fields...)
}

// OnChange registers a callback for configuration changes
func (m *HotReloadManager) OnChange(callback ChangeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changeCallbacks = append(m.changeCallbacks, callback)
}

// OnReload registers a callback for configuration reloads
func (m *HotReloadManager) OnReload(callback ReloadCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCallbacks = append(m.reloadCallbacks, callback)
}

// GetConfig returns the current configuration
func (m *HotReloadManager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetChangeLog returns the configuration change log
func (m *HotReloadManager) GetChangeLog(limit int) []ConfigChange {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.changeLog) {
		limit = len(m.changeLog)
	}

	// Return most recent changes
	start := len(m.changeLog) - limit
	result := make([]ConfigChange, limit)
	copy(result, m.changeLog[start:])

	return result
}

// UpdateField updates a single configuration field
func (m *HotReloadManager) UpdateField(path string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if field is known
	field, known := hotReloadableFields[path]
	if !known {
		return fmt.Errorf("unknown configuration field: %s", path)
	}

	// Validate if validator exists
	if field.Validator != nil {
		if err := field.Validator(value); err != nil {
			return fmt.Errorf("validation failed for %s: %w", path, err)
		}
	}

	// Get old value
	oldValue, err := m.getFieldValue(path)
	if err != nil {
		return fmt.Errorf("failed to get old value: %w", err)
	}

	// Set new value
	if err := m.setFieldValue(path, value); err != nil {
		return fmt.Errorf("failed to set value: %w", err)
	}

	// Create change record
	change := ConfigChange{
		Timestamp:       time.Now(),
		Source:          "api",
		Path:            path,
		OldValue:        oldValue,
		NewValue:        value,
		RequiresRestart: field.RequiresRestart,
		Applied:         true,
	}

	if field.Sensitive {
		change.OldValue = "[REDACTED]"
		change.NewValue = "[REDACTED]"
	}

	// Log and notify
	m.logChange(change)
	m.changeLog = append(m.changeLog, change)

	for _, cb := range m.changeCallbacks {
		cb(change)
	}

	return nil
}

// getFieldValue gets a field value by path
func (m *HotReloadManager) getFieldValue(path string) (interface{}, error) {
	val := reflect.ValueOf(m.config).Elem()
	return getNestedField(val, path)
}

// setFieldValue sets a field value by path
func (m *HotReloadManager) setFieldValue(path string, value interface{}) error {
	val := reflect.ValueOf(m.config).Elem()
	return setNestedField(val, path, value)
}

// getNestedField gets a nested field by dot-separated path
func getNestedField(v reflect.Value, path string) (interface{}, error) {
	parts := splitPath(path)

	for _, part := range parts {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return nil, fmt.Errorf("not a struct at %s", part)
		}
		v = v.FieldByName(part)
		if !v.IsValid() {
			return nil, fmt.Errorf("field not found: %s", part)
		}
	}

	return v.Interface(), nil
}

// setNestedField sets a nested field by dot-separated path
func setNestedField(v reflect.Value, path string, value interface{}) error {
	parts := splitPath(path)

	for i, part := range parts {
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return fmt.Errorf("not a struct at %s", part)
		}
		v = v.FieldByName(part)
		if !v.IsValid() {
			return fmt.Errorf("field not found: %s", part)
		}

		// If this is the last part, set the value
		if i == len(parts)-1 {
			if !v.CanSet() {
				return fmt.Errorf("cannot set field: %s", part)
			}

			newVal := reflect.ValueOf(value)
			if newVal.Type().ConvertibleTo(v.Type()) {
				v.Set(newVal.Convert(v.Type()))
			} else {
				return fmt.Errorf("type mismatch: expected %s, got %s", v.Type(), newVal.Type())
			}
		}
	}

	return nil
}

// splitPath splits a dot-separated path
func splitPath(path string) []string {
	var parts []string
	var current string

	for _, c := range path {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// GetHotReloadableFields returns the list of hot reloadable fields
func GetHotReloadableFields() map[string]HotReloadableField {
	result := make(map[string]HotReloadableField)
	for k, v := range hotReloadableFields {
		result[k] = v
	}
	return result
}

// IsHotReloadable checks if a field can be hot reloaded
func IsHotReloadable(path string) bool {
	field, known := hotReloadableFields[path]
	return known && !field.RequiresRestart
}

// =============================================================================
// Sanitized Config for API
// =============================================================================

// SanitizedConfig returns a copy of the configuration with sensitive fields redacted
func (m *HotReloadManager) SanitizedConfig() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Convert to JSON and back to get a map
	data, err := json.Marshal(m.config)
	if err != nil {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}

	// Redact sensitive fields
	redactSensitiveFields(result, "")

	return result
}

// redactSensitiveFields recursively redacts sensitive fields
func redactSensitiveFields(data map[string]interface{}, prefix string) {
	sensitiveKeys := map[string]bool{
		"password":   true,
		"api_key":    true,
		"apikey":     true,
		"secret":     true,
		"token":      true,
		"credential": true,
	}

	for key, value := range data {
		fullPath := key
		if prefix != "" {
			fullPath = prefix + "." + key
		}

		// Check if this is a sensitive field
		lowerKey := toLower(key)
		for sensitiveKey := range sensitiveKeys {
			if contains(lowerKey, sensitiveKey) {
				if str, ok := value.(string); ok && str != "" {
					data[key] = "[REDACTED]"
				}
				break
			}
		}

		// Recurse into nested maps
		if nested, ok := value.(map[string]interface{}); ok {
			redactSensitiveFields(nested, fullPath)
		}
	}
}

// toLower converts string to lowercase
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
