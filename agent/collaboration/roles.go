package collaboration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ============================================================================
// 基于角色的代理管弦乐团
// ============================================================================
//
// 在AI-Researcher的多代理管道的启发下,这个模块提供
// 一种基于角色的管弦系统,其中指定了代理人的具体角色
// 具有确定的能力、责任和交流模式。
//
// 研究程序方面的例子作用:
//   - 收集者:从外部来源收集资源
//   - 过滤器:按质量评价和过滤资源
//   - 发电机:从过滤的资源中产生出新想法
//   - 设计师:从想法创造设计规格
//   - 执行者:将设计纳入工作守则
//   - 验证器:测试和验证执行
//   - 作者:产生报告和文献
// ============================================================================

// RoleType 角色类型
type RoleType string

const (
	RoleCollector   RoleType = "collector"   // 资源收集者
	RoleFilter      RoleType = "filter"      // 质量过滤者
	RoleGenerator   RoleType = "generator"   // 想法生成者
	RoleDesigner    RoleType = "designer"    // 方案设计者
	RoleImplementer RoleType = "implementer" // 实现者
	RoleValidator   RoleType = "validator"   // 验证者
	RoleWriter      RoleType = "writer"      // 报告撰写者
	RoleCoordinator RoleType = "coordinator" // 协调者
	RoleCustom      RoleType = "custom"      // 自定义角色
)

// RoleStatus 角色状态
type RoleStatus string

const (
	RoleStatusIdle    RoleStatus = "idle"     // 空闲
	RoleStatusActive  RoleStatus = "active"   // 活跃
	RoleStatusBlocked RoleStatus = "blocked"  // 阻塞
	RoleStatusDone    RoleStatus = "done"     // 完成
	RoleStatusFailed  RoleStatus = "failed"   // 失败
)

// RoleCapability 角色能力定义
type RoleCapability struct {
	Name        string   `json:"name"`        // 能力名称
	Description string   `json:"description"` // 能力描述
	Tools       []string `json:"tools"`       // 可用工具列表
	InputTypes  []string `json:"input_types"` // 接受的输入类型
	OutputTypes []string `json:"output_types"` // 产出的输出类型
}

// RoleDefinition 角色定义
type RoleDefinition struct {
	Type          RoleType         `json:"type"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	SystemPrompt  string           `json:"system_prompt"`   // 角色的系统提示词
	Capabilities  []RoleCapability `json:"capabilities"`
	Dependencies  []RoleType       `json:"dependencies"`    // 依赖的前置角色
	MaxConcurrent int              `json:"max_concurrent"`  // 最大并发实例数
	Timeout       time.Duration    `json:"timeout"`         // 角色执行超时
	RetryPolicy   *RetryPolicy     `json:"retry_policy"`    // 重试策略
	Priority      int              `json:"priority"`        // 优先级 (越高越优先)
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries int           `json:"max_retries"` // 最大重试次数
	Delay      time.Duration `json:"delay"`       // 重试间隔
	BackoffMul float64       `json:"backoff_mul"` // 退避乘数
}

// RoleInstance 角色实例（运行时状态）
type RoleInstance struct {
	ID         string         `json:"id"`
	Definition RoleDefinition `json:"definition"`
	AgentID    string         `json:"agent_id"`    // 绑定的 Agent ID
	Status     RoleStatus     `json:"status"`
	Input      interface{}    `json:"input,omitempty"`
	Output     interface{}    `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// RoleTransition 角色间的数据传递
type RoleTransition struct {
	FromRole RoleType    `json:"from_role"`
	ToRole   RoleType    `json:"to_role"`
	Data     interface{} `json:"data"`
	Timestamp time.Time  `json:"timestamp"`
}

// ============================================================================
// 书记官处
// ============================================================================

// RoleRegistry 角色注册表
type RoleRegistry struct {
	definitions map[RoleType]*RoleDefinition
	mu          sync.RWMutex
	logger      *zap.Logger
}

// NewRoleRegistry 创建角色注册表
func NewRoleRegistry(logger *zap.Logger) *RoleRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RoleRegistry{
		definitions: make(map[RoleType]*RoleDefinition),
		logger:      logger,
	}
}

// Register 注册角色定义
func (r *RoleRegistry) Register(def *RoleDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[def.Type]; exists {
		return fmt.Errorf("role %s already registered", def.Type)
	}

	r.definitions[def.Type] = def
	r.logger.Info("role registered",
		zap.String("type", string(def.Type)),
		zap.String("name", def.Name))
	return nil
}

// Get 获取角色定义
func (r *RoleRegistry) Get(roleType RoleType) (*RoleDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.definitions[roleType]
	return def, ok
}

// List 列出所有角色定义
func (r *RoleRegistry) List() []*RoleDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]*RoleDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, def)
	}
	return defs
}

// Unregister 注销角色定义
func (r *RoleRegistry) Unregister(roleType RoleType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.definitions[roleType]; !exists {
		return fmt.Errorf("role %s not found", roleType)
	}

	delete(r.definitions, roleType)
	r.logger.Info("role unregistered", zap.String("type", string(roleType)))
	return nil
}

// ============================================================================
// 角色管道 Orchestrator( 管道管管)
// ============================================================================

// PipelineConfig 流水线配置
type PipelineConfig struct {
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	MaxConcurrency int           `json:"max_concurrency"` // 最大并行角色数
	Timeout        time.Duration `json:"timeout"`         // 整体超时
	StopOnFailure  bool          `json:"stop_on_failure"` // 失败时停止
}

// DefaultPipelineConfig 返回默认流水线配置
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		Name:           "default-pipeline",
		MaxConcurrency: 3,
		Timeout:        30 * time.Minute,
		StopOnFailure:  true,
	}
}

// RolePipeline 角色流水线编排器
// 按照依赖关系自动编排角色执行顺序
type RolePipeline struct {
	config     PipelineConfig
	registry   *RoleRegistry
	stages     [][]RoleType           // 执行阶段（每个阶段内的角色可并行）
	instances  map[string]*RoleInstance
	transitions []RoleTransition
	executeFn  RoleExecuteFunc        // 角色执行函数
	logger     *zap.Logger
	mu         sync.RWMutex
}

// RoleExecuteFunc 角色执行函数签名
// 接收角色定义和输入，返回输出
type RoleExecuteFunc func(ctx context.Context, role *RoleDefinition, input interface{}) (interface{}, error)

// NewRolePipeline 创建角色流水线
func NewRolePipeline(config PipelineConfig, registry *RoleRegistry, executeFn RoleExecuteFunc, logger *zap.Logger) *RolePipeline {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RolePipeline{
		config:      config,
		registry:    registry,
		stages:      make([][]RoleType, 0),
		instances:   make(map[string]*RoleInstance),
		transitions: make([]RoleTransition, 0),
		executeFn:   executeFn,
		logger:      logger,
	}
}

// AddStage 添加执行阶段（阶段内角色可并行执行）
func (p *RolePipeline) AddStage(roles ...RoleType) *RolePipeline {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 对所有角色进行验证
	for _, role := range roles {
		if _, ok := p.registry.Get(role); !ok {
			p.logger.Warn("role not registered, skipping", zap.String("role", string(role)))
		}
	}

	p.stages = append(p.stages, roles)
	return p
}

// Execute 执行流水线
func (p *RolePipeline) Execute(ctx context.Context, initialInput interface{}) (map[RoleType]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	p.logger.Info("starting role pipeline",
		zap.String("name", p.config.Name),
		zap.Int("stages", len(p.stages)))

	results := make(map[RoleType]interface{})
	currentInput := initialInput

	for stageIdx, stage := range p.stages {
		p.logger.Info("executing pipeline stage",
			zap.Int("stage", stageIdx+1),
			zap.Int("roles", len(stage)))

		stageResults, err := p.executeStage(ctx, stage, currentInput, results)
		if err != nil {
			if p.config.StopOnFailure {
				return results, fmt.Errorf("stage %d failed: %w", stageIdx+1, err)
			}
			p.logger.Warn("stage failed, continuing", zap.Int("stage", stageIdx+1), zap.Error(err))
		}

		// 合并阶段结果
		for role, output := range stageResults {
			results[role] = output
			// 使用最后阶段的输出作为下一阶段的输入
			currentInput = output
		}
	}

	p.logger.Info("role pipeline completed",
		zap.String("name", p.config.Name),
		zap.Int("total_results", len(results)))

	return results, nil
}

// executeStage 执行单个阶段（阶段内角色并行）
func (p *RolePipeline) executeStage(
	ctx context.Context,
	roles []RoleType,
	input interface{},
	previousResults map[RoleType]interface{},
) (map[RoleType]interface{}, error) {
	results := make(map[RoleType]interface{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	// 限制货币
	sem := make(chan struct{}, p.config.MaxConcurrency)

	for _, roleType := range roles {
		def, ok := p.registry.Get(roleType)
		if !ok {
			p.logger.Warn("role not found, skipping", zap.String("role", string(roleType)))
			continue
		}

		// 确定输入:如果可用,使用依赖性输出
		roleInput := input
		for _, dep := range def.Dependencies {
			if depOutput, ok := previousResults[dep]; ok {
				roleInput = depOutput
				break
			}
		}

		wg.Add(1)
		go func(rt RoleType, rd *RoleDefinition, ri interface{}) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			instance := &RoleInstance{
				ID:         fmt.Sprintf("%s_%d", rt, time.Now().UnixNano()),
				Definition: *rd,
				Status:     RoleStatusActive,
				Input:      ri,
				StartedAt:  time.Now(),
				Metadata:   make(map[string]any),
			}

			p.mu.Lock()
			p.instances[instance.ID] = instance
			p.mu.Unlock()

			p.logger.Info("executing role",
				zap.String("role", string(rt)),
				zap.String("instance", instance.ID))

			// 以超时执行
			roleCtx := ctx
			if rd.Timeout > 0 {
				var roleCancel context.CancelFunc
				roleCtx, roleCancel = context.WithTimeout(ctx, rd.Timeout)
				defer roleCancel()
			}

			// 用重试执行
			var output interface{}
			var err error
			maxAttempts := 1
			if rd.RetryPolicy != nil {
				maxAttempts = rd.RetryPolicy.MaxRetries + 1
			}

			for attempt := 0; attempt < maxAttempts; attempt++ {
				if attempt > 0 {
					delay := rd.RetryPolicy.Delay
					if rd.RetryPolicy.BackoffMul > 0 {
						for i := 0; i < attempt-1; i++ {
							delay = time.Duration(float64(delay) * rd.RetryPolicy.BackoffMul)
						}
					}
					select {
					case <-roleCtx.Done():
						err = roleCtx.Err()
						break
					case <-time.After(delay):
					}
				}

				output, err = p.executeFn(roleCtx, rd, ri)
				if err == nil {
					break
				}
				p.logger.Warn("role execution failed, retrying",
					zap.String("role", string(rt)),
					zap.Int("attempt", attempt+1),
					zap.Error(err))
			}

			now := time.Now()
			instance.CompletedAt = &now

			if err != nil {
				instance.Status = RoleStatusFailed
				instance.Error = err.Error()
				p.logger.Error("role failed",
					zap.String("role", string(rt)),
					zap.Error(err))

				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("role %s failed: %w", rt, err)
				}
				mu.Unlock()
				return
			}

			instance.Status = RoleStatusDone
			instance.Output = output

			mu.Lock()
			results[rt] = output
			mu.Unlock()

			// 记录过渡
			p.mu.Lock()
			p.transitions = append(p.transitions, RoleTransition{
				FromRole:  rt,
				ToRole:    "", // Will be filled when next stage runs
				Data:      output,
				Timestamp: time.Now(),
			})
			p.mu.Unlock()

			p.logger.Info("role completed",
				zap.String("role", string(rt)),
				zap.Duration("duration", now.Sub(instance.StartedAt)))
		}(roleType, def, roleInput)
	}

	wg.Wait()
	return results, firstErr
}

// GetInstances 获取所有角色实例
func (p *RolePipeline) GetInstances() []*RoleInstance {
	p.mu.RLock()
	defer p.mu.RUnlock()

	instances := make([]*RoleInstance, 0, len(p.instances))
	for _, inst := range p.instances {
		instances = append(instances, inst)
	}
	return instances
}

// GetTransitions 获取所有角色转换记录
func (p *RolePipeline) GetTransitions() []RoleTransition {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]RoleTransition{}, p.transitions...)
}

// ============================================================================
// 预定义的研究角色
// ============================================================================

// NewResearchCollectorRole 创建研究资源收集者角色
func NewResearchCollectorRole() *RoleDefinition {
	return &RoleDefinition{
		Type:        RoleCollector,
		Name:        "Research Collector",
		Description: "Collects research papers, code repositories, and datasets from academic databases and code platforms.",
		SystemPrompt: "You are a research resource collector. Your job is to find relevant papers, code, and datasets for the given research topic. Search arXiv, IEEE Xplore, GitHub, and HuggingFace for the most relevant and recent resources.",
		Capabilities: []RoleCapability{
			{
				Name:        "paper_search",
				Description: "Search academic databases for papers",
				Tools:       []string{"web_search", "arxiv_search"},
				InputTypes:  []string{"research_topic"},
				OutputTypes: []string{"paper_list"},
			},
			{
				Name:        "code_search",
				Description: "Search code repositories",
				Tools:       []string{"web_search", "github_search"},
				InputTypes:  []string{"research_topic"},
				OutputTypes: []string{"repo_list"},
			},
		},
		MaxConcurrent: 2,
		Timeout:       10 * time.Minute,
		RetryPolicy:   &RetryPolicy{MaxRetries: 2, Delay: 5 * time.Second, BackoffMul: 2.0},
		Priority:      10,
	}
}

// NewResearchFilterRole 创建研究质量过滤者角色
func NewResearchFilterRole() *RoleDefinition {
	return &RoleDefinition{
		Type:         RoleFilter,
		Name:         "Quality Filter",
		Description:  "Evaluates and filters collected resources based on quality metrics like citations, recency, and relevance.",
		SystemPrompt: "You are a research quality evaluator. Assess each resource based on citation count, publication venue, recency, methodology quality, and relevance to the research topic. Filter out low-quality or irrelevant resources.",
		Capabilities: []RoleCapability{
			{
				Name:        "quality_assessment",
				Description: "Assess resource quality",
				Tools:       []string{},
				InputTypes:  []string{"paper_list", "repo_list"},
				OutputTypes: []string{"filtered_paper_list", "filtered_repo_list"},
			},
		},
		Dependencies:  []RoleType{RoleCollector},
		MaxConcurrent: 1,
		Timeout:       5 * time.Minute,
		Priority:      9,
	}
}

// NewResearchGeneratorRole 创建研究想法生成者角色
func NewResearchGeneratorRole() *RoleDefinition {
	return &RoleDefinition{
		Type:         RoleGenerator,
		Name:         "Idea Generator",
		Description:  "Generates novel research ideas by analyzing gaps and trends in filtered resources.",
		SystemPrompt: "You are a creative research scientist. Analyze the provided papers and resources to identify research gaps, emerging trends, and novel combinations. Generate innovative research ideas that are both novel and feasible.",
		Capabilities: []RoleCapability{
			{
				Name:        "idea_generation",
				Description: "Generate novel research ideas",
				Tools:       []string{},
				InputTypes:  []string{"filtered_paper_list"},
				OutputTypes: []string{"research_ideas"},
			},
		},
		Dependencies:  []RoleType{RoleFilter},
		MaxConcurrent: 1,
		Timeout:       10 * time.Minute,
		Priority:      8,
	}
}

// NewResearchValidatorRole 创建研究验证者角色
func NewResearchValidatorRole() *RoleDefinition {
	return &RoleDefinition{
		Type:         RoleValidator,
		Name:         "Experiment Validator",
		Description:  "Validates implementations through experiments, benchmarks, and statistical analysis.",
		SystemPrompt: "You are a rigorous experimental scientist. Design and execute experiments to validate the implementation. Compare against baselines, perform statistical significance tests, and report results objectively.",
		Capabilities: []RoleCapability{
			{
				Name:        "experiment_design",
				Description: "Design validation experiments",
				Tools:       []string{},
				InputTypes:  []string{"implementation"},
				OutputTypes: []string{"experiment_plan"},
			},
			{
				Name:        "result_analysis",
				Description: "Analyze experimental results",
				Tools:       []string{},
				InputTypes:  []string{"experiment_results"},
				OutputTypes: []string{"validation_report"},
			},
		},
		Dependencies:  []RoleType{RoleImplementer},
		MaxConcurrent: 2,
		Timeout:       15 * time.Minute,
		RetryPolicy:   &RetryPolicy{MaxRetries: 1, Delay: 10 * time.Second, BackoffMul: 1.5},
		Priority:      7,
	}
}

// NewResearchWriterRole 创建研究报告撰写者角色
func NewResearchWriterRole() *RoleDefinition {
	return &RoleDefinition{
		Type:         RoleWriter,
		Name:         "Report Writer",
		Description:  "Generates comprehensive research reports and academic papers from validated results.",
		SystemPrompt: "You are an academic writer. Generate a well-structured research report that includes introduction, methodology, results, discussion, and conclusion. Follow academic writing conventions and cite all sources properly.",
		Capabilities: []RoleCapability{
			{
				Name:        "report_generation",
				Description: "Generate research reports",
				Tools:       []string{},
				InputTypes:  []string{"validation_report", "research_ideas"},
				OutputTypes: []string{"research_report"},
			},
		},
		Dependencies:  []RoleType{RoleValidator},
		MaxConcurrent: 1,
		Timeout:       10 * time.Minute,
		Priority:      6,
	}
}

// RegisterResearchRoles 注册所有预定义的研究角色
func RegisterResearchRoles(registry *RoleRegistry) error {
	roles := []*RoleDefinition{
		NewResearchCollectorRole(),
		NewResearchFilterRole(),
		NewResearchGeneratorRole(),
		NewResearchValidatorRole(),
		NewResearchWriterRole(),
	}

	for _, role := range roles {
		if err := registry.Register(role); err != nil {
			return fmt.Errorf("failed to register role %s: %w", role.Type, err)
		}
	}

	return nil
}
