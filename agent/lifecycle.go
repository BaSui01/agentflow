package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// LifecycleManager 管理 Agent 的生命周期
// 提供启动、停止、健康检查等功能
type LifecycleManager struct {
	agent  Agent
	logger *zap.Logger

	mu       sync.RWMutex
	running  bool
	stopChan chan struct{}
	doneChan chan struct{}

	// 健康检查
	healthCheckInterval time.Duration
	lastHealthCheck     time.Time
	healthStatus        HealthStatus
}

// HealthStatus 健康状态
type HealthStatus struct {
	Healthy   bool      `json:"healthy"`
	State     State     `json:"state"`
	LastCheck time.Time `json:"last_check"`
	Message   string    `json:"message,omitempty"`
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager(agent Agent, logger *zap.Logger) *LifecycleManager {
	return &LifecycleManager{
		agent:               agent,
		logger:              logger,
		stopChan:            make(chan struct{}),
		doneChan:            make(chan struct{}),
		healthCheckInterval: 30 * time.Second,
		healthStatus: HealthStatus{
			Healthy: false,
			State:   agent.State(),
		},
	}
}

// Start 启动 Agent
func (lm *LifecycleManager) Start(ctx context.Context) error {
	lm.mu.Lock()
	if lm.running {
		lm.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	lm.running = true
	lm.mu.Unlock()

	lm.logger.Info("starting agent lifecycle manager",
		zap.String("agent_id", lm.agent.ID()),
		zap.String("agent_name", lm.agent.Name()),
	)

	// 初始化 Agent
	if err := lm.agent.Init(ctx); err != nil {
		lm.mu.Lock()
		lm.running = false
		lm.mu.Unlock()
		return fmt.Errorf("failed to initialize agent: %w", err)
	}

	// 启动健康检查
	go lm.healthCheckLoop(ctx)

	lm.logger.Info("agent lifecycle manager started")
	return nil
}

// Stop 停止 Agent
func (lm *LifecycleManager) Stop(ctx context.Context) error {
	lm.mu.Lock()
	if !lm.running {
		lm.mu.Unlock()
		return fmt.Errorf("agent not running")
	}
	// 在同一个临界区内设置 running = false 并 close channel，
	// 防止两个并发 Stop() 都通过检查后 double-close panic。
	lm.running = false
	close(lm.stopChan)
	lm.mu.Unlock()

	lm.logger.Info("stopping agent lifecycle manager",
		zap.String("agent_id", lm.agent.ID()),
	)

	// 等待健康检查循环结束
	select {
	case <-lm.doneChan:
	case <-time.After(5 * time.Second):
		lm.logger.Warn("health check loop did not stop in time")
	}

	// 清理 Agent 资源
	if err := lm.agent.Teardown(ctx); err != nil {
		lm.logger.Error("failed to teardown agent", zap.Error(err))
		return err
	}

	lm.logger.Info("agent lifecycle manager stopped")
	return nil
}

// IsRunning 检查是否正在运行
func (lm *LifecycleManager) IsRunning() bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.running
}

// GetHealthStatus 获取健康状态
func (lm *LifecycleManager) GetHealthStatus() HealthStatus {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.healthStatus
}

// healthCheckLoop 健康检查循环
func (lm *LifecycleManager) healthCheckLoop(ctx context.Context) {
	defer close(lm.doneChan)

	ticker := time.NewTicker(lm.healthCheckInterval)
	defer ticker.Stop()

	// 立即执行一次健康检查
	lm.performHealthCheck()

	for {
		select {
		case <-lm.stopChan:
			lm.logger.Info("health check loop stopped")
			return
		case <-ticker.C:
			lm.performHealthCheck()
		case <-ctx.Done():
			lm.logger.Info("health check loop cancelled")
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (lm *LifecycleManager) performHealthCheck() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	state := lm.agent.State()
	now := time.Now()

	// 判断健康状态
	healthy := state == StateReady || state == StateRunning
	message := ""

	if !healthy {
		message = fmt.Sprintf("agent in unhealthy state: %s", state)
	}

	lm.healthStatus = HealthStatus{
		Healthy:   healthy,
		State:     state,
		LastCheck: now,
		Message:   message,
	}

	lm.lastHealthCheck = now

	if !healthy {
		lm.logger.Warn("agent health check failed",
			zap.String("agent_id", lm.agent.ID()),
			zap.String("state", string(state)),
			zap.String("message", message),
		)
	} else {
		lm.logger.Debug("agent health check passed",
			zap.String("agent_id", lm.agent.ID()),
			zap.String("state", string(state)),
		)
	}
}

// Restart 重启 Agent
func (lm *LifecycleManager) Restart(ctx context.Context) error {
	lm.logger.Info("restarting agent",
		zap.String("agent_id", lm.agent.ID()),
	)

	// 停止
	if err := lm.Stop(ctx); err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	// 等待一小段时间
	time.Sleep(1 * time.Second)

	// 在锁保护下重新创建通道，防止与并发读取竞争
	lm.mu.Lock()
	lm.stopChan = make(chan struct{})
	lm.doneChan = make(chan struct{})
	lm.mu.Unlock()

	// 启动
	if err := lm.Start(ctx); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	lm.logger.Info("agent restarted successfully")
	return nil
}
