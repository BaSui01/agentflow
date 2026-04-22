package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CapaliableComposer是编译器接口的默认执行.
// 它提供能力构成、依赖解决和冲突探测。
type CapabilityComposer struct {
	registry Registry
	matcher  Matcher
	config   *ComposerConfig
	logger   *zap.Logger

	// 依赖性Graph存储已知的能力之间的依赖性.
	dependencyGraph map[string][]string
	depMu           sync.RWMutex

	// 独家集团储存具有相互排斥能力的集团.
	exclusiveGroups [][]string
	exclMu          sync.RWMutex

	// 资源需求储存能力所需资源。
	resourceRequirements map[string]*ResourceRequirement
	resMu                sync.RWMutex
}

// 作曲家Config持有能力作曲的配置.
type ComposerConfig struct {
	// MaxComposition深度是依赖分辨率的最大深度.
	MaxCompositionDepth int `json:"max_composition_depth"`

	// 默认超时是组件操作的默认超时.
	DefaultTimeout time.Duration `json:"default_timeout"`

	// 如果并非所有能力都具备,允许部分组成。
	AllowPartialComposition bool `json:"allow_partial_composition"`

	// 启用冲突探测可以检测冲突。
	EnableConflictDetection bool `json:"enable_conflict_detection"`

	// 启用依赖性决议允许自动依赖性解析 。
	EnableDependencyResolution bool `json:"enable_dependency_resolution"`
}

// 默认composerConfig 返回带有合理默认的 ComposerConfig 。
func DefaultComposerConfig() *ComposerConfig {
	return &ComposerConfig{
		MaxCompositionDepth:        10,
		DefaultTimeout:             10 * time.Second,
		AllowPartialComposition:    false,
		EnableConflictDetection:    true,
		EnableDependencyResolution: true,
	}
}

// 资源需求界定了能力所需资源。
type ResourceRequirement struct {
	// 能力 名称是能力的名称.
	CapabilityName string `json:"capability_name"`

	// CPUCores是所需的CPU核心.
	CPUCores float64 `json:"cpu_cores"`

	// 内存MB是MB中所需的内存.
	MemoryMB int `json:"memory_mb"`

	// GPUrered表示是否需要GPU.
	GPURequired bool `json:"gpu_required"`

	// 专属资源是一份必须专属的资源清单。
	ExclusiveResources []string `json:"exclusive_resources"`
}

// New CapabilityComposer)创建出一个新的能力作曲家.
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

// 作曲从多个代理中产生能力组成.
func (c *CapabilityComposer) Compose(ctx context.Context, req *CompositionRequest) (*CompositionResult, error) {
	if err := validateCompositionRequest(req); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, compositionTimeout(req, c.config))
	defer cancel()

	result := &CompositionResult{
		Agents:        make([]*AgentInfo, 0),
		CapabilityMap: make(map[string]string),
		Dependencies:  make(map[string][]string),
	}

	allCapabilities, err := c.resolveCompositionCapabilities(ctx, req, result)
	if err != nil {
		return nil, err
	}
	if err := c.populateCompositionConflicts(ctx, req, result, allCapabilities); err != nil {
		return nil, err
	}
	agentSet, missingCapabilities := c.composeAgentsForCapabilities(ctx, result, allCapabilities)
	if err := c.finalizeCompositionResult(req, result, agentSet, missingCapabilities); err != nil {
		return nil, err
	}
	result.ExecutionOrder = c.calculateExecutionOrder(allCapabilities, result.Dependencies)
	c.logger.Info("composition completed",
		zap.Int("agents", len(result.Agents)),
		zap.Int("capabilities", len(result.CapabilityMap)),
		zap.Bool("complete", result.Complete),
	)

	return result, nil
}

func validateCompositionRequest(req *CompositionRequest) error {
	if req == nil {
		return fmt.Errorf("composition request is nil")
	}
	if len(req.RequiredCapabilities) == 0 {
		return fmt.Errorf("no required capabilities specified")
	}
	return nil
}

func compositionTimeout(req *CompositionRequest, config *ComposerConfig) time.Duration {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = config.DefaultTimeout
	}
	return timeout
}

func (c *CapabilityComposer) resolveCompositionCapabilities(ctx context.Context, req *CompositionRequest, result *CompositionResult) ([]string, error) {
	allCapabilities := append([]string(nil), req.RequiredCapabilities...)
	if !c.config.EnableDependencyResolution {
		return allCapabilities, nil
	}
	deps, err := c.ResolveDependencies(ctx, req.RequiredCapabilities)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependencies: %w", err)
	}
	result.Dependencies = deps
	for _, depList := range deps {
		for _, dep := range depList {
			if !c.contains(allCapabilities, dep) {
				allCapabilities = append(allCapabilities, dep)
			}
		}
	}
	return allCapabilities, nil
}

func (c *CapabilityComposer) populateCompositionConflicts(ctx context.Context, req *CompositionRequest, result *CompositionResult, allCapabilities []string) error {
	if !c.config.EnableConflictDetection {
		return nil
	}
	conflicts, err := c.DetectConflicts(ctx, allCapabilities)
	if err != nil {
		return fmt.Errorf("failed to detect conflicts: %w", err)
	}
	result.Conflicts = conflicts
	for _, conflict := range conflicts {
		if conflict.Resolution == "" && !req.AllowPartial && !c.config.AllowPartialComposition {
			return fmt.Errorf("unresolvable conflict: %s", conflict.Description)
		}
	}
	return nil
}

func (c *CapabilityComposer) composeAgentsForCapabilities(ctx context.Context, result *CompositionResult, allCapabilities []string) (map[string]*AgentInfo, []string) {
	agentSet := make(map[string]*AgentInfo)
	missingCapabilities := make([]string, 0)
	for _, capabilityName := range allCapabilities {
		if !c.composeCapabilityAgent(ctx, result, agentSet, capabilityName) {
			missingCapabilities = append(missingCapabilities, capabilityName)
		}
	}
	return agentSet, missingCapabilities
}

func (c *CapabilityComposer) composeCapabilityAgent(ctx context.Context, result *CompositionResult, agentSet map[string]*AgentInfo, capabilityName string) bool {
	caps, err := c.registry.FindCapabilities(ctx, capabilityName)
	if err != nil {
		c.logger.Warn("failed to find capability", zap.String("capability", capabilityName), zap.Error(err))
		return false
	}
	if len(caps) == 0 {
		return false
	}
	bestCap := c.selectBestCapability(caps)
	result.CapabilityMap[capabilityName] = bestCap.AgentID
	if _, exists := agentSet[bestCap.AgentID]; exists {
		return true
	}
	agentInfo, err := c.registry.GetAgent(ctx, bestCap.AgentID)
	if err != nil {
		c.logger.Warn("failed to get agent", zap.String("agent_id", bestCap.AgentID), zap.Error(err))
		return false
	}
	agentSet[bestCap.AgentID] = agentInfo
	return true
}

func (c *CapabilityComposer) finalizeCompositionResult(req *CompositionRequest, result *CompositionResult, agentSet map[string]*AgentInfo, missingCapabilities []string) error {
	result.MissingCapabilities = missingCapabilities
	result.Complete = len(missingCapabilities) == 0
	if !result.Complete && !req.AllowPartial && !c.config.AllowPartialComposition {
		return fmt.Errorf("incomplete composition: missing capabilities: %v", missingCapabilities)
	}
	for _, agentInfo := range agentSet {
		result.Agents = append(result.Agents, agentInfo)
	}
	if req.MaxAgents > 0 && len(result.Agents) > req.MaxAgents {
		c.trimCompositionAgents(req, result)
	}
	return nil
}

func (c *CapabilityComposer) trimCompositionAgents(req *CompositionRequest, result *CompositionResult) {
	sort.Slice(result.Agents, func(i, j int) bool {
		countI := c.countCapabilitiesForAgent(result.CapabilityMap, result.Agents[i].Card.Name)
		countJ := c.countCapabilitiesForAgent(result.CapabilityMap, result.Agents[j].Card.Name)
		return countI > countJ
	})
	result.Agents = result.Agents[:req.MaxAgents]
	newCapMap := make(map[string]string)
	for capabilityName, agentID := range result.CapabilityMap {
		for _, agentInfo := range result.Agents {
			if agentInfo.Card.Name == agentID {
				newCapMap[capabilityName] = agentID
				break
			}
		}
	}
	result.CapabilityMap = newCapMap
}

// 解决依赖解决了能力之间的依赖.
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

// 侦测冲突能发现能力之间的冲突。
func (c *CapabilityComposer) DetectConflicts(ctx context.Context, capabilities []string) ([]Conflict, error) {
	conflicts := make([]Conflict, 0)
	conflicts = append(conflicts, c.detectExclusiveGroupConflicts(capabilities)...)
	conflicts = append(conflicts, c.detectResourceConflicts(capabilities)...)
	conflicts = append(conflicts, c.detectDependencyConflicts(capabilities)...)
	return conflicts, nil
}

func (c *CapabilityComposer) detectExclusiveGroupConflicts(capabilities []string) []Conflict {
	conflicts := make([]Conflict, 0)
	c.exclMu.RLock()
	defer c.exclMu.RUnlock()
	for _, group := range c.exclusiveGroups {
		matchedCaps := make([]string, 0)
		for _, capabilityName := range capabilities {
			for _, exclCap := range group {
				if strings.EqualFold(capabilityName, exclCap) {
					matchedCaps = append(matchedCaps, capabilityName)
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
	return conflicts
}

func (c *CapabilityComposer) detectResourceConflicts(capabilities []string) []Conflict {
	conflicts := make([]Conflict, 0)
	resourceUsage := make(map[string][]string)
	c.resMu.RLock()
	for _, capabilityName := range capabilities {
		req, exists := c.resourceRequirements[capabilityName]
		if !exists {
			continue
		}
		for _, res := range req.ExclusiveResources {
			resourceUsage[res] = append(resourceUsage[res], capabilityName)
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
	return conflicts
}

func (c *CapabilityComposer) detectDependencyConflicts(capabilities []string) []Conflict {
	conflicts := make([]Conflict, 0)
	c.depMu.RLock()
	defer c.depMu.RUnlock()
	for _, capabilityName := range capabilities {
		if c.hasCircularDependency(capabilityName, make(map[string]bool)) {
			conflicts = append(conflicts, Conflict{
				Type:         ConflictTypeDependency,
				Capabilities: []string{capabilityName},
				Description:  fmt.Sprintf("capability %s has circular dependencies", capabilityName),
			})
		}
	}
	return conflicts
}

// 登记册的依赖性对各种能力之间的依赖性进行登记。
func (c *CapabilityComposer) RegisterDependency(capability string, dependencies []string) {
	c.depMu.Lock()
	defer c.depMu.Unlock()

	c.dependencyGraph[capability] = dependencies
	c.logger.Debug("dependency registered",
		zap.String("capability", capability),
		zap.Strings("dependencies", dependencies),
	)
}

// 登记小组登记一组相互排斥的能力。
func (c *CapabilityComposer) RegisterExclusiveGroup(capabilities []string) {
	c.exclMu.Lock()
	defer c.exclMu.Unlock()

	c.exclusiveGroups = append(c.exclusiveGroups, capabilities)
	c.logger.Debug("exclusive group registered",
		zap.Strings("capabilities", capabilities),
	)
}

// 登记资源需求登记能力所需资源。
func (c *CapabilityComposer) RegisterResourceRequirement(req *ResourceRequirement) {
	c.resMu.Lock()
	defer c.resMu.Unlock()

	c.resourceRequirements[req.CapabilityName] = req
	c.logger.Debug("resource requirement registered",
		zap.String("capability", req.CapabilityName),
	)
}

// 从列表中选择最佳能力。
func (c *CapabilityComposer) selectBestCapability(caps []CapabilityInfo) *CapabilityInfo {
	if len(caps) == 0 {
		return nil
	}

	// 依积分递减排序,再依负载递增排序
	sort.Slice(caps, func(i, j int) bool {
		if caps[i].Score != caps[j].Score {
			return caps[i].Score > caps[j].Score
		}
		return caps[i].Load < caps[j].Load
	})

	return &caps[0]
}

// 计数能力
func (c *CapabilityComposer) countCapabilitiesForAgent(capMap map[string]string, agentID string) int {
	count := 0
	for _, id := range capMap {
		if id == agentID {
			count++
		}
	}
	return count
}

// 计算Execution Order根据依赖性计算执行命令.
func (c *CapabilityComposer) calculateExecutionOrder(capabilities []string, dependencies map[string][]string) []string {
	// 地形类型
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

	// 找到没有边缘的所有节点
	queue := make([]string, 0)
	for capabilityName, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, capabilityName)
		}
	}

	order := make([]string, 0, len(capabilities))
	for len(queue) > 0 {
		// 从队列中弹出
		capabilityName := queue[0]
		queue = queue[1:]
		order = append(order, capabilityName)

		// 减少依赖能力的学位
		if deps, exists := dependencies[capabilityName]; exists {
			for _, dep := range deps {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					queue = append(queue, dep)
				}
			}
		}
	}

	// 反转顺序，让依赖项优先执行。
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order
}

// 如果某个能力具有循环依赖性,则有循环依赖性检查。
func (c *CapabilityComposer) hasCircularDependency(capabilityName string, visited map[string]bool) bool {
	if visited[capabilityName] {
		return true
	}

	visited[capabilityName] = true
	deps, exists := c.dependencyGraph[capabilityName]
	if !exists {
		return false
	}

	for _, dep := range deps {
		if c.hasCircularDependency(dep, visited) {
			return true
		}
	}

	delete(visited, capabilityName)
	return false
}

// 如果切片含有字符串,则包含检查。
func (c *CapabilityComposer) contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

// GetDependency返回一个能力的依赖性.
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

// GetExclusive Groups 返回所有专属组 。
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

// 清除依赖性清除所有注册的依赖性 。
func (c *CapabilityComposer) ClearDependencies() {
	c.depMu.Lock()
	defer c.depMu.Unlock()

	c.dependencyGraph = make(map[string][]string)
}

// ClearExclusive Groups 清除所有专属组.
func (c *CapabilityComposer) ClearExclusiveGroups() {
	c.exclMu.Lock()
	defer c.exclMu.Unlock()

	c.exclusiveGroups = make([][]string, 0)
}

// 确保 CapableComposer 执行 Composer 接口.
var _ Composer = (*CapabilityComposer)(nil)
