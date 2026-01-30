package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"go.uber.org/zap"
)

// AgentCapabilityProvider defines the interface for agents that provide capabilities.
type AgentCapabilityProvider interface {
	// ID returns the agent's unique identifier.
	ID() string

	// Name returns the agent's name.
	Name() string

	// GetCapabilities returns the agent's capabilities.
	GetCapabilities() []a2a.Capability

	// GetAgentCard returns the agent's A2A card.
	GetAgentCard() *a2a.AgentCard
}

// AgentDiscoveryIntegration provides integration between agents and the discovery system.
type AgentDiscoveryIntegration struct {
	service *DiscoveryService
	logger  *zap.Logger

	// Registered agents
	agents   map[string]AgentCapabilityProvider
	agentsMu sync.RWMutex

	// Load reporters
	loadReporters   map[string]func() float64
	loadReportersMu sync.RWMutex

	// Configuration
	config *IntegrationConfig

	// State
	running bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// IntegrationConfig holds configuration for agent discovery integration.
type IntegrationConfig struct {
	// AutoRegister enables automatic registration of agents.
	AutoRegister bool `json:"auto_register"`

	// AutoUnregister enables automatic unregistration when agents stop.
	AutoUnregister bool `json:"auto_unregister"`

	// LoadReportInterval is the interval for reporting agent load.
	LoadReportInterval time.Duration `json:"load_report_interval"`

	// DefaultEndpoint is the default endpoint for local agents.
	DefaultEndpoint string `json:"default_endpoint"`

	// DefaultVersion is the default version for agents.
	DefaultVersion string `json:"default_version"`
}

// DefaultIntegrationConfig returns an IntegrationConfig with sensible defaults.
func DefaultIntegrationConfig() *IntegrationConfig {
	return &IntegrationConfig{
		AutoRegister:       true,
		AutoUnregister:     true,
		LoadReportInterval: 10 * time.Second,
		DefaultEndpoint:    "http://localhost:8080",
		DefaultVersion:     "1.0.0",
	}
}

// NewAgentDiscoveryIntegration creates a new agent discovery integration.
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

// Start starts the integration.
func (i *AgentDiscoveryIntegration) Start(ctx context.Context) error {
	if i.running {
		return fmt.Errorf("integration already running")
	}

	// Start load reporting loop
	i.wg.Add(1)
	go i.loadReportLoop()

	i.running = true
	i.logger.Info("agent discovery integration started")

	return nil
}

// Stop stops the integration.
func (i *AgentDiscoveryIntegration) Stop(ctx context.Context) error {
	if !i.running {
		return nil
	}

	close(i.done)
	i.wg.Wait()

	// Unregister all agents if auto-unregister is enabled
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

// RegisterAgent registers an agent with the discovery system.
func (i *AgentDiscoveryIntegration) RegisterAgent(ctx context.Context, agent AgentCapabilityProvider) error {
	if agent == nil {
		return fmt.Errorf("agent is nil")
	}

	agentID := agent.ID()

	// Check if already registered
	i.agentsMu.RLock()
	_, exists := i.agents[agentID]
	i.agentsMu.RUnlock()

	if exists {
		return fmt.Errorf("agent %s already registered", agentID)
	}

	// Create agent info
	info := i.createAgentInfo(agent)

	// Register with discovery service
	if err := i.service.RegisterAgent(ctx, info); err != nil {
		return fmt.Errorf("failed to register agent with discovery service: %w", err)
	}

	// Store agent reference
	i.agentsMu.Lock()
	i.agents[agentID] = agent
	i.agentsMu.Unlock()

	i.logger.Info("agent registered with discovery",
		zap.String("agent_id", agentID),
		zap.Int("capabilities", len(info.Capabilities)),
	)

	return nil
}

// UnregisterAgent unregisters an agent from the discovery system.
func (i *AgentDiscoveryIntegration) UnregisterAgent(ctx context.Context, agentID string) error {
	// Remove from local storage
	i.agentsMu.Lock()
	delete(i.agents, agentID)
	i.agentsMu.Unlock()

	// Remove load reporter
	i.loadReportersMu.Lock()
	delete(i.loadReporters, agentID)
	i.loadReportersMu.Unlock()

	// Unregister from discovery service
	if err := i.service.UnregisterAgent(ctx, agentID); err != nil {
		return fmt.Errorf("failed to unregister agent from discovery service: %w", err)
	}

	i.logger.Info("agent unregistered from discovery", zap.String("agent_id", agentID))

	return nil
}

// UpdateAgentCapabilities updates an agent's capabilities in the discovery system.
func (i *AgentDiscoveryIntegration) UpdateAgentCapabilities(ctx context.Context, agentID string) error {
	i.agentsMu.RLock()
	agent, exists := i.agents[agentID]
	i.agentsMu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s not registered", agentID)
	}

	// Create updated agent info
	info := i.createAgentInfo(agent)

	// Update in registry
	if reg, ok := i.service.Registry().(*CapabilityRegistry); ok {
		if err := reg.UpdateAgent(ctx, info); err != nil {
			return fmt.Errorf("failed to update agent: %w", err)
		}
	}

	i.logger.Debug("agent capabilities updated", zap.String("agent_id", agentID))

	return nil
}

// SetLoadReporter sets a load reporter function for an agent.
func (i *AgentDiscoveryIntegration) SetLoadReporter(agentID string, reporter func() float64) {
	i.loadReportersMu.Lock()
	defer i.loadReportersMu.Unlock()

	i.loadReporters[agentID] = reporter
}

// RecordExecution records an execution result for an agent's capability.
func (i *AgentDiscoveryIntegration) RecordExecution(ctx context.Context, agentID, capabilityName string, success bool, latency time.Duration) error {
	return i.service.RecordExecution(ctx, agentID, capabilityName, success, latency)
}

// FindAgentForTask finds the best agent for a task.
func (i *AgentDiscoveryIntegration) FindAgentForTask(ctx context.Context, taskDescription string, requiredCapabilities []string) (*AgentInfo, error) {
	return i.service.FindAgent(ctx, taskDescription, requiredCapabilities)
}

// FindAgentsForTask finds multiple agents for a task.
func (i *AgentDiscoveryIntegration) FindAgentsForTask(ctx context.Context, req *MatchRequest) ([]*MatchResult, error) {
	return i.service.FindAgents(ctx, req)
}

// ComposeAgentsForTask creates a composition of agents for a complex task.
func (i *AgentDiscoveryIntegration) ComposeAgentsForTask(ctx context.Context, req *CompositionRequest) (*CompositionResult, error) {
	return i.service.ComposeCapabilities(ctx, req)
}

// createAgentInfo creates an AgentInfo from an AgentCapabilityProvider.
func (i *AgentDiscoveryIntegration) createAgentInfo(agent AgentCapabilityProvider) *AgentInfo {
	// Try to get agent card
	card := agent.GetAgentCard()
	if card == nil {
		// Create a basic card
		card = a2a.NewAgentCard(
			agent.ID(),
			agent.Name(),
			i.config.DefaultEndpoint,
			i.config.DefaultVersion,
		)

		// Add capabilities
		for _, cap := range agent.GetCapabilities() {
			card.AddCapability(cap.Name, cap.Description, cap.Type)
		}
	}

	// Create capability info
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

// loadReportLoop periodically reports agent loads.
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

// reportLoads reports loads for all registered agents.
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

// GetRegisteredAgents returns all registered agents.
func (i *AgentDiscoveryIntegration) GetRegisteredAgents() []string {
	i.agentsMu.RLock()
	defer i.agentsMu.RUnlock()

	ids := make([]string, 0, len(i.agents))
	for id := range i.agents {
		ids = append(ids, id)
	}
	return ids
}

// IsAgentRegistered checks if an agent is registered.
func (i *AgentDiscoveryIntegration) IsAgentRegistered(agentID string) bool {
	i.agentsMu.RLock()
	defer i.agentsMu.RUnlock()

	_, exists := i.agents[agentID]
	return exists
}

// DiscoveryService returns the underlying discovery service.
func (i *AgentDiscoveryIntegration) DiscoveryService() *DiscoveryService {
	return i.service
}

// Global integration instance
var (
	globalIntegration     *AgentDiscoveryIntegration
	globalIntegrationOnce sync.Once
	globalIntegrationMu   sync.RWMutex
)

// InitGlobalIntegration initializes the global agent discovery integration.
func InitGlobalIntegration(service *DiscoveryService, config *IntegrationConfig, logger *zap.Logger) {
	globalIntegrationOnce.Do(func() {
		globalIntegration = NewAgentDiscoveryIntegration(service, config, logger)
	})
}

// GetGlobalIntegration returns the global agent discovery integration.
func GetGlobalIntegration() *AgentDiscoveryIntegration {
	globalIntegrationMu.RLock()
	defer globalIntegrationMu.RUnlock()
	return globalIntegration
}

// SetGlobalIntegration sets the global agent discovery integration.
func SetGlobalIntegration(integration *AgentDiscoveryIntegration) {
	globalIntegrationMu.Lock()
	defer globalIntegrationMu.Unlock()
	globalIntegration = integration
}
