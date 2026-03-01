package workflow

import (
	"encoding/json"
	"fmt"
	"time"
)

// VisualWorkflow represents a workflow designed in visual builder.
type VisualWorkflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Nodes       []VisualNode   `json:"nodes"`
	Edges       []VisualEdge   `json:"edges"`
	Variables   []Variable     `json:"variables,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// VisualNode represents a node in the visual workflow.
type VisualNode struct {
	ID       string         `json:"id"`
	Type     VisualNodeType `json:"type"`
	Label    string         `json:"label"`
	Position Position       `json:"position"`
	Config   NodeConfig     `json:"config"`
	Inputs   []Port         `json:"inputs,omitempty"`
	Outputs  []Port         `json:"outputs,omitempty"`
}

// VisualNodeType defines visual node types.
type VisualNodeType string

const (
	VNodeStart     VisualNodeType = "start"
	VNodeEnd       VisualNodeType = "end"
	VNodeLLM       VisualNodeType = "llm"
	VNodeTool      VisualNodeType = "tool"
	VNodeCondition VisualNodeType = "condition"
	VNodeLoop      VisualNodeType = "loop"
	VNodeParallel  VisualNodeType = "parallel"
	VNodeHuman     VisualNodeType = "human_input"
	VNodeCode      VisualNodeType = "code"
	VNodeSubflow   VisualNodeType = "subflow"
)

// Position represents node position in visual canvas.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Port represents an input/output port on a node.
type Port struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // string, number, boolean, object, array
}

// NodeConfig contains node-specific configuration.
type NodeConfig struct {
	// LLM node config
	Model       string  `json:"model,omitempty"`
	Prompt      string  `json:"prompt,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`

	// Tool node config
	ToolName   string         `json:"tool_name,omitempty"`
	ToolParams map[string]any `json:"tool_params,omitempty"`

	// Condition node config
	Condition  string `json:"condition,omitempty"`
	Expression string `json:"expression,omitempty"`

	// Loop node config
	LoopType      string `json:"loop_type,omitempty"`
	MaxIterations int    `json:"max_iterations,omitempty"`

	// Code node config
	Code     string `json:"code,omitempty"`
	Language string `json:"language,omitempty"`

	// Human input config
	InputPrompt string   `json:"input_prompt,omitempty"`
	InputType   string   `json:"input_type,omitempty"`
	Options     []string `json:"options,omitempty"`
	Timeout     int      `json:"timeout_seconds,omitempty"`

	// Subflow config
	SubflowID string `json:"subflow_id,omitempty"`
}

// VisualEdge represents a connection between nodes.
type VisualEdge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	SourcePort string `json:"source_port,omitempty"`
	Target     string `json:"target"`
	TargetPort string `json:"target_port,omitempty"`
	Label      string `json:"label,omitempty"`
	Condition  string `json:"condition,omitempty"` // For conditional edges
}

// Variable represents a workflow variable.
type Variable struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	DefaultValue any    `json:"default_value,omitempty"`
	Description  string `json:"description,omitempty"`
}

// VisualBuilder builds DAG workflows from visual definitions.
type VisualBuilder struct {
	stepRegistry map[string]Step
}

// NewVisualBuilder creates a new visual builder.
func NewVisualBuilder() *VisualBuilder {
	return &VisualBuilder{
		stepRegistry: make(map[string]Step),
	}
}

// RegisterStep registers a step implementation.
func (b *VisualBuilder) RegisterStep(name string, step Step) {
	b.stepRegistry[name] = step
}

// Build converts a visual workflow to executable DAG.
func (b *VisualBuilder) Build(vw *VisualWorkflow) (*DAGWorkflow, error) {
	graph := NewDAGGraph()

	// Create nodes
	for _, vnode := range vw.Nodes {
		dagNode, err := b.convertNode(vnode)
		if err != nil {
			return nil, fmt.Errorf("failed to convert node %s: %w", vnode.ID, err)
		}
		graph.AddNode(dagNode)

		if vnode.Type == VNodeStart {
			graph.SetEntry(vnode.ID)
		}
	}

	// Create edges
	for _, edge := range vw.Edges {
		graph.AddEdge(edge.Source, edge.Target)
	}

	return NewDAGWorkflow(vw.Name, vw.Description, graph), nil
}

func (b *VisualBuilder) convertNode(vnode VisualNode) (*DAGNode, error) {
	dagNode := &DAGNode{
		ID:       vnode.ID,
		Metadata: make(map[string]any),
	}

	switch vnode.Type {
	case VNodeStart, VNodeEnd:
		dagNode.Type = NodeTypeAction
		dagNode.Step = &PassthroughStep{}

	case VNodeLLM:
		dagNode.Type = NodeTypeAction
		dagNode.Step = &LLMStep{
			Model:       vnode.Config.Model,
			Prompt:      vnode.Config.Prompt,
			Temperature: vnode.Config.Temperature,
			MaxTokens:   vnode.Config.MaxTokens,
		}

	case VNodeTool:
		dagNode.Type = NodeTypeAction
		if step, ok := b.stepRegistry[vnode.Config.ToolName]; ok {
			dagNode.Step = step
		} else {
			dagNode.Step = &ToolStep{
				ToolName: vnode.Config.ToolName,
				Params:   vnode.Config.ToolParams,
			}
		}

	case VNodeCondition:
		dagNode.Type = NodeTypeCondition

	case VNodeLoop:
		dagNode.Type = NodeTypeLoop
		dagNode.LoopConfig = &LoopConfig{
			Type:          LoopType(vnode.Config.LoopType),
			MaxIterations: vnode.Config.MaxIterations,
		}

	case VNodeParallel:
		dagNode.Type = NodeTypeParallel

	case VNodeHuman:
		dagNode.Type = NodeTypeAction
		dagNode.Step = &HumanInputStep{
			Prompt:  vnode.Config.InputPrompt,
			Type:    vnode.Config.InputType,
			Options: vnode.Config.Options,
			Timeout: vnode.Config.Timeout,
		}

	case VNodeSubflow:
		dagNode.Type = NodeTypeSubGraph

	default:
		return nil, fmt.Errorf("unknown node type: %s", vnode.Type)
	}

	dagNode.Metadata["label"] = vnode.Label
	dagNode.Metadata["position"] = vnode.Position

	return dagNode, nil
}

// Export exports visual workflow to JSON.
func (vw *VisualWorkflow) Export() ([]byte, error) {
	return json.MarshalIndent(vw, "", "  ")
}

// Import imports visual workflow from JSON.
func Import(data []byte) (*VisualWorkflow, error) {
	var vw VisualWorkflow
	if err := json.Unmarshal(data, &vw); err != nil {
		return nil, err
	}
	return &vw, nil
}

// Validate validates the visual workflow.
func (vw *VisualWorkflow) Validate() error {
	if vw.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(vw.Nodes) == 0 {
		return fmt.Errorf("workflow must have at least one node")
	}

	// Check for start node
	hasStart := false
	for _, node := range vw.Nodes {
		if node.Type == VNodeStart {
			hasStart = true
			break
		}
	}
	if !hasStart {
		return fmt.Errorf("workflow must have a start node")
	}

	// Validate edges reference existing nodes
	nodeIDs := make(map[string]bool)
	for _, node := range vw.Nodes {
		nodeIDs[node.ID] = true
	}
	for _, edge := range vw.Edges {
		if !nodeIDs[edge.Source] {
			return fmt.Errorf("edge references unknown source: %s", edge.Source)
		}
		if !nodeIDs[edge.Target] {
			return fmt.Errorf("edge references unknown target: %s", edge.Target)
		}
	}

	return nil
}
