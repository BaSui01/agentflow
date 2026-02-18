# P2 优化提示词

## P2-1: Config Hot Reload 回滚机制

### 需求背景
`config/hotreload.go` 的 `HotReloadManager` 有文件监听和配置重载功能，但缺少回滚机制。当前 `ApplyConfig`（第 416 行）直接将 `m.config = newConfig` 覆盖，如果新配置导致下游组件异常（如无效的日志级别、不合法的超时值），无法自动恢复到上一个有效配置。需要添加：回滚能力、配置变更历史、验证钩子、回滚事件通知。

### 需要修改的文件

#### 文件：config/hotreload.go

**改动 1 — HotReloadManager 结构体添加回滚相关字段（第 29-53 行）**

当前结构体：
```go
type HotReloadManager struct {
	mu sync.RWMutex
	config     *Config
	configPath string
	watcher *FileWatcher
	changeCallbacks []ChangeCallback
	reloadCallbacks []ReloadCallback
	changeLog []ConfigChange
	logger *zap.Logger
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}
```

改为：
```go
type HotReloadManager struct {
	mu sync.RWMutex
	config     *Config
	configPath string

	// Rollback support
	previousConfig  *Config          // 上一个成功应用的配置（用于回滚）
	configHistory   []ConfigSnapshot // 配置变更历史（环形缓冲）
	maxHistorySize  int              // 最大历史记录数，默认 10
	validateFunc    ValidateFunc     // 配置验证钩子（可选）

	watcher *FileWatcher
	changeCallbacks   []ChangeCallback
	reloadCallbacks   []ReloadCallback
	rollbackCallbacks []RollbackCallback // 回滚事件回调
	changeLog []ConfigChange
	logger *zap.Logger
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}
```

**改动 2 — 添加新类型定义（在第 86 行 ConfigChange 结构体之后）**

```go
// ConfigSnapshot 配置快照（用于历史记录和回滚）
type ConfigSnapshot struct {
	Config    *Config   `json:"config"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`    // 变更来源：file, api, env
	Version   int       `json:"version"`   // 递增版本号
	Checksum  string    `json:"checksum"`  // 配置内容校验和
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
```

**改动 3 — 添加 Option 函数（在第 293 行 WithConfigPath 之后）**

```go
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
```

**改动 4 — 修改 NewHotReloadManager（第 301-315 行）**

当前代码：
```go
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
```

改为：
```go
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
```

**改动 5 — 添加配置历史管理辅助方法（在 NewHotReloadManager 之后）**

```go
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
```

**改动 6 — 重写 ApplyConfig 方法（第 416-485 行）**

当前 `ApplyConfig` 直接覆盖配置无回滚。改为带验证钩子和自动回滚：

```go
func (m *HotReloadManager) ApplyConfig(newConfig *Config, source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldConfig := m.config

	// 1. 执行自定义验证钩子
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

	// 5. 通知回调（失败则自动回滚）
	if err := m.notifyCallbacksSafe(oldConfig, newConfig, appliedChanges); err != nil {
		m.logger.Error("callback failed, rolling back", zap.Error(err))
		m.rollbackLocked(oldConfig, fmt.Sprintf("callback error: %v", err), err)
		return fmt.Errorf("config applied but callback failed, rolled back: %w", err)
	}

	// 6. 推入历史记录
	m.pushHistory(newConfig, source)

	// 7. 更新变更日志
	m.changeLog = append(m.changeLog, appliedChanges...)
	if len(m.changeLog) > 1000 {
		m.changeLog = m.changeLog[len(m.changeLog)-1000:]
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
func (m *HotReloadManager) notifyCallbacksSafe(oldConfig, newConfig *Config, changes []ConfigChange) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("callback panicked: %v", r)
		}
	}()
	for _, cb := range m.changeCallbacks {
		for _, change := range changes {
			cb(change)
		}
	}
	for _, cb := range m.reloadCallbacks {
		cb(oldConfig, newConfig)
	}
	return nil
}
```

**改动 7 — 添加 Rollback 方法和回滚事件通知**

```go
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
```

**改动 8 — 修改 ReloadFromFile 增强错误处理（第 394-413 行）**

当前代码直接调用 `m.ApplyConfig(newConfig, "file")`，改为增加日志：

```go
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
```

### 修改步骤

1. 在 `HotReloadManager` 结构体中添加 `previousConfig`、`configHistory`、`maxHistorySize`、`validateFunc`、`rollbackCallbacks` 字段
2. 添加 `ConfigSnapshot`、`ValidateFunc`、`RollbackCallback`、`RollbackEvent` 类型定义
3. 添加 `WithMaxHistorySize`、`WithValidateFunc` Option 函数
4. 修改 `NewHotReloadManager` 初始化新字段并推入初始快照
5. 添加 `pushHistory`、`deepCopyConfig`、`computeConfigChecksum` 辅助方法
6. 重写 `ApplyConfig`：先验证 -> 保存 previousConfig -> 应用 -> 通知回调（失败则回滚）-> 推入历史
7. 添加 `Rollback`、`RollbackToVersion`、`rollbackLocked`、`OnRollback`、`GetConfigHistory`、`GetCurrentVersion` 方法
8. 修改 `ReloadFromFile` 增强错误日志

### 验证方法

```go
// config/hotreload_rollback_test.go

func TestRollback_ManualRollback(t *testing.T) {
    cfg1 := &Config{}
    cfg1.Agent.MaxIterations = 10
    mgr := NewHotReloadManager(cfg1)

    cfg2 := &Config{}
    cfg2.Agent.MaxIterations = 20
    require.NoError(t, mgr.ApplyConfig(cfg2, "test"))
    assert.Equal(t, 20, mgr.GetConfig().Agent.MaxIterations)

    require.NoError(t, mgr.Rollback())
    assert.Equal(t, 10, mgr.GetConfig().Agent.MaxIterations)
}

func TestRollback_AutoRollbackOnValidationFailure(t *testing.T) {
    cfg1 := &Config{}
    cfg1.Agent.MaxIterations = 10
    mgr := NewHotReloadManager(cfg1, WithValidateFunc(func(c *Config) error {
        if c.Agent.MaxIterations > 100 {
            return fmt.Errorf("max_iterations too large")
        }
        return nil
    }))

    cfg2 := &Config{}
    cfg2.Agent.MaxIterations = 999
    err := mgr.ApplyConfig(cfg2, "test")
    assert.Error(t, err)
    assert.Equal(t, 10, mgr.GetConfig().Agent.MaxIterations)
}

func TestRollback_RollbackToVersion(t *testing.T) {
    cfg1 := &Config{}
    mgr := NewHotReloadManager(cfg1)
    for i := 1; i <= 5; i++ {
        cfg := &Config{}
        cfg.Agent.MaxIterations = i * 10
        mgr.ApplyConfig(cfg, "test")
    }
    require.NoError(t, mgr.RollbackToVersion(3))
    assert.Equal(t, 20, mgr.GetConfig().Agent.MaxIterations)
}

func TestRollback_HistoryRingBuffer(t *testing.T) {
    cfg1 := &Config{}
    mgr := NewHotReloadManager(cfg1, WithMaxHistorySize(3))
    for i := 1; i <= 5; i++ {
        cfg := &Config{}
        mgr.ApplyConfig(cfg, "test")
    }
    assert.Len(t, mgr.GetConfigHistory(), 3)
}

func TestRollback_CallbackNotification(t *testing.T) {
    cfg1 := &Config{}
    mgr := NewHotReloadManager(cfg1)
    var event *RollbackEvent
    mgr.OnRollback(func(e RollbackEvent) { event = &e })

    cfg2 := &Config{}
    mgr.ApplyConfig(cfg2, "test")
    mgr.Rollback()
    assert.NotNil(t, event)
    assert.Equal(t, "manual rollback", event.Reason)
}
```

### 注意事项
- `deepCopyConfig` 通过 JSON 序列化实现深拷贝，确保历史快照不被后续修改影响。如果 `Config` 包含不可 JSON 序列化的字段需特殊处理
- `rollbackLocked` 假设调用方已持有写锁，避免死锁
- 回滚回调中的 panic 被捕获，不影响回滚本身
- `configHistory` 使用环形缓冲，默认保留最近 10 个版本，可通过 `WithMaxHistorySize` 调整
- `ValidateFunc` 可选，为 nil 时跳过自定义验证
- 需确保 `encoding/json` 已导入（当前文件已有）

---

## P2-2: base.go 拆分重构

### 需求背景
`agent/base.go` 当前有 1210 行，职责过多：Agent 接口定义、Config/Input/Output 类型、BaseAgent 结构体与生命周期、ChatCompletion（含流式 ReAct 循环）、StreamCompletion、Execute（含 Guardrails 验证和重试）、Plan、Observe、记忆管理、Guardrails 集成。需要按职责拆分为多个文件，保持所有导出接口不变（向后兼容）。

注意：`agent/integration.go` 已经存在，包含 `Enable*` 方法和 `ExecuteEnhanced`。本次拆分不涉及该文件。

### 需要修改的文件

#### 当前 agent/base.go 结构分析

| 行号范围 | 内容 | 目标文件 |
|---------|------|---------|
| 1-17 | package + import | 各文件各自 import |
| 19-31 | AgentType 常量 | agent/base.go（保留） |
| 33-57 | Agent 接口、ContextManager 接口 | agent/base.go（保留） |
| 59-93 | Input/Output/PlanResult/Feedback 类型 | agent/base.go（保留） |
| 95-121 | Config 结构体 | agent/base.go（保留） |
| 123-185 | BaseAgent 结构体 + NewBaseAgent | agent/base.go（保留） |
| 187-238 | initGuardrails | agent/base.go（保留） |
| 240-348 | toolManagerExecutor | agent/base.go（保留） |
| 350-415 | ID/Name/Type/State/Transition/Init/Teardown | agent/base.go（保留） |
| 417-612 | ChatCompletion + StreamCompletion | **agent/completion.go**（移出） |
| 614-675 | TryLockExec/UnlockExec/EnsureReady/SaveMemory/RecallMemory | agent/base.go（保留） |
| 676-735 | Provider/ToolProvider/SetToolProvider/Memory/Tools/Config/Logger/SetContextManager/ContextEngineEnabled | agent/base.go（保留） |
| 737-793 | Plan | **agent/react.go**（移出） |
| 796-1048 | Execute + buildValidationFeedbackMessage | **agent/react.go**（移出） |
| 1050-1093 | Observe | **agent/react.go**（移出） |
| 1095-1127 | parsePlanSteps | **agent/react.go**（移出） |
| 1129-1210 | GuardrailsError + SetGuardrails/GuardrailsEnabled/AddInputValidator/AddOutputValidator/AddOutputFilter | agent/base.go（保留） |

#### 新建文件：agent/completion.go

将 ChatCompletion 和 StreamCompletion 移到此文件：

```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/tools"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// ChatCompletion 调用 LLM 完成对话
func (b *BaseAgent) ChatCompletion(ctx context.Context, messages []llm.Message) (*llm.ChatResponse, error) {
	// ... 从 base.go 第 418-612 行完整移入 ...
}

// StreamCompletion 流式调用 LLM
func (b *BaseAgent) StreamCompletion(ctx context.Context, messages []llm.Message) (<-chan llm.StreamChunk, error) {
	// ... 从 base.go 第 615-656 行完整移入 ...
}

// filterToolSchemasByWhitelist 按白名单过滤工具（如果此函数仅在 completion 中使用）
// 注意：如果 filterToolSchemasByWhitelist 在其他文件也有引用，保留在 base.go
```

#### 新建文件：agent/react.go

将 Plan、Execute、Observe 和相关辅助函数移到此文件：

```go
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/llm"

	"go.uber.org/zap"
)

// Plan 生成执行计划
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	// ... 从 base.go 第 739-793 行完整移入 ...
}

// Execute 执行任务（完整的 ReAct 循环）
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	// ... 从 base.go 第 800-1037 行完整移入 ...
}

// buildValidationFeedbackMessage creates a feedback message for retry
func (b *BaseAgent) buildValidationFeedbackMessage(result *guardrails.ValidationResult) string {
	// ... 从 base.go 第 1040-1048 行完整移入 ...
}

// Observe 处理反馈并更新 Agent 状态
func (b *BaseAgent) Observe(ctx context.Context, feedback *Feedback) error {
	// ... 从 base.go 第 1052-1093 行完整移入 ...
}

// parsePlanSteps 从 LLM 响应中解析执行步骤
func parsePlanSteps(content string) []string {
	// ... 从 base.go 第 1096-1127 行完整移入 ...
}
```

#### 修改文件：agent/base.go

从 base.go 中删除已移出的函数，保留：
- package 声明和 import（按需调整）
- AgentType 常量和预定义类型
- Agent 接口、ContextManager 接口
- Input/Output/PlanResult/Feedback 类型
- Config 结构体
- BaseAgent 结构体 + NewBaseAgent + initGuardrails
- toolManagerExecutor 及其方法
- ID/Name/Type/State/Transition/Init/Teardown
- TryLockExec/UnlockExec/EnsureReady/SaveMemory/RecallMemory
- Provider/ToolProvider/SetToolProvider/Memory/Tools/Config/Logger/SetContextManager/ContextEngineEnabled
- GuardrailsError 类型和 SetGuardrails/GuardrailsEnabled/AddInputValidator/AddOutputValidator/AddOutputFilter

base.go 的 import 需要移除不再使用的包（如 `encoding/json` 如果只在 ChatCompletion 中使用）。

### 修改步骤

1. 创建 `agent/completion.go`，添加 `package agent` 和必要的 import
2. 从 `agent/base.go` 剪切 `ChatCompletion` 方法（第 417-612 行）到 `completion.go`
3. 从 `agent/base.go` 剪切 `StreamCompletion` 方法（第 614-656 行）到 `completion.go`
4. 创建 `agent/react.go`，添加 `package agent` 和必要的 import
5. 从 `agent/base.go` 剪切 `Plan` 方法（第 737-793 行）到 `react.go`
6. 从 `agent/base.go` 剪切 `Execute` 方法（第 796-1037 行）到 `react.go`
7. 从 `agent/base.go` 剪切 `buildValidationFeedbackMessage` 方法（第 1039-1048 行）到 `react.go`
8. 从 `agent/base.go` 剪切 `Observe` 方法（第 1050-1093 行）到 `react.go`
9. 从 `agent/base.go` 剪切 `parsePlanSteps` 函数（第 1095-1127 行）到 `react.go`
10. 清理 `agent/base.go` 的 import，移除不再需要的包
11. 运行 `go build ./agent/...` 确认编译通过
12. 运行 `go test ./agent/...` 确认所有测试通过

### 验证方法

```bash
# 1. 编译检查
cd D:/code/agentflow
go build ./agent/...

# 2. 运行现有测试（确保向后兼容）
go test ./agent/... -v -count=1

# 3. 检查导出符号未变化（使用 go doc）
go doc ./agent/ BaseAgent.ChatCompletion
go doc ./agent/ BaseAgent.StreamCompletion
go doc ./agent/ BaseAgent.Execute
go doc ./agent/ BaseAgent.Plan
go doc ./agent/ BaseAgent.Observe

# 4. 检查没有循环依赖
go vet ./agent/...

# 5. 行数检查
wc -l agent/base.go agent/completion.go agent/react.go
# 预期：base.go ~650 行, completion.go ~250 行, react.go ~350 行
```

### 注意事项
- 同一个 package 内的文件可以互相访问未导出的字段和方法，所以拆分不会破坏任何内部访问
- `filterToolSchemasByWhitelist` 函数需要检查是否在 `completion.go` 和 `react.go` 中都有引用。如果是，保留在 `base.go`；如果仅在 `completion.go` 中使用，移到 `completion.go`
- `runtimeStreamEmitterFromContext` 在 `agent/runtime_stream.go` 中定义，`completion.go` 可以直接引用
- 不要创建 `agent/features.go`，因为 `agent/integration.go` 已经承担了 Enable* 方法和增强功能管理的职责
- 每个新文件的 import 只包含该文件实际使用的包，避免 unused import 编译错误

---

## P2-3: RAG Multi-hop 去重修复

### 需求背景
`rag/multi_hop.go` 的 `MultiHopReasoner` 在多跳推理过程中，`executeHop`（第 350 行）仅通过 `seenDocIDs[result.Document.ID]` 做基于文档 ID 的去重。存在以下问题：

1. **跨 hop 去重不完整**：`Reason` 方法（第 202 行）中 `seenDocIDs` 传入 `executeHop`，但 `executeHop` 内部的去重（第 401 行）只检查 `seenDocIDs`，不处理同一 hop 内的重复
2. **缺少内容相似度去重**：不同 ID 但内容相同/高度相似的文档不会被去重（如同一文档被不同 chunk 策略切分后产生不同 ID）
3. **去重后未重新计算相关性分数**：去重后直接截断到 `ResultsPerHop`，没有重新排序
4. **缺少去重统计指标**：无法知道每个 hop 去重了多少文档

### 需要修改的文件

#### 文件：rag/multi_hop.go

**改动 1 — ReasoningHop 结构体添加去重统计字段（第 42-55 行）**

在 `ReasoningHop` 结构体中添加：

```go
type ReasoningHop struct {
	ID               string            `json:"id"`
	HopNumber        int               `json:"hop_number"`
	Type             HopType           `json:"type"`
	Query            string            `json:"query"`
	TransformedQuery string            `json:"transformed_query,omitempty"`
	Results          []RetrievalResult `json:"results"`
	Context          string            `json:"context,omitempty"`
	Reasoning        string            `json:"reasoning,omitempty"`
	Confidence       float64           `json:"confidence"`
	Duration         time.Duration     `json:"duration"`
	Metadata         map[string]any    `json:"metadata,omitempty"`
	Timestamp        time.Time         `json:"timestamp"`

	// 去重统计（新增）
	DedupStats *DedupStats `json:"dedup_stats,omitempty"`
}

// DedupStats 去重统计
type DedupStats struct {
	TotalRetrieved    int `json:"total_retrieved"`     // 原始检索结果数
	DedupByID         int `json:"dedup_by_id"`         // 按 ID 去重数量
	DedupBySimilarity int `json:"dedup_by_similarity"` // 按内容相似度去重数量
	FinalCount        int `json:"final_count"`         // 去重后最终数量
}
```

**改动 2 — ReasoningChain 结构体添加全局去重统计（第 58-71 行）**

在 `ReasoningChain` 中添加：

```go
type ReasoningChain struct {
	ID              string          `json:"id"`
	OriginalQuery   string          `json:"original_query"`
	Hops            []ReasoningHop  `json:"hops"`
	FinalAnswer     string          `json:"final_answer,omitempty"`
	FinalContext    string          `json:"final_context"`
	Status          ReasoningStatus `json:"status"`
	TotalDuration   time.Duration   `json:"total_duration"`
	TotalRetrieval  int             `json:"total_retrieval"`
	UniqueDocuments int             `json:"unique_documents"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	CompletedAt     time.Time       `json:"completed_at,omitempty"`

	// 全局去重统计（新增）
	TotalDedupByID         int `json:"total_dedup_by_id"`
	TotalDedupBySimilarity int `json:"total_dedup_by_similarity"`
}
```

**改动 3 — MultiHopReasoner 结构体添加 embedding 函数引用（已有，确认）**

当前结构体第 125-133 行已有 `embeddingFunc`，用于生成查询 embedding。我们将复用此函数来计算文档间的内容相似度。

**改动 4 — 重写 executeHop 的去重逻辑（第 350-443 行）**

当前 `executeHop` 的去重逻辑（第 398-411 行）：
```go
// Filter and deduplicate results
filteredResults := make([]RetrievalResult, 0, r.config.ResultsPerHop)
for _, result := range results {
	if r.config.DeduplicateResults && seenDocIDs[result.Document.ID] {
		continue
	}
	if result.FinalScore < r.config.MinConfidence {
		continue
	}
	filteredResults = append(filteredResults, result)
	if len(filteredResults) >= r.config.ResultsPerHop {
		break
	}
}
```

改为：
```go
// Filter and deduplicate results
stats := &DedupStats{
	TotalRetrieved: len(results),
}

// Phase 1: 基于文档 ID 去重 + 最低分数过滤
idFilteredResults := make([]RetrievalResult, 0, len(results))
hopSeenIDs := make(map[string]bool) // 同一 hop 内的 ID 去重
for _, result := range results {
	// 跨 hop ID 去重
	if r.config.DeduplicateResults && seenDocIDs[result.Document.ID] {
		stats.DedupByID++
		continue
	}
	// 同一 hop 内 ID 去重
	if hopSeenIDs[result.Document.ID] {
		stats.DedupByID++
		continue
	}
	// 最低分数过滤
	if result.FinalScore < r.config.MinConfidence {
		continue
	}
	hopSeenIDs[result.Document.ID] = true
	idFilteredResults = append(idFilteredResults, result)
}

// Phase 2: 基于内容相似度去重
filteredResults := r.deduplicateBySimilarity(hopCtx, idFilteredResults, stats)

// Phase 3: 去重后重新排序（按 FinalScore 降序）
sort.Slice(filteredResults, func(i, j int) bool {
	return filteredResults[i].FinalScore > filteredResults[j].FinalScore
})

// Phase 4: 截断到 ResultsPerHop
if len(filteredResults) > r.config.ResultsPerHop {
	filteredResults = filteredResults[:r.config.ResultsPerHop]
}

stats.FinalCount = len(filteredResults)
hop.DedupStats = stats
```

**改动 5 — 添加 deduplicateBySimilarity 方法**

在 `executeHop` 方法之后添加：

```go
// deduplicateBySimilarity 基于内容相似度去重
// 使用文档 embedding 计算余弦相似度，超过阈值的视为重复
func (r *MultiHopReasoner) deduplicateBySimilarity(
	ctx context.Context,
	results []RetrievalResult,
	stats *DedupStats,
) []RetrievalResult {
	if !r.config.DeduplicateResults || len(results) <= 1 {
		return results
	}

	threshold := r.config.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.85 // 默认阈值
	}

	deduplicated := make([]RetrievalResult, 0, len(results))

	for _, candidate := range results {
		isDuplicate := false

		for _, existing := range deduplicated {
			similarity := r.computeContentSimilarity(ctx, candidate.Document, existing.Document)
			if similarity >= threshold {
				isDuplicate = true
				stats.DedupBySimilarity++

				// 如果候选文档分数更高，替换已有文档
				if candidate.FinalScore > existing.FinalScore {
					for i, d := range deduplicated {
						if d.Document.ID == existing.Document.ID {
							deduplicated[i] = candidate
							break
						}
					}
				}
				break
			}
		}

		if !isDuplicate {
			deduplicated = append(deduplicated, candidate)
		}
	}

	return deduplicated
}

// computeContentSimilarity 计算两个文档的内容相似度
// 优先使用 embedding 余弦相似度，fallback 到 Jaccard 相似度
func (r *MultiHopReasoner) computeContentSimilarity(
	ctx context.Context,
	doc1, doc2 Document,
) float64 {
	// 策略 1：如果两个文档都有 embedding，使用余弦相似度
	if len(doc1.Embedding) > 0 && len(doc2.Embedding) > 0 && len(doc1.Embedding) == len(doc2.Embedding) {
		return cosineSimilarity(doc1.Embedding, doc2.Embedding)
	}

	// 策略 2：如果有 embeddingFunc，动态生成 embedding
	if r.embeddingFunc != nil {
		emb1, err1 := r.embeddingFunc(ctx, doc1.Content)
		emb2, err2 := r.embeddingFunc(ctx, doc2.Content)
		if err1 == nil && err2 == nil && len(emb1) == len(emb2) {
			return cosineSimilarity(emb1, emb2)
		}
	}

	// 策略 3：Fallback 到 Jaccard 相似度（基于词集合）
	return jaccardSimilarity(doc1.Content, doc2.Content)
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// jaccardSimilarity 计算 Jaccard 相似度（基于词集合）
func jaccardSimilarity(text1, text2 string) float64 {
	words1 := tokenizeToSet(text1)
	words2 := tokenizeToSet(text2)

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range words1 {
		if words2[w] {
			intersection++
		}
	}

	union := len(words1) + len(words2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// tokenizeToSet 将文本分词为集合
func tokenizeToSet(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}
```

**改动 6 — 更新 Reason 方法中的去重统计汇总（第 304-311 行）**

当前代码：
```go
// Track unique documents
for _, result := range hop.Results {
	if !seenDocIDs[result.Document.ID] {
		seenDocIDs[result.Document.ID] = true
		chain.UniqueDocuments++
	}
	chain.TotalRetrieval++
}
```

改为：
```go
// Track unique documents
for _, result := range hop.Results {
	if !seenDocIDs[result.Document.ID] {
		seenDocIDs[result.Document.ID] = true
		chain.UniqueDocuments++
	}
	chain.TotalRetrieval++
}

// 汇总去重统计
if hop.DedupStats != nil {
	chain.TotalDedupByID += hop.DedupStats.DedupByID
	chain.TotalDedupBySimilarity += hop.DedupStats.DedupBySimilarity
}
```

**改动 7 — 更新 import（文件头部）**

确保 import 中包含 `"math"` 和 `"sort"`（当前已有 `"sort"`，需添加 `"math"`）：

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"math"  // 新增
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)
```

**改动 8 — 更新 Reason 方法的完成日志（第 340-344 行）**

```go
r.logger.Info("reasoning completed",
	zap.String("query", query),
	zap.Int("hops", len(chain.Hops)),
	zap.Int("unique_docs", chain.UniqueDocuments),
	zap.Int("total_retrieved", chain.TotalRetrieval),
	zap.Int("dedup_by_id", chain.TotalDedupByID),
	zap.Int("dedup_by_similarity", chain.TotalDedupBySimilarity),
	zap.Duration("duration", chain.TotalDuration))
```

### 修改步骤

1. 在 `ReasoningHop` 结构体中添加 `DedupStats` 字段
2. 添加 `DedupStats` 结构体定义
3. 在 `ReasoningChain` 结构体中添加 `TotalDedupByID` 和 `TotalDedupBySimilarity` 字段
4. 在 import 中添加 `"math"`
5. 重写 `executeHop` 中的去重逻辑（三阶段：ID 去重 -> 内容相似度去重 -> 重新排序）
6. 添加 `deduplicateBySimilarity`、`computeContentSimilarity`、`cosineSimilarity`、`jaccardSimilarity`、`tokenizeToSet` 方法
7. 更新 `Reason` 方法中的去重统计汇总
8. 更新完成日志添加去重指标

### 验证方法

```go
// rag/multi_hop_dedup_test.go

func TestMultiHop_DeduplicateByID(t *testing.T) {
    // 构造包含重复 ID 的检索结果
    config := DefaultMultiHopConfig()
    config.DeduplicateResults = true
    config.MaxHops = 2
    config.EnableLLMReasoning = false

    // 创建 mock retriever 返回重复文档
    retriever := newMockRetriever([]RetrievalResult{
        {Document: Document{ID: "doc1", Content: "content A"}, FinalScore: 0.9},
        {Document: Document{ID: "doc1", Content: "content A"}, FinalScore: 0.8}, // 重复 ID
        {Document: Document{ID: "doc2", Content: "content B"}, FinalScore: 0.7},
    })

    reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())
    chain, err := reasoner.Reason(context.Background(), "test query")

    require.NoError(t, err)
    // 第一个 hop 应该去重 doc1
    assert.Equal(t, 2, len(chain.Hops[0].Results)) // doc1 + doc2
    assert.Equal(t, 1, chain.Hops[0].DedupStats.DedupByID)
}

func TestMultiHop_DeduplicateBySimilarity(t *testing.T) {
    config := DefaultMultiHopConfig()
    config.DeduplicateResults = true
    config.SimilarityThreshold = 0.8
    config.MaxHops = 1
    config.EnableLLMReasoning = false

    // 不同 ID 但内容几乎相同
    retriever := newMockRetriever([]RetrievalResult{
        {Document: Document{ID: "doc1", Content: "the quick brown fox jumps over the lazy dog"}, FinalScore: 0.9},
        {Document: Document{ID: "doc2", Content: "the quick brown fox jumps over the lazy dog today"}, FinalScore: 0.85},
        {Document: Document{ID: "doc3", Content: "completely different content about cats"}, FinalScore: 0.7},
    })

    reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())
    chain, err := reasoner.Reason(context.Background(), "test query")

    require.NoError(t, err)
    hop := chain.Hops[0]
    // doc1 和 doc2 内容高度相似，应该去重
    assert.LessOrEqual(t, len(hop.Results), 2)
    assert.Greater(t, hop.DedupStats.DedupBySimilarity, 0)
}

func TestMultiHop_DedupStatsAccumulation(t *testing.T) {
    config := DefaultMultiHopConfig()
    config.DeduplicateResults = true
    config.MaxHops = 3
    config.EnableLLMReasoning = false
    config.EnableQueryRefinement = false

    // 多个 hop 的去重统计应该累加
    reasoner := NewMultiHopReasoner(config, newMockRetriever(nil), nil, nil, nil, zap.NewNop())
    chain, err := reasoner.Reason(context.Background(), "test")

    require.NoError(t, err)
    // 验证全局统计 = 各 hop 统计之和
    totalByID := 0
    totalBySim := 0
    for _, hop := range chain.Hops {
        if hop.DedupStats != nil {
            totalByID += hop.DedupStats.DedupByID
            totalBySim += hop.DedupStats.DedupBySimilarity
        }
    }
    assert.Equal(t, totalByID, chain.TotalDedupByID)
    assert.Equal(t, totalBySim, chain.TotalDedupBySimilarity)
}

func TestMultiHop_ReorderAfterDedup(t *testing.T) {
    config := DefaultMultiHopConfig()
    config.DeduplicateResults = true
    config.MaxHops = 1
    config.EnableLLMReasoning = false

    // 去重后应按分数重新排序
    retriever := newMockRetriever([]RetrievalResult{
        {Document: Document{ID: "doc1", Content: "A"}, FinalScore: 0.5},
        {Document: Document{ID: "doc2", Content: "B"}, FinalScore: 0.9},
        {Document: Document{ID: "doc3", Content: "C"}, FinalScore: 0.7},
    })

    reasoner := NewMultiHopReasoner(config, retriever, nil, nil, nil, zap.NewNop())
    chain, err := reasoner.Reason(context.Background(), "test")

    require.NoError(t, err)
    results := chain.Hops[0].Results
    // 应按分数降序排列
    for i := 1; i < len(results); i++ {
        assert.GreaterOrEqual(t, results[i-1].FinalScore, results[i].FinalScore)
    }
}

func TestJaccardSimilarity(t *testing.T) {
    // 完全相同
    assert.InDelta(t, 1.0, jaccardSimilarity("hello world", "hello world"), 0.01)
    // 完全不同
    assert.InDelta(t, 0.0, jaccardSimilarity("hello world", "foo bar"), 0.01)
    // 部分重叠
    sim := jaccardSimilarity("the quick brown fox", "the quick red fox")
    assert.Greater(t, sim, 0.5)
    assert.Less(t, sim, 1.0)
}

func TestCosineSimilarity(t *testing.T) {
    // 相同向量
    assert.InDelta(t, 1.0, cosineSimilarity([]float64{1, 0, 0}, []float64{1, 0, 0}), 0.01)
    // 正交向量
    assert.InDelta(t, 0.0, cosineSimilarity([]float64{1, 0, 0}, []float64{0, 1, 0}), 0.01)
    // 长度不同
    assert.InDelta(t, 0.0, cosineSimilarity([]float64{1, 0}, []float64{1, 0, 0}), 0.01)
}
```

### 注意事项
- `cosineSimilarity` 函数与 `hybrid_retrieval.go` 中的 `HybridRetriever.cosineSimilarity` 方法功能相同，但这里是包级函数而非方法。如果想复用，可以将 `HybridRetriever.cosineSimilarity` 提取为包级函数，两处共用
- `deduplicateBySimilarity` 的时间复杂度为 O(n^2)，对于每个 hop 的 `ResultsPerHop`（默认 5）来说完全可接受。如果 `ResultsPerHop` 很大（>100），考虑使用 LSH 等近似方法
- 内容相似度去重优先使用 embedding 余弦相似度（更准确），fallback 到 Jaccard 相似度（不需要 embedding 模型）
- 当两个文档内容相似但分数不同时，保留分数更高的那个
- `SimilarityThreshold` 默认 0.85，可通过 `MultiHopConfig` 配置。阈值过低会误去重，过高则去重效果不明显
