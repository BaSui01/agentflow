package dsl

import (
	"fmt"
	"strings"
)

// Validator DSL 验证器
type Validator struct{}

// NewValidator 创建验证器
func NewValidator() *Validator {
	return &Validator{}
}

// Validate 验证 DSL 定义
func (v *Validator) Validate(dsl *WorkflowDSL) []error {
	var errs []error

	// 基础字段验证
	if dsl.Version == "" {
		errs = append(errs, fmt.Errorf("version is required"))
	}
	if dsl.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	}
	if dsl.Workflow.Entry == "" {
		errs = append(errs, fmt.Errorf("workflow.entry is required"))
	}
	if len(dsl.Workflow.Nodes) == 0 {
		errs = append(errs, fmt.Errorf("workflow.nodes must have at least one node"))
	}

	// V-015: Node count upper bound to prevent resource exhaustion
	if len(dsl.Workflow.Nodes) > 10000 {
		errs = append(errs, fmt.Errorf("workflow.nodes count %d exceeds maximum of 10000", len(dsl.Workflow.Nodes)))
	}

	// 收集所有节点 ID
	nodeIDs := make(map[string]bool)
	for _, node := range dsl.Workflow.Nodes {
		if node.ID == "" {
			errs = append(errs, fmt.Errorf("node ID is required"))
			continue
		}
		if nodeIDs[node.ID] {
			errs = append(errs, fmt.Errorf("duplicate node ID: %s", node.ID))
		}
		nodeIDs[node.ID] = true
	}

	// 验证 entry 节点存在
	if dsl.Workflow.Entry != "" && !nodeIDs[dsl.Workflow.Entry] {
		errs = append(errs, fmt.Errorf("entry node %q does not exist", dsl.Workflow.Entry))
	}

	// 验证每个节点
	for _, node := range dsl.Workflow.Nodes {
		errs = append(errs, v.validateNode(&node, dsl, nodeIDs)...)
	}

	// 验证 Step 定义与类型约束
	errs = append(errs, v.validateStepDefinitions(dsl)...)

	// 验证引用完整性
	errs = append(errs, v.validateReferences(dsl, nodeIDs)...)

	return errs
}

// validateNode 验证单个节点
func (v *Validator) validateNode(node *NodeDef, dsl *WorkflowDSL, nodeIDs map[string]bool) []error {
	var errs []error

	validTypes := map[string]bool{
		"action": true, "condition": true, "loop": true,
		"parallel": true, "subgraph": true, "checkpoint": true,
	}
	if !validTypes[node.Type] {
		errs = append(errs, fmt.Errorf("node %s: invalid type %q", node.ID, node.Type))
	}

	switch node.Type {
	case "action":
		if node.Step == "" && node.StepDef == nil {
			errs = append(errs, fmt.Errorf("node %s: action node requires step or step_def", node.ID))
		}
		if node.Step != "" && dsl.Steps != nil {
			if _, ok := dsl.Steps[node.Step]; !ok {
				errs = append(errs, fmt.Errorf("node %s: step %q not found in steps", node.ID, node.Step))
			}
		}
		if node.StepDef != nil {
			errs = append(errs, v.validateStepDefinition(fmt.Sprintf("node %s inline step_def", node.ID), *node.StepDef)...)
			errs = append(errs, v.validateStepReferences(fmt.Sprintf("node %s inline step_def", node.ID), *node.StepDef, dsl)...)
		}

	case "condition":
		if node.Condition == "" {
			errs = append(errs, fmt.Errorf("node %s: condition node requires condition expression", node.ID))
		}
		if len(node.OnTrue) == 0 && len(node.OnFalse) == 0 {
			errs = append(errs, fmt.Errorf("node %s: condition node requires on_true or on_false", node.ID))
		}

	case "loop":
		if node.Loop == nil {
			errs = append(errs, fmt.Errorf("node %s: loop node requires loop definition", node.ID))
		} else {
			if node.Loop.Type == "" {
				errs = append(errs, fmt.Errorf("node %s: loop type is required", node.ID))
			}
			if node.Loop.Type == "while" && node.Loop.Condition == "" {
				errs = append(errs, fmt.Errorf("node %s: while loop requires condition", node.ID))
			}
			if node.Loop.Type == "for" && node.Loop.MaxIterations <= 0 {
				errs = append(errs, fmt.Errorf("node %s: for loop requires positive max_iterations", node.ID))
			}
		}

	case "parallel":
		if len(node.Next) < 2 && len(node.Parallel) < 2 {
			errs = append(errs, fmt.Errorf("node %s: parallel node requires at least 2 branches", node.ID))
		}

	case "subgraph":
		if node.SubGraph == nil {
			errs = append(errs, fmt.Errorf("node %s: subgraph node requires subgraph definition", node.ID))
		}
	}

	// 验证引用的节点存在
	for _, nextID := range node.Next {
		if !nodeIDs[nextID] {
			errs = append(errs, fmt.Errorf("node %s: next node %q does not exist", node.ID, nextID))
		}
	}
	for _, id := range node.OnTrue {
		if !nodeIDs[id] {
			errs = append(errs, fmt.Errorf("node %s: on_true node %q does not exist", node.ID, id))
		}
	}
	for _, id := range node.OnFalse {
		if !nodeIDs[id] {
			errs = append(errs, fmt.Errorf("node %s: on_false node %q does not exist", node.ID, id))
		}
	}

	return errs
}

// validateReferences 验证所有引用的完整性
func (v *Validator) validateReferences(dsl *WorkflowDSL, _ map[string]bool) []error {
	var errs []error

	// 验证 step 中引用的 agent 和 tool 存在
	for stepName, step := range dsl.Steps {
		errs = append(errs, v.validateStepReferences(stepName, step, dsl)...)
	}

	// 验证 agent 中引用的 tool 存在
	for agentName, agent := range dsl.Agents {
		for _, toolName := range agent.Tools {
			if dsl.Tools != nil {
				if _, ok := dsl.Tools[toolName]; !ok {
					errs = append(errs, fmt.Errorf("agent %s: tool %q not found", agentName, toolName))
				}
			}
		}
	}

	return errs
}

func (v *Validator) validateStepDefinitions(dsl *WorkflowDSL) []error {
	var errs []error
	for stepName, step := range dsl.Steps {
		errs = append(errs, v.validateStepDefinition(stepName, step)...)
	}
	return errs
}

func (v *Validator) validateStepDefinition(stepName string, step StepDef) []error {
	var errs []error

	validStepTypes := map[string]bool{
		"llm":           true,
		"tool":          true,
		"human_input":   true,
		"code":          true,
		"agent":         true,
		"orchestration": true,
		"chain":         true,
		"passthrough":   true,
	}
	if !validStepTypes[step.Type] {
		errs = append(errs, fmt.Errorf("step %s: invalid type %q", stepName, step.Type))
		return errs
	}

	switch step.Type {
	case "llm":
		if strings.TrimSpace(step.Prompt) == "" {
			errs = append(errs, fmt.Errorf("step %s: llm step requires prompt", stepName))
		}
	case "tool":
		if strings.TrimSpace(step.Tool) == "" {
			errs = append(errs, fmt.Errorf("step %s: tool step requires tool", stepName))
		}
	case "human_input":
		if strings.TrimSpace(step.Prompt) == "" {
			errs = append(errs, fmt.Errorf("step %s: human_input step requires prompt", stepName))
		}
	case "orchestration":
		if step.Orchestration == nil {
			errs = append(errs, fmt.Errorf("step %s: orchestration step requires orchestration definition", stepName))
		} else if strings.TrimSpace(step.Orchestration.Mode) == "" {
			errs = append(errs, fmt.Errorf("step %s: orchestration step requires mode", stepName))
		} else if len(step.Orchestration.AgentIDs) == 0 {
			errs = append(errs, fmt.Errorf("step %s: orchestration step requires agent_ids", stepName))
		}
	case "chain":
		if step.Chain == nil {
			errs = append(errs, fmt.Errorf("step %s: chain step requires chain definition", stepName))
		} else if len(step.Chain.Steps) == 0 {
			errs = append(errs, fmt.Errorf("step %s: chain step requires at least one step", stepName))
		} else {
			for i, ce := range step.Chain.Steps {
				if strings.TrimSpace(ce.Tool) == "" {
					errs = append(errs, fmt.Errorf("step %s: chain step[%d] requires tool", stepName, i))
				}
			}
		}
	}

	return errs
}

func (v *Validator) validateStepReferences(stepName string, step StepDef, dsl *WorkflowDSL) []error {
	var errs []error

	if step.Agent != "" && dsl.Agents != nil {
		if _, ok := dsl.Agents[step.Agent]; !ok {
			errs = append(errs, fmt.Errorf("step %s: agent %q not found", stepName, step.Agent))
		}
	}
	if step.Tool != "" && dsl.Tools != nil {
		if _, ok := dsl.Tools[step.Tool]; !ok {
			errs = append(errs, fmt.Errorf("step %s: tool %q not found", stepName, step.Tool))
		}
	}

	if step.Prompt != "" {
		refs := extractVariableRefs(step.Prompt)
		for _, ref := range refs {
			if dsl.Variables != nil {
				if _, ok := dsl.Variables[ref]; !ok {
					errs = append(errs, fmt.Errorf("step %s: variable %q referenced in prompt not defined", stepName, ref))
				}
			}
		}
	}

	return errs
}

// extractVariableRefs 提取 ${var} 引用
func extractVariableRefs(s string) []string {
	var refs []string
	for {
		start := strings.Index(s, "${")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		ref := s[start+2 : start+end]
		refs = append(refs, ref)
		s = s[start+end+1:]
	}
	return refs
}
