package dsl

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Validator — basic field validation
// ============================================================

func TestValidator_EmptyDSL(t *testing.T) {
	v := NewValidator()
	errs := v.Validate(&WorkflowDSL{})
	assert.NotEmpty(t, errs)

	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "version is required")
	assert.Contains(t, errMsgs, "name is required")
	assert.Contains(t, errMsgs, "workflow.entry is required")
}

func TestValidator_ValidMinimalDSL(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "start",
			Nodes: []NodeDef{
				{
					ID:   "start",
					Type: "action",
					StepDef: &StepDef{
						Type:   "passthrough",
						Config: map[string]any{},
					},
				},
			},
		},
	}
	errs := v.Validate(dsl)
	assert.Empty(t, errs)
}

func TestValidator_DuplicateNodeID(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", StepDef: &StepDef{Type: "passthrough"}},
				{ID: "a", Type: "action", StepDef: &StepDef{Type: "passthrough"}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "duplicate node ID: a")
}

func TestValidator_EntryNodeNotExist(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "missing",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", StepDef: &StepDef{Type: "passthrough"}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `entry node "missing" does not exist`)
}

func TestValidator_EmptyNodeID(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "", Type: "action"},
				{ID: "a", Type: "action", StepDef: &StepDef{Type: "passthrough"}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node ID is required")
}

// ============================================================
// Validator — node type validation
// ============================================================

func TestValidator_InvalidNodeType(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "invalid_type"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `node a: invalid type "invalid_type"`)
}

func TestValidator_ActionNode_NoStep(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node a: action node requires step or step_def")
}

func TestValidator_ActionNode_StepNotFound(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Steps:   map[string]StepDef{},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "missing_step"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `node a: step "missing_step" not found in steps`)
}

func TestValidator_ConditionNode_NoCondition(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "c",
			Nodes: []NodeDef{
				{ID: "c", Type: "condition"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node c: condition node requires condition expression")
	assert.Contains(t, errMsgs, "node c: condition node requires on_true or on_false")
}

func TestValidator_LoopNode_NoLoopDef(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "l",
			Nodes: []NodeDef{
				{ID: "l", Type: "loop"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node l: loop node requires loop definition")
}

func TestValidator_LoopNode_WhileNoCondition(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "l",
			Nodes: []NodeDef{
				{ID: "l", Type: "loop", Loop: &LoopDef{Type: "while"}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node l: while loop requires condition")
}

func TestValidator_LoopNode_ForNoMaxIterations(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "l",
			Nodes: []NodeDef{
				{ID: "l", Type: "loop", Loop: &LoopDef{Type: "for", MaxIterations: 0}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node l: for loop requires positive max_iterations")
}

func TestValidator_LoopNode_EmptyType(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "l",
			Nodes: []NodeDef{
				{ID: "l", Type: "loop", Loop: &LoopDef{}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node l: loop type is required")
}

func TestValidator_ParallelNode_TooFewBranches(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "p",
			Nodes: []NodeDef{
				{ID: "p", Type: "parallel", Next: []string{"a"}},
				{ID: "a", Type: "action", StepDef: &StepDef{Type: "passthrough"}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node p: parallel node requires at least 2 branches")
}

func TestValidator_SubgraphNode_NoSubgraph(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "s",
			Nodes: []NodeDef{
				{ID: "s", Type: "subgraph"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, "node s: subgraph node requires subgraph definition")
}

// ============================================================
// Validator — reference validation
// ============================================================

func TestValidator_NextNodeNotExist(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", StepDef: &StepDef{Type: "passthrough"}, Next: []string{"missing"}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `node a: next node "missing" does not exist`)
}

func TestValidator_StepAgentNotFound(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Agents:  map[string]AgentDef{},
		Steps: map[string]StepDef{
			"s1": {Type: "llm", Agent: "missing_agent"},
		},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "s1"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `step s1: agent "missing_agent" not found`)
}

func TestValidator_StepToolNotFound(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Tools:   map[string]ToolDef{},
		Steps: map[string]StepDef{
			"s1": {Type: "tool", Tool: "missing_tool"},
		},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "s1"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `step s1: tool "missing_tool" not found`)
}

func TestValidator_AgentToolNotFound(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Agents: map[string]AgentDef{
			"a1": {Model: "gpt-4", Tools: []string{"missing_tool"}},
		},
		Tools: map[string]ToolDef{},
		Workflow: WorkflowNodesDef{
			Entry: "n",
			Nodes: []NodeDef{
				{ID: "n", Type: "action", StepDef: &StepDef{Type: "passthrough"}},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `agent a1: tool "missing_tool" not found`)
}

func TestValidator_VariableRefNotDefined(t *testing.T) {
	v := NewValidator()
	dsl := &WorkflowDSL{
		Version:   "1.0",
		Name:      "test",
		Variables: map[string]VariableDef{},
		Steps: map[string]StepDef{
			"s1": {Type: "llm", Prompt: "Hello ${undefined_var}"},
		},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "s1"},
			},
		},
	}
	errs := v.Validate(dsl)
	errMsgs := errStrings(errs)
	assert.Contains(t, errMsgs, `step s1: variable "undefined_var" referenced in prompt not defined`)
}

// ============================================================
// extractVariableRefs
// ============================================================

func TestExtractVariableRefs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"no refs", "hello world", nil},
		{"single ref", "Hello ${name}", []string{"name"}},
		{"multiple refs", "${a} and ${b}", []string{"a", "b"}},
		{"unclosed ref", "Hello ${name", nil},
		{"empty ref", "Hello ${}", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := extractVariableRefs(tt.input)
			assert.Equal(t, tt.expected, refs)
		})
	}
}

// ============================================================
// helpers
// ============================================================

func errStrings(errs []error) []string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return msgs
}

// ============================================================
// Parser
// ============================================================

func TestParser_RegisterStepAndCondition(t *testing.T) {
	p := NewParser()

	// Register custom step factory
	p.RegisterStep("custom", func(config map[string]any) (workflow.Step, error) {
		return &workflow.PassthroughStep{}, nil
	})

	// Register custom condition
	p.RegisterCondition("always_true", func(ctx context.Context, input any) (bool, error) {
		return true, nil
	})
	// No panic means success
}

func TestParser_Parse_MinimalYAML(t *testing.T) {
	yaml := `
version: "1.0"
name: "test-workflow"
description: "A test"
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step_def:
        type: passthrough
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "test-workflow", wf.Name())
	assert.Equal(t, "A test", wf.Description())
}

func TestParser_Parse_InvalidYAML(t *testing.T) {
	p := NewParser()
	_, err := p.Parse([]byte(`{invalid yaml`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse YAML")
}

func TestParser_Parse_ValidationError(t *testing.T) {
	yaml := `
version: ""
name: ""
workflow:
  entry: ""
  nodes: []
`
	p := NewParser()
	_, err := p.Parse([]byte(yaml))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validate DSL")
}

func TestParser_Parse_WithVariables(t *testing.T) {
	yaml := `
version: "1.0"
name: "var-test"
description: "test"
variables:
  greeting:
    type: string
    default: "Hello"
steps:
  greet:
    type: llm
    prompt: "${greeting} world"
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step: greet
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "var-test", wf.Name())
}

func TestParser_Parse_ConditionNode(t *testing.T) {
	yaml := `
version: "1.0"
name: "cond-test"
description: "test"
workflow:
  entry: check
  nodes:
    - id: check
      type: condition
      condition: "score > 0.5"
      on_true:
        - yes
      on_false:
        - no
    - id: "yes"
      type: action
      step_def:
        type: passthrough
    - id: "no"
      type: action
      step_def:
        type: passthrough
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestParser_Parse_LoopNode(t *testing.T) {
	yaml := `
version: "1.0"
name: "loop-test"
description: "test"
workflow:
  entry: loop
  nodes:
    - id: loop
      type: loop
      loop:
        type: for
        max_iterations: 3
      next:
        - body
    - id: body
      type: action
      step_def:
        type: passthrough
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestParser_Parse_WithMetadata(t *testing.T) {
	yaml := `
version: "1.0"
name: "meta-test"
description: "test"
metadata:
  author: "test"
  version: 2
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step_def:
        type: passthrough
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)

	v, ok := wf.GetMetadata("author")
	assert.True(t, ok)
	assert.Equal(t, "test", v)
}

func TestParser_Parse_ToolStep(t *testing.T) {
	yaml := `
version: "1.0"
name: "tool-test"
description: "test"
tools:
  calc:
    type: builtin
    description: "Calculator"
steps:
  use_calc:
    type: tool
    tool: calc
    config:
      op: add
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step: use_calc
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestParser_Parse_HumanInputStep(t *testing.T) {
	yaml := `
version: "1.0"
name: "human-test"
description: "test"
steps:
  ask:
    type: human_input
    prompt: "Approve?"
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step: ask
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestParser_Parse_UnknownStepType(t *testing.T) {
	yaml := `
version: "1.0"
name: "unknown-test"
description: "test"
steps:
  custom:
    type: unknown_type
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step: custom
`
	p := NewParser()
	_, err := p.Parse([]byte(yaml))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step type")
}

func TestParser_Parse_LLMStepWithAgent(t *testing.T) {
	yaml := `
version: "1.0"
name: "agent-test"
description: "test"
agents:
  gpt:
    model: gpt-4
    provider: openai
steps:
  ask:
    type: llm
    agent: gpt
    prompt: "Hello"
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step: ask
`
	p := NewParser()
	wf, err := p.Parse([]byte(yaml))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestParser_Interpolate(t *testing.T) {
	p := NewParser()
	vars := map[string]any{
		"name": "world",
		"num":  42,
	}
	result := p.interpolate("Hello ${name}, number ${num}", vars)
	assert.Equal(t, "Hello world, number 42", result)
}

func TestParser_Interpolate_NoVars(t *testing.T) {
	p := NewParser()
	result := p.interpolate("no vars here", map[string]any{})
	assert.Equal(t, "no vars here", result)
}

func TestParser_ResolveVariables(t *testing.T) {
	p := NewParser()
	vars := p.resolveVariables(map[string]VariableDef{
		"a": {Type: "string", Default: "hello"},
		"b": {Type: "int", Default: 42},
		"c": {Type: "string"}, // no default
	})
	assert.Equal(t, "hello", vars["a"])
	assert.Equal(t, 42, vars["b"])
	_, ok := vars["c"]
	assert.False(t, ok)
}

func TestParser_ParseFile_NotFound(t *testing.T) {
	p := NewParser()
	_, err := p.ParseFile("/nonexistent/file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read DSL file")
}

