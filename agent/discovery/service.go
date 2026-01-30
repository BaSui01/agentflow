package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"go.uber.org/zap"
)

// DiscoveryService provides a unified interface for agent capability discovery.
// It combines the registry, matcher, composer, and protocol into a single service.
type DiscoveryService struct {
	registry Registry
	matcher  Matcher
	composer Composer
	protocol Protocol

	config *ServiceConfig
	logger *zap.Logger

	// Local agent info for auto-registration
	localAgent *AgentInfo
	localMu    sync.RWMutex

	// State
	running bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// ServiceConfig holds configuration for the discovery service.
type ServiceConfig struct {
	// Registry configuration
	Registry *RegistryConfig `json:"registry"`

	// Matcher configuration
	Matcher *MatcherConfig `json:"matcher"`

	// Composer configuration
	Composer *ComposerConfig `json:"composer"`

	// Protocol configuration
	Protocol *ProtocolConfig `json:"protocol"`

	// EnableAutoRegistration enables automatic registration of local agents.
	EnableAutoRegistration bool `json:"enable_auto_registration"`

	// HeartbeatInterval is the interval for sending heartbeats.
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`

	// EnableMetrics enables metrics collection.
	EnableMetrics bool `json:"enable_metrics"`
}

// DefaultServiceConfig returns a ServiceConfig with sensible defaults.
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

// NewDiscoveryService creates a new discovery service.
func NewDiscoveryService(config *ServiceConfig, logger *zap.Logger) *DiscoveryService {
	if config == nil {
		config = DefaultServiceConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create registry
	registry := NewCapabilityRegistry(config.Registry, logger)

	// Create matcher
	matcher := NewCapabilityMatcher(registry, config.Matcher, logger)

	// Create composer
	composer := NewCapabilityComposer(registry, matcher, config.Composer, logger)

	// Create protocol
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

// Start starts the discovery service.
func (s *DiscoveryService) Start(ctx context.Context) error {
	if s.running {
		return fmt.Errorf("service already running")
	}

	// Start registry
	if reg, ok := s.registry.(*CapabilityRegistry); ok {
		if err := reg.Start(ctx); err != nil {
			return fmt.Errorf("failed to start registry: %w", err)
		}
	}

	// Start protocol
	if err := s.protocol.Start(ctx); err != nil {
		return fmt.Errorf("failed to start protocol: %w", err)
	}

	// Start heartbeat if auto-registration is enabled
	if s.config.EnableAutoRegistration {
		s.wg.Add(1)
		go s.heartbeatLoop()
	}

	s.running = true
	s.logger.Info("discovery service started")

	return nil
}

// Stop stops the discovery service.
func (s *DiscoveryService) Stop(ctx context.Context) error {
	if !s.running {
		return nil
	}

	close(s.done)
	s.wg.Wait()

	// Stop protocol
	if err := s.protocol.Stop(ctx); err != nil {
		s.logger.Error("failed to stop protocol", zap.Error(err))
	}

	// Stop registry
	if err := s.registry.Close(); err != nil {
		s.logger.Error("failed to close registry", zap.Error(err))
	}

	s.running = false
	s.logger.Info("discovery service stopped")

	return nil
}

// RegisterAgent registers an agent with the discovery service.
func (s *DiscoveryService) RegisterAgent(ctx context.Context, info *AgentInfo) error {
	// Register with registry
	if err := s.registry.RegisterAgent(ctx, info); err != nil {
		return err
	}

	// Announce via protocol
	if err := s.protocol.Announce(ctx, info); err != nil {
		s.logger.Warn("failed to announce agent", zap.Error(err))
	}

	return nil
}

// UnregisterAgent unregisters an agent from the discovery service.
func (s *DiscoveryService) UnregisterAgent(ctx context.Context, agentID string) error {
	return s.registry.UnregisterAgent(ctx, agentID)
}

// RegisterLocalAgent registers the local agent for auto-heartbeat.
func (s *DiscoveryService) RegisterLocalAgent(info *AgentInfo) error {
	s.localMu.Lock()
	defer s.localMu.Unlock()

	info.IsLocal = true
	s.localAgent = info

	// Register immediately
	ctx := context.Background()
	return s.RegisterAgent(ctx, info)
}

// UpdateLocalAgentLoad updates the load of the local agent.
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

// FindAgent finds the best agent for a task.
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

// FindAgents finds multiple agents matching the criteria.
func (s *DiscoveryService) FindAgents(ctx context.Context, req *MatchRequest) ([]*MatchResult, error) {
	return s.matcher.Match(ctx, req)
}

// ComposeCapabilities creates a composition of capabilities from multiple agents.
func (s *DiscoveryService) ComposeCapabilities(ctx context.Context, req *CompositionRequest) (*CompositionResult, error) {
	return s.composer.Compose(ctx, req)
}

// DiscoverAgents discovers agents on the network.
func (s *DiscoveryService) DiscoverAgents(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	return s.protocol.Discover(ctx, filter)
}

// GetAgent retrieves an agent by ID.
func (s *DiscoveryService) GetAgent(ctx context.Context, agentID string) (*AgentInfo, error) {
	return s.registry.GetAgent(ctx, agentID)
}

// ListAgents lists all registered agents.
func (s *DiscoveryService) ListAgents(ctx context.Context) ([]*AgentInfo, error) {
	return s.registry.ListAgents(ctx)
}

// GetCapability retrieves a capability by agent ID and name.
func (s *DiscoveryService) GetCapability(ctx context.Context, agentID, capabilityName string) (*CapabilityInfo, error) {
	return s.registry.GetCapability(ctx, agentID, capabilityName)
}

// FindCapabilities finds capabilities by name across all agents.
func (s *DiscoveryService) FindCapabilities(ctx context.Context, capabilityName string) ([]CapabilityInfo, error) {
	return s.registry.FindCapabilities(ctx, capabilityName)
}

// RecordExecution records an execution result for a capability.
func (s *DiscoveryService) RecordExecution(ctx context.Context, agentID, capabilityName string, success bool, latency time.Duration) error {
	return s.registry.RecordExecution(ctx, agentID, capabilityName, success, latency)
}

// Subscribe subscribes to discovery events.
func (s *DiscoveryService) Subscribe(handler DiscoveryEventHandler) string {
	return s.registry.Subscribe(handler)
}

// Unsubscribe unsubscribes from discovery events.
func (s *DiscoveryService) Unsubscribe(subscriptionID string) {
	s.registry.Unsubscribe(subscriptionID)
}

// SubscribeToAnnouncements subscribes to agent announcements.
func (s *DiscoveryService) SubscribeToAnnouncements(handler func(*AgentInfo)) string {
	return s.protocol.Subscribe(handler)
}

// UnsubscribeFromAnnouncements unsubscribes from agent announcements.
func (s *DiscoveryService) UnsubscribeFromAnnouncements(subscriptionID string) {
	s.protocol.Unsubscribe(subscriptionID)
}

// RegisterDependency registers a dependency between capabilities.
func (s *DiscoveryService) RegisterDependency(capability string, dependencies []string) {
	if comp, ok := s.composer.(*CapabilityComposer); ok {
		comp.RegisterDependency(capability, dependencies)
	}
}

// RegisterExclusiveGroup registers a group of mutually exclusive capabilities.
func (s *DiscoveryService) RegisterExclusiveGroup(capabilities []string) {
	if comp, ok := s.composer.(*CapabilityComposer); ok {
		comp.RegisterExclusiveGroup(capabilities)
	}
}

// heartbeatLoop sends periodic heartbeats for the local agent.
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

// sendHeartbeat sends a heartbeat for the local agent.
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

// Registry returns the underlying registry.
func (s *DiscoveryService) Registry() Registry {
	return s.registry
}

// Matcher returns the underlying matcher.
func (s *DiscoveryService) Matcher() Matcher {
	return s.matcher
}

// Composer returns the underlying composer.
func (s *DiscoveryService) Composer() Composer {
	return s.composer
}

// Protocol returns the underlying protocol.
func (s *DiscoveryService) Protocol() Protocol {
	return s.protocol
}

// AgentInfoFromCard creates an AgentInfo from an A2A AgentCard.
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

	// Convert capabilities
	for _, cap := range card.Capabilities {
		info.Capabilities = append(info.Capabilities, CapabilityInfo{
			Capability: cap,
			Status:     CapabilityStatusActive,
			Score:      50.0, // Default score
		})
	}

	return info
}

// CreateAgentCard creates an A2A AgentCard from agent configuration.
func CreateAgentCard(name, description, url, version string, capabilities []a2a.Capability) *a2a.AgentCard {
	card := a2a.NewAgentCard(name, description, url, version)
	for _, cap := range capabilities {
		card.AddCapability(cap.Name, cap.Description, cap.Type)
	}
	return card
}

// Global discovery service instance
var (
	globalService     *DiscoveryService
	globalServiceOnce sync.Once
	globalServiceMu   sync.RWMutex
)

// InitGlobalDiscoveryService initializes the global discovery service.
func InitGlobalDiscoveryService(config *ServiceConfig, logger *zap.Logger) {
	globalServiceOnce.Do(func() {
		globalService = NewDiscoveryService(config, logger)
	})
}

// GetGlobalDiscoveryService returns the global discovery service.
func GetGlobalDiscoveryService() *DiscoveryService {
	globalServiceMu.RLock()
	defer globalServiceMu.RUnlock()
	return globalService
}

// SetGlobalDiscoveryService sets the global discovery service.
func SetGlobalDiscoveryService(service *DiscoveryService) {
	globalServiceMu.Lock()
	defer globalServiceMu.Unlock()
	globalService = service
}
