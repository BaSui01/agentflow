// Package discovery provides Agent capability discovery and matching for multi-agent collaboration.
// It implements a capability registry, semantic matching engine, and service discovery protocols.
package discovery

import (
	"context"
	"encoding/json"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
)

// CapabilityStatus represents the status of a capability.
type CapabilityStatus string

const (
	// CapabilityStatusActive indicates the capability is active and available.
	CapabilityStatusActive CapabilityStatus = "active"
	// CapabilityStatusInactive indicates the capability is temporarily unavailable.
	CapabilityStatusInactive CapabilityStatus = "inactive"
	// CapabilityStatusDegraded indicates the capability is available but with reduced performance.
	CapabilityStatusDegraded CapabilityStatus = "degraded"
	// CapabilityStatusUnknown indicates the capability status is unknown.
	CapabilityStatusUnknown CapabilityStatus = "unknown"
)

// AgentStatus represents the status of an agent.
type AgentStatus string

const (
	// AgentStatusOnline indicates the agent is online and healthy.
	AgentStatusOnline AgentStatus = "online"
	// AgentStatusOffline indicates the agent is offline.
	AgentStatusOffline AgentStatus = "offline"
	// AgentStatusBusy indicates the agent is busy processing tasks.
	AgentStatusBusy AgentStatus = "busy"
	// AgentStatusUnhealthy indicates the agent is unhealthy.
	AgentStatusUnhealthy AgentStatus = "unhealthy"
)

// CapabilityInfo contains detailed information about a capability.
type CapabilityInfo struct {
	// Capability is the base capability definition from A2A protocol.
	Capability a2a.Capability `json:"capability"`

	// AgentID is the ID of the agent providing this capability.
	AgentID string `json:"agent_id"`

	// AgentName is the name of the agent providing this capability.
	AgentName string `json:"agent_name"`

	// Status is the current status of this capability.
	Status CapabilityStatus `json:"status"`

	// Score is the capability score based on historical performance (0-100).
	Score float64 `json:"score"`

	// Load is the current load of the agent (0-1).
	Load float64 `json:"load"`

	// Tags are additional tags for capability categorization.
	Tags []string `json:"tags,omitempty"`

	// Metadata contains additional metadata.
	Metadata map[string]string `json:"metadata,omitempty"`

	// RegisteredAt is when this capability was registered.
	RegisteredAt time.Time `json:"registered_at"`

	// LastUpdatedAt is when this capability was last updated.
	LastUpdatedAt time.Time `json:"last_updated_at"`

	// LastHealthCheck is when the last health check was performed.
	LastHealthCheck time.Time `json:"last_health_check"`

	// SuccessCount is the number of successful executions.
	SuccessCount int64 `json:"success_count"`

	// FailureCount is the number of failed executions.
	FailureCount int64 `json:"failure_count"`

	// AvgLatency is the average execution latency.
	AvgLatency time.Duration `json:"avg_latency"`
}

// AgentInfo contains detailed information about a registered agent.
type AgentInfo struct {
	// Card is the A2A agent card.
	Card *a2a.AgentCard `json:"card"`

	// Status is the current status of the agent.
	Status AgentStatus `json:"status"`

	// Capabilities is the list of capabilities provided by this agent.
	Capabilities []CapabilityInfo `json:"capabilities"`

	// Load is the current load of the agent (0-1).
	Load float64 `json:"load"`

	// Priority is the agent's priority for task assignment.
	Priority int `json:"priority"`

	// Endpoint is the agent's endpoint URL (for remote agents).
	Endpoint string `json:"endpoint,omitempty"`

	// IsLocal indicates if this is a local (in-process) agent.
	IsLocal bool `json:"is_local"`

	// RegisteredAt is when this agent was registered.
	RegisteredAt time.Time `json:"registered_at"`

	// LastHeartbeat is when the last heartbeat was received.
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// Metadata contains additional metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MatchRequest represents a request to find matching agents.
type MatchRequest struct {
	// TaskDescription is the natural language description of the task.
	TaskDescription string `json:"task_description"`

	// RequiredCapabilities is the list of required capability names.
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`

	// PreferredCapabilities is the list of preferred capability names.
	PreferredCapabilities []string `json:"preferred_capabilities,omitempty"`

	// RequiredTags is the list of required tags.
	RequiredTags []string `json:"required_tags,omitempty"`

	// ExcludedAgents is the list of agent IDs to exclude.
	ExcludedAgents []string `json:"excluded_agents,omitempty"`

	// MinScore is the minimum capability score required.
	MinScore float64 `json:"min_score,omitempty"`

	// MaxLoad is the maximum acceptable load.
	MaxLoad float64 `json:"max_load,omitempty"`

	// Limit is the maximum number of results to return.
	Limit int `json:"limit,omitempty"`

	// Strategy is the matching strategy to use.
	Strategy MatchStrategy `json:"strategy,omitempty"`

	// Timeout is the timeout for the match operation.
	Timeout time.Duration `json:"timeout,omitempty"`
}

// MatchStrategy defines the strategy for matching agents.
type MatchStrategy string

const (
	// MatchStrategyBestMatch returns the best matching agent.
	MatchStrategyBestMatch MatchStrategy = "best_match"
	// MatchStrategyLeastLoaded returns the least loaded matching agent.
	MatchStrategyLeastLoaded MatchStrategy = "least_loaded"
	// MatchStrategyHighestScore returns the highest scoring matching agent.
	MatchStrategyHighestScore MatchStrategy = "highest_score"
	// MatchStrategyRoundRobin returns agents in round-robin order.
	MatchStrategyRoundRobin MatchStrategy = "round_robin"
	// MatchStrategyRandom returns a random matching agent.
	MatchStrategyRandom MatchStrategy = "random"
)

// MatchResult represents the result of a capability match.
type MatchResult struct {
	// Agent is the matched agent information.
	Agent *AgentInfo `json:"agent"`

	// MatchedCapabilities is the list of matched capabilities.
	MatchedCapabilities []CapabilityInfo `json:"matched_capabilities"`

	// Score is the overall match score (0-100).
	Score float64 `json:"score"`

	// Confidence is the confidence level of the match (0-1).
	Confidence float64 `json:"confidence"`

	// Reason is the reason for the match.
	Reason string `json:"reason,omitempty"`
}

// CompositionRequest represents a request to compose capabilities.
type CompositionRequest struct {
	// TaskDescription is the natural language description of the task.
	TaskDescription string `json:"task_description"`

	// RequiredCapabilities is the list of required capability names.
	RequiredCapabilities []string `json:"required_capabilities"`

	// AllowPartial allows partial composition if not all capabilities are available.
	AllowPartial bool `json:"allow_partial"`

	// MaxAgents is the maximum number of agents to include in the composition.
	MaxAgents int `json:"max_agents,omitempty"`

	// Timeout is the timeout for the composition operation.
	Timeout time.Duration `json:"timeout,omitempty"`
}

// CompositionResult represents the result of a capability composition.
type CompositionResult struct {
	// Agents is the list of agents in the composition.
	Agents []*AgentInfo `json:"agents"`

	// CapabilityMap maps capability names to agent IDs.
	CapabilityMap map[string]string `json:"capability_map"`

	// Dependencies is the dependency graph between capabilities.
	Dependencies map[string][]string `json:"dependencies,omitempty"`

	// ExecutionOrder is the recommended execution order.
	ExecutionOrder []string `json:"execution_order,omitempty"`

	// Conflicts is the list of detected conflicts.
	Conflicts []Conflict `json:"conflicts,omitempty"`

	// Complete indicates if all required capabilities are satisfied.
	Complete bool `json:"complete"`

	// MissingCapabilities is the list of missing capabilities.
	MissingCapabilities []string `json:"missing_capabilities,omitempty"`
}

// Conflict represents a conflict between capabilities.
type Conflict struct {
	// Type is the type of conflict.
	Type ConflictType `json:"type"`

	// Capabilities is the list of conflicting capabilities.
	Capabilities []string `json:"capabilities"`

	// Agents is the list of agents involved in the conflict.
	Agents []string `json:"agents"`

	// Description is a description of the conflict.
	Description string `json:"description"`

	// Resolution is the suggested resolution.
	Resolution string `json:"resolution,omitempty"`
}

// ConflictType defines the type of conflict.
type ConflictType string

const (
	// ConflictTypeResource indicates a resource conflict.
	ConflictTypeResource ConflictType = "resource"
	// ConflictTypeDependency indicates a dependency conflict.
	ConflictTypeDependency ConflictType = "dependency"
	// ConflictTypeExclusive indicates mutually exclusive capabilities.
	ConflictTypeExclusive ConflictType = "exclusive"
	// ConflictTypeVersion indicates a version conflict.
	ConflictTypeVersion ConflictType = "version"
)

// DiscoveryEvent represents an event in the discovery system.
type DiscoveryEvent struct {
	// Type is the event type.
	Type DiscoveryEventType `json:"type"`

	// AgentID is the ID of the agent involved.
	AgentID string `json:"agent_id"`

	// Capability is the capability involved (if applicable).
	Capability string `json:"capability,omitempty"`

	// Data contains additional event data.
	Data json.RawMessage `json:"data,omitempty"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
}

// DiscoveryEventType defines the type of discovery event.
type DiscoveryEventType string

const (
	// DiscoveryEventAgentRegistered indicates an agent was registered.
	DiscoveryEventAgentRegistered DiscoveryEventType = "agent_registered"
	// DiscoveryEventAgentUnregistered indicates an agent was unregistered.
	DiscoveryEventAgentUnregistered DiscoveryEventType = "agent_unregistered"
	// DiscoveryEventAgentUpdated indicates an agent was updated.
	DiscoveryEventAgentUpdated DiscoveryEventType = "agent_updated"
	// DiscoveryEventCapabilityAdded indicates a capability was added.
	DiscoveryEventCapabilityAdded DiscoveryEventType = "capability_added"
	// DiscoveryEventCapabilityRemoved indicates a capability was removed.
	DiscoveryEventCapabilityRemoved DiscoveryEventType = "capability_removed"
	// DiscoveryEventCapabilityUpdated indicates a capability was updated.
	DiscoveryEventCapabilityUpdated DiscoveryEventType = "capability_updated"
	// DiscoveryEventHealthCheckFailed indicates a health check failed.
	DiscoveryEventHealthCheckFailed DiscoveryEventType = "health_check_failed"
	// DiscoveryEventHealthCheckRecovered indicates a health check recovered.
	DiscoveryEventHealthCheckRecovered DiscoveryEventType = "health_check_recovered"
)

// DiscoveryEventHandler is a function that handles discovery events.
type DiscoveryEventHandler func(event *DiscoveryEvent)

// HealthCheckResult represents the result of a health check.
type HealthCheckResult struct {
	// AgentID is the ID of the agent.
	AgentID string `json:"agent_id"`

	// Healthy indicates if the agent is healthy.
	Healthy bool `json:"healthy"`

	// Status is the agent status.
	Status AgentStatus `json:"status"`

	// Latency is the health check latency.
	Latency time.Duration `json:"latency"`

	// Message is an optional message.
	Message string `json:"message,omitempty"`

	// Timestamp is when the health check was performed.
	Timestamp time.Time `json:"timestamp"`
}

// Registry defines the interface for capability registry operations.
type Registry interface {
	// RegisterAgent registers an agent with its capabilities.
	RegisterAgent(ctx context.Context, info *AgentInfo) error

	// UnregisterAgent unregisters an agent.
	UnregisterAgent(ctx context.Context, agentID string) error

	// UpdateAgent updates an agent's information.
	UpdateAgent(ctx context.Context, info *AgentInfo) error

	// GetAgent retrieves an agent by ID.
	GetAgent(ctx context.Context, agentID string) (*AgentInfo, error)

	// ListAgents lists all registered agents.
	ListAgents(ctx context.Context) ([]*AgentInfo, error)

	// RegisterCapability registers a capability for an agent.
	RegisterCapability(ctx context.Context, agentID string, cap *CapabilityInfo) error

	// UnregisterCapability unregisters a capability.
	UnregisterCapability(ctx context.Context, agentID string, capabilityName string) error

	// UpdateCapability updates a capability.
	UpdateCapability(ctx context.Context, agentID string, cap *CapabilityInfo) error

	// GetCapability retrieves a capability by agent ID and name.
	GetCapability(ctx context.Context, agentID string, capabilityName string) (*CapabilityInfo, error)

	// ListCapabilities lists all capabilities for an agent.
	ListCapabilities(ctx context.Context, agentID string) ([]CapabilityInfo, error)

	// FindCapabilities finds capabilities by name across all agents.
	FindCapabilities(ctx context.Context, capabilityName string) ([]CapabilityInfo, error)

	// UpdateAgentStatus updates an agent's status.
	UpdateAgentStatus(ctx context.Context, agentID string, status AgentStatus) error

	// UpdateAgentLoad updates an agent's load.
	UpdateAgentLoad(ctx context.Context, agentID string, load float64) error

	// RecordExecution records an execution result for a capability.
	RecordExecution(ctx context.Context, agentID string, capabilityName string, success bool, latency time.Duration) error

	// Subscribe subscribes to discovery events.
	Subscribe(handler DiscoveryEventHandler) string

	// Unsubscribe unsubscribes from discovery events.
	Unsubscribe(subscriptionID string)

	// Close closes the registry.
	Close() error
}

// Matcher defines the interface for capability matching operations.
type Matcher interface {
	// Match finds agents matching the given request.
	Match(ctx context.Context, req *MatchRequest) ([]*MatchResult, error)

	// MatchOne finds the best matching agent for the given request.
	MatchOne(ctx context.Context, req *MatchRequest) (*MatchResult, error)

	// Score calculates the match score for an agent against a request.
	Score(ctx context.Context, agent *AgentInfo, req *MatchRequest) (float64, error)
}

// Composer defines the interface for capability composition operations.
type Composer interface {
	// Compose creates a composition of capabilities from multiple agents.
	Compose(ctx context.Context, req *CompositionRequest) (*CompositionResult, error)

	// ResolveDependencies resolves dependencies between capabilities.
	ResolveDependencies(ctx context.Context, capabilities []string) (map[string][]string, error)

	// DetectConflicts detects conflicts between capabilities.
	DetectConflicts(ctx context.Context, capabilities []string) ([]Conflict, error)
}

// Protocol defines the interface for service discovery protocol operations.
type Protocol interface {
	// Start starts the discovery protocol.
	Start(ctx context.Context) error

	// Stop stops the discovery protocol.
	Stop(ctx context.Context) error

	// Announce announces the local agent to the network.
	Announce(ctx context.Context, info *AgentInfo) error

	// Discover discovers agents on the network.
	Discover(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error)

	// Subscribe subscribes to agent announcements.
	Subscribe(handler func(*AgentInfo)) string

	// Unsubscribe unsubscribes from agent announcements.
	Unsubscribe(subscriptionID string)
}

// DiscoveryFilter defines filters for agent discovery.
type DiscoveryFilter struct {
	// Capabilities filters by capability names.
	Capabilities []string `json:"capabilities,omitempty"`

	// Tags filters by tags.
	Tags []string `json:"tags,omitempty"`

	// Status filters by agent status.
	Status []AgentStatus `json:"status,omitempty"`

	// Local filters for local agents only.
	Local *bool `json:"local,omitempty"`

	// Remote filters for remote agents only.
	Remote *bool `json:"remote,omitempty"`
}
