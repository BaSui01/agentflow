package a2a

import (
	"fmt"

	"go.uber.org/zap"
)

func (s *HTTPServer) RegisterAgent(ag Agent) error {
	if ag == nil {
		return fmt.Errorf("%w: nil agent", ErrInvalidMessage)
	}

	agentID := ag.ID()
	if agentID == "" {
		return fmt.Errorf("%w: agent has empty ID", ErrInvalidMessage)
	}

	s.agentsMu.Lock()
	s.agents[agentID] = ag
	s.agentsMu.Unlock()

	// 使用适配器生成和缓存代理卡
	adapter := newAgentAdapter(ag)
	card := s.cardGenerator.Generate(adapter, s.config.BaseURL)
	s.agentCardsMu.Lock()
	s.agentCards[agentID] = card
	s.agentCardsMu.Unlock()

	s.logger.Info("agent registered",
		zap.String("agent_id", agentID),
		zap.String("agent_name", ag.Name()),
	)

	return nil
}

// 特工Adapter 适应代理。 代理ConfigProvider接口的代理服务器.
type agentAdapter struct {
	ag Agent
}

func newAgentAdapter(ag Agent) *agentAdapter {
	return &agentAdapter{ag: ag}
}

func (a *agentAdapter) ID() string {
	return a.ag.ID()
}

func (a *agentAdapter) Name() string {
	return a.ag.Name()
}

func (a *agentAdapter) Type() AgentType {
	return AgentType(a.ag.Type())
}

func (a *agentAdapter) Description() string {
	// 如果执行描述方法, 请尝试从代理获取描述
	if desc, ok := a.ag.(agentDescriber); ok {
		return desc.Description()
	}
	// 基于名称和类型的默认描述
	return fmt.Sprintf("%s agent of type %s", a.ag.Name(), a.ag.Type())
}

func (a *agentAdapter) Tools() []string {
	// 执行工具方法时尝试从代理获取工具
	if tools, ok := a.ag.(agentToolProvider); ok {
		return tools.Tools()
	}
	return nil
}

func (a *agentAdapter) Metadata() map[string]string {
	// 如果执行元数据方法, 尝试从代理获取元数据
	if meta, ok := a.ag.(agentMetadataProvider); ok {
		return meta.Metadata()
	}
	return nil
}

// Unregister Agent 从服务器中删除一个代理 。
func (s *HTTPServer) UnregisterAgent(agentID string) error {
	s.agentsMu.Lock()
	delete(s.agents, agentID)
	s.agentsMu.Unlock()

	s.agentCardsMu.Lock()
	delete(s.agentCards, agentID)
	s.agentCardsMu.Unlock()

	s.logger.Info("agent unregistered", zap.String("agent_id", agentID))
	return nil
}

// Get AgentCard为注册代理人取回代理卡.
func (s *HTTPServer) GetAgentCard(agentID string) (*AgentCard, error) {
	s.agentCardsMu.RLock()
	card, ok := s.agentCards[agentID]
	s.agentCardsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	return card, nil
}

// 获得代理通过身份检索注册代理。
func (s *HTTPServer) getAgent(agentID string) (Agent, error) {
	s.agentsMu.RLock()
	ag, ok := s.agents[agentID]
	s.agentsMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	return ag, nil
}

// 获得Default Agent 返回默认代理或第一个注册代理。
func (s *HTTPServer) getDefaultAgent() (Agent, error) {
	s.agentsMu.RLock()
	defer s.agentsMu.RUnlock()

	// 首先尝试默认代理ID
	if s.config.DefaultAgentID != "" {
		if ag, ok := s.agents[s.config.DefaultAgentID]; ok {
			return ag, nil
		}
	}

	// 返回第一个可用的代理
	for _, ag := range s.agents {
		return ag, nil
	}

	return nil, ErrAgentNotFound
}

// ServiHTTP 执行 http. 服务A2A请求的掌上电脑
