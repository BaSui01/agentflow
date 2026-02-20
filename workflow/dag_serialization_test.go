package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDAGDefinition_JSONSerialization(t *testing.T) {
	// Create a sample DAG definition
	def := &DAGDefinition{
		Name:        "test-workflow",
		Description: "A test workflow",
		Entry:       "start",
		Nodes: []NodeDefinition{
			{
				ID:   "start",
				Type: "action",
				Step: "step1",
				Next: []string{"end"},
			},
			{
				ID:   "end",
				Type: "action",
				Step: "step2",
			},
		},
		Metadata: map[string]any{
			"version": "1.0",
			"author":  "test",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(def)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Unmarshal from JSON
	var decoded DAGDefinition
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, def.Name, decoded.Name)
	assert.Equal(t, def.Description, decoded.Description)
	assert.Equal(t, def.Entry, decoded.Entry)
	assert.Equal(t, len(def.Nodes), len(decoded.Nodes))
	assert.Equal(t, def.Metadata["version"], decoded.Metadata["version"])
	assert.Equal(t, def.Metadata["author"], decoded.Metadata["author"])
}

func TestDAGDefinition_YAMLSerialization(t *testing.T) {
	// Create a sample DAG definition
	def := &DAGDefinition{
		Name:        "test-workflow",
		Description: "A test workflow",
		Entry:       "start",
		Nodes: []NodeDefinition{
			{
				ID:   "start",
				Type: "action",
				Step: "step1",
				Next: []string{"end"},
			},
			{
				ID:   "end",
				Type: "action",
				Step: "step2",
			},
		},
		Metadata: map[string]any{
			"version": "1.0",
			"author":  "test",
		},
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(def)
	require.NoError(t, err)
	assert.NotEmpty(t, yamlData)

	// Unmarshal from YAML
	var decoded DAGDefinition
	err = yaml.Unmarshal(yamlData, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, def.Name, decoded.Name)
	assert.Equal(t, def.Description, decoded.Description)
	assert.Equal(t, def.Entry, decoded.Entry)
	assert.Equal(t, len(def.Nodes), len(decoded.Nodes))
	assert.Equal(t, def.Metadata["version"], decoded.Metadata["version"])
	assert.Equal(t, def.Metadata["author"], decoded.Metadata["author"])
}

func TestDAGDefinition_ToJSON(t *testing.T) {
	def := &DAGDefinition{
		Name:        "test-workflow",
		Description: "A test workflow",
		Entry:       "start",
		Nodes: []NodeDefinition{
			{
				ID:   "start",
				Type: "action",
				Step: "step1",
				Next: []string{"end"},
			},
			{
				ID:   "end",
				Type: "action",
				Step: "step2",
			},
		},
	}

	jsonStr, err := def.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, jsonStr)
	assert.Contains(t, jsonStr, "test-workflow")
	assert.Contains(t, jsonStr, "start")
	assert.Contains(t, jsonStr, "step1")
}

func TestDAGDefinition_ToYAML(t *testing.T) {
	def := &DAGDefinition{
		Name:        "test-workflow",
		Description: "A test workflow",
		Entry:       "start",
		Nodes: []NodeDefinition{
			{
				ID:   "start",
				Type: "action",
				Step: "step1",
				Next: []string{"end"},
			},
			{
				ID:   "end",
				Type: "action",
				Step: "step2",
			},
		},
	}

	yamlStr, err := def.ToYAML()
	require.NoError(t, err)
	assert.NotEmpty(t, yamlStr)
	assert.Contains(t, yamlStr, "test-workflow")
	assert.Contains(t, yamlStr, "start")
	assert.Contains(t, yamlStr, "step1")
}

func TestFromJSON(t *testing.T) {
	jsonStr := `{
		"name": "test-workflow",
		"description": "A test workflow",
		"entry": "start",
		"nodes": [
			{
				"id": "start",
				"type": "action",
				"step": "step1",
				"next": ["end"]
			},
			{
				"id": "end",
				"type": "action",
				"step": "step2"
			}
		]
	}`

	def, err := FromJSON(jsonStr)
	require.NoError(t, err)
	assert.NotNil(t, def)
	assert.Equal(t, "test-workflow", def.Name)
	assert.Equal(t, "start", def.Entry)
	assert.Equal(t, 2, len(def.Nodes))
}

func TestFromYAML(t *testing.T) {
	yamlStr := `
name: test-workflow
description: A test workflow
entry: start
nodes:
  - id: start
    type: action
    step: step1
    next:
      - end
  - id: end
    type: action
    step: step2
`

	def, err := FromYAML(yamlStr)
	require.NoError(t, err)
	assert.NotNil(t, def)
	assert.Equal(t, "test-workflow", def.Name)
	assert.Equal(t, "start", def.Entry)
	assert.Equal(t, 2, len(def.Nodes))
}

func TestLoadFromJSONFile(t *testing.T) {
	// Create a temporary JSON file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "workflow.json")

	jsonContent := `{
		"name": "file-workflow",
		"description": "Loaded from file",
		"entry": "start",
		"nodes": [
			{
				"id": "start",
				"type": "action",
				"step": "step1",
				"next": ["end"]
			},
			{
				"id": "end",
				"type": "action",
				"step": "step2"
			}
		]
	}`

	err := os.WriteFile(filename, []byte(jsonContent), 0644)
	require.NoError(t, err)

	// Load from file
	def, err := LoadFromJSONFile(filename)
	require.NoError(t, err)
	assert.NotNil(t, def)
	assert.Equal(t, "file-workflow", def.Name)
	assert.Equal(t, "Loaded from file", def.Description)
}

func TestLoadFromYAMLFile(t *testing.T) {
	// Create a temporary YAML file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "workflow.yaml")

	yamlContent := `
name: file-workflow
description: Loaded from file
entry: start
nodes:
  - id: start
    type: action
    step: step1
    next:
      - end
  - id: end
    type: action
    step: step2
`

	err := os.WriteFile(filename, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Load from file
	def, err := LoadFromYAMLFile(filename)
	require.NoError(t, err)
	assert.NotNil(t, def)
	assert.Equal(t, "file-workflow", def.Name)
	assert.Equal(t, "Loaded from file", def.Description)
}

func TestSaveToJSONFile(t *testing.T) {
	def := &DAGDefinition{
		Name:        "save-workflow",
		Description: "Saved to file",
		Entry:       "start",
		Nodes: []NodeDefinition{
			{
				ID:   "start",
				Type: "action",
				Step: "step1",
				Next: []string{"end"},
			},
			{
				ID:   "end",
				Type: "action",
				Step: "step2",
			},
		},
	}

	// Save to file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "workflow.json")

	err := def.SaveToJSONFile(filename)
	require.NoError(t, err)

	// Verify file exists and can be loaded
	loaded, err := LoadFromJSONFile(filename)
	require.NoError(t, err)
	assert.Equal(t, def.Name, loaded.Name)
	assert.Equal(t, def.Description, loaded.Description)
}

func TestSaveToYAMLFile(t *testing.T) {
	def := &DAGDefinition{
		Name:        "save-workflow",
		Description: "Saved to file",
		Entry:       "start",
		Nodes: []NodeDefinition{
			{
				ID:   "start",
				Type: "action",
				Step: "step1",
				Next: []string{"end"},
			},
			{
				ID:   "end",
				Type: "action",
				Step: "step2",
			},
		},
	}

	// Save to file
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "workflow.yaml")

	err := def.SaveToYAMLFile(filename)
	require.NoError(t, err)

	// Verify file exists and can be loaded
	loaded, err := LoadFromYAMLFile(filename)
	require.NoError(t, err)
	assert.Equal(t, def.Name, loaded.Name)
	assert.Equal(t, def.Description, loaded.Description)
}

func TestValidateDAGDefinition(t *testing.T) {
	tests := []struct {
		name        string
		def         *DAGDefinition
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid workflow",
			def: &DAGDefinition{
				Name:  "valid",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "action", Step: "step1"},
				},
			},
			expectError: false,
		},
		{
			name: "missing name",
			def: &DAGDefinition{
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "action", Step: "step1"},
				},
			},
			expectError: true,
			errorMsg:    "workflow name is required",
		},
		{
			name: "no nodes",
			def: &DAGDefinition{
				Name:  "empty",
				Entry: "start",
				Nodes: []NodeDefinition{},
			},
			expectError: true,
			errorMsg:    "workflow must have at least one node",
		},
		{
			name: "missing entry",
			def: &DAGDefinition{
				Name: "no-entry",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "action", Step: "step1"},
				},
			},
			expectError: true,
			errorMsg:    "entry node is required",
		},
		{
			name: "entry node does not exist",
			def: &DAGDefinition{
				Name:  "invalid-entry",
				Entry: "nonexistent",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "action", Step: "step1"},
				},
			},
			expectError: true,
			errorMsg:    "entry node nonexistent does not exist",
		},
		{
			name: "duplicate node ID",
			def: &DAGDefinition{
				Name:  "duplicate",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "action", Step: "step1"},
					{ID: "start", Type: "action", Step: "step2"},
				},
			},
			expectError: true,
			errorMsg:    "duplicate node ID: start",
		},
		{
			name: "missing node ID",
			def: &DAGDefinition{
				Name:  "no-id",
				Entry: "start",
				Nodes: []NodeDefinition{
					{Type: "action", Step: "step1"},
				},
			},
			expectError: true,
			errorMsg:    "node ID is required",
		},
		{
			name: "missing node type",
			def: &DAGDefinition{
				Name:  "no-type",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Step: "step1"},
				},
			},
			expectError: true,
			errorMsg:    "type is required",
		},
		{
			name: "action node without step",
			def: &DAGDefinition{
				Name:  "no-step",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "action"},
				},
			},
			expectError: true,
			errorMsg:    "action node requires step",
		},
		{
			name: "condition node without condition",
			def: &DAGDefinition{
				Name:  "no-condition",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "condition", OnTrue: []string{"end"}},
					{ID: "end", Type: "action", Step: "step1"},
				},
			},
			expectError: true,
			errorMsg:    "condition node requires condition",
		},
		{
			name: "condition node without branches",
			def: &DAGDefinition{
				Name:  "no-branches",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "condition", Condition: "cond1"},
				},
			},
			expectError: true,
			errorMsg:    "condition node requires at least one branch",
		},
		{
			name: "loop node without config",
			def: &DAGDefinition{
				Name:  "no-loop-config",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "loop"},
				},
			},
			expectError: true,
			errorMsg:    "loop node requires loop configuration",
		},
		{
			name: "loop node without type",
			def: &DAGDefinition{
				Name:  "no-loop-type",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "loop", Loop: &LoopDefinition{}},
				},
			},
			expectError: true,
			errorMsg:    "loop type is required",
		},
		{
			name: "while loop without condition",
			def: &DAGDefinition{
				Name:  "while-no-condition",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "loop", Loop: &LoopDefinition{Type: "while", MaxIterations: 10}},
				},
			},
			expectError: true,
			errorMsg:    "while loop requires condition",
		},
		{
			name: "for loop without max iterations",
			def: &DAGDefinition{
				Name:  "for-no-iterations",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "loop", Loop: &LoopDefinition{Type: "for"}},
				},
			},
			expectError: true,
			errorMsg:    "for loop requires positive max_iterations",
		},
		{
			name: "foreach loop without max iterations",
			def: &DAGDefinition{
				Name:  "foreach-no-iterations",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "loop", Loop: &LoopDefinition{Type: "foreach"}},
				},
			},
			expectError: true,
			errorMsg:    "foreach loop requires positive max_iterations",
		},
		{
			name: "parallel node with insufficient next nodes",
			def: &DAGDefinition{
				Name:  "parallel-insufficient",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "parallel", Next: []string{"end"}},
					{ID: "end", Type: "action", Step: "step1"},
				},
			},
			expectError: true,
			errorMsg:    "parallel node requires at least 2 next nodes",
		},
		{
			name: "subgraph node without subgraph",
			def: &DAGDefinition{
				Name:  "no-subgraph",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "subgraph"},
				},
			},
			expectError: true,
			errorMsg:    "subgraph node requires subgraph",
		},
		{
			name: "invalid node type",
			def: &DAGDefinition{
				Name:  "invalid-type",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "invalid"},
				},
			},
			expectError: true,
			errorMsg:    "invalid node type",
		},
		{
			name: "next node does not exist",
			def: &DAGDefinition{
				Name:  "invalid-next",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "action", Step: "step1", Next: []string{"nonexistent"}},
				},
			},
			expectError: true,
			errorMsg:    "next node nonexistent does not exist",
		},
		{
			name: "on_true node does not exist",
			def: &DAGDefinition{
				Name:  "invalid-on-true",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "condition", Condition: "cond1", OnTrue: []string{"nonexistent"}},
				},
			},
			expectError: true,
			errorMsg:    "on_true node nonexistent does not exist",
		},
		{
			name: "on_false node does not exist",
			def: &DAGDefinition{
				Name:  "invalid-on-false",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "condition", Condition: "cond1", OnFalse: []string{"nonexistent"}},
				},
			},
			expectError: true,
			errorMsg:    "on_false node nonexistent does not exist",
		},
		{
			name: "valid checkpoint node",
			def: &DAGDefinition{
				Name:  "checkpoint",
				Entry: "start",
				Nodes: []NodeDefinition{
					{ID: "start", Type: "checkpoint"},
				},
			},
			expectError: false,
		},
		{
			name: "valid subgraph",
			def: &DAGDefinition{
				Name:  "with-subgraph",
				Entry: "start",
				Nodes: []NodeDefinition{
					{
						ID:   "start",
						Type: "subgraph",
						SubGraph: &DAGDefinition{
							Name:  "subgraph",
							Entry: "sub_start",
							Nodes: []NodeDefinition{
								{ID: "sub_start", Type: "action", Step: "sub_step"},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid subgraph",
			def: &DAGDefinition{
				Name:  "invalid-subgraph",
				Entry: "start",
				Nodes: []NodeDefinition{
					{
						ID:   "start",
						Type: "subgraph",
						SubGraph: &DAGDefinition{
							Name:  "subgraph",
							Entry: "nonexistent",
							Nodes: []NodeDefinition{
								{ID: "sub_start", Type: "action", Step: "sub_step"},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "subgraph validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDAGDefinition(tt.def)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDAGWorkflow_ToDAGDefinition(t *testing.T) {
	// Build a workflow
	workflow, err := NewDAGBuilder("test-workflow").
		WithDescription("A test workflow").
		AddNode("start", NodeTypeAction).
		WithStep(&mockStep{name: "step1"}).
		WithMetadata("priority", "high").
		Done().
		AddNode("end", NodeTypeAction).
		WithStep(&mockStep{name: "step2"}).
		Done().
		AddEdge("start", "end").
		SetEntry("start").
		Build()

	require.NoError(t, err)

	// Convert to definition
	def := workflow.ToDAGDefinition()
	require.NotNil(t, def)

	// Verify fields
	assert.Equal(t, "test-workflow", def.Name)
	assert.Equal(t, "A test workflow", def.Description)
	assert.Equal(t, "start", def.Entry)
	assert.Equal(t, 2, len(def.Nodes))

	// Find start node
	var startNode *NodeDefinition
	for i := range def.Nodes {
		if def.Nodes[i].ID == "start" {
			startNode = &def.Nodes[i]
			break
		}
	}
	require.NotNil(t, startNode)
	assert.Equal(t, "action", startNode.Type)
	assert.Equal(t, "step1", startNode.Step)
	assert.Equal(t, "high", startNode.Metadata["priority"])
	assert.Contains(t, startNode.Next, "end")
}

func TestComplexWorkflowSerialization(t *testing.T) {
	// Create a complex workflow definition
	def := &DAGDefinition{
		Name:        "complex-workflow",
		Description: "A complex workflow with multiple node types",
		Entry:       "start",
		Nodes: []NodeDefinition{
			{
				ID:   "start",
				Type: "action",
				Step: "initialize",
				Next: []string{"condition"},
			},
			{
				ID:        "condition",
				Type:      "condition",
				Condition: "check_value",
				OnTrue:    []string{"parallel"},
				OnFalse:   []string{"end"},
			},
			{
				ID:   "parallel",
				Type: "parallel",
				Next: []string{"task1", "task2"},
			},
			{
				ID:   "task1",
				Type: "action",
				Step: "process_task1",
				Next: []string{"loop"},
			},
			{
				ID:   "task2",
				Type: "action",
				Step: "process_task2",
				Next: []string{"loop"},
			},
			{
				ID:   "loop",
				Type: "loop",
				Loop: &LoopDefinition{
					Type:          "while",
					MaxIterations: 10,
					Condition:     "continue_loop",
				},
				Next: []string{"checkpoint"},
			},
			{
				ID:   "checkpoint",
				Type: "checkpoint",
				Next: []string{"end"},
			},
			{
				ID:   "end",
				Type: "action",
				Step: "finalize",
			},
		},
		Metadata: map[string]any{
			"version": "1.0",
			"tags":    []string{"complex", "test"},
		},
	}

	// Test JSON round-trip
	jsonStr, err := def.ToJSON()
	require.NoError(t, err)

	jsonDef, err := FromJSON(jsonStr)
	require.NoError(t, err)
	assert.Equal(t, def.Name, jsonDef.Name)
	assert.Equal(t, len(def.Nodes), len(jsonDef.Nodes))

	// Test YAML round-trip
	yamlStr, err := def.ToYAML()
	require.NoError(t, err)

	yamlDef, err := FromYAML(yamlStr)
	require.NoError(t, err)
	assert.Equal(t, def.Name, yamlDef.Name)
	assert.Equal(t, len(def.Nodes), len(yamlDef.Nodes))

	// Test file round-trip
	tmpDir := t.TempDir()

	jsonFile := filepath.Join(tmpDir, "workflow.json")
	err = def.SaveToJSONFile(jsonFile)
	require.NoError(t, err)

	loadedJSON, err := LoadFromJSONFile(jsonFile)
	require.NoError(t, err)
	assert.Equal(t, def.Name, loadedJSON.Name)

	yamlFile := filepath.Join(tmpDir, "workflow.yaml")
	err = def.SaveToYAMLFile(yamlFile)
	require.NoError(t, err)

	loadedYAML, err := LoadFromYAMLFile(yamlFile)
	require.NoError(t, err)
	assert.Equal(t, def.Name, loadedYAML.Name)
}
