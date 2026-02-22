package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"go.uber.org/zap"
)

// 能力登记是书记官处接口的默认执行。
// 它为特工提供内存,并提供能力信息
// 支持健康检查和事件通知。
type CapabilityRegistry struct {
	mu sync.RWMutex

	// 特工用身份证储存注册探员
	agents map[string]*AgentInfo

	// 能力 按名称索引快速检索的能力。
	capabilityIndex map[string]map[string]*CapabilityInfo // capability name -> agent ID -> capability

	// 事件 Handlers 存储事件处理器。
	eventHandlers map[string]DiscoveryEventHandler
	handlerMu     sync.RWMutex

	// 健康检查员定期进行健康检查。
	healthChecker *HealthChecker

	// 配置包含注册配置 。
	config *RegistryConfig

	// store is an optional persistence backend. When non-nil, agent data is
	// also persisted through this store. When nil, the registry operates
	// purely in-memory (preserving backward compatibility).
	store RegistryStore

	// logger 是日志实例 。
	logger *zap.Logger

	// 信号关闭了
	done      chan struct{}
	closeOnce sync.Once

	// subscriptionCounter 原子计数器，用于生成唯一订阅 ID
	subscriptionCounter atomic.Uint64
}

// 登记册Config拥有能力登记册的配置。
type RegistryConfig struct {
	// 健康检查Interval是健康检查的间隔.
	HealthCheckInterval time.Duration `json:"health_check_interval"`

	// 健康检查 暂停是健康检查的暂停。
	HealthCheckTimeout time.Duration `json:"health_check_timeout"`

	// 体质不健康 阈值是指在标记不健康之前,健康检查失败的次数.
	UnhealthyThreshold int `json:"unhealthy_threshold"`

	// 移除Unhealty 之后是清除不健康剂的期限。
	RemoveUnhealthyAfter time.Duration `json:"remove_unhealthy_after"`

	// 启用健康检查可以定期进行健康检查。
	EnableHealthCheck bool `json:"enable_health_check"`

	// 默认能力分数是新能力的默认分数.
	DefaultCapabilityScore float64 `json:"default_capability_score"`
}

// 默认 RegistryConfig 返回带有合理默认的注册Config 。
func DefaultRegistryConfig() *RegistryConfig {
	return &RegistryConfig{
		HealthCheckInterval:    30 * time.Second,
		HealthCheckTimeout:     5 * time.Second,
		UnhealthyThreshold:     3,
		RemoveUnhealthyAfter:   5 * time.Minute,
		EnableHealthCheck:      true,
		DefaultCapabilityScore: 50.0,
	}
}

// RegistryOption configures a CapabilityRegistry.
type RegistryOption func(*CapabilityRegistry)

// WithStore sets a persistence backend for the registry.
// When set, agent data is persisted through the store in addition to the
// in-memory map. When not set, the registry operates purely in-memory.
func WithStore(store RegistryStore) RegistryOption {
	return func(r *CapabilityRegistry) {
		r.store = store
	}
}

// 新能力登记系统建立了一个新的能力登记册。
func NewCapabilityRegistry(config *RegistryConfig, logger *zap.Logger, opts ...RegistryOption) *CapabilityRegistry {
	if config == nil {
		config = DefaultRegistryConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	r := &CapabilityRegistry{
		agents:          make(map[string]*AgentInfo),
		capabilityIndex: make(map[string]map[string]*CapabilityInfo),
		eventHandlers:   make(map[string]DiscoveryEventHandler),
		config:          config,
		logger:          logger.With(zap.String("component", "capability_registry")),
		done:            make(chan struct{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	// 如果启用, 初始化健康检查器
	if config.EnableHealthCheck {
		r.healthChecker = NewHealthChecker(&HealthCheckerConfig{
			Interval:           config.HealthCheckInterval,
			Timeout:            config.HealthCheckTimeout,
			UnhealthyThreshold: config.UnhealthyThreshold,
		}, r, logger)
	}

	return r
}

// 启动登记册背景进程。
func (r *CapabilityRegistry) Start(ctx context.Context) error {
	if r.healthChecker != nil {
		if err := r.healthChecker.Start(ctx); err != nil {
			return fmt.Errorf("failed to start health checker: %w", err)
		}
	}

	r.logger.Info("capability registry started")
	return nil
}

// 代理人对具有其能力的代理人进行登记。
func (r *CapabilityRegistry) RegisterAgent(ctx context.Context, info *AgentInfo) error {
	if info == nil {
		return fmt.Errorf("agent info is nil")
	}
	if info.Card == nil {
		return fmt.Errorf("agent card is nil")
	}
	if info.Card.Name == "" {
		return fmt.Errorf("agent name is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	agentID := info.Card.Name

	// 检查代理已存在
	if _, exists := r.agents[agentID]; exists {
		return fmt.Errorf("agent %s already registered", agentID)
	}

	// 设定默认
	now := time.Now()
	info.RegisteredAt = now
	info.LastHeartbeat = now
	if info.Status == "" {
		info.Status = AgentStatusOnline
	}

	// 初始化能力
	for i := range info.Capabilities {
		cap := &info.Capabilities[i]
		cap.AgentID = agentID
		cap.AgentName = info.Card.Name
		cap.RegisteredAt = now
		cap.LastUpdatedAt = now
		cap.LastHealthCheck = now
		if cap.Status == "" {
			cap.Status = CapabilityStatusActive
		}
		if cap.Score == 0 {
			cap.Score = r.config.DefaultCapabilityScore
		}

		// 添加到能力指数
		r.indexCapability(cap)
	}

	// 存储代理
	r.agents[agentID] = info

	// Persist to store if configured
	if r.store != nil {
		if err := r.store.Save(ctx, info); err != nil {
			r.logger.Error("failed to persist agent to store", zap.String("agent_id", agentID), zap.Error(err))
		}
	}

	r.logger.Info("agent registered",
		zap.String("agent_id", agentID),
		zap.Int("capabilities", len(info.Capabilities)),
	)

	// 释放事件
	r.emitEvent(&DiscoveryEvent{
		Type:      DiscoveryEventAgentRegistered,
		AgentID:   agentID,
		Timestamp: now,
	})

	return nil
}

// 未注册代理 未经注册代理。
func (r *CapabilityRegistry) UnregisterAgent(ctx context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// 从索引中删除能力
	for _, cap := range info.Capabilities {
		r.removeCapabilityFromIndex(cap.Capability.Name, agentID)
	}

	// 删除代理
	delete(r.agents, agentID)

	// Persist deletion to store if configured
	if r.store != nil {
		if err := r.store.Delete(ctx, agentID); err != nil {
			r.logger.Error("failed to delete agent from store", zap.String("agent_id", agentID), zap.Error(err))
		}
	}

	r.logger.Info("agent unregistered", zap.String("agent_id", agentID))

	// 释放事件
	r.emitEvent(&DiscoveryEvent{
		Type:      DiscoveryEventAgentUnregistered,
		AgentID:   agentID,
		Timestamp: time.Now(),
	})

	return nil
}

// 更新代理更新一个代理的信息 。
func (r *CapabilityRegistry) UpdateAgent(ctx context.Context, info *AgentInfo) error {
	if info == nil || info.Card == nil {
		return fmt.Errorf("invalid agent info")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	agentID := info.Card.Name
	existing, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// 更新字段
	now := time.Now()
	info.RegisteredAt = existing.RegisteredAt
	info.LastHeartbeat = now

	// 更新能力指数
	// 首先去掉旧能力
	for _, cap := range existing.Capabilities {
		r.removeCapabilityFromIndex(cap.Capability.Name, agentID)
	}

	// 然后,增加新的能力
	for i := range info.Capabilities {
		cap := &info.Capabilities[i]
		cap.AgentID = agentID
		cap.AgentName = info.Card.Name
		cap.LastUpdatedAt = now
		if cap.RegisteredAt.IsZero() {
			cap.RegisteredAt = now
		}
		r.indexCapability(cap)
	}

	// 存储更新代理
	r.agents[agentID] = info

	// Persist to store if configured
	if r.store != nil {
		if err := r.store.Save(ctx, info); err != nil {
			r.logger.Error("failed to persist updated agent to store", zap.String("agent_id", agentID), zap.Error(err))
		}
	}

	r.logger.Info("agent updated", zap.String("agent_id", agentID))

	// 释放事件
	r.emitEvent(&DiscoveryEvent{
		Type:      DiscoveryEventAgentUpdated,
		AgentID:   agentID,
		Timestamp: now,
	})

	return nil
}

// Get Agent通过身份识别找到一个特工.
func (r *CapabilityRegistry) GetAgent(ctx context.Context, agentID string) (*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// 返回副本
	return r.copyAgentInfo(info), nil
}

// ListAgents列出所有注册代理.
func (r *CapabilityRegistry) ListAgents(ctx context.Context) ([]*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		agents = append(agents, r.copyAgentInfo(info))
	}

	return agents, nil
}

// 注册能力登记一种代理的能力。
func (r *CapabilityRegistry) RegisterCapability(ctx context.Context, agentID string, cap *CapabilityInfo) error {
	if cap == nil {
		return fmt.Errorf("capability info is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// 检查是否已经存在能力
	for _, existing := range info.Capabilities {
		if existing.Capability.Name == cap.Capability.Name {
			return fmt.Errorf("capability %s already registered for agent %s", cap.Capability.Name, agentID)
		}
	}

	// 设定默认
	now := time.Now()
	cap.AgentID = agentID
	cap.AgentName = info.Card.Name
	cap.RegisteredAt = now
	cap.LastUpdatedAt = now
	cap.LastHealthCheck = now
	if cap.Status == "" {
		cap.Status = CapabilityStatusActive
	}
	if cap.Score == 0 {
		cap.Score = r.config.DefaultCapabilityScore
	}

	// 添加到代理服务器
	info.Capabilities = append(info.Capabilities, *cap)

	// 添加到索引中
	r.indexCapability(cap)

	r.logger.Info("capability registered",
		zap.String("agent_id", agentID),
		zap.String("capability", cap.Capability.Name),
	)

	// 释放事件
	r.emitEvent(&DiscoveryEvent{
		Type:       DiscoveryEventCapabilityAdded,
		AgentID:    agentID,
		Capability: cap.Capability.Name,
		Timestamp:  now,
	})

	return nil
}

// 未注册能力不注册 一种能力。
func (r *CapabilityRegistry) UnregisterCapability(ctx context.Context, agentID string, capabilityName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// 查找并删除能力
	found := false
	for i, cap := range info.Capabilities {
		if cap.Capability.Name == capabilityName {
			info.Capabilities = append(info.Capabilities[:i], info.Capabilities[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("capability %s not found for agent %s", capabilityName, agentID)
	}

	// 从索引中删除
	r.removeCapabilityFromIndex(capabilityName, agentID)

	r.logger.Info("capability unregistered",
		zap.String("agent_id", agentID),
		zap.String("capability", capabilityName),
	)

	// 释放事件
	r.emitEvent(&DiscoveryEvent{
		Type:       DiscoveryEventCapabilityRemoved,
		AgentID:    agentID,
		Capability: capabilityName,
		Timestamp:  time.Now(),
	})

	return nil
}

// 更新能力更新一个能力.
func (r *CapabilityRegistry) UpdateCapability(ctx context.Context, agentID string, cap *CapabilityInfo) error {
	if cap == nil {
		return fmt.Errorf("capability info is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// 查找和更新能力
	found := false
	for i, existing := range info.Capabilities {
		if existing.Capability.Name == cap.Capability.Name {
			// 保留注册时间
			cap.RegisteredAt = existing.RegisteredAt
			cap.AgentID = agentID
			cap.AgentName = info.Card.Name
			cap.LastUpdatedAt = time.Now()

			info.Capabilities[i] = *cap
			found = true

			// 更新索引
			r.indexCapability(cap)
			break
		}
	}

	if !found {
		return fmt.Errorf("capability %s not found for agent %s", cap.Capability.Name, agentID)
	}

	r.logger.Debug("capability updated",
		zap.String("agent_id", agentID),
		zap.String("capability", cap.Capability.Name),
	)

	// 释放事件
	r.emitEvent(&DiscoveryEvent{
		Type:       DiscoveryEventCapabilityUpdated,
		AgentID:    agentID,
		Capability: cap.Capability.Name,
		Timestamp:  time.Now(),
	})

	return nil
}

// Get Capability通过代理身份和姓名检索能力.
func (r *CapabilityRegistry) GetCapability(ctx context.Context, agentID string, capabilityName string) (*CapabilityInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	for _, cap := range info.Capabilities {
		if cap.Capability.Name == capabilityName {
			capCopy := cap
			return &capCopy, nil
		}
	}

	return nil, fmt.Errorf("capability %s not found for agent %s", capabilityName, agentID)
}

// List Capabilitys 列出一个代理的所有能力.
func (r *CapabilityRegistry) ListCapabilities(ctx context.Context, agentID string) ([]CapabilityInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// 返回副本
	caps := make([]CapabilityInfo, len(info.Capabilities))
	copy(caps, info.Capabilities)
	return caps, nil
}

// Find Capabilitys 在所有特工中按名称找到能力.
func (r *CapabilityRegistry) FindCapabilities(ctx context.Context, capabilityName string) ([]CapabilityInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agentCaps, exists := r.capabilityIndex[capabilityName]
	if !exists {
		return nil, nil
	}

	caps := make([]CapabilityInfo, 0, len(agentCaps))
	for _, cap := range agentCaps {
		caps = append(caps, *cap)
	}

	return caps, nil
}

// 更新代理状态更新代理状态 。
func (r *CapabilityRegistry) UpdateAgentStatus(ctx context.Context, agentID string, status AgentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	oldStatus := info.Status
	info.Status = status
	info.LastHeartbeat = time.Now()

	r.logger.Debug("agent status updated",
		zap.String("agent_id", agentID),
		zap.String("old_status", string(oldStatus)),
		zap.String("new_status", string(status)),
	)

	return nil
}

// 更新 AgentLoad 更新一个代理的负载 。
func (r *CapabilityRegistry) UpdateAgentLoad(ctx context.Context, agentID string, load float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	info.Load = load
	info.LastHeartbeat = time.Now()

	// 更新容量负荷
	for i := range info.Capabilities {
		info.Capabilities[i].Load = load
	}

	return nil
}

// 记录 Execution 记录一个执行结果 一个能力。
func (r *CapabilityRegistry) RecordExecution(ctx context.Context, agentID string, capabilityName string, success bool, latency time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	for i, cap := range info.Capabilities {
		if cap.Capability.Name == capabilityName {
			if success {
				info.Capabilities[i].SuccessCount++
			} else {
				info.Capabilities[i].FailureCount++
			}

			// 更新平均延迟
			totalCount := info.Capabilities[i].SuccessCount + info.Capabilities[i].FailureCount
			if totalCount == 1 {
				info.Capabilities[i].AvgLatency = latency
			} else {
				// 指示移动平均值
				alpha := 0.2
				info.Capabilities[i].AvgLatency = time.Duration(
					float64(info.Capabilities[i].AvgLatency)*(1-alpha) + float64(latency)*alpha,
				)
			}

			// 根据成功率更新分数
			successRate := float64(info.Capabilities[i].SuccessCount) / float64(totalCount)
			info.Capabilities[i].Score = successRate * 100

			// 更新索引
			r.indexCapability(&info.Capabilities[i])

			return nil
		}
	}

	return fmt.Errorf("capability %s not found for agent %s", capabilityName, agentID)
}

// 订阅了发现事件。
func (r *CapabilityRegistry) Subscribe(handler DiscoveryEventHandler) string {
	r.handlerMu.Lock()
	defer r.handlerMu.Unlock()

	id := fmt.Sprintf("sub-%d", r.subscriptionCounter.Add(1))
	r.eventHandlers[id] = handler
	return id
}

// 不订阅来自发现事件的用户 。
func (r *CapabilityRegistry) Unsubscribe(subscriptionID string) {
	r.handlerMu.Lock()
	defer r.handlerMu.Unlock()

	delete(r.eventHandlers, subscriptionID)
}

// 关闭注册 。
func (r *CapabilityRegistry) Close() error {
	r.closeOnce.Do(func() { close(r.done) })

	if r.healthChecker != nil {
		if err := r.healthChecker.Stop(context.Background()); err != nil {
			r.logger.Error("failed to stop health checker", zap.Error(err))
		}
	}

	r.logger.Info("capability registry closed")
	return nil
}

// 指数能力为指数增加了一种能力。
func (r *CapabilityRegistry) indexCapability(cap *CapabilityInfo) {
	capName := cap.Capability.Name
	if r.capabilityIndex[capName] == nil {
		r.capabilityIndex[capName] = make(map[string]*CapabilityInfo)
	}
	r.capabilityIndex[capName][cap.AgentID] = cap
}

// 从Index中去掉Capability,从索引中去掉一个能力.
func (r *CapabilityRegistry) removeCapabilityFromIndex(capabilityName, agentID string) {
	if agentCaps, exists := r.capabilityIndex[capabilityName]; exists {
		delete(agentCaps, agentID)
		if len(agentCaps) == 0 {
			delete(r.capabilityIndex, capabilityName)
		}
	}
}

// Event向所有订阅者发布发现事件。
func (r *CapabilityRegistry) emitEvent(event *DiscoveryEvent) {
	r.handlerMu.RLock()
	handlers := make([]DiscoveryEventHandler, 0, len(r.eventHandlers))
	for _, h := range r.eventHandlers {
		handlers = append(handlers, h)
	}
	r.handlerMu.RUnlock()

	for _, handler := range handlers {
		h := handler
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					r.logger.Error("event handler panicked", zap.Any("recover", rec))
				}
			}()
			h(event)
		}()
	}
}

// 复制 AgentInfo 创建 AgentInfo 的深层副本.
func (r *CapabilityRegistry) copyAgentInfo(info *AgentInfo) *AgentInfo {
	if info == nil {
		return nil
	}

	copy := &AgentInfo{
		Status:        info.Status,
		Load:          info.Load,
		Priority:      info.Priority,
		Endpoint:      info.Endpoint,
		IsLocal:       info.IsLocal,
		RegisteredAt:  info.RegisteredAt,
		LastHeartbeat: info.LastHeartbeat,
	}

	if info.Card != nil {
		cardCopy := *info.Card
		copy.Card = &cardCopy
	}

	if len(info.Capabilities) > 0 {
		copy.Capabilities = make([]CapabilityInfo, len(info.Capabilities))
		for i, cap := range info.Capabilities {
			copy.Capabilities[i] = cap
		}
	}

	if info.Metadata != nil {
		copy.Metadata = make(map[string]string)
		for k, v := range info.Metadata {
			copy.Metadata[k] = v
		}
	}

	return copy
}

// Get AgentsBy Capability 返回所有具有特定能力的代理.
func (r *CapabilityRegistry) GetAgentsByCapability(ctx context.Context, capabilityName string) ([]*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agentCaps, exists := r.capabilityIndex[capabilityName]
	if !exists {
		return nil, nil
	}

	agents := make([]*AgentInfo, 0, len(agentCaps))
	for agentID := range agentCaps {
		if info, ok := r.agents[agentID]; ok {
			agents = append(agents, r.copyAgentInfo(info))
		}
	}

	return agents, nil
}

// GetAactiveAgents返回所有具有在线状态的代理.
func (r *CapabilityRegistry) GetActiveAgents(ctx context.Context) ([]*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*AgentInfo, 0)
	for _, info := range r.agents {
		if info.Status == AgentStatusOnline {
			agents = append(agents, r.copyAgentInfo(info))
		}
	}

	return agents, nil
}

// Heartbeat为代理更新了心跳时间戳.
func (r *CapabilityRegistry) Heartbeat(ctx context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	info.LastHeartbeat = time.Now()
	return nil
}

// 健康检查员定期对注册的代理人进行健康检查。
type HealthChecker struct {
	config   *HealthCheckerConfig
	registry *CapabilityRegistry
	logger   *zap.Logger

	// httpClient 共享的健康检查 HTTP 客户端
	httpClient *http.Client

	// 失败 。
	failureCounts map[string]int
	failureMu     sync.Mutex

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// 健康检查员Config拥有健康检查员的配置.
type HealthCheckerConfig struct {
	// 间距是指健康检查之间的间隔.
	Interval time.Duration

	// 暂停是健康检查的暂停。
	Timeout time.Duration

	// 体质不健康 阈值是标记不健康前连续失败的次数.
	UnhealthyThreshold int
}

// 新健康检查器创造了一个新的健康检查器。
func NewHealthChecker(config *HealthCheckerConfig, registry *CapabilityRegistry, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		config:        config,
		registry:      registry,
		logger:        logger.With(zap.String("component", "health_checker")),
		httpClient:    tlsutil.SecureHTTPClient(5 * time.Second),
		failureCounts: make(map[string]int),
		done:          make(chan struct{}),
	}
}

// 开始体检
func (h *HealthChecker) Start(ctx context.Context) error {
	h.wg.Add(1)
	go h.run()
	h.logger.Info("health checker started")
	return nil
}

// 停止停止健康检查。
func (h *HealthChecker) Stop(ctx context.Context) error {
	h.closeOnce.Do(func() { close(h.done) })
	h.wg.Wait()
	h.logger.Info("health checker stopped")
	return nil
}

// 运行是主要的健康检查循环。
func (h *HealthChecker) run() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-h.done:
			return
		}
	}
}

// 对所有注册代理人进行健康检查。
func (h *HealthChecker) checkAll() {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
	defer cancel()

	agents, err := h.registry.ListAgents(ctx)
	if err != nil {
		h.logger.Error("failed to list agents for health check", zap.Error(err))
		return
	}

	for _, agent := range agents {
		h.checkAgent(ctx, agent)
	}
}

// 代理对单一代理进行健康检查。
func (h *HealthChecker) checkAgent(ctx context.Context, agent *AgentInfo) {
	agentID := agent.Card.Name
	result := h.performHealthCheck(ctx, agent)

	h.failureMu.Lock()
	defer h.failureMu.Unlock()

	if result.Healthy {
		// 重设失败取决于成功
		if h.failureCounts[agentID] > 0 {
			h.logger.Info("agent health recovered",
				zap.String("agent_id", agentID),
			)
			h.registry.emitEvent(&DiscoveryEvent{
				Type:      DiscoveryEventHealthCheckRecovered,
				AgentID:   agentID,
				Timestamp: time.Now(),
			})
		}
		h.failureCounts[agentID] = 0
		h.registry.UpdateAgentStatus(ctx, agentID, AgentStatusOnline)
	} else {
		h.failureCounts[agentID]++
		h.logger.Warn("agent health check failed",
			zap.String("agent_id", agentID),
			zap.Int("consecutive_failures", h.failureCounts[agentID]),
			zap.String("message", result.Message),
		)

		if h.failureCounts[agentID] >= h.config.UnhealthyThreshold {
			h.registry.UpdateAgentStatus(ctx, agentID, AgentStatusUnhealthy)
			h.registry.emitEvent(&DiscoveryEvent{
				Type:      DiscoveryEventHealthCheckFailed,
				AgentID:   agentID,
				Timestamp: time.Now(),
				Data:      mustMarshal(result),
			})
		}
	}
}

// 进行健康检查
func (h *HealthChecker) performHealthCheck(ctx context.Context, agent *AgentInfo) *HealthCheckResult {
	start := time.Now()
	result := &HealthCheckResult{
		AgentID:   agent.Card.Name,
		Timestamp: start,
	}

	// 对本地特工,请检查他们是否还在登记和反应
	if agent.IsLocal {
		// 检查心跳新鲜度
		if time.Since(agent.LastHeartbeat) > h.config.Interval*3 {
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = "heartbeat timeout"
		} else {
			result.Healthy = true
			result.Status = AgentStatusOnline
		}
		result.Latency = time.Since(start)
		return result
	}

	// 对远程特工进行HTTP健康检查
	if agent.Endpoint != "" {
		healthURL := strings.TrimRight(agent.Endpoint, "/") + "/health"
		client := h.httpClient
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = fmt.Sprintf("failed to create health check request: %v", err)
			result.Latency = time.Since(start)
			return result
		}
		resp, err := client.Do(req)
		if err != nil {
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = fmt.Sprintf("health check request failed: %v", err)
			result.Latency = time.Since(start)
			return result
		}
		resp.Body.Close()
		result.Latency = time.Since(start)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.Healthy = true
			result.Status = AgentStatusOnline
		} else {
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = fmt.Sprintf("health check returned status %d", resp.StatusCode)
		}
		return result
	}

	result.Healthy = true
	result.Status = AgentStatusOnline
	result.Latency = time.Since(start)
	return result
}

// must Marshal 向 JSON 输入数据, 错误时返回零 。
func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// 确保能力登记工具注册界面。
var _ Registry = (*CapabilityRegistry)(nil)
