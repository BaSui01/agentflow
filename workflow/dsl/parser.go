package dsl

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agent "github.com/BaSui01/agentflow/agent/execution/runtime"
	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/engine"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Parser DSL 解析器
type Parser struct {
	// stepRegistry 步骤注册表（step name -> Step 工厂函数）
	stepRegistry map[string]func(config map[string]any) (workflow.Step, error)
	// conditionRegistry 条件表达式注册表
	conditionRegistry map[string]workflow.ConditionFunc
	// stepDeps 为 engine-step integration 注入依赖（可选）
	stepDeps engine.StepDependencies
}

// NewParser 创建 DSL 解析器
func NewParser() *Parser {
	p := &Parser{
		stepRegistry:      make(map[string]func(config map[string]any) (workflow.Step, error)),
		conditionRegistry: make(map[string]workflow.ConditionFunc),
	}
	p.registerBuiltinSteps()
	return p
}

// RegisterStep 注册自定义步骤工厂
func (p *Parser) RegisterStep(name string, factory func(config map[string]any) (workflow.Step, error)) {
	p.stepRegistry[name] = factory
}

// RegisterCondition 注册命名条件
func (p *Parser) RegisterCondition(name string, fn workflow.ConditionFunc) {
	p.conditionRegistry[name] = fn
}

// WithStepDependencies configures shared dependencies for engine-backed step creation.
func (p *Parser) WithStepDependencies(deps engine.StepDependencies) *Parser {
	p.stepDeps = deps
	return p
}

// ParseFile 从文件解析 DSL
func (p *Parser) ParseFile(filename string) (*workflow.DAGWorkflow, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read DSL file: %w", err)
	}
	return p.Parse(data)
}

// Parse 从 YAML 字节解析 DSL
func (p *Parser) Parse(data []byte) (*workflow.DAGWorkflow, error) {
	var dsl WorkflowDSL
	if err := yaml.Unmarshal(data, &dsl); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	// 1. 验证 DSL
	if err := p.validate(&dsl); err != nil {
		return nil, fmt.Errorf("validate DSL: %w", err)
	}

	// 2. 解析变量，构建插值上下文
	vars := p.resolveVariables(dsl.Variables)

	// 3. 构建并验证 DAGWorkflow（统一走 DAGBuilder）
	wf, err := p.buildWorkflow(dsl.Name, dsl.Description, dsl.Workflow, &dsl, vars)
	if err != nil {
		return nil, fmt.Errorf("build workflow: %w", err)
	}

	// 4. 注入 workflow metadata
	for k, v := range dsl.Metadata {
		wf.SetMetadata(k, v)
	}

	return wf, nil
}

// validate 验证 DSL
func (p *Parser) validate(dsl *WorkflowDSL) error {
	v := NewValidator()
	errs := v.Validate(dsl)
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("validation errors: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// resolveVariables 解析变量默认值
func (p *Parser) resolveVariables(varDefs map[string]VariableDef) map[string]any {
	vars := make(map[string]any)
	for name, def := range varDefs {
		if def.Default != nil {
			vars[name] = def.Default
		}
	}
	return vars
}

// interpolate 变量插值（替换 ${var_name}）
func (p *Parser) interpolate(template string, vars map[string]any) string {
	result := template
	for name, value := range vars {
		placeholder := "${" + name + "}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}
	return result
}

// buildWorkflow 从 DSL 节点定义构建 DAGWorkflow。
func (p *Parser) buildWorkflow(
	name string,
	description string,
	nodesDef WorkflowNodesDef,
	dsl *WorkflowDSL,
	vars map[string]any,
) (*workflow.DAGWorkflow, error) {
	builder := workflow.NewDAGBuilder(name).
		WithDescription(description).
		WithLogger(zap.NewNop())

	for _, nodeDef := range nodesDef.Nodes {
		node, err := p.buildNode(&nodeDef, dsl, vars)
		if err != nil {
			return nil, fmt.Errorf("build node %s: %w", nodeDef.ID, err)
		}

		nodeBuilder := builder.AddNode(node.ID, node.Type)
		switch node.Type {
		case workflow.NodeTypeAction:
			if node.Step != nil {
				nodeBuilder.WithStep(node.Step)
			}
		case workflow.NodeTypeCondition:
			if node.Condition != nil {
				nodeBuilder.WithCondition(node.Condition)
			}
			if len(nodeDef.OnTrue) > 0 {
				nodeBuilder.WithOnTrue(nodeDef.OnTrue...)
			}
			if len(nodeDef.OnFalse) > 0 {
				nodeBuilder.WithOnFalse(nodeDef.OnFalse...)
			}
		case workflow.NodeTypeLoop:
			if node.LoopConfig != nil {
				nodeBuilder.WithLoop(*node.LoopConfig)
			}
		case workflow.NodeTypeSubGraph:
			if node.SubGraph != nil {
				nodeBuilder.WithSubGraph(node.SubGraph)
			}
		}

		for k, v := range node.Metadata {
			if k == "on_true" || k == "on_false" {
				continue
			}
			nodeBuilder.WithMetadata(k, v)
		}

		if node.ErrorConfig != nil {
			nodeBuilder.WithErrorConfig(*node.ErrorConfig)
		}
		nodeBuilder.Done()

		for _, nextID := range nodeDef.Next {
			builder.AddEdge(nodeDef.ID, nextID)
		}
		for _, trueID := range nodeDef.OnTrue {
			builder.AddEdge(nodeDef.ID, trueID)
		}
		for _, falseID := range nodeDef.OnFalse {
			builder.AddEdge(nodeDef.ID, falseID)
		}
	}

	builder.SetEntry(nodesDef.Entry)
	wf, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return wf, nil
}

// buildNode 构建单个节点
func (p *Parser) buildNode(def *NodeDef, dsl *WorkflowDSL, vars map[string]any) (*workflow.DAGNode, error) {
	node := &workflow.DAGNode{
		ID:       def.ID,
		Type:     workflow.NodeType(def.Type),
		Metadata: make(map[string]any),
	}

	// 复制 metadata
	for k, v := range def.Metadata {
		node.Metadata[k] = v
	}

	switch workflow.NodeType(def.Type) {
	case workflow.NodeTypeAction:
		step, err := p.resolveStep(def, dsl, vars)
		if err != nil {
			return nil, err
		}
		node.Step = step

	case workflow.NodeTypeCondition:
		condFn, err := p.resolveCondition(def.Condition, vars)
		if err != nil {
			return nil, err
		}
		node.Condition = condFn
		if len(def.OnTrue) > 0 {
			node.Metadata["on_true"] = def.OnTrue
		}
		if len(def.OnFalse) > 0 {
			node.Metadata["on_false"] = def.OnFalse
		}

	case workflow.NodeTypeLoop:
		if def.Loop == nil {
			return nil, fmt.Errorf("loop node requires loop definition")
		}
		loopConfig, err := p.resolveLoop(def.Loop, vars)
		if err != nil {
			return nil, err
		}
		node.LoopConfig = loopConfig

	case workflow.NodeTypeParallel:
		// parallel 节点的边在 buildGraph 中处理

	case workflow.NodeTypeSubGraph:
		if def.SubGraph != nil {
			subWf, err := p.buildWorkflow(dsl.Name+"_sub", "subgraph", *def.SubGraph, dsl, vars)
			if err != nil {
				return nil, fmt.Errorf("build subgraph: %w", err)
			}
			node.SubGraph = subWf.Graph()
		}
	}

	// 错误处理配置
	if def.Error != nil {
		node.ErrorConfig = &workflow.ErrorConfig{
			Strategy:      workflow.ErrorStrategy(def.Error.Strategy),
			MaxRetries:    def.Error.MaxRetries,
			RetryDelayMs:  def.Error.RetryDelayMs,
			FallbackValue: def.Error.FallbackValue,
		}
	}

	return node, nil
}

// resolveStep 解析步骤（引用或内联）
func (p *Parser) resolveStep(def *NodeDef, dsl *WorkflowDSL, vars map[string]any) (workflow.Step, error) {
	var stepDef *StepDef

	if def.StepDef != nil {
		// 内联步骤定义
		stepDef = def.StepDef
	} else if def.Step != "" {
		// 引用已定义的步骤
		sd, ok := dsl.Steps[def.Step]
		if !ok {
			return nil, fmt.Errorf("step %q not found in steps definitions", def.Step)
		}
		stepDef = &sd
	} else {
		return nil, fmt.Errorf("action node requires step or step_def")
	}

	// 根据类型创建 Step
	switch stepDef.Type {
	case string(core.StepTypeLLM):
		prompt := p.interpolate(stepDef.Prompt, vars)
		// Resolve model name: if an agent is referenced, look up its model;
		// otherwise fall back to config["model"] or empty string.
		model := ""
		if stepDef.Agent != "" {
			if agentDef, ok := dsl.Agents[stepDef.Agent]; ok {
				model = agentDef.Model
			}
		}
		if model == "" {
			if m, ok := stepDef.Config["model"].(string); ok {
				model = m
			}
		}
		spec := engine.StepSpec{
			ID:          def.ID,
			Type:        core.StepTypeLLM,
			Model:       model,
			Prompt:      prompt,
			Temperature: readFloat64(stepDef.Config, "temperature"),
			MaxTokens:   readInt(stepDef.Config, "max_tokens"),
		}
		return p.newEngineBackedStep(spec, "llm")

	case string(core.StepTypeTool):
		params := make(map[string]any)
		for k, v := range stepDef.Config {
			if s, ok := v.(string); ok {
				params[k] = p.interpolate(s, vars)
			} else {
				params[k] = v
			}
		}
		spec := engine.StepSpec{
			ID:         def.ID,
			Type:       core.StepTypeTool,
			ToolName:   stepDef.Tool,
			ToolParams: params,
		}
		return p.newEngineBackedStep(spec, stepDef.Tool)

	case string(core.StepTypeHumanInput):
		spec := engine.StepSpec{
			ID:          def.ID,
			Type:        core.StepTypeHuman,
			InputPrompt: p.interpolate(stepDef.Prompt, vars),
		}
		if inputType, ok := stepDef.Config["type"].(string); ok {
			spec.InputType = inputType
		}
		return p.newEngineBackedStep(spec, string(core.StepTypeHumanInput))

	case string(core.StepTypeCode):
		spec := engine.StepSpec{
			ID:   def.ID,
			Type: core.StepTypeCode,
		}
		return p.newEngineBackedStep(spec, "code")

	case string(core.StepTypeAgent):
		if stepDef.InlineAgent != nil {
			return nil, fmt.Errorf("agent step does not support inline_agent")
		}
		spec := engine.StepSpec{
			ID:      def.ID,
			Type:    core.StepTypeAgent,
			AgentID: stepDef.Agent,
		}
		return p.newEngineBackedStep(spec, "agent")

	case string(core.StepTypeOrchestration):
		if stepDef.Orchestration == nil {
			return nil, fmt.Errorf("orchestration step requires orchestration definition")
		}
		spec := engine.StepSpec{
			ID:                     def.ID,
			Type:                   core.StepTypeOrchestration,
			OrchestrationMode:      stepDef.Orchestration.Mode,
			OrchestrationAgents:    append([]string(nil), stepDef.Orchestration.AgentIDs...),
			OrchestrationMaxRounds: stepDef.Orchestration.MaxRounds,
		}
		if stepDef.Orchestration.TimeoutMs > 0 {
			spec.OrchestrationTimeout = time.Duration(stepDef.Orchestration.TimeoutMs) * time.Millisecond
		}
		return p.newEngineBackedStep(spec, "orchestration")

	case string(core.StepTypeChain):
		if stepDef.Chain == nil {
			return nil, fmt.Errorf("chain step requires chain definition")
		}
		chainSteps := make([]tools.ChainStep, len(stepDef.Chain.Steps))
		for i, ce := range stepDef.Chain.Steps {
			args := make(map[string]any)
			for k, v := range ce.Args {
				if s, ok := v.(string); ok {
					args[k] = p.interpolate(s, vars)
				} else {
					args[k] = v
				}
			}
			chainSteps[i] = tools.ChainStep{
				ToolName:   ce.Tool,
				Args:       args,
				ArgMapping: ce.ArgMapping,
				OnError:    ce.OnError,
				MaxRetries: ce.MaxRetries,
			}
		}
		spec := engine.StepSpec{
			ID:         def.ID,
			Type:       core.StepTypeChain,
			ChainSteps: chainSteps,
		}
		return p.newEngineBackedStep(spec, "chain")

	case string(core.StepTypePassthrough):
		return &workflow.PassthroughStep{}, nil

	default:
		// 查找注册的自定义步骤
		factory, ok := p.stepRegistry[stepDef.Type]
		if !ok {
			return nil, fmt.Errorf("unknown step type: %s", stepDef.Type)
		}
		return factory(stepDef.Config)
	}
}

// resolveCondition 解析条件表达式
func (p *Parser) resolveCondition(expr string, vars map[string]any) (workflow.ConditionFunc, error) {
	// 1. 检查是否是注册的命名条件
	if fn, ok := p.conditionRegistry[expr]; ok {
		return fn, nil
	}

	// 2. 简单表达式解析
	return p.parseSimpleExpression(expr, vars)
}

// parseSimpleExpression 解析条件表达式。
// 支持比较运算符 (==, !=, >, <, >=, <=)、逻辑运算符 (&&, ||, !)、
// 字段访问 (result.score)、字面量 (数字、字符串、布尔) 和括号分组。
func (p *Parser) parseSimpleExpression(expr string, vars map[string]any) (workflow.ConditionFunc, error) {
	eval := &exprEvaluator{}
	return func(_ context.Context, input any) (bool, error) {
		// Merge static vars with runtime input
		mergedVars := make(map[string]any, len(vars)+1)
		for k, v := range vars {
			mergedVars[k] = v
		}
		if inputMap, ok := input.(map[string]any); ok {
			for k, v := range inputMap {
				mergedVars[k] = v
			}
		} else if input != nil {
			mergedVars["input"] = input
		}

		// Interpolate ${var} placeholders first
		resolved := p.interpolate(expr, mergedVars)

		// Try expression evaluation
		result, err := eval.Evaluate(resolved, mergedVars)
		if err != nil {
			return false, fmt.Errorf("evaluate condition expression %q: %w", resolved, err)
		}
		return result, nil
	}, nil
}

func (p *Parser) newEngineBackedStep(spec engine.StepSpec, name string) (workflow.Step, error) {
	node, err := engine.BuildExecutionNode(spec, p.effectiveStepDeps())
	if err != nil {
		return nil, err
	}
	return &protocolStepAdapter{
		name:     name,
		stepType: spec.Type,
		step:     node.Step,
	}, nil
}

func (p *Parser) effectiveStepDeps() engine.StepDependencies {
	deps := p.stepDeps
	if deps.Gateway == nil {
		deps.Gateway = noopGateway{}
	}
	if deps.ToolRegistry == nil {
		deps.ToolRegistry = noopToolRegistry{}
	}
	if deps.HumanHandler == nil {
		deps.HumanHandler = noopHumanHandler{}
	}
	if deps.AgentExecutor == nil {
		deps.AgentExecutor = noopAgentExecutor{}
	}
	if deps.AgentResolver == nil {
		deps.AgentResolver = noopAgentResolver{}
	}
	if deps.CodeHandler == nil {
		deps.CodeHandler = func(ctx context.Context, input core.StepInput) (map[string]any, error) {
			return nil, fmt.Errorf("step dependency not configured")
		}
	}
	return deps
}

func readInt(cfg map[string]any, key string) int {
	if cfg == nil {
		return 0
	}
	v, ok := cfg[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func readFloat64(cfg map[string]any, key string) float64 {
	if cfg == nil {
		return 0
	}
	v, ok := cfg[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

type protocolStepAdapter struct {
	name     string
	stepType core.StepType
	step     core.StepProtocol
}

type noopGateway struct{}

func (noopGateway) Invoke(ctx context.Context, req *core.LLMRequest) (*core.LLMResponse, error) {
	return nil, fmt.Errorf("step dependency not configured")
}

func (noopGateway) Stream(ctx context.Context, req *core.LLMRequest) (<-chan core.LLMStreamChunk, error) {
	return nil, fmt.Errorf("step dependency not configured")
}

type noopToolRegistry struct{}

func (noopToolRegistry) ExecuteTool(ctx context.Context, name string, params map[string]any) (any, error) {
	return nil, fmt.Errorf("step dependency not configured")
}

type noopHumanHandler struct{}

func (noopHumanHandler) RequestInput(ctx context.Context, prompt string, inputType string, options []string) (*core.HumanInputResult, error) {
	return nil, fmt.Errorf("step dependency not configured")
}

type noopAgentExecutor struct{}

func (noopAgentExecutor) Execute(ctx context.Context, input map[string]any) (*core.AgentExecutionOutput, error) {
	return nil, fmt.Errorf("step dependency not configured")
}

type noopAgentResolver struct{}

func (noopAgentResolver) ResolveAgent(ctx context.Context, agentID string) (agent.Agent, error) {
	return nil, fmt.Errorf("step dependency not configured")
}

func (s *protocolStepAdapter) Name() string {
	if s.name != "" {
		return s.name
	}
	return string(s.stepType)
}

func (s *protocolStepAdapter) Execute(ctx context.Context, input any) (any, error) {
	stepInput := core.StepInput{Data: make(map[string]any)}
	if inputMap, ok := input.(map[string]any); ok {
		stepInput.Data = inputMap
	} else if input != nil {
		stepInput.Data["input"] = input
	}

	nodeID := s.step.ID()
	result, err := engine.NewExecutor().Execute(
		ctx,
		engine.ModeSequential,
		[]*engine.ExecutionNode{
			{
				ID:    nodeID,
				Step:  s.step,
				Input: stepInput,
			},
		},
		engine.DefaultStepRunner,
	)
	if err != nil {
		return nil, err
	}
	out := result.Outputs[nodeID]

	switch s.stepType {
	case core.StepTypeLLM:
		if v, ok := out.Data["content"]; ok {
			return v, nil
		}
	case core.StepTypeTool:
		if v, ok := out.Data["result"]; ok {
			return v, nil
		}
	case core.StepTypeHuman:
		if v, ok := out.Data["input"]; ok {
			return v, nil
		}
	case core.StepTypeOrchestration:
		if v, ok := out.Data["result"]; ok {
			return v, nil
		}
	case core.StepTypeChain:
		if v, ok := out.Data["result"]; ok {
			return v, nil
		}
	}
	if len(out.Data) == 1 {
		for _, v := range out.Data {
			return v, nil
		}
	}
	return out.Data, nil
}

// resolveLoop 解析循环配置
func (p *Parser) resolveLoop(def *LoopDef, vars map[string]any) (*workflow.LoopConfig, error) {
	config := &workflow.LoopConfig{
		Type:          workflow.LoopType(def.Type),
		MaxIterations: def.MaxIterations,
	}

	if def.Condition != "" {
		condFn, err := p.resolveCondition(def.Condition, vars)
		if err != nil {
			return nil, err
		}
		config.Condition = condFn
	}

	return config, nil
}

// registerBuiltinSteps 注册内置步骤
func (p *Parser) registerBuiltinSteps() {
	p.RegisterStep("passthrough", func(_ map[string]any) (workflow.Step, error) {
		return &workflow.PassthroughStep{}, nil
	})
}
