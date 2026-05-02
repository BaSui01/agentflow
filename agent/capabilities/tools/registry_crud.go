package tools

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

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

	// Persist to store
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

	// Persist deletion to store
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

	// Persist to store
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
		return []CapabilityInfo{}, nil
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
