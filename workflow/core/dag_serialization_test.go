package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDAGDefinitionJSONYAMLRoundTripAndFiles(t *testing.T) {
	def := sampleSerializableDAGDefinition()

	jsonText, err := def.ToJSON()
	require.NoError(t, err)
	assert.Contains(t, jsonText, `"name": "serial"`)
	fromJSON, err := FromJSON(jsonText)
	require.NoError(t, err)
	assert.Equal(t, def.Name, fromJSON.Name)
	assert.Equal(t, def.Entry, fromJSON.Entry)

	yamlText, err := def.ToYAML()
	require.NoError(t, err)
	assert.Contains(t, yamlText, "name: serial")
	fromYAML, err := FromYAML(yamlText)
	require.NoError(t, err)
	assert.Equal(t, def.Name, fromYAML.Name)
	assert.Equal(t, len(def.Nodes), len(fromYAML.Nodes))

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "workflow.json")
	yamlPath := filepath.Join(dir, "workflow.yaml")
	require.NoError(t, def.SaveToJSONFile(jsonPath))
	require.NoError(t, def.SaveToYAMLFile(yamlPath))
	loadedJSON, err := LoadFromJSONFile(jsonPath)
	require.NoError(t, err)
	loadedYAML, err := LoadFromYAMLFile(yamlPath)
	require.NoError(t, err)
	assert.Equal(t, "serial", loadedJSON.Name)
	assert.Equal(t, "serial", loadedYAML.Name)
}

func TestValidateDAGDefinitionRejectsInvalidShapes(t *testing.T) {
	cases := []struct {
		name string
		def  *DAGDefinition
		want string
	}{
		{"missing name", &DAGDefinition{}, "workflow name is required"},
		{"missing nodes", &DAGDefinition{Name: "x", Entry: "start"}, "workflow must have at least one node"},
		{"missing entry", &DAGDefinition{Name: "x", Nodes: []NodeDefinition{{ID: "n", Type: string(NodeTypeCheckpoint)}}}, "entry node is required"},
		{"duplicate node", &DAGDefinition{Name: "x", Entry: "n", Nodes: []NodeDefinition{{ID: "n", Type: string(NodeTypeCheckpoint)}, {ID: "n", Type: string(NodeTypeCheckpoint)}}}, "duplicate node ID"},
		{"missing action step", &DAGDefinition{Name: "x", Entry: "n", Nodes: []NodeDefinition{{ID: "n", Type: string(NodeTypeAction)}}}, "action node requires step"},
		{"bad next", &DAGDefinition{Name: "x", Entry: "n", Nodes: []NodeDefinition{{ID: "n", Type: string(NodeTypeCheckpoint), Next: []string{"missing"}}}}, "next node missing does not exist"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDAGDefinition(tt.def)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDAGDefinitionConvertsToWorkflowAndBack(t *testing.T) {
	workflow, err := sampleSerializableDAGDefinition().ToDAGWorkflow()
	require.NoError(t, err)
	assert.Equal(t, "serial", workflow.Name())
	assert.Equal(t, "serialization coverage", workflow.Description())
	assert.Equal(t, "start", workflow.Graph().GetEntry())
	_, ok := workflow.GetMetadata("owner")
	assert.True(t, ok)

	def := workflow.ToDAGDefinition()
	assert.Equal(t, "serial", def.Name)
	assert.Equal(t, "start", def.Entry)
	assert.NotEmpty(t, def.Nodes)
}

func TestDAGSerializationErrorPaths(t *testing.T) {
	_, err := FromJSON(`{"name":`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal from JSON")

	_, err = FromYAML("name: [")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal from YAML")

	_, err = LoadFromJSONFile(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")

	badPath := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(badPath, []byte(`{"name":"bad"}`), 0o644))
	_, err = LoadFromJSONFile(badPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")

	err = (&DAGDefinition{}).SaveToJSONFile(strings.Repeat("x", 40000))
	require.Error(t, err)
}

func sampleSerializableDAGDefinition() *DAGDefinition {
	return &DAGDefinition{
		Name:        "serial",
		Description: "serialization coverage",
		Entry:       "start",
		Metadata:    map[string]any{"owner": "test"},
		Nodes: []NodeDefinition{
			{ID: "start", Type: string(NodeTypeAction), Step: "passthrough", Next: []string{"check"}, Metadata: map[string]any{"kind": "entry"}},
			{ID: "check", Type: string(NodeTypeCondition), Condition: "ok", OnTrue: []string{"loop"}, OnFalse: []string{"done"}},
			{ID: "loop", Type: string(NodeTypeLoop), Loop: &LoopDefinition{Type: string(LoopTypeFor), MaxIterations: 2}, Next: []string{"done"}},
			{ID: "done", Type: string(NodeTypeCheckpoint)},
		},
	}
}
