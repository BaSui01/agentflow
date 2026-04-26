package runtime

import (
	"context"
	"time"
	types "github.com/BaSui01/agentflow/types"
	zap "go.uber.org/zap"
)

// ID 返回 Agent ID
func (b *BaseAgent) ID() string { return b.config.Core.ID }

// Name 返回 Agent 名称
func (b *BaseAgent) Name() string { return b.config.Core.Name }

// Type 返回 Agent 类型
func (b *BaseAgent) Type() AgentType { return AgentType(b.config.Core.Type) }

// State 返回当前状态
func (b *BaseAgent) State() State {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	return b.state
}

// Transition 状态转换（带校验）
func (b *BaseAgent) Transition(ctx context.Context, to State) error {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()

	from := b.state
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}

	b.state = to
	b.logger.Info("state transition",
		zap.String("agent_id", b.config.Core.ID),
		zap.String("from", string(from)),
		zap.String("to", string(to)),
	)

	// 发布状态变更事件
	if b.bus != nil {
		b.bus.Publish(&StateChangeEvent{
			AgentID_:   b.config.Core.ID,
			FromState:  from,
			ToState:    to,
			Timestamp_: time.Now(),
		})
	}

	return nil
}
// Init 初始化 Agent
func (b *BaseAgent) Init(ctx context.Context) error {
	b.logger.Info("initializing agent")

	// 加载记忆（如果有）并缓存
	if b.memory != nil {
		records, err := b.memory.LoadRecent(ctx, b.config.Core.ID, MemoryShortTerm, defaultMaxRecentMemory)
		if err != nil {
			b.logger.Warn("failed to load memory", zap.Error(err))
		} else {
			b.recentMemoryMu.Lock()
			b.recentMemory = records
			b.recentMemoryMu.Unlock()
		}
	}

	return b.Transition(ctx, StateReady)
}

// Teardown 清理资源
func (b *BaseAgent) Teardown(ctx context.Context) error {
	b.logger.Info("tearing down agent")
	return b.extensions.TeardownExtensions(ctx)
}
// execLockWaitTimeout 短超时等待，避免并发请求直接返回 ErrAgentBusy
const execLockWaitTimeout = 100 * time.Millisecond

// TryLockExec 尝试获取执行槽位，防止并发执行超出限制。
// 在超时时间内（默认 100ms）会等待，而非立即返回失败。
func (b *BaseAgent) TryLockExec() bool {
	ctx, cancel := context.WithTimeout(context.Background(), execLockWaitTimeout)
	defer cancel()
	return b.execSem.Acquire(ctx, 1) == nil
}

// UnlockExec 释放执行槽位。
func (b *BaseAgent) UnlockExec() {
	b.execSem.Release(1)
}
// EnsureReady 确保 Agent 处于就绪状态
func (b *BaseAgent) EnsureReady() error {
	state := b.State()
	if state != StateReady && state != StateRunning {
		return ErrAgentNotReady
	}
	return nil
}

// SaveMemory 保存记忆并同步更新本地缓存
func (b *BaseAgent) SaveMemory(ctx context.Context, content string, kind MemoryKind, metadata map[string]any) error {
	if b.memory == nil {
		return nil
	}

	rec := MemoryRecord{
		AgentID:   b.config.Core.ID,
		Kind:      kind,
		Content:   content,
		Metadata:  metadata,
		CreatedAt: time.Now(),
	}

	if err := b.memory.Save(ctx, rec); err != nil {
		return err
	}

	// Write-through: keep the in-process cache consistent so that
	// subsequent Execute() calls within the same agent instance see
	// the newly saved record without a full reload.
	b.recentMemoryMu.Lock()
	b.recentMemory = append(b.recentMemory, rec)
	if len(b.recentMemory) > defaultMaxRecentMemory {
		b.recentMemory = b.recentMemory[len(b.recentMemory)-defaultMaxRecentMemory:]
	}
	b.recentMemoryMu.Unlock()

	return nil
}

// RecallMemory 检索记忆
func (b *BaseAgent) RecallMemory(ctx context.Context, query string, topK int) ([]MemoryRecord, error) {
	if b.memory == nil {
		return []MemoryRecord{}, nil
	}
	return b.memory.Search(ctx, b.config.Core.ID, query, topK)
}
// maxReActIterations 返回 ReAct 最大迭代次数，默认 10
func (b *BaseAgent) maxReActIterations() int {
	if value := b.config.ExecutionOptions().Control.MaxReActIterations; value > 0 {
		return value
	}
	return 10
}

// Memory 返回记忆管理器
func (b *BaseAgent) Memory() MemoryManager { return b.memory }

// Tools 返回工具注册中心
func (b *BaseAgent) Tools() ToolManager { return b.toolManager }
// Config 返回配置
func (b *BaseAgent) Config() types.AgentConfig { return b.config }

// Logger 返回日志器
func (b *BaseAgent) Logger() *zap.Logger { return b.logger }
// ContextEngineEnabled 返回上下文工程是否启用
func (b *BaseAgent) ContextEngineEnabled() bool {
	return b.contextEngineEnabled
}
