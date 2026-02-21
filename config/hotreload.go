// 配置热重载管理器实现。
//
// 支持局部更新、变更通知、应用前校验与审计记录。
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// --- 热重载类型定义 ---

// HotReloadManager 管理配置热重载
type HotReloadManager struct {
	mu sync.RWMutex

	// 当前配置
	config     *Config
	configPath string

	// 回滚支持
	previousConfig *Config          // 上一个成功应用的配置（用于回滚）
	configHistory  []ConfigSnapshot // 配置变更历史（环形缓冲）
	maxHistorySize int              // 最大历史记录数，默认 10
	validateFunc   ValidateFunc     // 配置验证钩子（可选）

	// 文件观察者
	watcher *FileWatcher

	// 回调
	changeCallbacks   []ChangeCallback
	reloadCallbacks   []ReloadCallback
	rollbackCallbacks []RollbackCallback // 回滚事件回调

	// 变更日志
	changeLog []ConfigChange

	// 记录器
	logger *zap.Logger

	// 运行状态
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// ChangeCallback 配置更改时调用
type ChangeCallback func(change ConfigChange)

// ReloadCallback 重新加载配置后调用
type ReloadCallback func(oldConfig, newConfig *Config)

// ConfigChange 代表配置更改
type ConfigChange struct {
	// 变更的时间戳
	Timestamp time.Time `json:"timestamp"`

	// 更改的来源（文件、api、env）
	Source string `json:"source"`

	// 已更改字段的路径（例如“Server.HTTPPort”）
	Path string `json:"path"`

	// 更改前的 OldValue（可能会对敏感字段进行编辑）
	OldValue any `json:"old_value,omitempty"`

	// 更改后的 NewValue（可能会对敏感字段进行编辑）
	NewValue any `json:"new_value,omitempty"`

	// RequiresRestart 指示此更改是否需要重新启动
	RequiresRestart bool `json:"requires_restart"`

	// 已应用指示是否应用了更改
	Applied bool `json:"applied"`

	// 如果更改失败则出错
	Error string `json:"error,omitempty"`
}

// ConfigSnapshot 配置快照（用于历史记录和回滚）
type ConfigSnapshot struct {
	Config    *Config   `json:"config"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`   // 变更来源：file, api, env
	Version   int       `json:"version"`  // 递增版本号
	Checksum  string    `json:"checksum"` // 配置内容校验和
}

// ValidateFunc 配置验证钩子函数
// 接收新配置，返回 error 表示验证失败
type ValidateFunc func(newConfig *Config) error

// RollbackCallback 回滚事件回调
type RollbackCallback func(event RollbackEvent)

// RollbackEvent 回滚事件
type RollbackEvent struct {
	Timestamp      time.Time `json:"timestamp"`
	Reason         string    `json:"reason"`
	FailedConfig   *Config   `json:"failed_config"`
	RestoredConfig *Config   `json:"restored_config"`
	Version        int       `json:"version"`
	Error          error     `json:"error,omitempty"`
}

// HotReloadableField 定义哪些字段可以热重载
type HotReloadableField struct {
	// Path 是字段路径（例如“Log.Level”）
	Path string

	// 字段描述
	Description string

	// RequiresRestart 指示更改此字段是否需要重新启动
	RequiresRestart bool

	// Sensitive 表示该字段是否包含敏感数据
	Sensitive bool

	// Validator 是可选的校验函数
	Validator func(value any) error
}

// --- 可热重载字段注册表 ---

// hotReloadableFields 定义哪些配置字段可以热重载
var hotReloadableFields = map[string]HotReloadableField{
	// 日志配置-可以热重载
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

	// 代理配置 - 可以热重载
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

	// LLM配置-可以热重载
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

	// 遥测配置 - 可以热重载
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

	// 服务器配置 - 需要重新启动
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

	// 数据库配置 - 需要重新启动
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

	// Redis 配置 - 需要重新启动
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

	// LLM API 密钥 - 需要重新启动
	"LLM.APIKey": {
		Path:            "LLM.APIKey",
		Description:     "LLM API key",
		RequiresRestart: true,
		Sensitive:       true,
	},

	// Qdrant 配置 - 需要重新启动
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

// --- 热重载管理器选项 ---

// HotReloadOption 配置 HotReloadManager
type HotReloadOption func(*HotReloadManager)

// WithHotReloadLogger 设置记录器
func WithHotReloadLogger(logger *zap.Logger) HotReloadOption {
	return func(m *HotReloadManager) {
		m.logger = logger
	}
}

// WithConfigPath 设置配置文件路径
func WithConfigPath(path string) HotReloadOption {
	return func(m *HotReloadManager) {
		m.configPath = path
	}
}

// WithMaxHistorySize 设置配置历史最大记录数
func WithMaxHistorySize(size int) HotReloadOption {
	return func(m *HotReloadManager) {
		if size > 0 {
			m.maxHistorySize = size
		}
	}
}

// WithValidateFunc 设置配置验证钩子
func WithValidateFunc(fn ValidateFunc) HotReloadOption {
	return func(m *HotReloadManager) {
		m.validateFunc = fn
	}
}

// --- 热重载管理器实现 ---

// NewHotReloadManager 创建一个新的热重载管理器
func NewHotReloadManager(config *Config, opts ...HotReloadOption) *HotReloadManager {
	m := &HotReloadManager{
		config:            config,
		previousConfig:    nil,
		configHistory:     make([]ConfigSnapshot, 0, 10),
		maxHistorySize:    10,
		changeCallbacks:   make([]ChangeCallback, 0),
		reloadCallbacks:   make([]ReloadCallback, 0),
		rollbackCallbacks: make([]RollbackCallback, 0),
		changeLog:         make([]ConfigChange, 0, 100),
		logger:            zap.NewNop(),
	}

	for _, opt := range opts {
		opt(m)
	}

	// 将初始配置作为第一个历史快照
	m.pushHistory(config, "init")

	return m
}

// pushHistory 将配置快照推入历史记录（环形缓冲）
func (m *HotReloadManager) pushHistory(config *Config, source string) {
	version := 1
	if len(m.configHistory) > 0 {
		version = m.configHistory[len(m.configHistory)-1].Version + 1
	}
	snapshot := ConfigSnapshot{
		Config:    deepCopyConfig(config),
		Timestamp: time.Now(),
		Source:    source,
		Version:   version,
		Checksum:  computeConfigChecksum(config),
	}
	m.configHistory = append(m.configHistory, snapshot)
	if len(m.configHistory) > m.maxHistorySize {
		m.configHistory = m.configHistory[len(m.configHistory)-m.maxHistorySize:]
	}
}

// deepCopyConfig 深拷贝配置（通过 JSON 序列化/反序列化）
func deepCopyConfig(config *Config) *Config {
	data, err := json.Marshal(config)
	if err != nil {
		return config
	}
	var copied Config
	if err := json.Unmarshal(data, &copied); err != nil {
		return config
	}
	return &copied
}

// computeConfigChecksum 计算配置校验和（FNV hash）
func computeConfigChecksum(config *Config) string {
	data, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	var hash uint64
	for _, b := range data {
		hash ^= uint64(b)
		hash *= 1099511628211
	}
	return fmt.Sprintf("%016x", hash)
}

// Start 启动热重载管理器
func (m *HotReloadManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("hot reload manager already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// 如果设置了配置路径则启动文件监视程序
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

// Stop 停止热重载管理器
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

// handleFileChange 处理文件更改事件
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

// ReloadFromFile 从文件重新加载配置
func (m *HotReloadManager) ReloadFromFile() error {
	if m.configPath == "" {
		return fmt.Errorf("no config path set")
	}

	loader := NewLoader().WithConfigPath(m.configPath)
	newConfig, err := loader.Load()
	if err != nil {
		m.logger.Error("failed to load config from file, keeping current config",
			zap.Error(err), zap.String("path", m.configPath))
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := newConfig.Validate(); err != nil {
		m.logger.Error("invalid config from file, keeping current config",
			zap.Error(err), zap.String("path", m.configPath))
		return fmt.Errorf("invalid config: %w", err)
	}

	// ApplyConfig 内部会处理 validateFunc 和自动回滚
	if err := m.ApplyConfig(newConfig, "file"); err != nil {
		m.logger.Error("failed to apply config, auto-rollback may have occurred",
			zap.Error(err))
		return err
	}
	return nil
}

// ApplyConfig 应用新配置
// 修复 TOCTOU 竞态：validate、apply、pushHistory 和 changeLog 更新
// 全部在同一把锁内完成，确保原子性。回调通知在锁外执行以避免死锁。
func (m *HotReloadManager) ApplyConfig(newConfig *Config, source string) error {
	m.mu.Lock()

	oldConfig := m.config

	// 1. 执行自定义验证钩子（持有锁，防止 TOCTOU）
	if m.validateFunc != nil {
		if err := m.validateFunc(newConfig); err != nil {
			m.logger.Warn("config validation hook failed",
				zap.Error(err), zap.String("source", source))
			m.changeLog = append(m.changeLog, ConfigChange{
				Timestamp: time.Now(),
				Source:    source,
				Path:      "(validation_hook)",
				Applied:   false,
				Error:     fmt.Sprintf("validation hook failed: %v", err),
			})
			m.mu.Unlock()
			return fmt.Errorf("config validation failed: %w", err)
		}
	}

	// 2. 检测变更
	changes := m.detectChanges(oldConfig, newConfig)
	var requiresRestart bool
	var appliedChanges []ConfigChange

	for _, change := range changes {
		change.Source = source
		change.Timestamp = time.Now()
		field, known := hotReloadableFields[change.Path]
		if known {
			change.RequiresRestart = field.RequiresRestart
			if field.Sensitive {
				change.OldValue = "[REDACTED]"
				change.NewValue = "[REDACTED]"
			}
		} else {
			change.RequiresRestart = true
		}
		if change.RequiresRestart {
			requiresRestart = true
		}
		change.Applied = true
		appliedChanges = append(appliedChanges, change)
		m.logChange(change)
	}

	// 3. 保存当前配置为 previousConfig
	m.previousConfig = deepCopyConfig(oldConfig)

	// 4. 应用新配置
	m.config = newConfig

	// 5. 推入历史记录和更新变更日志（在锁内完成，消除 TOCTOU 窗口）
	m.pushHistory(newConfig, source)
	m.changeLog = append(m.changeLog, appliedChanges...)
	if len(m.changeLog) > 1000 {
		m.changeLog = m.changeLog[len(m.changeLog)-1000:]
	}

	// 复制回调列表，在锁外安全调用
	changeCallbacks := append([]ChangeCallback(nil), m.changeCallbacks...)
	reloadCallbacks := append([]ReloadCallback(nil), m.reloadCallbacks...)
	m.mu.Unlock()

	// 6. 通知回调（失败则自动回滚）
	if err := m.notifyCallbacksSafe(changeCallbacks, reloadCallbacks, oldConfig, newConfig, appliedChanges); err != nil {
		m.mu.Lock()
		if m.config == newConfig {
			m.logger.Error("callback failed, rolling back", zap.Error(err))
			m.rollbackLocked(oldConfig, fmt.Sprintf("callback error: %v", err), err)
		} else {
			m.logger.Warn("callback failed but config changed concurrently, skip rollback",
				zap.Error(err),
			)
		}
		m.mu.Unlock()
		return fmt.Errorf("config applied but callback failed: %w", err)
	}

	if requiresRestart {
		m.logger.Warn("Some configuration changes require restart to take effect")
	}
	m.logger.Info("Configuration reloaded",
		zap.Int("changes", len(appliedChanges)),
		zap.Bool("requires_restart", requiresRestart))
	return nil
}

// notifyCallbacksSafe 安全地通知回调（捕获 panic）
func (m *HotReloadManager) notifyCallbacksSafe(changeCallbacks []ChangeCallback, reloadCallbacks []ReloadCallback, oldConfig, newConfig *Config, changes []ConfigChange) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("callback panicked: %v", r)
		}
	}()
	for _, cb := range changeCallbacks {
		for _, change := range changes {
			cb(change)
		}
	}
	for _, cb := range reloadCallbacks {
		cb(oldConfig, newConfig)
	}
	return nil
}

// detectChanges 检测新旧配置之间的变化
func (m *HotReloadManager) detectChanges(oldConfig, newConfig *Config) []ConfigChange {
	var changes []ConfigChange

	oldVal := reflect.ValueOf(oldConfig).Elem()
	newVal := reflect.ValueOf(newConfig).Elem()

	m.compareStructs("", oldVal, newVal, &changes)

	return changes
}

// compareStructs 递归比较结构体字段
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

// logChange 记录配置更改
func (m *HotReloadManager) logChange(change ConfigChange) {
	fields := []zap.Field{
		zap.String("path", change.Path),
		zap.String("source", change.Source),
		zap.Bool("requires_restart", change.RequiresRestart),
	}

	// 仅记录不敏感的值
	field, known := hotReloadableFields[change.Path]
	if !known || !field.Sensitive {
		fields = append(fields,
			zap.Any("old_value", change.OldValue),
			zap.Any("new_value", change.NewValue),
		)
	}

	m.logger.Info("Configuration changed", fields...)
}

// OnChange 注册配置更改的回调
func (m *HotReloadManager) OnChange(callback ChangeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changeCallbacks = append(m.changeCallbacks, callback)
}

// OnReload 注册配置重新加载的回调
func (m *HotReloadManager) OnReload(callback ReloadCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCallbacks = append(m.reloadCallbacks, callback)
}

// Rollback 回滚到上一个有效配置
func (m *HotReloadManager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.previousConfig == nil {
		return fmt.Errorf("no previous config available for rollback")
	}
	m.rollbackLocked(m.previousConfig, "manual rollback", nil)
	return nil
}

// RollbackToVersion 回滚到指定版本
func (m *HotReloadManager) RollbackToVersion(version int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, snapshot := range m.configHistory {
		if snapshot.Version == version {
			m.rollbackLocked(snapshot.Config, fmt.Sprintf("rollback to version %d", version), nil)
			return nil
		}
	}
	return fmt.Errorf("config version %d not found in history", version)
}

// rollbackLocked 执行回滚（调用方必须持有 m.mu 写锁）
func (m *HotReloadManager) rollbackLocked(targetConfig *Config, reason string, originalErr error) {
	failedConfig := m.config
	restoredConfig := deepCopyConfig(targetConfig)
	m.config = restoredConfig

	restoredVersion := 0
	checksum := computeConfigChecksum(targetConfig)
	for _, snapshot := range m.configHistory {
		if snapshot.Checksum == checksum {
			restoredVersion = snapshot.Version
			break
		}
	}

	event := RollbackEvent{
		Timestamp:      time.Now(),
		Reason:         reason,
		FailedConfig:   failedConfig,
		RestoredConfig: restoredConfig,
		Version:        restoredVersion,
		Error:          originalErr,
	}

	m.changeLog = append(m.changeLog, ConfigChange{
		Timestamp: time.Now(),
		Source:    "rollback",
		Path:      "(rollback)",
		Applied:   true,
		Error:     reason,
	})

	for _, cb := range m.rollbackCallbacks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("rollback callback panicked", zap.Any("panic", r))
				}
			}()
			cb(event)
		}()
	}

	m.logger.Warn("configuration rolled back",
		zap.String("reason", reason),
		zap.Int("restored_version", restoredVersion))
}

// OnRollback 注册回滚事件回调
func (m *HotReloadManager) OnRollback(callback RollbackCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollbackCallbacks = append(m.rollbackCallbacks, callback)
}

// GetConfigHistory 获取配置变更历史
func (m *HotReloadManager) GetConfigHistory() []ConfigSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ConfigSnapshot, len(m.configHistory))
	copy(result, m.configHistory)
	return result
}

// GetCurrentVersion 获取当前配置版本号
func (m *HotReloadManager) GetCurrentVersion() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.configHistory) == 0 {
		return 0
	}
	return m.configHistory[len(m.configHistory)-1].Version
}

// GetConfig 返回当前配置
func (m *HotReloadManager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return deepCopyConfig(m.config)
}

// GetChangeLog 返回配置变更日志
func (m *HotReloadManager) GetChangeLog(limit int) []ConfigChange {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.changeLog) {
		limit = len(m.changeLog)
	}

	// 返回最近的更改
	start := len(m.changeLog) - limit
	result := make([]ConfigChange, limit)
	copy(result, m.changeLog[start:])

	return result
}

// UpdateField 更新单个配置字段
func (m *HotReloadManager) UpdateField(path string, value any) error {
	m.mu.Lock()

	oldConfigSnapshot := deepCopyConfig(m.config)

	// 检查字段是否已知
	field, known := hotReloadableFields[path]
	if !known {
		m.mu.Unlock()
		return fmt.Errorf("unknown configuration field: %s", path)
	}

	// 验证验证器是否存在
	if field.Validator != nil {
		if err := field.Validator(value); err != nil {
			m.mu.Unlock()
			return fmt.Errorf("validation failed for %s: %w", path, err)
		}
	}

	// 获取旧值
	oldValue, err := m.getFieldValue(path)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to get old value: %w", err)
	}

	// 设置新值
	if err := m.setFieldValue(path, value); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to set value: %w", err)
	}

	// 创建变更记录
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

	// 记录并通知
	m.logChange(change)
	m.changeLog = append(m.changeLog, change)
	callbacks := append([]ChangeCallback(nil), m.changeCallbacks...)
	// Bug fix: 在锁内捕获当前配置快照，避免锁外读取 m.config 导致的竞态
	newConfigSnapshot := deepCopyConfig(m.config)
	m.mu.Unlock()

	if err := m.notifyCallbacksSafe(callbacks, nil, oldConfigSnapshot, newConfigSnapshot, []ConfigChange{change}); err != nil {
		m.mu.Lock()
		m.rollbackLocked(oldConfigSnapshot, fmt.Sprintf("callback error: %v", err), err)
		m.mu.Unlock()
		return fmt.Errorf("field updated but callback failed, rolled back: %w", err)
	}

	return nil
}

// getFieldValue 通过路径获取字段值
func (m *HotReloadManager) getFieldValue(path string) (any, error) {
	val := reflect.ValueOf(m.config).Elem()
	return getNestedField(val, path)
}

// setFieldValue 通过路径设置字段值
func (m *HotReloadManager) setFieldValue(path string, value any) error {
	val := reflect.ValueOf(m.config).Elem()
	return setNestedField(val, path, value)
}

// getNestedField 通过点分隔路径获取嵌套字段
func getNestedField(v reflect.Value, path string) (any, error) {
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

// setNestedField 通过点分隔路径设置嵌套字段
func setNestedField(v reflect.Value, path string, value any) error {
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

		// 如果这是最后一部分，请设置该值
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

// splitPath 分割点分隔的路径
func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(c rune) bool { return c == '.' })
}

// GetHotReloadableFields 返回可热重载字段的列表
func GetHotReloadableFields() map[string]HotReloadableField {
	result := make(map[string]HotReloadableField)
	for k, v := range hotReloadableFields {
		result[k] = v
	}
	return result
}

// IsHotReloadable 检查字段是否可以热重载
func IsHotReloadable(path string) bool {
	field, known := hotReloadableFields[path]
	return known && !field.RequiresRestart
}

// --- API 脱敏配置视图 ---

// SanitizedConfig 返回包含敏感字段的配置副本
func (m *HotReloadManager) SanitizedConfig() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 转换为 JSON 并返回以获取地图
	data, err := json.Marshal(m.config)
	if err != nil {
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}

	// 编辑敏感字段
	redactSensitiveFields(result, "")

	return result
}

// redactSensitiveFields 递归地编辑敏感字段
func redactSensitiveFields(data map[string]any, prefix string) {
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

		// 检查这是否是敏感字段
		lowerKey := strings.ToLower(key)
		for sensitiveKey := range sensitiveKeys {
			if strings.Contains(lowerKey, sensitiveKey) {
				if str, ok := value.(string); ok && str != "" {
					data[key] = "[REDACTED]"
				}
				break
			}
		}

		// 递归到嵌套映射
		if nested, ok := value.(map[string]any); ok {
			redactSensitiveFields(nested, fullPath)
		}
	}
}
