package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CapabilityComposer is the default implementation of the Composer interface.
// It provides capability composition, dependency resolution, and conflict detection.
type CapabilityComposer struct {
	registry Registry
	matcher  Matcher
	config   *ComposerConfig
	logger   *zap.Logger

	// dependencyGraph stores known dependencies between capabilities.
	dependencyGraph map[string][]string
	depMu           sync.RWMutex

	// exclusiveGroups stores groups of mutually exclusive capabilities.
	exclusiveGroups [][]string
	exclMu          sync.RWMutex

	// resourceRequirements stores resource requirements for capabilities.
	resourceRequirements map[string]*ResourceRequirement
	resMu                sync.RWMutex
}

// ComposerConfig holds configuration for the capability composer.
type ComposerConfig struct {
	// MaxCompositionDepth is the maximum depth for dependency resolution.
	MaxCompositionDepth int `json:"max_composition_depth"`

	// DefaultTimeout is the default timeout for composition operations.
	DefaultTimeout time.Duration `json:"default_timeout"`

	// AllowPartialComposition allows partial composition if not all capabilities are available.
	AllowPartialComposition bool `json:"allow_partial_composition"`

	// EnableConflictDetection enables conflict detection.
	EnableConflictDetection bool `json:"enable_conflict_detection"`

	// EnableDependencyResolution enables automatic dependency resolution.
	EnableDependencyResolution bool `json:"enable_dependency_resolution"`
}

// DefaultComposerConfig returns a ComposerConfig with sensible defaults.
func DefaultComposerConfig() *ComposerConfig {
	return &ComposerConfig{
		MaxCompositionDepth:        10,
		DefaultTimeout:             10 * time.Second,
		AllowPartialComposition:    false,
		EnableConflictDetection:    true,
		EnableDependencyResolution: true,
	}
}

// ResourceRequirement defines resource requirements for a capability.
type ResourceRequirement struct {
	// CapabilityName is the name of the capability.
	CapabilityName string `json:"capability_name"`

	// CPUCores is the required CPU cores.
	CPUCores float64 `json:"cpu_cores"`

	// MemoryMB is the required memory in MB.
	MemoryMB int `json:"memory_mb"`

	// GPURequired indicates if GPU is required.
	GPURequired bool `json:"gpu_required"`

	// ExclusiveResources is a list of resources that must be exclusive.
	ExclusiveResources []string `json:"exclusive_resources"`
}

// NewCapabilityComposer creates a new capability composer.
func NewCapabilityComposer(registry Registry, matcher Matcher, config *ComposerConfig, logger *zap.Logger) *CapabilityComposer {
	if config == nil {
		config = DefaultComposerConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &CapabilityComposer{
		registry:             registry,
		matcher:              matcher,
		config:               config,
		logger:               logger.With(zap.String("component", "capability_composer")),
		dependencyGraph:      make(map[string][]string),
		exclusiveGroups:      make([][]string, 0),
		resourceRequirements: make(map[string]*ResourceRequirement),
	}
}

// Compose creates a composition of capabilities from multiple agents.
func (c *CapabilityComposer) Compose(ctx context.Context, req *CompositionRequest) (*CompositionResult, error) {
	if req == nil {
		return nil, fmt.Errorf("composition request is nil")
	}

	if len(req.RequiredCapabilities) == 0 {
		return nil, fmt.Errorf("no required capabilities specified")
	}

	// Apply defaults
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.config.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := &CompositionResult{
		Agents:        make([]*AgentInfo, 0),
		CapabilityMap: make(map[string]string),
		Dependencies:  make(map[string][]string),
	}

	// 1. Resolve dependencies
	allCapabilities := req.RequiredCapabilities
	if c.config.EnableDependencyResolution {
		deps, err := c.ResolveDependencies(ctx, req.RequiredCapabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependencies: %w", err)
		}
		result.Dependencies = deps

		// Add dependencies to required capabilities
		for _, depList := range deps {
			for _, dep := range depList {
				if !c.contains(allCapabilities, dep) {
					allCapabilities = append(allCapabilities, dep)
				}
			}
		}
	}

	// 2. Detect conflicts
	if c.config.EnableConflictDetection {
		conflicts, err := c.DetectConflicts(ctx, allCapabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to detect conflicts: %w", err)
		}
		result.Conflicts = conflicts

		// If there are unresolvable conflicts, return error
		for _, conflict := range conflicts {
			if conflict.Resolution == "" {
				if !req.AllowPartial && !c.config.AllowPartialComposition {
					return nil, fmt.Errorf("unresolvable conflict: %s", conflict.Description)
				}
			}
		}
	}

	// 3. Find agents for each capability
	agentSet := make(map[string]*AgentInfo)
	missingCapabilities := make([]string, 0)

	for _, capName := range allCapabilities {
		// Find agents with this capability
		caps, err := c.registry.FindCapabilities(ctx, capName)
		if err != nil {
			c.logger.Warn("failed to find capability", zap.String("capability", capName), zap.Error(err))
			missingCapabilities = append(missingCapabilities, capName)
			continue
		}

		if len(caps) == 0 {
			missingCapabilities = append(missingCapabilities, capName)
			continue
		}

		// Select the best agent for this capability
		bestCap := c.selectBestCapability(caps)
		result.CapabilityMap[capName] = bestCap.AgentID

		// Add agent to set if not already present
		if _, exists := agentSet[bestCap.AgentID]; !exists {
			agent, err := c.registry.GetAgent(ctx, bestCap.AgentID)
			if err != nil {
				c.logger.Warn("failed to get agent", zap.String("agent_id", bestCap.AgentID), zap.Error(err))
				continue
			}
			agentSet[bestCap.AgentID] = agent
		}
	}

	// 4. Check if composition is complete
	result.MissingCapabilities = missingCapabilities
	result.Complete = len(missingCapabilities) == 0

	if !result.Complete && !req.AllowPartial && !c.config.AllowPartialComposition {
		return nil, fmt.Errorf("incomplete composition: missing capabilities %v", missingCapabilities)
	}

	// 5. Apply max agents limit
	for _, agent := range agentSet {
		result.Agents = append(result.Agents, agent)
	}

	if req.MaxAgents > 0 && len(result.Agents) > req.MaxAgents {
		// Prioritize agents with more capabilities
		sort.Slice(result.Agents, func(i, j int) bool {
			countI := c.countCapabilitiesForAgent(result.CapabilityMap, result.Agents[i].Card.Name)
			countJ := c.countCapabilitiesForAgent(result.CapabilityMap, result.Agents[j].Card.Name)
			return countI > countJ
		})
		result.Agents = result.Agents[:req.MaxAgents]

		// Update capability map to only include selected agents
		newCapMap := make(map[string]string)
		for cap, agentID := range result.CapabilityMap {
			for _, agent := range result.Agents {
				if agent.Card.Name == agentID {
					newCapMap[cap] = agentID
					break
				}
			}
		}
		result.CapabilityMap = newCapMap
	}

	// 6. Calculate execution order
	result.ExecutionOrder = c.calculateExecutionOrder(allCapabilities, result.Dependencies)

	c.logger.Info("composition completed",
		zap.Int("agents", len(result.Agents)),
		zap.Int("capabilities", len(result.CapabilityMap)),
		zap.Bool("complete", result.Complete),
	)

	return result, nil
}

// ResolveDependencies resolves dependencies between capabilities.
func (c *CapabilityComposer) ResolveDependencies(ctx context.Context, capabilities []string) (map[string][]string, error) {
	c.depMu.RLock()
	defer c.depMu.RUnlock()

	result := make(map[string][]string)
	visited := make(map[string]bool)

	var resolve func(cap string, depth int) error
	resolve = func(cap string, depth int) error {
		if depth > c.config.MaxCompositionDepth {
			return fmt.Errorf("dependency resolution exceeded max depth for capability %s", cap)
		}

		if visited[cap] {
			return nil
		}
		visited[cap] = true

		deps, exists := c.dependencyGraph[cap]
		if !exists {
			return nil
		}

		result[cap] = deps

		for _, dep := range deps {
			if err := resolve(dep, depth+1); err != nil {
				return err
			}
		}

		return nil
	}

	for _, cap := range capabilities {
		if err := resolve(cap, 0); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// DetectConflicts detects conflicts between capabilities.
func (c *CapabilityComposer) DetectConflicts(ctx context.Context, capabilities []string) ([]Conflict, error) {
	conflicts := make([]Conflict, 0)

	// 1. Check exclusive groups
	c.exclMu.RLock()
	for _, group := range c.exclusiveGroups {
		matchedCaps := make([]string, 0)
		for _, cap := range capabilities {
			for _, exclCap := range group {
				if strings.EqualFold(cap, exclCap) {
					matchedCaps = append(matchedCaps, cap)
					break
				}
			}
		}

		if len(matchedCaps) > 1 {
			conflicts = append(conflicts, Conflict{
				Type:         ConflictTypeExclusive,
				Capabilities: matchedCaps,
				Description:  fmt.Sprintf("capabilities %v are mutually exclusive", matchedCaps),
				Resolution:   fmt.Sprintf("select only one of: %v", matchedCaps),
			})
		}
	}
	c.exclMu.RUnlock()

	// 2. Check resource conflicts
	c.resMu.RLock()
	resourceUsage := make(map[string][]string) // resource -> capabilities using it
	for _, cap := range capabilities {
		req, exists := c.resourceRequirements[cap]
		if !exists {
			continue
		}

		for _, res := range req.ExclusiveResources {
			resourceUsage[res] = append(resourceUsage[res], cap)
		}
	}
	c.resMu.RUnlock()

	for res, caps := range resourceUsage {
		if len(caps) > 1 {
			conflicts = append(conflicts, Conflict{
				Type:         ConflictTypeResource,
				Capabilities: caps,
				Description:  fmt.Sprintf("capabilities %v require exclusive access to resource %s", caps, res),
			})
		}
	}

	// 3. Check dependency conflicts (circular dependencies)
	c.depMu.RLock()
	for _, cap := range capabilities {
		if c.hasCircularDependency(cap, make(map[string]bool)) {
			conflicts = append(conflicts, Conflict{
				Type:         ConflictTypeDependency,
				Capabilities: []string{cap},
				Description:  fmt.Sprintf("capability %s has circular dependencies", cap),
			})
		}
	}
	c.depMu.RUnlock()

	return conflicts, nil
}

// RegisterDependency registers a dependency between capabilities.
func (c *CapabilityComposer) RegisterDependency(capability string, dependencies []string) {
	c.depMu.Lock()
	defer c.depMu.Unlock()

	c.dependencyGraph[capability] = dependencies
	c.logger.Debug("dependency registered",
		zap.String("capability", capability),
		zap.Strings("dependencies", dependencies),
	)
}

// RegisterExclusiveGroup registers a group of mutually exclusive capabilities.
func (c *CapabilityComposer) RegisterExclusiveGroup(capabilities []string) {
	c.exclMu.Lock()
	defer c.exclMu.Unlock()

	c.exclusiveGroups = append(c.exclusiveGroups, capabilities)
	c.logger.Debug("exclusive group registered",
		zap.Strings("capabilities", capabilities),
	)
}

// RegisterResourceRequirement registers resource requirements for a capability.
func (c *CapabilityComposer) RegisterResourceRequirement(req *ResourceRequirement) {
	c.resMu.Lock()
	defer c.resMu.Unlock()

	c.resourceRequirements[req.CapabilityName] = req
	c.logger.Debug("resource requirement registered",
		zap.String("capability", req.CapabilityName),
	)
}

// selectBestCapability selects the best capability from a list.
func (c *CapabilityComposer) selectBestCapability(caps []CapabilityInfo) *CapabilityInfo {
	if len(caps) == 0 {
		return nil
	}

	// Sort by score descending, then by load ascending
	sort.Slice(caps, func(i, j int) bool {
		if caps[i].Score != caps[j].Score {
			return caps[i].Score > caps[j].Score
		}
		return caps[i].Load < caps[j].Load
	})

	return &caps[0]
}

// countCapabilitiesForAgent counts how many capabilities an agent provides in the composition.
func (c *CapabilityComposer) countCapabilitiesForAgent(capMap map[string]string, agentID string) int {
	count := 0
	for _, id := range capMap {
		if id == agentID {
			count++
		}
	}
	return count
}

// calculateExecutionOrder calculates the execution order based on dependencies.
func (c *CapabilityComposer) calculateExecutionOrder(capabilities []string, dependencies map[string][]string) []string {
	// Topological sort
	inDegree := make(map[string]int)
	for _, cap := range capabilities {
		if _, exists := inDegree[cap]; !exists {
			inDegree[cap] = 0
		}
	}

	for _, deps := range dependencies {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// Find all nodes with no incoming edges
	queue := make([]string, 0)
	for cap, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, cap)
		}
	}

	order := make([]string, 0, len(capabilities))
	for len(queue) > 0 {
		// Pop from queue
		cap := queue[0]
		queue = queue[1:]
		order = append(order, cap)

		// Reduce in-degree for dependent capabilities
		if deps, exists := dependencies[cap]; exists {
			for _, dep := range deps {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					queue = append(queue, dep)
				}
			}
		}
	}

	// Reverse order (dependencies should come first)
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order
}

// hasCircularDependency checks if a capability has circular dependencies.
func (c *CapabilityComposer) hasCircularDependency(cap string, visited map[string]bool) bool {
	if visited[cap] {
		return true
	}

	visited[cap] = true
	deps, exists := c.dependencyGraph[cap]
	if !exists {
		return false
	}

	for _, dep := range deps {
		if c.hasCircularDependency(dep, visited) {
			return true
		}
	}

	delete(visited, cap)
	return false
}

// contains checks if a slice contains a string.
func (c *CapabilityComposer) contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

// GetDependencies returns the dependencies for a capability.
func (c *CapabilityComposer) GetDependencies(capability string) []string {
	c.depMu.RLock()
	defer c.depMu.RUnlock()

	deps, exists := c.dependencyGraph[capability]
	if !exists {
		return nil
	}

	result := make([]string, len(deps))
	copy(result, deps)
	return result
}

// GetExclusiveGroups returns all exclusive groups.
func (c *CapabilityComposer) GetExclusiveGroups() [][]string {
	c.exclMu.RLock()
	defer c.exclMu.RUnlock()

	result := make([][]string, len(c.exclusiveGroups))
	for i, group := range c.exclusiveGroups {
		result[i] = make([]string, len(group))
		copy(result[i], group)
	}
	return result
}

// ClearDependencies clears all registered dependencies.
func (c *CapabilityComposer) ClearDependencies() {
	c.depMu.Lock()
	defer c.depMu.Unlock()

	c.dependencyGraph = make(map[string][]string)
}

// ClearExclusiveGroups clears all exclusive groups.
func (c *CapabilityComposer) ClearExclusiveGroups() {
	c.exclMu.Lock()
	defer c.exclMu.Unlock()

	c.exclusiveGroups = make([][]string, 0)
}

// Ensure CapabilityComposer implements Composer interface.
var _ Composer = (*CapabilityComposer)(nil)
