package workflow

import (
	"fmt"

	"go.uber.org/zap"
)

// DAGBuilder provides a fluent API for constructing DAG workflows
type DAGBuilder struct {
	graph  *DAGGraph
	name   string
	desc   string
	logger *zap.Logger
}

// NewDAGBuilder creates a new DAG builder with the given name
func NewDAGBuilder(name string) *DAGBuilder {
	logger, _ := zap.NewProduction()
	return &DAGBuilder{
		graph:  NewDAGGraph(),
		name:   name,
		logger: logger.With(zap.String("component", "dag_builder")),
	}
}

// WithDescription sets the workflow description
func (b *DAGBuilder) WithDescription(desc string) *DAGBuilder {
	b.desc = desc
	return b
}

// WithLogger sets a custom logger
func (b *DAGBuilder) WithLogger(logger *zap.Logger) *DAGBuilder {
	b.logger = logger.With(zap.String("component", "dag_builder"))
	return b
}

// AddNode adds a node to the graph and returns a NodeBuilder for configuration
func (b *DAGBuilder) AddNode(id string, nodeType NodeType) *NodeBuilder {
	node := &DAGNode{
		ID:       id,
		Type:     nodeType,
		Metadata: make(map[string]any),
	}
	b.graph.AddNode(node)

	return &NodeBuilder{
		node:   node,
		parent: b,
	}
}

// AddEdge adds a directed edge from one node to another
func (b *DAGBuilder) AddEdge(from, to string) *DAGBuilder {
	b.graph.AddEdge(from, to)
	return b
}

// SetEntry sets the entry node for the workflow
func (b *DAGBuilder) SetEntry(nodeID string) *DAGBuilder {
	b.graph.SetEntry(nodeID)
	return b
}

// Build validates the DAG and creates a DAGWorkflow
func (b *DAGBuilder) Build() (*DAGWorkflow, error) {
	// Validate the graph
	if err := b.validate(); err != nil {
		return nil, fmt.Errorf("DAG validation failed: %w", err)
	}

	// Create the workflow
	workflow := NewDAGWorkflow(b.name, b.desc, b.graph)

	b.logger.Info("DAG workflow built successfully",
		zap.String("name", b.name),
		zap.Int("nodes", len(b.graph.nodes)),
		zap.String("entry", b.graph.entry),
	)

	return workflow, nil
}

// validate performs comprehensive validation of the DAG
func (b *DAGBuilder) validate() error {
	// Check if graph has nodes
	if len(b.graph.nodes) == 0 {
		return fmt.Errorf("graph has no nodes")
	}

	// Check if entry node is set
	if b.graph.entry == "" {
		return fmt.Errorf("entry node not set")
	}

	// Check if entry node exists
	if _, exists := b.graph.GetNode(b.graph.entry); !exists {
		return fmt.Errorf("entry node does not exist: %s", b.graph.entry)
	}

	// Validate all edges reference existing nodes
	for fromID, toIDs := range b.graph.edges {
		if _, exists := b.graph.GetNode(fromID); !exists {
			return fmt.Errorf("edge references non-existent source node: %s", fromID)
		}
		for _, toID := range toIDs {
			if _, exists := b.graph.GetNode(toID); !exists {
				return fmt.Errorf("edge references non-existent target node: %s", toID)
			}
		}
	}

	// Detect cycles
	if err := b.detectCycles(); err != nil {
		return err
	}

	// Detect orphaned nodes (nodes not reachable from entry)
	if err := b.detectOrphanedNodes(); err != nil {
		return err
	}

	// Validate node configurations
	if err := b.validateNodes(); err != nil {
		return err
	}

	return nil
}

// detectCycles detects cycles in the graph using DFS
func (b *DAGBuilder) detectCycles() error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	// Check for cycles starting from each node
	for nodeID := range b.graph.nodes {
		if !visited[nodeID] {
			if b.hasCycleDFS(nodeID, visited, recStack) {
				return fmt.Errorf("cycle detected in graph involving node: %s", nodeID)
			}
		}
	}

	return nil
}

// hasCycleDFS performs DFS to detect cycles
func (b *DAGBuilder) hasCycleDFS(nodeID string, visited, recStack map[string]bool) bool {
	visited[nodeID] = true
	recStack[nodeID] = true

	// Visit all neighbors
	for _, neighborID := range b.graph.GetEdges(nodeID) {
		if !visited[neighborID] {
			if b.hasCycleDFS(neighborID, visited, recStack) {
				return true
			}
		} else if recStack[neighborID] {
			// Back edge found - cycle detected
			return true
		}
	}

	recStack[nodeID] = false
	return false
}

// detectOrphanedNodes detects nodes not reachable from the entry node
func (b *DAGBuilder) detectOrphanedNodes() error {
	reachable := make(map[string]bool)
	b.markReachable(b.graph.entry, reachable)

	// Check if all nodes are reachable
	orphaned := []string{}
	for nodeID := range b.graph.nodes {
		if !reachable[nodeID] {
			orphaned = append(orphaned, nodeID)
		}
	}

	if len(orphaned) > 0 {
		return fmt.Errorf("orphaned nodes detected (not reachable from entry): %v", orphaned)
	}

	return nil
}

// markReachable marks all nodes reachable from the given node
func (b *DAGBuilder) markReachable(nodeID string, reachable map[string]bool) {
	if reachable[nodeID] {
		return
	}

	reachable[nodeID] = true

	// Recursively mark neighbors
	for _, neighborID := range b.graph.GetEdges(nodeID) {
		b.markReachable(neighborID, reachable)
	}

	// For condition nodes, also mark on_true and on_false branches
	if node, exists := b.graph.GetNode(nodeID); exists {
		if node.Type == NodeTypeCondition {
			if onTrue, ok := node.Metadata["on_true"].([]string); ok {
				for _, id := range onTrue {
					b.markReachable(id, reachable)
				}
			}
			if onFalse, ok := node.Metadata["on_false"].([]string); ok {
				for _, id := range onFalse {
					b.markReachable(id, reachable)
				}
			}
		}
	}
}

// validateNodes validates individual node configurations
func (b *DAGBuilder) validateNodes() error {
	for nodeID, node := range b.graph.nodes {
		switch node.Type {
		case NodeTypeAction:
			if node.Step == nil {
				return fmt.Errorf("action node %s has no step configured", nodeID)
			}

		case NodeTypeCondition:
			if node.Condition == nil {
				return fmt.Errorf("condition node %s has no condition function configured", nodeID)
			}
			// Condition nodes should have on_true or on_false metadata or edges
			hasRouting := false
			if _, ok := node.Metadata["on_true"]; ok {
				hasRouting = true
			}
			if _, ok := node.Metadata["on_false"]; ok {
				hasRouting = true
			}
			if len(b.graph.GetEdges(nodeID)) > 0 {
				hasRouting = true
			}
			if !hasRouting {
				return fmt.Errorf("condition node %s has no routing configured", nodeID)
			}

		case NodeTypeLoop:
			if node.LoopConfig == nil {
				return fmt.Errorf("loop node %s has no loop configuration", nodeID)
			}
			// Validate loop configuration
			if err := b.validateLoopConfig(nodeID, node.LoopConfig); err != nil {
				return err
			}

		case NodeTypeSubGraph:
			if node.SubGraph == nil {
				return fmt.Errorf("subgraph node %s has no subgraph configured", nodeID)
			}

		case NodeTypeParallel:
			// Parallel nodes should have multiple outgoing edges
			if len(b.graph.GetEdges(nodeID)) < 2 {
				return fmt.Errorf("parallel node %s should have at least 2 outgoing edges", nodeID)
			}

		case NodeTypeCheckpoint:
			// Checkpoint nodes don't require special configuration

		default:
			return fmt.Errorf("unknown node type: %s", node.Type)
		}
	}

	return nil
}

// validateLoopConfig validates loop configuration
func (b *DAGBuilder) validateLoopConfig(nodeID string, config *LoopConfig) error {
	switch config.Type {
	case LoopTypeWhile:
		if config.Condition == nil {
			return fmt.Errorf("while loop node %s requires condition function", nodeID)
		}

	case LoopTypeFor:
		if config.MaxIterations <= 0 {
			return fmt.Errorf("for loop node %s requires positive max_iterations", nodeID)
		}

	case LoopTypeForEach:
		if config.Iterator == nil {
			return fmt.Errorf("foreach loop node %s requires iterator function", nodeID)
		}

	default:
		return fmt.Errorf("unknown loop type: %s", config.Type)
	}

	return nil
}

// NodeBuilder provides a fluent API for configuring individual nodes
type NodeBuilder struct {
	node   *DAGNode
	parent *DAGBuilder
}

// WithStep sets the step for an action node
func (nb *NodeBuilder) WithStep(step Step) *NodeBuilder {
	nb.node.Step = step
	return nb
}

// WithCondition sets the condition function for a conditional node
func (nb *NodeBuilder) WithCondition(cond ConditionFunc) *NodeBuilder {
	nb.node.Condition = cond
	return nb
}

// WithOnTrue sets the nodes to execute when condition is true
func (nb *NodeBuilder) WithOnTrue(nodeIDs ...string) *NodeBuilder {
	nb.node.Metadata["on_true"] = nodeIDs
	return nb
}

// WithOnFalse sets the nodes to execute when condition is false
func (nb *NodeBuilder) WithOnFalse(nodeIDs ...string) *NodeBuilder {
	nb.node.Metadata["on_false"] = nodeIDs
	return nb
}

// WithLoop sets the loop configuration for a loop node
func (nb *NodeBuilder) WithLoop(config LoopConfig) *NodeBuilder {
	nb.node.LoopConfig = &config
	return nb
}

// WithSubGraph sets the subgraph for a subgraph node
func (nb *NodeBuilder) WithSubGraph(subGraph *DAGGraph) *NodeBuilder {
	nb.node.SubGraph = subGraph
	return nb
}

// WithMetadata sets a metadata value
func (nb *NodeBuilder) WithMetadata(key string, value any) *NodeBuilder {
	nb.node.Metadata[key] = value
	return nb
}

// WithErrorConfig sets the error handling configuration for a node
func (nb *NodeBuilder) WithErrorConfig(config ErrorConfig) *NodeBuilder {
	nb.node.ErrorConfig = &config
	return nb
}

// Done completes node configuration and returns to the DAGBuilder
func (nb *NodeBuilder) Done() *DAGBuilder {
	return nb.parent
}
