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
	if req == nil {
		return nil, fmt.Errorf("composition request is nil")
	}

	if len(req.RequiredCapabilities) == 0 {
		return nil, fmt.Errorf("no required capabilities specified")
	}

	// 应用默认
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

	// 1. 解决依赖关系
	allCapabilities := req.RequiredCapabilities
	if c.config.EnableDependencyResolution {
		deps, err := c.ResolveDependencies(ctx, req.RequiredCapabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependencies: %w", err)
		}
		result.Dependencies = deps

		// 增加所需能力的依赖性
		for _, depList := range deps {
			for _, dep := range depList {
				if !c.contains(allCapabilities, dep) {
					allCapabilities = append(allCapabilities, dep)
				}
			}
		}
	}

	// 2. 侦测冲突
	if c.config.EnableConflictDetection {
		conflicts, err := c.DetectConflicts(ctx, allCapabilities)
		if err != nil {
			return nil, fmt.Errorf("failed to detect conflicts: %w", err)
		}
		result.Conflicts = conflicts

		// 如果存在无法解决的冲突, 返回错误
		for _, conflict := range conflicts {
			if conflict.Resolution == "" {
				if !req.AllowPartial && !c.config.AllowPartialComposition {
					return nil, fmt.Errorf("unresolvable conflict: %s", conflict.Description)
				}
			}
		}
	}

	// 3. 为每种能力寻找代理人
	agentSet := make(map[string]*AgentInfo)
	missingCapabilities := make([]string, 0)

	for _, capName := range allCapabilities {
		// 找到有这种能力的特工
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

		// 选择此能力的最佳代理
		bestCap := c.selectBestCapability(caps)
		result.CapabilityMap[capName] = bestCap.AgentID

		// 如果尚未设置, 则添加代理设置
		if _, exists := agentSet[bestCap.AgentID]; !exists {
			agent, err := c.registry.GetAgent(ctx, bestCap.AgentID)
			if err != nil {
				c.logger.Warn("failed to get agent", zap.String("agent_id", bestCap.AgentID), zap.Error(err))
				continue
			}
			agentSet[bestCap.AgentID] = agent
		}
	}

	// 4. 检查组成是否完整
	result.MissingCapabilities = missingCapabilities
	result.Complete = len(missingCapabilities) == 0

	if !result.Complete && !req.AllowPartial && !c.config.AllowPartialComposition {
		return nil, fmt.Errorf("incomplete composition: missing capabilities %v", missingCapabilities)
	}

	// 5. 应用最大剂限
	for _, agent := range agentSet {
		result.Agents = append(result.Agents, agent)
	}

	if req.MaxAgents > 0 && len(result.Agents) > req.MaxAgents {
		// 优先考虑能力较强的代理人
		sort.Slice(result.Agents, func(i, j int) bool {
			countI := c.countCapabilitiesForAgent(result.CapabilityMap, result.Agents[i].Card.Name)
			countJ := c.countCapabilitiesForAgent(result.CapabilityMap, result.Agents[j].Card.Name)
			return countI > countJ
		})
		result.Agents = result.Agents[:req.MaxAgents]

		// 更新能力映射只包含选定的代理
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

	// 6. 计算执行令
	result.ExecutionOrder = c.calculateExecutionOrder(allCapabilities, result.Dependencies)

	c.logger.Info("composition completed",
		zap.Int("agents", len(result.Agents)),
		zap.Int("capabilities", len(result.CapabilityMap)),
		zap.Bool("complete", result.Complete),
	)

	return result, nil
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

	// 1. 检查专属团体
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

	// 2. 检查资源冲突
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

	// 3. 检查依赖性冲突(依赖性)
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
	for cap, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, cap)
		}
	}

	order := make([]string, 0, len(capabilities))
	for len(queue) > 0 {
		// 从队列中弹出
		cap := queue[0]
		queue = queue[1:]
		order = append(order, cap)

		// 减少依赖能力的学位
		if deps, exists := dependencies[cap]; exists {
			for _, dep := range deps {
				inDegree[dep]--
				if inDegree[dep] == 0 {
					queue = append(queue, dep)
				}
			}
		}
	}

	// 倒置顺序( 相互依存应优先)
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order
}

// 如果某个能力具有循环依赖性,则有循环依赖性检查。
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
