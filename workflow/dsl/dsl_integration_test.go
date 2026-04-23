package dsl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflow "github.com/BaSui01/agentflow/workflow/core"
	"github.com/BaSui01/agentflow/workflow/steps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// =============================================================================
// Integration: YAML Load → Parse → Validate full chain
// =============================================================================

func TestIntegration_LoadParseValidate_ExampleYAML(t *testing.T) {
	examplePath := filepath.Join("examples", "customer_support.yaml")
	data, err := os.ReadFile(examplePath)
	require.NoError(t, err, "should read example YAML file")

	// Step 1: YAML unmarshal
	var dslDef WorkflowDSL
	require.NoError(t, yaml.Unmarshal(data, &dslDef), "YAML unmarshal should succeed")

	// Step 2: Validate — the example YAML has known issues (escalate prompt
	// is nested inside config, classify references ${input} not in variables),
	// so we just verify the validator runs and returns errors.
	v := NewValidator()
	errs := v.Validate(&dslDef)
	assert.NotEmpty(t, errs, "example YAML has known validation issues")

	// Step 3: Verify parsed structure regardless of validation
	assert.Equal(t, "1.0", dslDef.Version)
	assert.Equal(t, "customer-support-workflow", dslDef.Name)
	assert.Equal(t, "classify_input", dslDef.Workflow.Entry)
	assert.Len(t, dslDef.Workflow.Nodes, 7)

	// Variables
	assert.Contains(t, dslDef.Variables, "language")
	assert.Contains(t, dslDef.Variables, "max_search_results")

	// Agents
	assert.Contains(t, dslDef.Agents, "classifier")
	assert.Contains(t, dslDef.Agents, "responder")

	// Tools
	assert.Contains(t, dslDef.Tools, "knowledge_search")

	// Steps
	assert.Contains(t, dslDef.Steps, "classify")
	assert.Contains(t, dslDef.Steps, "search_kb")
	assert.Contains(t, dslDef.Steps, "generate_response")
	assert.Contains(t, dslDef.Steps, "escalate")
}

func TestIntegration_ParseFile_ExampleYAML(t *testing.T) {
	// The example YAML has known validation issues, so ParseFile returns an error.
	// We verify that ParseFile correctly reads the file and reports validation errors.
	examplePath := filepath.Join("examples", "customer_support.yaml")
	p := NewParser()
	_, err := p.ParseFile(examplePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate DSL")
}

func TestIntegration_ParseFile_ValidYAML(t *testing.T) {
	// Test ParseFile success path with a valid inline-written temp file.
	dir := t.TempDir()
	yamlContent := `
version: "1.0"
name: "file-test"
description: "test"
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step_def:
        type: passthrough
`
	tmpFile := filepath.Join(dir, "valid.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(yamlContent), 0644))

	p := NewParser()
	wf, err := p.ParseFile(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, wf)
	assert.Equal(t, "file-test", wf.Name())
}

func TestIntegration_Parse_FullWorkflowWithAllNodeTypes(t *testing.T) {
	yamlData := `
version: "1.0"
name: "full-integration"
description: "Tests all node types in one workflow"
variables:
  threshold:
    type: float
    default: 0.8
  lang:
    type: string
    default: "en"
agents:
  main_agent:
    model: gpt-4
    system_prompt: "You are helpful"
    temperature: 0.5
    max_tokens: 1000
tools:
  search:
    type: builtin
    description: "Search tool"
steps:
  greet:
    type: llm
    agent: main_agent
    prompt: "Hello in ${lang}"
  do_search:
    type: tool
    tool: search
    config:
      query: "test"
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step: greet
      next: [decide]
    - id: decide
      type: condition
      condition: "score > 0.5"
      on_true: [search_node]
      on_false: [fallback]
    - id: search_node
      type: action
      step: do_search
      next: []
      error:
        strategy: retry
        max_retries: 3
        retry_delay_ms: 100
    - id: fallback
      type: action
      step_def:
        type: passthrough
      next: []
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	require.NotNil(t, wf)
	assert.Equal(t, "full-integration", wf.Name())
	assert.Equal(t, "Tests all node types in one workflow", wf.Description())
}

// =============================================================================
// Parser: resolveStep coverage — agent, code, chain step types
// =============================================================================

func TestParse_AgentStepType(t *testing.T) {
	yamlData := `
version: "1.0"
name: "agent-step-test"
description: "test"
agents:
  helper:
    model: gpt-4
    system_prompt: "help"
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step_def:
        type: agent
        agent: helper
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)

	node, ok := wf.Graph().GetNode("a")
	require.True(t, ok)
	adapter, ok := node.Step.(*protocolStepAdapter)
	require.True(t, ok)
	step, ok := adapter.step.(*steps.AgentStep)
	require.True(t, ok)
	assert.Equal(t, "helper", step.AgentID)
}

func TestParse_AgentStepType_RejectsInlineAgent(t *testing.T) {
	yamlData := `
version: "1.0"
name: "inline-agent-test"
description: "test"
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step_def:
        type: agent
        inline_agent:
          model: gpt-4
          system_prompt: "inline helper"
          tools: [tool_a, tool_b]
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.Error(t, err)
	assert.Nil(t, wf)
	assert.Contains(t, err.Error(), "agent step does not support inline_agent")
}

func TestParse_CodeStepType(t *testing.T) {
	yamlData := `
version: "1.0"
name: "code-step-test"
description: "test"
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step_def:
        type: code
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestParse_HumanInputStepType_WithConfig(t *testing.T) {
	yamlData := `
version: "1.0"
name: "human-config-test"
description: "test"
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step_def:
        type: human_input
        prompt: "Please confirm"
        config:
          type: "confirmation"
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

func TestParse_CustomRegisteredStep(t *testing.T) {
	// The default branch in resolveStep looks up the stepRegistry for types
	// not handled by the switch. "hybrid_retrieve" passes the validator but
	// is not in the parser switch, so it falls through to the custom registry.
	yamlData := `
version: "1.0"
name: "custom-step-test"
description: "test"
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step_def:
        type: hybrid_retrieve
        config:
          key: value
`
	called := false
	p := NewParser()
	p.RegisterStep("hybrid_retrieve", func(config map[string]any) (workflow.Step, error) {
		called = true
		return &workflow.PassthroughStep{}, nil
	})
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
	assert.True(t, called, "custom factory should have been invoked")
}

// =============================================================================
// Parser: loop node with while condition
// =============================================================================

func TestParse_LoopNode_WhileCondition(t *testing.T) {
	yamlData := `
version: "1.0"
name: "while-loop-test"
description: "test"
workflow:
  entry: loop
  nodes:
    - id: loop
      type: loop
      loop:
        type: while
        condition: "count < 10"
        max_iterations: 100
      next:
        - body
    - id: body
      type: action
      step_def:
        type: passthrough
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

// =============================================================================
// Parser: subgraph node
// =============================================================================

func TestParse_SubgraphNode(t *testing.T) {
	yamlData := `
version: "1.0"
name: "subgraph-test"
description: "test"
workflow:
  entry: sub
  nodes:
    - id: sub
      type: subgraph
      subgraph:
        entry: inner_start
        nodes:
          - id: inner_start
            type: action
            step_def:
              type: passthrough
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

// =============================================================================
// Parser: parallel node
// =============================================================================

func TestParse_ParallelNode(t *testing.T) {
	yamlData := `
version: "1.0"
name: "parallel-test"
description: "test"
workflow:
  entry: par
  nodes:
    - id: par
      type: parallel
      next: [branch_a, branch_b]
    - id: branch_a
      type: action
      step_def:
        type: passthrough
    - id: branch_b
      type: action
      step_def:
        type: passthrough
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

// =============================================================================
// Parser: error config on node
// =============================================================================

func TestParse_NodeWithErrorConfig(t *testing.T) {
	yamlData := `
version: "1.0"
name: "error-config-test"
description: "test"
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step_def:
        type: passthrough
      error:
        strategy: retry
        max_retries: 5
        retry_delay_ms: 200
        fallback_value: "default"
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

// =============================================================================
// Parser: named condition via RegisterCondition
// =============================================================================

func TestParse_NamedCondition(t *testing.T) {
	yamlData := `
version: "1.0"
name: "named-cond-test"
description: "test"
workflow:
  entry: check
  nodes:
    - id: check
      type: condition
      condition: "always_pass"
      on_true: [ok]
      on_false: [fail]
    - id: ok
      type: action
      step_def:
        type: passthrough
    - id: fail
      type: action
      step_def:
        type: passthrough
`
	p := NewParser()
	p.RegisterCondition("always_pass", func(_ context.Context, _ any) (bool, error) {
		return true, nil
	})
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

// =============================================================================
// Parser: LLM step with model from config (no agent)
// =============================================================================

func TestParse_LLMStep_ModelFromConfig(t *testing.T) {
	yamlData := `
version: "1.0"
name: "llm-config-model"
description: "test"
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step_def:
        type: llm
        prompt: "Hello"
        config:
          model: "gpt-3.5-turbo"
          temperature: 0.9
          max_tokens: 500
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

// =============================================================================
// Parser: tool step with variable interpolation in config
// =============================================================================

func TestParse_ToolStep_ConfigInterpolation(t *testing.T) {
	yamlData := `
version: "1.0"
name: "tool-interp-test"
description: "test"
variables:
  query_text:
    type: string
    default: "hello world"
tools:
  search:
    type: builtin
    description: "search"
steps:
  do_search:
    type: tool
    tool: search
    config:
      query: "${query_text}"
      limit: 10
workflow:
  entry: a
  nodes:
    - id: a
      type: action
      step: do_search
`
	p := NewParser()
	wf, err := p.Parse([]byte(yamlData))
	require.NoError(t, err)
	assert.NotNil(t, wf)
}

// =============================================================================
// readInt / readFloat64 coverage
// =============================================================================

func TestReadInt(t *testing.T) {
	tests := []struct {
		name     string
		cfg      map[string]any
		key      string
		expected int
	}{
		{"nil config", nil, "k", 0},
		{"missing key", map[string]any{}, "k", 0},
		{"int value", map[string]any{"k": 42}, "k", 42},
		{"int32 value", map[string]any{"k": int32(10)}, "k", 10},
		{"int64 value", map[string]any{"k": int64(99)}, "k", 99},
		{"float64 value", map[string]any{"k": float64(7.9)}, "k", 7},
		{"string value (unsupported)", map[string]any{"k": "abc"}, "k", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, readInt(tt.cfg, tt.key))
		})
	}
}

func TestReadFloat64(t *testing.T) {
	tests := []struct {
		name     string
		cfg      map[string]any
		key      string
		expected float64
	}{
		{"nil config", nil, "k", 0},
		{"missing key", map[string]any{}, "k", 0},
		{"float64 value", map[string]any{"k": 3.14}, "k", 3.14},
		{"float32 value", map[string]any{"k": float32(2.5)}, "k", float64(float32(2.5))},
		{"int value", map[string]any{"k": 7}, "k", 7.0},
		{"int32 value", map[string]any{"k": int32(8)}, "k", 8.0},
		{"int64 value", map[string]any{"k": int64(9)}, "k", 9.0},
		{"string value (unsupported)", map[string]any{"k": "abc"}, "k", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, readFloat64(tt.cfg, tt.key))
		})
	}
}

// =============================================================================
// evalComparison: nil edge cases
// =============================================================================

func TestEvalComparison_NilCases(t *testing.T) {
	// nil == nil
	assert.True(t, evalComparison(nil, "==", nil))
	assert.True(t, evalComparison(nil, ">=", nil))
	assert.True(t, evalComparison(nil, "<=", nil))
	assert.False(t, evalComparison(nil, "!=", nil))
	assert.False(t, evalComparison(nil, ">", nil))
	assert.False(t, evalComparison(nil, "<", nil))

	// nil vs non-nil
	assert.True(t, evalComparison(nil, "!=", 1))
	assert.False(t, evalComparison(nil, "==", 1))
	assert.True(t, evalComparison(nil, "<", 1))
	assert.True(t, evalComparison(nil, "<=", 1))
	assert.False(t, evalComparison(nil, ">", 1))
	assert.False(t, evalComparison(nil, ">=", 1))

	// non-nil vs nil
	assert.True(t, evalComparison(1, "!=", nil))
	assert.False(t, evalComparison(1, "==", nil))
	assert.True(t, evalComparison(1, ">", nil))
	assert.True(t, evalComparison(1, ">=", nil))
	assert.False(t, evalComparison(1, "<", nil))
	assert.False(t, evalComparison(1, "<=", nil))
}

func TestEvalComparison_StringFallback(t *testing.T) {
	assert.True(t, evalComparison("abc", "==", "abc"))
	assert.True(t, evalComparison("abc", "!=", "def"))
	assert.True(t, evalComparison("b", ">", "a"))
	assert.True(t, evalComparison("a", "<", "b"))
	assert.True(t, evalComparison("a", "<=", "a"))
	assert.True(t, evalComparison("a", ">=", "a"))
}

// =============================================================================
// toBool: type branch coverage
// =============================================================================

func TestToBool(t *testing.T) {
	assert.False(t, toBool(nil))
	assert.True(t, toBool(true))
	assert.False(t, toBool(false))
	assert.True(t, toBool(1.0))
	assert.False(t, toBool(0.0))
	assert.True(t, toBool(1))
	assert.False(t, toBool(0))
	assert.True(t, toBool("hello"))
	assert.False(t, toBool(""))
	assert.False(t, toBool("false"))
	assert.False(t, toBool("0"))
	// Non-standard type -> true
	assert.True(t, toBool([]int{1, 2}))
}

// =============================================================================
// toFloat64: type branch coverage
// =============================================================================

func TestToFloat64(t *testing.T) {
	f, ok := toFloat64(3.14)
	assert.True(t, ok)
	assert.Equal(t, 3.14, f)

	f, ok = toFloat64(42)
	assert.True(t, ok)
	assert.Equal(t, 42.0, f)

	f, ok = toFloat64(int64(99))
	assert.True(t, ok)
	assert.Equal(t, 99.0, f)

	f, ok = toFloat64(float32(1.5))
	assert.True(t, ok)
	assert.InDelta(t, 1.5, f, 0.001)

	f, ok = toFloat64("2.5")
	assert.True(t, ok)
	assert.Equal(t, 2.5, f)

	_, ok = toFloat64("not_a_number")
	assert.False(t, ok)

	_, ok = toFloat64([]int{1})
	assert.False(t, ok)
}

// =============================================================================
// Validator: node count upper bound (V-015)
// =============================================================================

func TestValidator_NodeCountUpperBound(t *testing.T) {
	v := NewValidator()
	nodes := make([]NodeDef, 10001)
	for i := range nodes {
		nodes[i] = NodeDef{ID: fmt.Sprintf("n%d", i), Type: "action", StepDef: &StepDef{Type: "passthrough"}}
	}
	dslDef := &WorkflowDSL{
		Version: "1.0",
		Name:    "big",
		Workflow: WorkflowNodesDef{
			Entry: nodes[0].ID,
			Nodes: nodes,
		},
	}
	errs := v.Validate(dslDef)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "exceeds maximum of 10000") {
			found = true
			break
		}
	}
	assert.True(t, found, "should report node count exceeds maximum")
}

// =============================================================================
// Validator: orchestration step validation
// =============================================================================

func TestValidator_OrchestrationStep_NoOrchestration(t *testing.T) {
	v := NewValidator()
	dslDef := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Steps: map[string]StepDef{
			"orch": {Type: "orchestration"},
		},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "orch"},
			},
		},
	}
	errs := v.Validate(dslDef)
	msgs := errStrings(errs)
	assert.Contains(t, msgs, "step orch: orchestration step requires orchestration definition")
}

func TestValidator_OrchestrationStep_NoMode(t *testing.T) {
	v := NewValidator()
	dslDef := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Steps: map[string]StepDef{
			"orch": {Type: "orchestration", Orchestration: &OrchestrationStepDef{AgentIDs: []string{"a1"}}},
		},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "orch"},
			},
		},
	}
	errs := v.Validate(dslDef)
	msgs := errStrings(errs)
	assert.Contains(t, msgs, "step orch: orchestration step requires mode")
}

func TestValidator_OrchestrationStep_NoAgentIDs(t *testing.T) {
	v := NewValidator()
	dslDef := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Steps: map[string]StepDef{
			"orch": {Type: "orchestration", Orchestration: &OrchestrationStepDef{Mode: "debate"}},
		},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "orch"},
			},
		},
	}
	errs := v.Validate(dslDef)
	msgs := errStrings(errs)
	assert.Contains(t, msgs, "step orch: orchestration step requires agent_ids")
}

// =============================================================================
// Validator: chain step with missing tool in chain entry
// =============================================================================

func TestValidator_ChainStep_EntryMissingTool(t *testing.T) {
	v := NewValidator()
	dslDef := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Steps: map[string]StepDef{
			"ch": {Type: "chain", Chain: &ChainStepDef{Steps: []ChainStepEntry{{Tool: ""}, {Tool: "ok"}}}},
		},
		Workflow: WorkflowNodesDef{
			Entry: "a",
			Nodes: []NodeDef{
				{ID: "a", Type: "action", Step: "ch"},
			},
		},
	}
	errs := v.Validate(dslDef)
	msgs := errStrings(errs)
	assert.Contains(t, msgs, "step ch: chain step[0] requires tool")
}

// =============================================================================
// Validator: on_true / on_false reference non-existent nodes
// =============================================================================

func TestValidator_ConditionNode_BranchRefNotExist(t *testing.T) {
	v := NewValidator()
	dslDef := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "c",
			Nodes: []NodeDef{
				{
					ID:        "c",
					Type:      "condition",
					Condition: "true",
					OnTrue:    []string{"missing_true"},
					OnFalse:   []string{"missing_false"},
				},
			},
		},
	}
	errs := v.Validate(dslDef)
	msgs := errStrings(errs)
	assert.Contains(t, msgs, `node c: on_true node "missing_true" does not exist`)
	assert.Contains(t, msgs, `node c: on_false node "missing_false" does not exist`)
}

// =============================================================================
// Validator: checkpoint node type is valid
// =============================================================================

func TestValidator_CheckpointNodeType(t *testing.T) {
	v := NewValidator()
	dslDef := &WorkflowDSL{
		Version: "1.0",
		Name:    "test",
		Workflow: WorkflowNodesDef{
			Entry: "cp",
			Nodes: []NodeDef{
				{ID: "cp", Type: "checkpoint"},
			},
		},
	}
	errs := v.Validate(dslDef)
	for _, e := range errs {
		assert.NotContains(t, e.Error(), "invalid type")
	}
}
