package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CapabilityRegistry is the default implementation of the Registry interface.
// It provides in-memory storage for agent and capability information with
// support for health checking and event notifications.
type CapabilityRegistry struct {
	mu sync.RWMutex

	// agents stores registered agents by ID.
	agents map[string]*AgentInfo

	// capabilityIndex indexes capabilities by name for fast lookup.
	capabilityIndex map[string]map[string]*CapabilityInfo // capability name -> agent ID -> capability

	// eventHandlers stores event handlers.
	eventHandlers map[string]DiscoveryEventHandler
	handlerMu     sync.RWMutex

	// healthChecker performs periodic health checks.
	healthChecker *HealthChecker

	// config holds registry configuration.
	config *RegistryConfig

	// logger is the logger instance.
	logger *zap.Logger

	// done signals shutdown.
	done chan struct{}
}

// RegistryConfig holds configuration for the capability registry.
type RegistryConfig struct {
	// HealthCheckInterval is the interval between health checks.
	HealthCheckInterval time.Duration `json:"health_check_interval"`

	// HealthCheckTimeout is the timeout for health checks.
	HealthCheckTimeout time.Duration `json:"health_check_timeout"`

	// UnhealthyThreshold is the number of failed health checks before marking unhealthy.
	UnhealthyThreshold int `json:"unhealthy_threshold"`

	// RemoveUnhealthyAfter is the duration after which unhealthy agents are removed.
	RemoveUnhealthyAfter time.Duration `json:"remove_unhealthy_after"`

	// EnableHealthCheck enables periodic health checking.
	EnableHealthCheck bool `json:"enable_health_check"`

	// DefaultCapabilityScore is the default score for new capabilities.
	DefaultCapabilityScore float64 `json:"default_capability_score"`
}

// DefaultRegistryConfig returns a RegistryConfig with sensible defaults.
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

// NewCapabilityRegistry creates a new capability registry.
func NewCapabilityRegistry(config *RegistryConfig, logger *zap.Logger) *CapabilityRegistry {
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

	// Initialize health checker if enabled
	if config.EnableHealthCheck {
		r.healthChecker = NewHealthChecker(&HealthCheckerConfig{
			Interval:           config.HealthCheckInterval,
			Timeout:            config.HealthCheckTimeout,
			UnhealthyThreshold: config.UnhealthyThreshold,
		}, r, logger)
	}

	return r
}

// Start starts the registry background processes.
func (r *CapabilityRegistry) Start(ctx context.Context) error {
	if r.healthChecker != nil {
		if err := r.healthChecker.Start(ctx); err != nil {
			return fmt.Errorf("failed to start health checker: %w", err)
		}
	}

	r.logger.Info("capability registry started")
	return nil
}

// RegisterAgent registers an agent with its capabilities.
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

	// Check if agent already exists
	if _, exists := r.agents[agentID]; exists {
		return fmt.Errorf("agent %s already registered", agentID)
	}

	// Set defaults
	now := time.Now()
	info.RegisteredAt = now
	info.LastHeartbeat = now
	if info.Status == "" {
		info.Status = AgentStatusOnline
	}

	// Initialize capabilities
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

		// Add to capability index
		r.indexCapability(cap)
	}

	// Store agent
	r.agents[agentID] = info

	r.logger.Info("agent registered",
		zap.String("agent_id", agentID),
		zap.Int("capabilities", len(info.Capabilities)),
	)

	// Emit event
	r.emitEvent(&DiscoveryEvent{
		Type:      DiscoveryEventAgentRegistered,
		AgentID:   agentID,
		Timestamp: now,
	})

	return nil
}

// UnregisterAgent unregisters an agent.
func (r *CapabilityRegistry) UnregisterAgent(ctx context.Context, agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// Remove capabilities from index
	for _, cap := range info.Capabilities {
		r.removeCapabilityFromIndex(cap.Capability.Name, agentID)
	}

	// Remove agent
	delete(r.agents, agentID)

	r.logger.Info("agent unregistered", zap.String("agent_id", agentID))

	// Emit event
	r.emitEvent(&DiscoveryEvent{
		Type:      DiscoveryEventAgentUnregistered,
		AgentID:   agentID,
		Timestamp: time.Now(),
	})

	return nil
}

// UpdateAgent updates an agent's information.
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

	// Update fields
	now := time.Now()
	info.RegisteredAt = existing.RegisteredAt
	info.LastHeartbeat = now

	// Update capability index
	// First, remove old capabilities
	for _, cap := range existing.Capabilities {
		r.removeCapabilityFromIndex(cap.Capability.Name, agentID)
	}

	// Then, add new capabilities
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

	// Store updated agent
	r.agents[agentID] = info

	r.logger.Info("agent updated", zap.String("agent_id", agentID))

	// Emit event
	r.emitEvent(&DiscoveryEvent{
		Type:      DiscoveryEventAgentUpdated,
		AgentID:   agentID,
		Timestamp: now,
	})

	return nil
}

// GetAgent retrieves an agent by ID.
func (r *CapabilityRegistry) GetAgent(ctx context.Context, agentID string) (*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// Return a copy
	return r.copyAgentInfo(info), nil
}

// ListAgents lists all registered agents.
func (r *CapabilityRegistry) ListAgents(ctx context.Context) ([]*AgentInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		agents = append(agents, r.copyAgentInfo(info))
	}

	return agents, nil
}

// RegisterCapability registers a capability for an agent.
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

	// Check if capability already exists
	for _, existing := range info.Capabilities {
		if existing.Capability.Name == cap.Capability.Name {
			return fmt.Errorf("capability %s already registered for agent %s", cap.Capability.Name, agentID)
		}
	}

	// Set defaults
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

	// Add to agent
	info.Capabilities = append(info.Capabilities, *cap)

	// Add to index
	r.indexCapability(cap)

	r.logger.Info("capability registered",
		zap.String("agent_id", agentID),
		zap.String("capability", cap.Capability.Name),
	)

	// Emit event
	r.emitEvent(&DiscoveryEvent{
		Type:       DiscoveryEventCapabilityAdded,
		AgentID:    agentID,
		Capability: cap.Capability.Name,
		Timestamp:  now,
	})

	return nil
}

// UnregisterCapability unregisters a capability.
func (r *CapabilityRegistry) UnregisterCapability(ctx context.Context, agentID string, capabilityName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// Find and remove capability
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

	// Remove from index
	r.removeCapabilityFromIndex(capabilityName, agentID)

	r.logger.Info("capability unregistered",
		zap.String("agent_id", agentID),
		zap.String("capability", capabilityName),
	)

	// Emit event
	r.emitEvent(&DiscoveryEvent{
		Type:       DiscoveryEventCapabilityRemoved,
		AgentID:    agentID,
		Capability: capabilityName,
		Timestamp:  time.Now(),
	})

	return nil
}

// UpdateCapability updates a capability.
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

	// Find and update capability
	found := false
	for i, existing := range info.Capabilities {
		if existing.Capability.Name == cap.Capability.Name {
			// Preserve registration time
			cap.RegisteredAt = existing.RegisteredAt
			cap.AgentID = agentID
			cap.AgentName = info.Card.Name
			cap.LastUpdatedAt = time.Now()

			info.Capabilities[i] = *cap
			found = true

			// Update index
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

	// Emit event
	r.emitEvent(&DiscoveryEvent{
		Type:       DiscoveryEventCapabilityUpdated,
		AgentID:    agentID,
		Capability: cap.Capability.Name,
		Timestamp:  time.Now(),
	})

	return nil
}

// GetCapability retrieves a capability by agent ID and name.
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

// ListCapabilities lists all capabilities for an agent.
func (r *CapabilityRegistry) ListCapabilities(ctx context.Context, agentID string) ([]CapabilityInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("agent %s not found", agentID)
	}

	// Return a copy
	caps := make([]CapabilityInfo, len(info.Capabilities))
	copy(caps, info.Capabilities)
	return caps, nil
}

// FindCapabilities finds capabilities by name across all agents.
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

// UpdateAgentStatus updates an agent's status.
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

// UpdateAgentLoad updates an agent's load.
func (r *CapabilityRegistry) UpdateAgentLoad(ctx context.Context, agentID string, load float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	info.Load = load
	info.LastHeartbeat = time.Now()

	// Update capability loads
	for i := range info.Capabilities {
		info.Capabilities[i].Load = load
	}

	return nil
}

// RecordExecution records an execution result for a capability.
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

			// Update average latency
			totalCount := info.Capabilities[i].SuccessCount + info.Capabilities[i].FailureCount
			if totalCount == 1 {
				info.Capabilities[i].AvgLatency = latency
			} else {
				// Exponential moving average
				alpha := 0.2
				info.Capabilities[i].AvgLatency = time.Duration(
					float64(info.Capabilities[i].AvgLatency)*(1-alpha) + float64(latency)*alpha,
				)
			}

			// Update score based on success rate
			successRate := float64(info.Capabilities[i].SuccessCount) / float64(totalCount)
			info.Capabilities[i].Score = successRate * 100

			// Update index
			r.indexCapability(&info.Capabilities[i])

			return nil
		}
	}

	return fmt.Errorf("capability %s not found for agent %s", capabilityName, agentID)
}

// Subscribe subscribes to discovery events.
func (r *CapabilityRegistry) Subscribe(handler DiscoveryEventHandler) string {
	r.handlerMu.Lock()
	defer r.handlerMu.Unlock()

	id := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	r.eventHandlers[id] = handler
	return id
}

// Unsubscribe unsubscribes from discovery events.
func (r *CapabilityRegistry) Unsubscribe(subscriptionID string) {
	r.handlerMu.Lock()
	defer r.handlerMu.Unlock()

	delete(r.eventHandlers, subscriptionID)
}

// Close closes the registry.
func (r *CapabilityRegistry) Close() error {
	close(r.done)

	if r.healthChecker != nil {
		if err := r.healthChecker.Stop(context.Background()); err != nil {
			r.logger.Error("failed to stop health checker", zap.Error(err))
		}
	}

	r.logger.Info("capability registry closed")
	return nil
}

// indexCapability adds a capability to the index.
func (r *CapabilityRegistry) indexCapability(cap *CapabilityInfo) {
	capName := cap.Capability.Name
	if r.capabilityIndex[capName] == nil {
		r.capabilityIndex[capName] = make(map[string]*CapabilityInfo)
	}
	r.capabilityIndex[capName][cap.AgentID] = cap
}

// removeCapabilityFromIndex removes a capability from the index.
func (r *CapabilityRegistry) removeCapabilityFromIndex(capabilityName, agentID string) {
	if agentCaps, exists := r.capabilityIndex[capabilityName]; exists {
		delete(agentCaps, agentID)
		if len(agentCaps) == 0 {
			delete(r.capabilityIndex, capabilityName)
		}
	}
}

// emitEvent emits a discovery event to all subscribers.
func (r *CapabilityRegistry) emitEvent(event *DiscoveryEvent) {
	r.handlerMu.RLock()
	handlers := make([]DiscoveryEventHandler, 0, len(r.eventHandlers))
	for _, h := range r.eventHandlers {
		handlers = append(handlers, h)
	}
	r.handlerMu.RUnlock()

	for _, handler := range handlers {
		go handler(event)
	}
}

// copyAgentInfo creates a deep copy of AgentInfo.
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

// GetAgentsByCapability returns all agents that have a specific capability.
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

// GetActiveAgents returns all agents with online status.
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

// Heartbeat updates the heartbeat timestamp for an agent.
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

// HealthChecker performs periodic health checks on registered agents.
type HealthChecker struct {
	config   *HealthCheckerConfig
	registry *CapabilityRegistry
	logger   *zap.Logger

	// failureCounts tracks consecutive failures per agent.
	failureCounts map[string]int
	failureMu     sync.Mutex

	done chan struct{}
	wg   sync.WaitGroup
}

// HealthCheckerConfig holds configuration for the health checker.
type HealthCheckerConfig struct {
	// Interval is the interval between health checks.
	Interval time.Duration

	// Timeout is the timeout for health checks.
	Timeout time.Duration

	// UnhealthyThreshold is the number of consecutive failures before marking unhealthy.
	UnhealthyThreshold int
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(config *HealthCheckerConfig, registry *CapabilityRegistry, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		config:        config,
		registry:      registry,
		logger:        logger.With(zap.String("component", "health_checker")),
		failureCounts: make(map[string]int),
		done:          make(chan struct{}),
	}
}

// Start starts the health checker.
func (h *HealthChecker) Start(ctx context.Context) error {
	h.wg.Add(1)
	go h.run()
	h.logger.Info("health checker started")
	return nil
}

// Stop stops the health checker.
func (h *HealthChecker) Stop(ctx context.Context) error {
	close(h.done)
	h.wg.Wait()
	h.logger.Info("health checker stopped")
	return nil
}

// run is the main health check loop.
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

// checkAll performs health checks on all registered agents.
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

// checkAgent performs a health check on a single agent.
func (h *HealthChecker) checkAgent(ctx context.Context, agent *AgentInfo) {
	agentID := agent.Card.Name
	result := h.performHealthCheck(ctx, agent)

	h.failureMu.Lock()
	defer h.failureMu.Unlock()

	if result.Healthy {
		// Reset failure count on success
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

// performHealthCheck performs the actual health check.
func (h *HealthChecker) performHealthCheck(ctx context.Context, agent *AgentInfo) *HealthCheckResult {
	start := time.Now()
	result := &HealthCheckResult{
		AgentID:   agent.Card.Name,
		Timestamp: start,
	}

	// For local agents, check if they're still registered and responsive
	if agent.IsLocal {
		// Check heartbeat freshness
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

	// For remote agents, perform HTTP health check
	if agent.Endpoint != "" {
		healthURL := strings.TrimRight(agent.Endpoint, "/") + "/health"
		client := &http.Client{Timeout: 5 * time.Second}
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

// mustMarshal marshals data to JSON, returning nil on error.
func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// Ensure CapabilityRegistry implements Registry interface.
var _ Registry = (*CapabilityRegistry)(nil)
