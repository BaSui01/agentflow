package dsl

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/BaSui01/agentflow/workflow"
	"gopkg.in/yaml.v3"
)

// Parser DSL 解析器
type Parser struct {
	// stepRegistry 步骤注册表（step name -> Step 工厂函数）
	stepRegistry map[string]func(config map[string]interface{}) (workflow.Step, error)
	// conditionRegistry 条件表达式注册表
	conditionRegistry map[string]workflow.ConditionFunc
}

// NewParser 创建 DSL 解析器
func NewParser() *Parser {
	p := &Parser{
		stepRegistry:      make(map[string]func(config map[string]interface{}) (workflow.Step, error)),
		conditionRegistry: make(map[string]workflow.ConditionFunc),
	}
	p.registerBuiltinSteps()
	return p
}

// RegisterStep 注册自定义步骤工厂
func (p *Parser) RegisterStep(name string, factory func(config map[string]interface{}) (workflow.Step, error)) {
	p.stepRegistry[name] = factory
}

// RegisterCondition 注册命名条件
func (p *Parser) RegisterCondition(name string, fn workflow.ConditionFunc) {
	p.conditionRegistry[name] = fn
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

	// 3. 构建 DAGGraph
	graph, err := p.buildGraph(&dsl, vars)
	if err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	// 4. 创建 DAGWorkflow
	wf := workflow.NewDAGWorkflow(dsl.Name, dsl.Description, graph)
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
func (p *Parser) resolveVariables(varDefs map[string]VariableDef) map[string]interface{} {
	vars := make(map[string]interface{})
	for name, def := range varDefs {
		if def.Default != nil {
			vars[name] = def.Default
		}
	}
	return vars
}

// interpolate 变量插值（替换 ${var_name}）
func (p *Parser) interpolate(template string, vars map[string]interface{}) string {
	result := template
	for name, value := range vars {
		placeholder := "${" + name + "}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}
	return result
}

// buildGraph 从 DSL 构建 DAGGraph
func (p *Parser) buildGraph(dsl *WorkflowDSL, vars map[string]interface{}) (*workflow.DAGGraph, error) {
	graph := workflow.NewDAGGraph()

	for _, nodeDef := range dsl.Workflow.Nodes {
		node, err := p.buildNode(&nodeDef, dsl, vars)
		if err != nil {
			return nil, fmt.Errorf("build node %s: %w", nodeDef.ID, err)
		}
		graph.AddNode(node)

		// 添加边
		for _, nextID := range nodeDef.Next {
			graph.AddEdge(nodeDef.ID, nextID)
		}

		// 条件节点的分支也作为边
		for _, trueID := range nodeDef.OnTrue {
			graph.AddEdge(nodeDef.ID, trueID)
		}
		for _, falseID := range nodeDef.OnFalse {
			graph.AddEdge(nodeDef.ID, falseID)
		}
	}

	graph.SetEntry(dsl.Workflow.Entry)
	return graph, nil
}

// buildNode 构建单个节点
func (p *Parser) buildNode(def *NodeDef, dsl *WorkflowDSL, vars map[string]interface{}) (*workflow.DAGNode, error) {
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
			subDSL := &WorkflowDSL{
				Name:     dsl.Name + "_sub",
				Workflow: *def.SubGraph,
			}
			subGraph, err := p.buildGraph(subDSL, vars)
			if err != nil {
				return nil, fmt.Errorf("build subgraph: %w", err)
			}
			node.SubGraph = subGraph
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
func (p *Parser) resolveStep(def *NodeDef, dsl *WorkflowDSL, vars map[string]interface{}) (workflow.Step, error) {
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
	case "llm":
		prompt := p.interpolate(stepDef.Prompt, vars)
		return &workflow.LLMStep{
			Model:  stepDef.Agent,
			Prompt: prompt,
		}, nil

	case "tool":
		params := make(map[string]any)
		for k, v := range stepDef.Config {
			if s, ok := v.(string); ok {
				params[k] = p.interpolate(s, vars)
			} else {
				params[k] = v
			}
		}
		return &workflow.ToolStep{
			ToolName: stepDef.Tool,
			Params:   params,
		}, nil

	case "human_input":
		return &workflow.HumanInputStep{
			Prompt: p.interpolate(stepDef.Prompt, vars),
		}, nil

	case "passthrough":
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
func (p *Parser) resolveCondition(expr string, vars map[string]interface{}) (workflow.ConditionFunc, error) {
	// 1. 检查是否是注册的命名条件
	if fn, ok := p.conditionRegistry[expr]; ok {
		return fn, nil
	}

	// 2. 简单表达式解析
	return p.parseSimpleExpression(expr, vars)
}

// parseSimpleExpression 解析简单条件表达式
func (p *Parser) parseSimpleExpression(expr string, vars map[string]interface{}) (workflow.ConditionFunc, error) {
	return func(_ context.Context, _ interface{}) (bool, error) {
		// 运行时求值
		resolved := p.interpolate(expr, vars)
		// 简化实现：非空字符串为 true
		return resolved != "" && resolved != "false" && resolved != "0", nil
	}, nil
}

// resolveLoop 解析循环配置
func (p *Parser) resolveLoop(def *LoopDef, vars map[string]interface{}) (*workflow.LoopConfig, error) {
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
	p.RegisterStep("passthrough", func(_ map[string]interface{}) (workflow.Step, error) {
		return &workflow.PassthroughStep{}, nil
	})
}
