package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"go.uber.org/zap"
)

// 发现服务为代理能力发现提供了一个统一的接口.
// 它将注册,配对,作曲,协议合并为单一服务.
type DiscoveryService struct {
	registry Registry
	matcher  Matcher
	composer Composer
	protocol Protocol

	config *ServiceConfig
	logger *zap.Logger

	// 用于自动登记的当地代理信息
	localAgent *AgentInfo
	localMu    sync.RWMutex

	// 状态
	running bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// ServiceConfig持有发现服务配置.
type ServiceConfig struct {
	// 书记官处配置
	Registry *RegistryConfig `json:"registry"`

	// 匹配器配置
	Matcher *MatcherConfig `json:"matcher"`

	// 作曲家配置
	Composer *ComposerConfig `json:"composer"`

	// 协议配置
	Protocol *ProtocolConfig `json:"protocol"`

	// 启用自动注册可自动注册本地代理。
	EnableAutoRegistration bool `json:"enable_auto_registration"`

	// Heartbeat Interval是发送心跳的间隔.
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`

	// 启用度量衡启用了度量衡收集 。
	EnableMetrics bool `json:"enable_metrics"`
}

// 默认ServiceConfig 返回带有合理默认的ServiceConfig 。
func DefaultServiceConfig() *ServiceConfig {
	return &ServiceConfig{
		Registry:               DefaultRegistryConfig(),
		Matcher:                DefaultMatcherConfig(),
		Composer:               DefaultComposerConfig(),
		Protocol:               DefaultProtocolConfig(),
		EnableAutoRegistration: true,
		HeartbeatInterval:      15 * time.Second,
		EnableMetrics:          true,
	}
}

// 新发现服务创建了新的发现服务.
func NewDiscoveryService(config *ServiceConfig, logger *zap.Logger) *DiscoveryService {
	if config == nil {
		config = DefaultServiceConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	// 创建注册
	registry := NewCapabilityRegistry(config.Registry, logger)

	// 创建匹配器
	matcher := NewCapabilityMatcher(registry, config.Matcher, logger)

	// 创建作曲
	composer := NewCapabilityComposer(registry, matcher, config.Composer, logger)

	// 创建协议
	protocol := NewDiscoveryProtocol(config.Protocol, registry, logger)

	return &DiscoveryService{
		registry: registry,
		matcher:  matcher,
		composer: composer,
		protocol: protocol,
		config:   config,
		logger:   logger.With(zap.String("component", "discovery_service")),
		done:     make(chan struct{}),
	}
}

// 启动发现服务.
func (s *DiscoveryService) Start(ctx context.Context) error {
	if s.running {
		return fmt.Errorf("service already running")
	}

	// 开始注册
	if reg, ok := s.registry.(*CapabilityRegistry); ok {
		if err := reg.Start(ctx); err != nil {
			return fmt.Errorf("failed to start registry: %w", err)
		}
	}

	// 开始协议
	if err := s.protocol.Start(ctx); err != nil {
		return fmt.Errorf("failed to start protocol: %w", err)
	}

	// 如果启用自动注册, 启动心跳
	if s.config.EnableAutoRegistration {
		s.wg.Add(1)
		go s.heartbeatLoop()
	}

	s.running = true
	s.logger.Info("discovery service started")

	return nil
}

// 停止发现服务。
func (s *DiscoveryService) Stop(ctx context.Context) error {
	if !s.running {
		return nil
	}

	close(s.done)
	s.wg.Wait()

	// 停止协议
	if err := s.protocol.Stop(ctx); err != nil {
		s.logger.Error("failed to stop protocol", zap.Error(err))
	}

	// 停止注册
	if err := s.registry.Close(); err != nil {
		s.logger.Error("failed to close registry", zap.Error(err))
	}

	s.running = false
	s.logger.Info("discovery service stopped")

	return nil
}

// 代理人在发现处登记。
func (s *DiscoveryService) RegisterAgent(ctx context.Context, info *AgentInfo) error {
	// 向登记册登记
	if err := s.registry.RegisterAgent(ctx, info); err != nil {
		return err
	}

	// 通过协议宣布
	if err := s.protocol.Announce(ctx, info); err != nil {
		s.logger.Warn("failed to announce agent", zap.Error(err))
	}

	return nil
}

// 未注册代理 未经注册的代理 从发现服务。
func (s *DiscoveryService) UnregisterAgent(ctx context.Context, agentID string) error {
	return s.registry.UnregisterAgent(ctx, agentID)
}

// 注册本地代理 注册本地代理自动心跳。
func (s *DiscoveryService) RegisterLocalAgent(info *AgentInfo) error {
	s.localMu.Lock()
	defer s.localMu.Unlock()

	info.IsLocal = true
	s.localAgent = info

	// 立即登记
	ctx := context.Background()
	return s.RegisterAgent(ctx, info)
}

// 更新本地代理Load 更新本地代理的负载 。
func (s *DiscoveryService) UpdateLocalAgentLoad(load float64) error {
	s.localMu.RLock()
	agent := s.localAgent
	s.localMu.RUnlock()

	if agent == nil {
		return fmt.Errorf("no local agent registered")
	}

	ctx := context.Background()
	return s.registry.UpdateAgentLoad(ctx, agent.Card.Name, load)
}

// FindAgent 找到任务的最佳代理 。
func (s *DiscoveryService) FindAgent(ctx context.Context, taskDescription string, requiredCapabilities []string) (*AgentInfo, error) {
	result, err := s.matcher.MatchOne(ctx, &MatchRequest{
		TaskDescription:      taskDescription,
		RequiredCapabilities: requiredCapabilities,
		Strategy:             MatchStrategyBestMatch,
	})
	if err != nil {
		return nil, err
	}
	return result.Agent, nil
}

// FindAgents发现多个符合标准的代理.
func (s *DiscoveryService) FindAgents(ctx context.Context, req *MatchRequest) ([]*MatchResult, error) {
	return s.matcher.Match(ctx, req)
}

// 由多种物剂构成的能力组成。
func (s *DiscoveryService) ComposeCapabilities(ctx context.Context, req *CompositionRequest) (*CompositionResult, error) {
	return s.composer.Compose(ctx, req)
}

// 发现特工在网络上发现了特工.
func (s *DiscoveryService) DiscoverAgents(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	return s.protocol.Discover(ctx, filter)
}

// Get Agent通过身份识别找到一个特工.
func (s *DiscoveryService) GetAgent(ctx context.Context, agentID string) (*AgentInfo, error) {
	return s.registry.GetAgent(ctx, agentID)
}

// ListAgents列出所有注册代理.
func (s *DiscoveryService) ListAgents(ctx context.Context) ([]*AgentInfo, error) {
	return s.registry.ListAgents(ctx)
}

// Get Capability通过代理身份和姓名检索能力.
func (s *DiscoveryService) GetCapability(ctx context.Context, agentID, capabilityName string) (*CapabilityInfo, error) {
	return s.registry.GetCapability(ctx, agentID, capabilityName)
}

// Find Capabilitys 在所有特工中按名称找到能力.
func (s *DiscoveryService) FindCapabilities(ctx context.Context, capabilityName string) ([]CapabilityInfo, error) {
	return s.registry.FindCapabilities(ctx, capabilityName)
}

// 记录 Execution 记录一个执行结果 一个能力。
func (s *DiscoveryService) RecordExecution(ctx context.Context, agentID, capabilityName string, success bool, latency time.Duration) error {
	return s.registry.RecordExecution(ctx, agentID, capabilityName, success, latency)
}

// 订阅了发现事件。
func (s *DiscoveryService) Subscribe(handler DiscoveryEventHandler) string {
	return s.registry.Subscribe(handler)
}

// 不订阅来自发现事件的用户 。
func (s *DiscoveryService) Unsubscribe(subscriptionID string) {
	s.registry.Unsubscribe(subscriptionID)
}

// 订阅通知订阅代理通知 。
func (s *DiscoveryService) SubscribeToAnnouncements(handler func(*AgentInfo)) string {
	return s.protocol.Subscribe(handler)
}

// 从代理通知中取消订阅 。
func (s *DiscoveryService) UnsubscribeFromAnnouncements(subscriptionID string) {
	s.protocol.Unsubscribe(subscriptionID)
}

// 登记册的依赖性对各种能力之间的依赖性进行登记。
func (s *DiscoveryService) RegisterDependency(capability string, dependencies []string) {
	if comp, ok := s.composer.(*CapabilityComposer); ok {
		comp.RegisterDependency(capability, dependencies)
	}
}

// 登记小组登记一组相互排斥的能力。
func (s *DiscoveryService) RegisterExclusiveGroup(capabilities []string) {
	if comp, ok := s.composer.(*CapabilityComposer); ok {
		comp.RegisterExclusiveGroup(capabilities)
	}
}

// 心跳Loop为本地代理发送定期心跳.
func (s *DiscoveryService) heartbeatLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sendHeartbeat()
		case <-s.done:
			return
		}
	}
}

// 让Heartbeat为本地特工发送心跳
func (s *DiscoveryService) sendHeartbeat() {
	s.localMu.RLock()
	agent := s.localAgent
	s.localMu.RUnlock()

	if agent == nil {
		return
	}

	ctx := context.Background()
	if reg, ok := s.registry.(*CapabilityRegistry); ok {
		if err := reg.Heartbeat(ctx, agent.Card.Name); err != nil {
			s.logger.Warn("failed to send heartbeat", zap.Error(err))
		}
	}
}

// 登记册返回基本登记册。
func (s *DiscoveryService) Registry() Registry {
	return s.registry
}

// 匹配者返回基本匹配者 。
func (s *DiscoveryService) Matcher() Matcher {
	return s.matcher
}

// 作曲家返回了基础作曲家.
func (s *DiscoveryService) Composer() Composer {
	return s.composer
}

// 协议返回基本协议。
func (s *DiscoveryService) Protocol() Protocol {
	return s.protocol
}

// Agent InfoFromCard从A2A AgentCard中创建了AgentInfo.
func AgentInfoFromCard(card *a2a.AgentCard, isLocal bool) *AgentInfo {
	if card == nil {
		return nil
	}

	info := &AgentInfo{
		Card:     card,
		Status:   AgentStatusOnline,
		IsLocal:  isLocal,
		Endpoint: card.URL,
		Metadata: card.Metadata,
	}

	// 转换能力
	for _, cap := range card.Capabilities {
		info.Capabilities = append(info.Capabilities, CapabilityInfo{
			Capability: cap,
			Status:     CapabilityStatusActive,
			Score:      50.0, // Default score
		})
	}

	return info
}

// CreateAgentCard通过代理配置创建了A2A AgentCard.
func CreateAgentCard(name, description, url, version string, capabilities []a2a.Capability) *a2a.AgentCard {
	card := a2a.NewAgentCard(name, description, url, version)
	for _, cap := range capabilities {
		card.AddCapability(cap.Name, cap.Description, cap.Type)
	}
	return card
}

// 全球发现服务实例
var (
	globalService     *DiscoveryService
	globalServiceOnce sync.Once
	globalServiceMu   sync.RWMutex
)

// InitGlobal Discovery Service初始化了全球发现服务.
func InitGlobalDiscoveryService(config *ServiceConfig, logger *zap.Logger) {
	globalServiceOnce.Do(func() {
		globalService = NewDiscoveryService(config, logger)
	})
}

// 获取全球 Discovery Service返回了全球发现服务.
func GetGlobalDiscoveryService() *DiscoveryService {
	globalServiceMu.RLock()
	defer globalServiceMu.RUnlock()
	return globalService
}

// 设置全局 发现服务设置全球发现服务.
func SetGlobalDiscoveryService(service *DiscoveryService) {
	globalServiceMu.Lock()
	defer globalServiceMu.Unlock()
	globalService = service
}
