package tools

import (
	"context"
	"fmt"
	"time"

	toolregistry "github.com/BaSui01/agentflow/agent/capabilities/tools/registry"
	"go.uber.org/zap"
)

func (r *CapabilityRegistry) indexCapability(cap *CapabilityInfo) {
	r.capabilityIndex.Add(cap.Capability.Name, cap.AgentID, cap)
}

// 从Index中去掉Capability,从索引中去掉一个能力.
func (r *CapabilityRegistry) removeCapabilityFromIndex(capabilityName, agentID string) {
	r.capabilityIndex.Remove(capabilityName, agentID)
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
					err := recoveredPanicToError(rec)
					r.logger.Error("event handler panicked",
						zap.Any("recover", rec),
						zap.Error(err),
						zap.String("event_type", string(event.Type)),
						zap.Stack("stack"),
					)
					if r.panicErrChan != nil {
						select {
						case r.panicErrChan <- err:
						default:
						}
					}
				}
			}()

			// 添加超时控制，防止 handler 阻塞导致 goroutine 堆积
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if rec := recover(); rec != nil {
						err := recoveredPanicToError(rec)
						r.logger.Error("inner event handler panicked",
							zap.Any("recover", rec),
							zap.Error(err),
							zap.String("event_type", string(event.Type)),
							zap.Stack("stack"),
						)
						if r.panicErrChan != nil {
							select {
							case r.panicErrChan <- err:
							default:
							}
						}
					}
				}()
				h(event)
			}()
			// P-007: 使用 NewTimer 替代 time.After，避免循环中持续分配 timer
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-done:
			case <-timer.C:
				r.logger.Warn("event handler timeout")
			}
		}()
	}
}

func recoveredPanicToError(v any) error {
	return toolregistry.RecoveredPanicToError(v)
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

	agentCaps := r.capabilityIndex.Capabilities(capabilityName)
	if len(agentCaps) == 0 {
		return []*AgentInfo{}, nil
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
