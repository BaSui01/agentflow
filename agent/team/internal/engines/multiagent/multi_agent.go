package multiagent

import (
	"context"
	"strings"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

// MultiAgentSystem 是 多 Agent 协作 surface。新接入优先使用
// agent/team 作为统一官方 facade。
type MultiAgentSystem struct {
	agents  map[string]agent.Agent
	pattern CollaborationPattern

	// 通信
	messageHub *MessageHub

	// 协调
	coordinator Coordinator

	// 配置
	config MultiAgentConfig

	logger *zap.Logger
}

// NewMultiAgentSystem 创建多 Agent 系统
func NewMultiAgentSystem(agents []agent.Agent, config MultiAgentConfig, logger *zap.Logger) *MultiAgentSystem {
	if logger == nil {
		logger = zap.NewNop()
	}

	agentMap := make(map[string]agent.Agent)
	for _, a := range agents {
		agentMap[a.ID()] = a
	}

	hub := NewMessageHub(logger)

	// 为每个 Agent 创建通道
	for _, a := range agents {
		hub.CreateChannel(a.ID())
	}

	var coordinator Coordinator
	normalizedPattern := CollaborationPattern(strings.ToLower(strings.TrimSpace(string(config.Pattern))))
	switch normalizedPattern {
	case PatternDebate:
		coordinator = NewDebateCoordinator(config, hub, logger)
	case PatternConsensus:
		coordinator = NewConsensusCoordinator(config, hub, logger)
	case PatternPipeline:
		coordinator = NewPipelineCoordinator(config, hub, logger)
	case PatternBroadcast:
		coordinator = NewBroadcastCoordinator(config, hub, logger)
	case PatternNetwork:
		coordinator = NewNetworkCoordinator(config, hub, logger)
	default:
		coordinator = NewDebateCoordinator(config, hub, logger)
	}

	return &MultiAgentSystem{
		agents:      agentMap,
		pattern:     normalizedPattern,
		messageHub:  hub,
		coordinator: coordinator,
		config:      config,
		logger:      logger.With(zap.String("component", "multi_agent_system")),
	}
}

// Execute 执行协作任务
func (m *MultiAgentSystem) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	m.logger.Info("multi-agent collaboration started",
		zap.String("pattern", string(m.pattern)),
		zap.Int("agents", len(m.agents)),
	)

	var coordinator Coordinator
	// Enforce runtime routing by pattern to avoid stale/miswired coordinator instances.
	switch m.pattern {
	case PatternConsensus:
		coordinator = NewConsensusCoordinator(m.config, m.messageHub, m.logger)
	case PatternPipeline:
		coordinator = NewPipelineCoordinator(m.config, m.messageHub, m.logger)
	case PatternBroadcast:
		coordinator = NewBroadcastCoordinator(m.config, m.messageHub, m.logger)
	case PatternNetwork:
		coordinator = NewNetworkCoordinator(m.config, m.messageHub, m.logger)
	default:
		coordinator = NewDebateCoordinator(m.config, m.messageHub, m.logger)
	}
	m.coordinator = coordinator

	output, err := coordinator.Coordinate(ctx, m.agents, input)
	if err != nil {
		return nil, err
	}

	if m.config.SharedState != nil {
		if err := m.config.SharedState.Set(ctx, "result:"+string(m.pattern), output); err != nil {
			m.logger.Warn("failed to set shared state result", zap.Error(err), zap.String("pattern", string(m.pattern)))
		}
	}

	m.logger.Info("multi-agent collaboration completed")

	return output, nil
}
