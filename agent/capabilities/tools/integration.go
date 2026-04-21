package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"go.uber.org/zap"
)

// Agent Capability Provider定义了提供能力的代理的接口.
type AgentCapabilityProvider interface {
	// ID 返回代理的唯一标识符 。
	ID() string

	// 名称返回代理名.
	Name() string

	// Get Capabilitys 返回代理的能力.
	GetCapabilities() []a2a.Capability

	// Get AgentCard返回代理的A2A卡.
	GetAgentCard() *a2a.AgentCard
}

// Agent Discovery Introduction提供物剂与发现系统之间的融合.
type AgentDiscoveryIntegration struct {
	service *DiscoveryService
	logger  *zap.Logger

	// 注册代理人
	agents   map[string]AgentCapabilityProvider
	agentsMu sync.RWMutex

	// 装入记者
	loadReporters   map[string]func() float64
	loadReportersMu sync.RWMutex

	// 配置
	config *IntegrationConfig

	// 状态
	running   bool
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// 集成Config持有代理发现集成的配置.
type IntegrationConfig struct {
	// 自动登记使代理自动登记成为可能。
	AutoRegister bool `json:"auto_register"`

	// 自动注销记录器允许在代理停止时自动注销登记 。
	AutoUnregister bool `json:"auto_unregister"`

	// 加载报告Interval是报告代理加载的间隔.
	LoadReportInterval time.Duration `json:"load_report_interval"`

	// 默认端点是本地代理的默认端点 。
	DefaultEndpoint string `json:"default_endpoint"`

	// 默认版本是代理的默认版本.
	DefaultVersion string `json:"default_version"`
}

// 默认集成Config 返回带有合理默认的集成Config 。
func DefaultIntegrationConfig() *IntegrationConfig {
	return &IntegrationConfig{
		AutoRegister:       true,
		AutoUnregister:     true,
		LoadReportInterval: 10 * time.Second,
		DefaultEndpoint:    "http://localhost:8080",
		DefaultVersion:     "1.0.0",
	}
}

// NewAgent Discovery Introduction 创造了新的代理发现集成.
func NewAgentDiscoveryIntegration(service *DiscoveryService, config *IntegrationConfig, logger *zap.Logger) *AgentDiscoveryIntegration {
	if config == nil {
		config = DefaultIntegrationConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &AgentDiscoveryIntegration{
		service:       service,
		logger:        logger.With(zap.String("component", "agent_discovery_integration")),
		agents:        make(map[string]AgentCapabilityProvider),
		loadReporters: make(map[string]func() float64),
		config:        config,
		done:          make(chan struct{}),
	}
}

// 开始整合
func (i *AgentDiscoveryIntegration) Start(ctx context.Context) error {
	if i.running {
		return fmt.Errorf("integration already running")
	}

	// 开始负载报告循环
	i.wg.Add(1)
	go i.loadReportLoop()

	i.running = true
	i.logger.Info("agent discovery integration started")

	return nil
}

// 停止停止整合。
func (i *AgentDiscoveryIntegration) Stop(ctx context.Context) error {
	if !i.running {
		return nil
	}

	i.closeOnce.Do(func() { close(i.done) })
	i.wg.Wait()

	// 启用自动注销注册时取消所有代理
	if i.config.AutoUnregister {
		i.agentsMu.RLock()
		agentIDs := make([]string, 0, len(i.agents))
		for id := range i.agents {
			agentIDs = append(agentIDs, id)
		}
		i.agentsMu.RUnlock()

		for _, id := range agentIDs {
			if err := i.UnregisterAgent(ctx, id); err != nil {
				i.logger.Warn("failed to unregister agent on stop", zap.String("agent_id", id), zap.Error(err))
			}
		}
	}

	i.running = false
	i.logger.Info("agent discovery integration stopped")

	return nil
}

// 物剂在发现系统登记。
func (i *AgentDiscoveryIntegration) RegisterAgent(ctx context.Context, agent AgentCapabilityProvider) error {
	if agent == nil {
		return fmt.Errorf("agent is nil")
	}

	agentID := agent.ID()

	// 检查是否已经注册
	i.agentsMu.RLock()
	_, exists := i.agents[agentID]
	i.agentsMu.RUnlock()

	if exists {
		return fmt.Errorf("agent %s already registered", agentID)
	}

	// 创建代理信息
	info := i.createAgentInfo(agent)

	// 有发现服务的登记
	if err := i.service.RegisterAgent(ctx, info); err != nil {
		return fmt.Errorf("failed to register agent with discovery service: %w", err)
	}

	// 存储代理参考
	i.agentsMu.Lock()
	i.agents[agentID] = agent
	i.agentsMu.Unlock()

	i.logger.Info("agent registered with discovery",
		zap.String("agent_id", agentID),
		zap.Int("capabilities", len(info.Capabilities)),
	)

	return nil
}

// 未注册的代理 未经注册 从发现系统。
func (i *AgentDiscoveryIntegration) UnregisterAgent(ctx context.Context, agentID string) error {
	// 从本地存储中删除
	i.agentsMu.Lock()
	delete(i.agents, agentID)
	i.agentsMu.Unlock()

	// 删除装入记录器
	i.loadReportersMu.Lock()
	delete(i.loadReporters, agentID)
	i.loadReportersMu.Unlock()

	// 发现服务未注册
	if err := i.service.UnregisterAgent(ctx, agentID); err != nil {
		return fmt.Errorf("failed to unregister agent from discovery service: %w", err)
	}

	i.logger.Info("agent unregistered from discovery", zap.String("agent_id", agentID))

	return nil
}

// 更新代理能力更新一个代理在发现系统中的能力.
func (i *AgentDiscoveryIntegration) UpdateAgentCapabilities(ctx context.Context, agentID string) error {
	i.agentsMu.RLock()
	agent, exists := i.agents[agentID]
	i.agentsMu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s not registered", agentID)
	}

	// 创建更新代理信息
	info := i.createAgentInfo(agent)

	// 登记册中的最新情况
	if reg, ok := i.service.Registry().(*CapabilityRegistry); ok {
		if err := reg.UpdateAgent(ctx, info); err != nil {
			return fmt.Errorf("failed to update agent: %w", err)
		}
	}

	i.logger.Debug("agent capabilities updated", zap.String("agent_id", agentID))

	return nil
}

// SetLoadReporter为代理设置了负载报告器功能.
func (i *AgentDiscoveryIntegration) SetLoadReporter(agentID string, reporter func() float64) {
	i.loadReportersMu.Lock()
	defer i.loadReportersMu.Unlock()

	i.loadReporters[agentID] = reporter
}

// RecordExecution 记录一个代理机能力的执行结果.
func (i *AgentDiscoveryIntegration) RecordExecution(ctx context.Context, agentID, capabilityName string, success bool, latency time.Duration) error {
	return i.service.RecordExecution(ctx, agentID, capabilityName, success, latency)
}

// Find AgentForTask 找到任务的最佳代理 。
func (i *AgentDiscoveryIntegration) FindAgentForTask(ctx context.Context, taskDescription string, requiredCapabilities []string) (*AgentInfo, error) {
	return i.service.FindAgent(ctx, taskDescription, requiredCapabilities)
}

// FindAgentsForTask为任务找到多个代理.
func (i *AgentDiscoveryIntegration) FindAgentsForTask(ctx context.Context, req *MatchRequest) ([]*MatchResult, error) {
	return i.service.FindAgents(ctx, req)
}

// Confose AgentsForTask为复杂的任务创建了代理组成.
func (i *AgentDiscoveryIntegration) ComposeAgentsForTask(ctx context.Context, req *CompositionRequest) (*CompositionResult, error) {
	return i.service.ComposeCapabilities(ctx, req)
}

// 创建 AgentInfo 从 Agent Capability Provider 创建 AgentInfo。
func (i *AgentDiscoveryIntegration) createAgentInfo(agent AgentCapabilityProvider) *AgentInfo {
	// 尝试获取代理卡
	card := agent.GetAgentCard()
	if card == nil {
		// 创建基本牌
		card = a2a.NewAgentCard(
			agent.ID(),
			agent.Name(),
			i.config.DefaultEndpoint,
			i.config.DefaultVersion,
		)

		// 添加能力
		for _, cap := range agent.GetCapabilities() {
			card.AddCapability(cap.Name, cap.Description, cap.Type)
		}
	}

	// 创建能力信息
	capabilities := make([]CapabilityInfo, 0)
	for _, cap := range agent.GetCapabilities() {
		capabilities = append(capabilities, CapabilityInfo{
			Capability: cap,
			AgentID:    agent.ID(),
			AgentName:  agent.Name(),
			Status:     CapabilityStatusActive,
			Score:      50.0, // Default score
		})
	}

	return &AgentInfo{
		Card:         card,
		Status:       AgentStatusOnline,
		IsLocal:      true,
		Capabilities: capabilities,
	}
}

// 载入 ReportLoop 定期报告代理载荷。
func (i *AgentDiscoveryIntegration) loadReportLoop() {
	defer i.wg.Done()

	ticker := time.NewTicker(i.config.LoadReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.reportLoads()
		case <-i.done:
			return
		}
	}
}

// 报告LOADs报告所有注册代理的装入量。
func (i *AgentDiscoveryIntegration) reportLoads() {
	i.loadReportersMu.RLock()
	reporters := make(map[string]func() float64)
	for id, reporter := range i.loadReporters {
		reporters[id] = reporter
	}
	i.loadReportersMu.RUnlock()

	ctx := context.Background()
	for agentID, reporter := range reporters {
		load := reporter()
		if err := i.service.Registry().UpdateAgentLoad(ctx, agentID, load); err != nil {
			i.logger.Debug("failed to update agent load",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
		}
	}
}

// GetRegistered Agents 返回所有注册代理.
func (i *AgentDiscoveryIntegration) GetRegisteredAgents() []string {
	i.agentsMu.RLock()
	defer i.agentsMu.RUnlock()

	ids := make([]string, 0, len(i.agents))
	for id := range i.agents {
		ids = append(ids, id)
	}
	return ids
}

// 代理人注册的检查。
func (i *AgentDiscoveryIntegration) IsAgentRegistered(agentID string) bool {
	i.agentsMu.RLock()
	defer i.agentsMu.RUnlock()

	_, exists := i.agents[agentID]
	return exists
}

// Discovery Service返回基础的发现服务.
func (i *AgentDiscoveryIntegration) DiscoveryService() *DiscoveryService {
	return i.service
}

// 全球一体化实例
var (
	globalIntegration     *AgentDiscoveryIntegration
	globalIntegrationOnce sync.Once
	globalIntegrationMu   sync.RWMutex
)

// InitGlobal集成初始化了全球物剂发现集成.
func InitGlobalIntegration(service *DiscoveryService, config *IntegrationConfig, logger *zap.Logger) {
	globalIntegrationOnce.Do(func() {
		globalIntegration = NewAgentDiscoveryIntegration(service, config, logger)
	})
}

// Get Global Introduction返回全球代理发现集成.
func GetGlobalIntegration() *AgentDiscoveryIntegration {
	globalIntegrationMu.RLock()
	defer globalIntegrationMu.RUnlock()
	return globalIntegration
}

// SetGlobal Introduction设定了全球物剂发现集成.
func SetGlobalIntegration(integration *AgentDiscoveryIntegration) {
	globalIntegrationMu.Lock()
	defer globalIntegrationMu.Unlock()
	globalIntegration = integration
}
