package agent

import (
	"context"
	"fmt"
	"sync"
)

// ============================================================
// 插件系统
// 为扩展代理能力提供可插件架构.
// ============================================================

// 插件Type 定义了插件的类型 。
type PluginType string

const (
	PluginTypePreProcess  PluginType = "pre_process"  // Runs before execution
	PluginTypePostProcess PluginType = "post_process" // Runs after execution
	PluginTypeMiddleware  PluginType = "middleware"   // Wraps execution
	PluginTypeExtension   PluginType = "extension"    // Adds new capabilities
)

// 插件定义代理插件的接口 。
type Plugin interface {
	// 名称返回插件名称 。
	Name() string
	// 类型返回插件类型 。
	Type() PluginType
	// Init 初始化插件 。
	Init(ctx context.Context) error
	// 关闭清理插件 。
	Close(ctx context.Context) error
}

// PrecessPlugin 在代理执行前运行.
type PreProcessPlugin interface {
	Plugin
	// Precess 执行前处理输入.
	PreProcess(ctx context.Context, input *Input) (*Input, error)
}

// 后ProcessPlugin在代理执行后运行.
type PostProcessPlugin interface {
	Plugin
	// 程序执行后处理输出 。
	PostProcess(ctx context.Context, output *Output) (*Output, error)
}

// MiddlewarePlugin 包装代理执行 。
type MiddlewarePlugin interface {
	Plugin
	// 环绕执行函数 。
	Wrap(next func(ctx context.Context, input *Input) (*Output, error)) func(ctx context.Context, input *Input) (*Output, error)
}

// ============================================================
// 插件登记
// ============================================================

// 插件注册管理插件注册和生命周期.
type PluginRegistry struct {
	plugins      map[string]Plugin
	preProcess   []PreProcessPlugin
	postProcess  []PostProcessPlugin
	middleware   []MiddlewarePlugin
	mu           sync.RWMutex
	initialized  bool
}

// NewPluginRegistry创建了新的插件注册.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins:     make(map[string]Plugin),
		preProcess:  make([]PreProcessPlugin, 0),
		postProcess: make([]PostProcessPlugin, 0),
		middleware:  make([]MiddlewarePlugin, 0),
	}
}

// 注册注册插件 。
func (r *PluginRegistry) Register(plugin Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := plugin.Name()
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin already registered: %s", name)
	}

	r.plugins[name] = plugin

	// 按类型分类
	switch p := plugin.(type) {
	case PreProcessPlugin:
		r.preProcess = append(r.preProcess, p)
	case PostProcessPlugin:
		r.postProcess = append(r.postProcess, p)
	case MiddlewarePlugin:
		r.middleware = append(r.middleware, p)
	}

	return nil
}

// 未注册删除插件 。
func (r *PluginRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	delete(r.plugins, name)

	// 从分类列表中删除
	switch p := plugin.(type) {
	case PreProcessPlugin:
		r.preProcess = removePreProcess(r.preProcess, p)
	case PostProcessPlugin:
		r.postProcess = removePostProcess(r.postProcess, p)
	case MiddlewarePlugin:
		r.middleware = removeMiddleware(r.middleware, p)
	}

	return nil
}

// 获取一个名称的插件 。
func (r *PluginRegistry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plugin, ok := r.plugins[name]
	return plugin, ok
}

// 列表返回所有已注册的插件 。
func (r *PluginRegistry) List() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// Init 初始化所有插件 。
func (r *PluginRegistry) Init(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.initialized {
		return nil
	}

	for name, plugin := range r.plugins {
		if err := plugin.Init(ctx); err != nil {
			return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
		}
	}

	r.initialized = true
	return nil
}

// 关闭所有插件 。
func (r *PluginRegistry) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, plugin := range r.plugins {
		if err := plugin.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to close plugin %s: %w", name, err))
		}
	}

	r.initialized = false

	if len(errs) > 0 {
		return fmt.Errorf("errors closing plugins: %v", errs)
	}
	return nil
}

// PrecessPlugins 返回所有预处理插件 。
func (r *PluginRegistry) PreProcessPlugins() []PreProcessPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]PreProcessPlugin{}, r.preProcess...)
}

// PostProcessPlugins 返回所有后进程插件.
func (r *PluginRegistry) PostProcessPlugins() []PostProcessPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]PostProcessPlugin{}, r.postProcess...)
}

// MiddlewarePlugins返回所有中间软件插件.
func (r *PluginRegistry) MiddlewarePlugins() []MiddlewarePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]MiddlewarePlugin{}, r.middleware...)
}

// 从切片中删除插件的辅助功能
func removePreProcess(slice []PreProcessPlugin, plugin PreProcessPlugin) []PreProcessPlugin {
	for i, p := range slice {
		if p.Name() == plugin.Name() {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func removePostProcess(slice []PostProcessPlugin, plugin PostProcessPlugin) []PostProcessPlugin {
	for i, p := range slice {
		if p.Name() == plugin.Name() {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func removeMiddleware(slice []MiddlewarePlugin, plugin MiddlewarePlugin) []MiddlewarePlugin {
	for i, p := range slice {
		if p.Name() == plugin.Name() {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// ============================================================
// 插件启用代理
// ============================================================

// 插件可启用代理用插件支持将代理包入。
type PluginEnabledAgent struct {
	agent    Agent
	registry *PluginRegistry
}

// NewPluginEnabled Agent 创建了插件启用的代理包.
func NewPluginEnabledAgent(agent Agent, registry *PluginRegistry) *PluginEnabledAgent {
	if registry == nil {
		registry = NewPluginRegistry()
	}
	return &PluginEnabledAgent{
		agent:    agent,
		registry: registry,
	}
}

// ID返回代理ID.
func (a *PluginEnabledAgent) ID() string { return a.agent.ID() }

// 名称返回代理名称 。
func (a *PluginEnabledAgent) Name() string { return a.agent.Name() }

// 类型返回代理类型。
func (a *PluginEnabledAgent) Type() AgentType { return a.agent.Type() }

// 国家归还代理国.
func (a *PluginEnabledAgent) State() State { return a.agent.State() }

// 初始化代理和插件 。
func (a *PluginEnabledAgent) Init(ctx context.Context) error {
	// 首先初始化插件
	if err := a.registry.Init(ctx); err != nil {
		return err
	}
	return a.agent.Init(ctx)
}

// 倒地清理代理和插件.
func (a *PluginEnabledAgent) Teardown(ctx context.Context) error {
	// 抢先拆掉
	if err := a.agent.Teardown(ctx); err != nil {
		return err
	}
	return a.registry.Close(ctx)
}

// 计划产生一个执行计划。
func (a *PluginEnabledAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	return a.agent.Plan(ctx, input)
}

// 用插件管道执行执行 。
func (a *PluginEnabledAgent) Execute(ctx context.Context, input *Input) (*Output, error) {
	var err error

	// 运行预处理插件
	for _, plugin := range a.registry.PreProcessPlugins() {
		input, err = plugin.PreProcess(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("pre-process plugin %s failed: %w", plugin.Name(), err)
		}
	}

	// 用中间软件构建执行链
	execFunc := a.agent.Execute
	for i := len(a.registry.MiddlewarePlugins()) - 1; i >= 0; i-- {
		execFunc = a.registry.MiddlewarePlugins()[i].Wrap(execFunc)
	}

	// 执行
	output, err := execFunc(ctx, input)
	if err != nil {
		return nil, err
	}

	// 运行进程后插件
	for _, plugin := range a.registry.PostProcessPlugins() {
		output, err = plugin.PostProcess(ctx, output)
		if err != nil {
			return nil, fmt.Errorf("post-process plugin %s failed: %w", plugin.Name(), err)
		}
	}

	return output, nil
}

// 观察处理反馈.
func (a *PluginEnabledAgent) Observe(ctx context.Context, feedback *Feedback) error {
	return a.agent.Observe(ctx, feedback)
}

// 注册返回插件注册 。
func (a *PluginEnabledAgent) Registry() *PluginRegistry {
	return a.registry
}

// 地下特工还原被包裹的特工.
func (a *PluginEnabledAgent) UnderlyingAgent() Agent {
	return a.agent
}
