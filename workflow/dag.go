package workflow

import (
	"context"
	"time"
)

// NodeType defines the type of a DAG node
type NodeType string

const (
	// NodeTypeAction executes a step
	NodeTypeAction NodeType = "action"
	// NodeTypeCondition performs conditional branching
	NodeTypeCondition NodeType = "condition"
	// NodeTypeLoop performs loop iteration
	NodeTypeLoop NodeType = "loop"
	// NodeTypeParallel executes nodes concurrently
	NodeTypeParallel NodeType = "parallel"
	// NodeTypeSubGraph executes a nested workflow
	NodeTypeSubGraph NodeType = "subgraph"
	// NodeTypeCheckpoint creates a checkpoint
	NodeTypeCheckpoint NodeType = "checkpoint"
)

// LoopType defines the type of loop
type LoopType string

const (
	// LoopTypeWhile executes while condition is true
	LoopTypeWhile LoopType = "while"
	// LoopTypeFor executes for a fixed number of iterations
	LoopTypeFor LoopType = "for"
	// LoopTypeForEach executes for each item in a collection
	LoopTypeForEach LoopType = "foreach"
)

// ErrorStrategy defines how errors should be handled
type ErrorStrategy string

const (
	// ErrorStrategyFailFast stops execution immediately on error
	ErrorStrategyFailFast ErrorStrategy = "fail_fast"
	// ErrorStrategySkip skips the failed node and continues
	ErrorStrategySkip ErrorStrategy = "skip"
	// ErrorStrategyRetry retries the failed node
	ErrorStrategyRetry ErrorStrategy = "retry"
)

// ErrorConfig defines error handling behavior for a node
type ErrorConfig struct {
	// Strategy specifies how to handle errors
	Strategy ErrorStrategy
	// MaxRetries is the maximum number of retry attempts (for retry strategy)
	MaxRetries int
	// RetryDelayMs is the delay between retries in milliseconds
	RetryDelayMs int
	// FallbackValue is the value to use when skipping a failed node
	FallbackValue any
}

// ConditionFunc evaluates a condition and returns true or false
type ConditionFunc func(ctx context.Context, input any) (bool, error)

// IteratorFunc generates a collection of items for iteration
type IteratorFunc func(ctx context.Context, input any) ([]any, error)

// LoopConfig defines loop behavior
type LoopConfig struct {
	// Type specifies the loop type (while, for, foreach)
	Type LoopType
	// MaxIterations limits the maximum number of iterations (0 = unlimited)
	MaxIterations int
	// Condition evaluates whether to continue looping (for while loops)
	Condition ConditionFunc
	// Iterator generates items to iterate over (for foreach loops)
	Iterator IteratorFunc
}

// DAGNode represents a single node in the workflow graph
type DAGNode struct {
	// ID is the unique identifier for this node
	ID string
	// Type specifies the node type
	Type NodeType
	// Step is the step to execute (for action nodes)
	Step Step
	// Condition evaluates branching logic (for conditional nodes)
	Condition ConditionFunc
	// LoopConfig defines loop behavior (for loop nodes)
	LoopConfig *LoopConfig
	// SubGraph is a nested workflow (for subgraph nodes)
	SubGraph *DAGGraph
	// ErrorConfig defines error handling behavior
	ErrorConfig *ErrorConfig
	// Metadata stores additional node information
	Metadata map[string]any
}

// DAGGraph represents the workflow structure as a directed acyclic graph
type DAGGraph struct {
	// nodes maps node IDs to node instances
	nodes map[string]*DAGNode
	// edges maps node IDs to their dependent node IDs
	// edges[nodeID] = [dependentNodeID1, dependentNodeID2, ...]
	edges map[string][]string
	// entry is the ID of the entry node
	entry string
}

// NewDAGGraph creates a new empty DAG graph
func NewDAGGraph() *DAGGraph {
	return &DAGGraph{
		nodes: make(map[string]*DAGNode),
		edges: make(map[string][]string),
	}
}

// AddNode adds a node to the graph
func (g *DAGGraph) AddNode(node *DAGNode) {
	g.nodes[node.ID] = node
}

// AddEdge adds a directed edge from one node to another
func (g *DAGGraph) AddEdge(fromID, toID string) {
	g.edges[fromID] = append(g.edges[fromID], toID)
}

// SetEntry sets the entry node for the graph
func (g *DAGGraph) SetEntry(nodeID string) {
	g.entry = nodeID
}

// GetNode retrieves a node by ID
func (g *DAGGraph) GetNode(nodeID string) (*DAGNode, bool) {
	node, exists := g.nodes[nodeID]
	return node, exists
}

// GetEdges retrieves the outgoing edges for a node
func (g *DAGGraph) GetEdges(nodeID string) []string {
	return g.edges[nodeID]
}

// GetEntry returns the entry node ID
func (g *DAGGraph) GetEntry() string {
	return g.entry
}

// Nodes returns all nodes in the graph
func (g *DAGGraph) Nodes() map[string]*DAGNode {
	return g.nodes
}

// Edges returns all edges in the graph
func (g *DAGGraph) Edges() map[string][]string {
	return g.edges
}

// DAGDefinition represents a serializable workflow definition
type DAGDefinition struct {
	// Name is the workflow name
	Name string `json:"name" yaml:"name"`
	// Description describes the workflow
	Description string `json:"description" yaml:"description"`
	// Entry is the ID of the entry node
	Entry string `json:"entry" yaml:"entry"`
	// Nodes contains all node definitions
	Nodes []NodeDefinition `json:"nodes" yaml:"nodes"`
	// Metadata stores additional workflow information
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// NodeDefinition represents a serializable node definition
type NodeDefinition struct {
	// ID is the unique node identifier
	ID string `json:"id" yaml:"id"`
	// Type is the node type
	Type string `json:"type" yaml:"type"`
	// Step is the step name (for action nodes)
	Step string `json:"step,omitempty" yaml:"step,omitempty"`
	// Condition is the condition name (for conditional nodes)
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
	// Next lists the next nodes to execute (for action nodes)
	Next []string `json:"next,omitempty" yaml:"next,omitempty"`
	// OnTrue lists nodes to execute when condition is true
	OnTrue []string `json:"on_true,omitempty" yaml:"on_true,omitempty"`
	// OnFalse lists nodes to execute when condition is false
	OnFalse []string `json:"on_false,omitempty" yaml:"on_false,omitempty"`
	// Loop defines loop configuration (for loop nodes)
	Loop *LoopDefinition `json:"loop,omitempty" yaml:"loop,omitempty"`
	// SubGraph defines a nested workflow (for subgraph nodes)
	SubGraph *DAGDefinition `json:"subgraph,omitempty" yaml:"subgraph,omitempty"`
	// Metadata stores additional node information
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// LoopDefinition represents a serializable loop configuration
type LoopDefinition struct {
	// Type is the loop type (while, for, foreach)
	Type string `json:"type" yaml:"type"`
	// MaxIterations limits the maximum number of iterations
	MaxIterations int `json:"max_iterations" yaml:"max_iterations"`
	// Condition is the condition name (for while loops)
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// DAGWorkflow represents a DAG-based workflow
type DAGWorkflow struct {
	name        string
	description string
	graph       *DAGGraph
	metadata    map[string]any
	executor    *DAGExecutor // Optional custom executor
}

// NewDAGWorkflow creates a new DAG workflow
func NewDAGWorkflow(name, description string, graph *DAGGraph) *DAGWorkflow {
	return &DAGWorkflow{
		name:        name,
		description: description,
		graph:       graph,
		metadata:    make(map[string]any),
	}
}

// Name returns the workflow name
func (w *DAGWorkflow) Name() string {
	return w.name
}

// Description returns the workflow description
func (w *DAGWorkflow) Description() string {
	return w.description
}

// Graph returns the underlying DAG graph
func (w *DAGWorkflow) Graph() *DAGGraph {
	return w.graph
}

// SetMetadata sets a metadata value
func (w *DAGWorkflow) SetMetadata(key string, value any) {
	w.metadata[key] = value
}

// GetMetadata retrieves a metadata value
func (w *DAGWorkflow) GetMetadata(key string) (any, bool) {
	value, exists := w.metadata[key]
	return value, exists
}

// Execute executes the DAG workflow using DAGExecutor
func (w *DAGWorkflow) Execute(ctx context.Context, input any) (any, error) {
	// Use custom executor if set, otherwise create default
	executor := w.executor
	if executor == nil {
		executor = NewDAGExecutor(nil, nil)
	}

	// Execute the graph
	return executor.Execute(ctx, w.graph, input)
}

// SetExecutor sets a custom executor for the workflow
func (w *DAGWorkflow) SetExecutor(executor *DAGExecutor) {
	w.executor = executor
}

// ExecutionContext captures the execution state for checkpointing
type ExecutionContext struct {
	// WorkflowID identifies the workflow being executed
	WorkflowID string `json:"workflow_id,omitempty"`
	// CurrentNode is the ID of the currently executing node
	CurrentNode string `json:"current_node,omitempty"`
	// NodeResults stores the results of completed nodes
	NodeResults map[string]any `json:"node_results,omitempty"`
	// Variables stores workflow variables
	Variables map[string]any `json:"variables,omitempty"`
	// StartTime is when the workflow execution started
	StartTime time.Time `json:"start_time,omitempty"`
	// LastUpdateTime is when the context was last updated
	LastUpdateTime time.Time `json:"last_update_time,omitempty"`
}

// NewExecutionContext creates a new execution context
func NewExecutionContext(workflowID string) *ExecutionContext {
	now := time.Now()
	return &ExecutionContext{
		WorkflowID:     workflowID,
		NodeResults:    make(map[string]any),
		Variables:      make(map[string]any),
		StartTime:      now,
		LastUpdateTime: now,
	}
}

// SetCurrentNode updates the currently executing node
func (ec *ExecutionContext) SetCurrentNode(nodeID string) {
	ec.CurrentNode = nodeID
	ec.LastUpdateTime = time.Now()
}

// SetNodeResult stores the result of a completed node
func (ec *ExecutionContext) SetNodeResult(nodeID string, result any) {
	ec.NodeResults[nodeID] = result
	ec.LastUpdateTime = time.Now()
}

// GetNodeResult retrieves the result of a completed node
func (ec *ExecutionContext) GetNodeResult(nodeID string) (any, bool) {
	result, exists := ec.NodeResults[nodeID]
	return result, exists
}

// SetVariable sets a workflow variable
func (ec *ExecutionContext) SetVariable(key string, value any) {
	ec.Variables[key] = value
	ec.LastUpdateTime = time.Now()
}

// GetVariable retrieves a workflow variable
func (ec *ExecutionContext) GetVariable(key string) (any, bool) {
	value, exists := ec.Variables[key]
	return value, exists
}
