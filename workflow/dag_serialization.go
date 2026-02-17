package workflow

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MarshalJSON serializes a DAGDefinition to JSON
func (d *DAGDefinition) MarshalJSON() ([]byte, error) {
	// Use default JSON marshaling
	type Alias DAGDefinition
	return json.Marshal((*Alias)(d))
}

// UnmarshalJSON deserializes a DAGDefinition from JSON
func (d *DAGDefinition) UnmarshalJSON(data []byte) error {
	// Use default JSON unmarshaling
	type Alias DAGDefinition
	aux := (*Alias)(d)
	if err := json.Unmarshal(data, aux); err != nil {
		return fmt.Errorf("failed to unmarshal DAGDefinition: %w", err)
	}
	return nil
}

// MarshalYAML serializes a DAGDefinition to YAML
func (d *DAGDefinition) MarshalYAML() (interface{}, error) {
	// Return the struct itself for YAML marshaling
	type Alias DAGDefinition
	return (*Alias)(d), nil
}

// UnmarshalYAML deserializes a DAGDefinition from YAML
func (d *DAGDefinition) UnmarshalYAML(node *yaml.Node) error {
	// Use default YAML unmarshaling
	type Alias DAGDefinition
	aux := (*Alias)(d)
	if err := node.Decode(aux); err != nil {
		return fmt.Errorf("failed to unmarshal DAGDefinition: %w", err)
	}
	return nil
}

// ToJSON converts a DAGDefinition to JSON string
func (d *DAGDefinition) ToJSON() (string, error) {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	return string(data), nil
}

// ToYAML converts a DAGDefinition to YAML string
func (d *DAGDefinition) ToYAML() (string, error) {
	data, err := yaml.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to YAML: %w", err)
	}
	return string(data), nil
}

// FromJSON creates a DAGDefinition from JSON string
func FromJSON(jsonStr string) (*DAGDefinition, error) {
	var def DAGDefinition
	if err := json.Unmarshal([]byte(jsonStr), &def); err != nil {
		return nil, fmt.Errorf("failed to unmarshal from JSON: %w", err)
	}
	
	// Validate the loaded definition
	if err := ValidateDAGDefinition(&def); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	
	return &def, nil
}

// FromYAML creates a DAGDefinition from YAML string
func FromYAML(yamlStr string) (*DAGDefinition, error) {
	var def DAGDefinition
	if err := yaml.Unmarshal([]byte(yamlStr), &def); err != nil {
		return nil, fmt.Errorf("failed to unmarshal from YAML: %w", err)
	}
	
	// Validate the loaded definition
	if err := ValidateDAGDefinition(&def); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	
	return &def, nil
}

// LoadFromJSONFile loads a DAGDefinition from a JSON file
func LoadFromJSONFile(filename string) (*DAGDefinition, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	return FromJSON(string(data))
}

// LoadFromYAMLFile loads a DAGDefinition from a YAML file
func LoadFromYAMLFile(filename string) (*DAGDefinition, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	return FromYAML(string(data))
}

// SaveToJSONFile saves a DAGDefinition to a JSON file
func (d *DAGDefinition) SaveToJSONFile(filename string) error {
	jsonStr, err := d.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal DAG to JSON: %w", err)
	}
	
	if err := os.WriteFile(filename, []byte(jsonStr), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	return nil
}

// SaveToYAMLFile saves a DAGDefinition to a YAML file
func (d *DAGDefinition) SaveToYAMLFile(filename string) error {
	yamlStr, err := d.ToYAML()
	if err != nil {
		return fmt.Errorf("marshal DAG to YAML: %w", err)
	}
	
	if err := os.WriteFile(filename, []byte(yamlStr), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	return nil
}

// ValidateDAGDefinition validates a loaded DAGDefinition
func ValidateDAGDefinition(def *DAGDefinition) error {
	if def.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	
	if len(def.Nodes) == 0 {
		return fmt.Errorf("workflow must have at least one node")
	}
	
	if def.Entry == "" {
		return fmt.Errorf("entry node is required")
	}
	
	// Check that entry node exists
	entryExists := false
	nodeIDs := make(map[string]bool)
	
	for _, node := range def.Nodes {
		if node.ID == "" {
			return fmt.Errorf("node ID is required")
		}
		
		if nodeIDs[node.ID] {
			return fmt.Errorf("duplicate node ID: %s", node.ID)
		}
		nodeIDs[node.ID] = true
		
		if node.ID == def.Entry {
			entryExists = true
		}
		
		// Validate node type
		if node.Type == "" {
			return fmt.Errorf("node %s: type is required", node.ID)
		}
		
		// Validate node-specific requirements
		switch NodeType(node.Type) {
		case NodeTypeAction:
			if node.Step == "" {
				return fmt.Errorf("node %s: action node requires step", node.ID)
			}
		case NodeTypeCondition:
			if node.Condition == "" {
				return fmt.Errorf("node %s: condition node requires condition", node.ID)
			}
			if len(node.OnTrue) == 0 && len(node.OnFalse) == 0 {
				return fmt.Errorf("node %s: condition node requires at least one branch (on_true or on_false)", node.ID)
			}
		case NodeTypeLoop:
			if node.Loop == nil {
				return fmt.Errorf("node %s: loop node requires loop configuration", node.ID)
			}
			if node.Loop.Type == "" {
				return fmt.Errorf("node %s: loop type is required", node.ID)
			}
			switch LoopType(node.Loop.Type) {
			case LoopTypeWhile:
				if node.Loop.Condition == "" {
					return fmt.Errorf("node %s: while loop requires condition", node.ID)
				}
			case LoopTypeFor:
				if node.Loop.MaxIterations <= 0 {
					return fmt.Errorf("node %s: for loop requires positive max_iterations", node.ID)
				}
			case LoopTypeForEach:
				// ForEach validation - iterator will be set at runtime
				if node.Loop.MaxIterations <= 0 {
					return fmt.Errorf("node %s: foreach loop requires positive max_iterations", node.ID)
				}
			default:
				return fmt.Errorf("node %s: invalid loop type: %s", node.ID, node.Loop.Type)
			}
		case NodeTypeParallel:
			if len(node.Next) < 2 {
				return fmt.Errorf("node %s: parallel node requires at least 2 next nodes", node.ID)
			}
		case NodeTypeSubGraph:
			if node.SubGraph == nil {
				return fmt.Errorf("node %s: subgraph node requires subgraph", node.ID)
			}
			// Recursively validate subgraph
			if err := ValidateDAGDefinition(node.SubGraph); err != nil {
				return fmt.Errorf("node %s: subgraph validation failed: %w", node.ID, err)
			}
		case NodeTypeCheckpoint:
			// Checkpoint nodes don't require additional validation
		default:
			return fmt.Errorf("node %s: invalid node type: %s", node.ID, node.Type)
		}
	}
	
	if !entryExists {
		return fmt.Errorf("entry node %s does not exist", def.Entry)
	}
	
	// Validate that all referenced nodes exist
	for _, node := range def.Nodes {
		// Check Next references
		for _, nextID := range node.Next {
			if !nodeIDs[nextID] {
				return fmt.Errorf("node %s: next node %s does not exist", node.ID, nextID)
			}
		}
		
		// Check OnTrue references
		for _, trueID := range node.OnTrue {
			if !nodeIDs[trueID] {
				return fmt.Errorf("node %s: on_true node %s does not exist", node.ID, trueID)
			}
		}
		
		// Check OnFalse references
		for _, falseID := range node.OnFalse {
			if !nodeIDs[falseID] {
				return fmt.Errorf("node %s: on_false node %s does not exist", node.ID, falseID)
			}
		}
	}
	
	return nil
}

// ToDAGDefinition converts a DAGWorkflow to a DAGDefinition for serialization
// Note: This only captures the structure, not the runtime functions (conditions, iterators, steps)
func (w *DAGWorkflow) ToDAGDefinition() *DAGDefinition {
	def := &DAGDefinition{
		Name:        w.name,
		Description: w.description,
		Entry:       w.graph.entry,
		Nodes:       make([]NodeDefinition, 0, len(w.graph.nodes)),
		Metadata:    w.metadata,
	}
	
	// Convert nodes
	for _, node := range w.graph.nodes {
		nodeDef := NodeDefinition{
			ID:       node.ID,
			Type:     string(node.Type),
			Metadata: node.Metadata,
		}
		
		// Set node-specific fields
		if node.Step != nil {
			nodeDef.Step = node.Step.Name()
		}
		
		if node.LoopConfig != nil {
			nodeDef.Loop = &LoopDefinition{
				Type:          string(node.LoopConfig.Type),
				MaxIterations: node.LoopConfig.MaxIterations,
			}
		}
		
		if node.SubGraph != nil {
			// Recursively convert subgraph
			subWorkflow := &DAGWorkflow{
				name:        w.name + "_subgraph",
				description: "Subgraph",
				graph:       node.SubGraph,
				metadata:    make(map[string]interface{}),
			}
			nodeDef.SubGraph = subWorkflow.ToDAGDefinition()
		}
		
		// Get edges for this node
		if edges, ok := w.graph.edges[node.ID]; ok {
			nodeDef.Next = edges
		}
		
		def.Nodes = append(def.Nodes, nodeDef)
	}
	
	return def
}
