package workflow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// DAGExecutor executes DAG workflows with dependency resolution
type DAGExecutor struct {
	checkpointMgr   CheckpointManager
	historyStore    *ExecutionHistoryStore
	logger          *zap.Logger
	circuitBreakers *CircuitBreakerRegistry

	// Execution state
	executionID  string
	threadID     string
	nodeResults  map[string]interface{}
	visitedNodes map[string]bool
	loopDepth    map[string]int // 循环深度追踪
	history      *ExecutionHistory
	mu           sync.RWMutex
}

// 最大循环深度限制
const maxLoopDepth = 1000

// CheckpointManager interface for checkpoint integration
type CheckpointManager interface {
	SaveCheckpoint(ctx context.Context, checkpoint interface{}) error
}

// NewDAGExecutor creates a new DAG executor
func NewDAGExecutor(checkpointMgr CheckpointManager, logger *zap.Logger) *DAGExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DAGExecutor{
		checkpointMgr:   checkpointMgr,
		historyStore:    NewExecutionHistoryStore(),
		logger:          logger.With(zap.String("component", "dag_executor")),
		nodeResults:     make(map[string]interface{}),
		visitedNodes:    make(map[string]bool),
		circuitBreakers: NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig(), nil, logger),
	}
}

// SetHistoryStore sets a custom history store
func (e *DAGExecutor) SetHistoryStore(store *ExecutionHistoryStore) {
	e.historyStore = store
}

// SetCircuitBreakerConfig 设置熔断器配置
func (e *DAGExecutor) SetCircuitBreakerConfig(config CircuitBreakerConfig, handler CircuitBreakerEventHandler) {
	e.circuitBreakers = NewCircuitBreakerRegistry(config, handler, e.logger)
}

// GetCircuitBreakerStates 获取所有熔断器状态
func (e *DAGExecutor) GetCircuitBreakerStates() map[string]CircuitState {
	return e.circuitBreakers.GetAllStates()
}

// Execute runs the DAG workflow with dependency resolution
func (e *DAGExecutor) Execute(ctx context.Context, graph *DAGGraph, input interface{}) (interface{}, error) {
	if graph == nil {
		return nil, fmt.Errorf("graph cannot be nil")
	}

	// Initialize execution state
	e.mu.Lock()
	e.executionID = generateExecutionID()
	e.nodeResults = make(map[string]interface{})
	e.visitedNodes = make(map[string]bool)
	e.loopDepth = make(map[string]int)
	e.history = NewExecutionHistory(e.executionID, "")
	e.mu.Unlock()

	e.logger.Info("starting DAG execution",
		zap.String("execution_id", e.executionID),
		zap.String("entry_node", graph.entry),
	)

	// Validate graph has entry node
	if graph.entry == "" {
		e.history.Complete(fmt.Errorf("graph has no entry node"))
		e.historyStore.Save(e.history)
		return nil, fmt.Errorf("graph has no entry node")
	}

	entryNode, exists := graph.GetNode(graph.entry)
	if !exists {
		err := fmt.Errorf("entry node not found: %s", graph.entry)
		e.history.Complete(err)
		e.historyStore.Save(e.history)
		return nil, err
	}

	// Execute from entry node
	result, err := e.executeNode(ctx, graph, entryNode, input)

	// Complete history
	e.history.Complete(err)
	e.historyStore.Save(e.history)

	if err != nil {
		e.logger.Error("DAG execution failed",
			zap.String("execution_id", e.executionID),
			zap.Error(err),
		)
		return nil, err
	}

	e.logger.Info("DAG execution completed",
		zap.String("execution_id", e.executionID),
		zap.Int("nodes_executed", len(e.visitedNodes)),
	)

	return result, nil
}

// GetHistory returns the execution history for the current execution
func (e *DAGExecutor) GetHistory() *ExecutionHistory {
	return e.history
}

// GetHistoryStore returns the history store
func (e *DAGExecutor) GetHistoryStore() *ExecutionHistoryStore {
	return e.historyStore
}

// executeNode executes a single node based on its type
func (e *DAGExecutor) executeNode(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}) (interface{}, error) {
	// Check if already visited and mark atomically (prevent cycles and duplicate execution)
	e.mu.Lock()
	if e.visitedNodes[node.ID] {
		result := e.nodeResults[node.ID]
		e.mu.Unlock()
		e.logger.Debug("node already visited, skipping",
			zap.String("node_id", node.ID),
		)
		return result, nil
	}
	e.visitedNodes[node.ID] = true
	e.mu.Unlock()

	// Record node start in history
	var nodeExec *NodeExecution
	if e.history != nil {
		nodeExec = e.history.RecordNodeStart(node.ID, node.Type, input)
	}

	e.logger.Debug("executing node",
		zap.String("node_id", node.ID),
		zap.String("node_type", string(node.Type)),
	)

	// 熔断器检查
	cb := e.circuitBreakers.GetOrCreate(node.ID)
	allowed, cbErr := cb.AllowRequest()
	if !allowed {
		e.logger.Warn("node circuit breaker tripped",
			zap.String("node_id", node.ID),
			zap.String("state", cb.GetState().String()),
			zap.Error(cbErr))
		if nodeExec != nil {
			e.history.RecordNodeEnd(nodeExec, nil, cbErr)
		}
		if node.ErrorConfig != nil && node.ErrorConfig.FallbackValue != nil {
			return node.ErrorConfig.FallbackValue, nil
		}
		return nil, cbErr
	}

	startTime := time.Now()
	var result interface{}
	var err error

	// Execute based on node type
	switch node.Type {
	case NodeTypeAction:
		result, err = e.executeActionNode(ctx, graph, node, input)
	case NodeTypeCondition:
		result, err = e.executeConditionNode(ctx, graph, node, input)
	case NodeTypeLoop:
		result, err = e.executeLoopNode(ctx, graph, node, input)
	case NodeTypeParallel:
		result, err = e.executeParallelNode(ctx, graph, node, input)
	case NodeTypeSubGraph:
		result, err = e.executeSubGraphNode(ctx, node, input)
	case NodeTypeCheckpoint:
		result, err = e.executeCheckpointNode(ctx, node, input)
	default:
		err = fmt.Errorf("unknown node type: %s", node.Type)
	}

	duration := time.Since(startTime)

	// Handle errors based on error strategy
	if err != nil {
		result, err = e.handleNodeError(ctx, graph, node, input, err, duration)
		if err != nil {
			cb.RecordFailure()
			// Record failure in history
			if nodeExec != nil {
				e.history.RecordNodeEnd(nodeExec, nil, err)
			}
			return nil, err
		}
	}

	// 记录成功
	cb.RecordSuccess()

	// Record success in history
	if nodeExec != nil {
		e.history.RecordNodeEnd(nodeExec, result, nil)
	}

	// Store result
	e.mu.Lock()
	e.nodeResults[node.ID] = result
	e.mu.Unlock()

	e.logger.Debug("node execution completed",
		zap.String("node_id", node.ID),
		zap.Duration("duration", duration),
	)

	return result, nil
}

// handleNodeError handles errors based on the node's error strategy
func (e *DAGExecutor) handleNodeError(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}, originalErr error, duration time.Duration) (interface{}, error) {
	// Default to fail-fast if no error config
	if node.ErrorConfig == nil {
		e.logger.Error("node execution failed",
			zap.String("node_id", node.ID),
			zap.String("node_type", string(node.Type)),
			zap.Duration("duration", duration),
			zap.Error(originalErr),
		)
		return nil, fmt.Errorf("node %s failed: %w", node.ID, originalErr)
	}

	switch node.ErrorConfig.Strategy {
	case ErrorStrategySkip:
		e.logger.Warn("node execution failed, skipping",
			zap.String("node_id", node.ID),
			zap.Error(originalErr),
		)
		return node.ErrorConfig.FallbackValue, nil

	case ErrorStrategyRetry:
		return e.retryNode(ctx, graph, node, input, originalErr)

	default: // ErrorStrategyFailFast
		e.logger.Error("node execution failed",
			zap.String("node_id", node.ID),
			zap.String("node_type", string(node.Type)),
			zap.Duration("duration", duration),
			zap.Error(originalErr),
		)
		return nil, fmt.Errorf("node %s failed: %w", node.ID, originalErr)
	}
}

// retryNode retries a failed node based on its retry configuration
func (e *DAGExecutor) retryNode(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}, originalErr error) (interface{}, error) {
	maxRetries := node.ErrorConfig.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3 // Default
	}

	retryDelay := time.Duration(node.ErrorConfig.RetryDelayMs) * time.Millisecond
	if retryDelay <= 0 {
		retryDelay = 100 * time.Millisecond // Default
	}

	var lastErr error = originalErr
	for attempt := 1; attempt <= maxRetries; attempt++ {
		e.logger.Debug("retrying node",
			zap.String("node_id", node.ID),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", maxRetries),
		)

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
		}

		// Unmark as visited to allow re-execution
		e.mu.Lock()
		delete(e.visitedNodes, node.ID)
		e.mu.Unlock()

		// Re-execute the node's step directly (not the full node to avoid recursion issues)
		var result interface{}
		var err error

		if node.Type == NodeTypeAction && node.Step != nil {
			result, err = node.Step.Execute(ctx, input)
		} else {
			err = fmt.Errorf("retry only supported for action nodes")
		}

		if err == nil {
			e.logger.Info("node retry succeeded",
				zap.String("node_id", node.ID),
				zap.Int("attempt", attempt),
			)
			return result, nil
		}

		lastErr = err
	}

	e.logger.Error("node retry exhausted",
		zap.String("node_id", node.ID),
		zap.Int("max_retries", maxRetries),
		zap.Error(lastErr),
	)

	// Check if we should use fallback after retry exhaustion
	if node.ErrorConfig.FallbackValue != nil {
		return node.ErrorConfig.FallbackValue, nil
	}

	return nil, fmt.Errorf("node %s failed after %d retries: %w", node.ID, maxRetries, lastErr)
}

// executeActionNode executes an action node and continues to next nodes
func (e *DAGExecutor) executeActionNode(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}) (interface{}, error) {
	if node.Step == nil {
		return nil, fmt.Errorf("action node %s has no step", node.ID)
	}

	e.logger.Debug("executing action step", zap.String("node_id", node.ID))

	result, err := node.Step.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	// Continue to next nodes
	nextNodeIDs := graph.GetEdges(node.ID)
	for _, nextNodeID := range nextNodeIDs {
		nextNode, exists := graph.GetNode(nextNodeID)
		if !exists {
			return nil, fmt.Errorf("next node not found: %s", nextNodeID)
		}
		result, err = e.executeNode(ctx, graph, nextNode, result)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// executeConditionNode executes a conditional node and routes to next nodes
func (e *DAGExecutor) executeConditionNode(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}) (interface{}, error) {
	if node.Condition == nil {
		return nil, fmt.Errorf("condition node %s has no condition function", node.ID)
	}

	e.logger.Debug("evaluating condition", zap.String("node_id", node.ID))

	// Evaluate condition
	conditionResult, err := node.Condition(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("condition evaluation failed: %w", err)
	}

	e.logger.Debug("condition evaluated",
		zap.String("node_id", node.ID),
		zap.Bool("result", conditionResult),
	)

	// Resolve next nodes based on condition
	nextNodes, err := e.resolveNextNodes(ctx, graph, node, conditionResult)
	if err != nil {
		return nil, err
	}

	// Execute next nodes
	var lastResult interface{} = input
	for _, nextNode := range nextNodes {
		lastResult, err = e.executeNode(ctx, graph, nextNode, lastResult)
		if err != nil {
			return nil, err
		}
	}

	return lastResult, nil
}

// executeLoopNode executes a loop node
func (e *DAGExecutor) executeLoopNode(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}) (interface{}, error) {
	if node.LoopConfig == nil {
		return nil, fmt.Errorf("loop node %s has no loop configuration", node.ID)
	}

	config := node.LoopConfig
	e.logger.Debug("executing loop",
		zap.String("node_id", node.ID),
		zap.String("loop_type", string(config.Type)),
		zap.Int("max_iterations", config.MaxIterations),
	)

	// 检查循环深度
	e.mu.Lock()
	e.loopDepth[node.ID]++
	currentDepth := e.loopDepth[node.ID]
	e.mu.Unlock()

	if currentDepth > maxLoopDepth {
		return nil, fmt.Errorf("loop node %s exceeded max depth %d, possible infinite loop", node.ID, maxLoopDepth)
	}

	defer func() {
		e.mu.Lock()
		e.loopDepth[node.ID]--
		e.mu.Unlock()
	}()

	var result interface{} = input
	iteration := 0

	switch config.Type {
	case LoopTypeWhile:
		// While loop: execute while condition is true
		for {
			// Check max iterations
			if config.MaxIterations > 0 && iteration >= config.MaxIterations {
				e.logger.Debug("loop max iterations reached",
					zap.String("node_id", node.ID),
					zap.Int("iterations", iteration),
				)
				break
			}

			// Evaluate condition
			if config.Condition == nil {
				return nil, fmt.Errorf("while loop requires condition function")
			}

			shouldContinue, err := config.Condition(ctx, result)
			if err != nil {
				return nil, fmt.Errorf("loop condition failed: %w", err)
			}

			if !shouldContinue {
				e.logger.Debug("loop condition false, exiting",
					zap.String("node_id", node.ID),
					zap.Int("iterations", iteration),
				)
				break
			}

			// Execute loop body (next nodes)
			nextNodes := graph.GetEdges(node.ID)
			for _, nextNodeID := range nextNodes {
				nextNode, exists := graph.GetNode(nextNodeID)
				if !exists {
					return nil, fmt.Errorf("next node not found: %s", nextNodeID)
				}

				// Temporarily unmark as visited to allow re-execution
				e.mu.Lock()
				delete(e.visitedNodes, nextNodeID)
				e.mu.Unlock()

				var execErr error
				result, execErr = e.executeNode(ctx, graph, nextNode, result)
				if execErr != nil {
					return nil, execErr
				}
			}

			iteration++
		}

	case LoopTypeFor:
		// For loop: execute fixed number of iterations
		maxIter := config.MaxIterations
		if maxIter <= 0 {
			maxIter = 1
		}

		for i := 0; i < maxIter; i++ {
			// Execute loop body
			nextNodes := graph.GetEdges(node.ID)
			for _, nextNodeID := range nextNodes {
				nextNode, exists := graph.GetNode(nextNodeID)
				if !exists {
					return nil, fmt.Errorf("next node not found: %s", nextNodeID)
				}

				// Temporarily unmark as visited
				e.mu.Lock()
				delete(e.visitedNodes, nextNodeID)
				e.mu.Unlock()

				var execErr error
				result, execErr = e.executeNode(ctx, graph, nextNode, result)
				if execErr != nil {
					return nil, execErr
				}
			}
			iteration++
		}

	case LoopTypeForEach:
		// ForEach loop: iterate over collection
		if config.Iterator == nil {
			return nil, fmt.Errorf("foreach loop requires iterator function")
		}

		items, err := config.Iterator(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("iterator failed: %w", err)
		}

		results := make([]interface{}, 0, len(items))
		for i, item := range items {
			// Check max iterations
			if config.MaxIterations > 0 && i >= config.MaxIterations {
				break
			}

			// Execute loop body for each item
			nextNodes := graph.GetEdges(node.ID)
			var itemResult interface{} = item
			for _, nextNodeID := range nextNodes {
				nextNode, exists := graph.GetNode(nextNodeID)
				if !exists {
					return nil, fmt.Errorf("next node not found: %s", nextNodeID)
				}

				// Temporarily unmark as visited
				e.mu.Lock()
				delete(e.visitedNodes, nextNodeID)
				e.mu.Unlock()

				var execErr error
				itemResult, execErr = e.executeNode(ctx, graph, nextNode, itemResult)
				if execErr != nil {
					return nil, execErr
				}
			}
			results = append(results, itemResult)
			iteration++
		}
		result = results
	}

	e.logger.Debug("loop completed",
		zap.String("node_id", node.ID),
		zap.Int("iterations", iteration),
	)

	return result, nil
}

// executeParallelNode executes parallel nodes concurrently
func (e *DAGExecutor) executeParallelNode(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}) (interface{}, error) {
	nextNodeIDs := graph.GetEdges(node.ID)
	if len(nextNodeIDs) == 0 {
		return input, nil
	}

	e.logger.Debug("executing parallel nodes",
		zap.String("node_id", node.ID),
		zap.Int("parallel_count", len(nextNodeIDs)),
	)

	// Execute all next nodes in parallel
	type result struct {
		nodeID string
		output interface{}
		err    error
	}

	resultChan := make(chan result, len(nextNodeIDs))
	var wg sync.WaitGroup

	for _, nextNodeID := range nextNodeIDs {
		wg.Add(1)
		go func(nodeID string) {
			defer wg.Done()

			nextNode, exists := graph.GetNode(nodeID)
			if !exists {
				resultChan <- result{
					nodeID: nodeID,
					err:    fmt.Errorf("node not found: %s", nodeID),
				}
				return
			}

			output, err := e.executeNode(ctx, graph, nextNode, input)
			resultChan <- result{
				nodeID: nodeID,
				output: output,
				err:    err,
			}
		}(nextNodeID)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(resultChan)

	// Collect results
	results := make(map[string]interface{})
	var errors []error

	for res := range resultChan {
		if res.err != nil {
			errors = append(errors, fmt.Errorf("node %s: %w", res.nodeID, res.err))
		} else {
			results[res.nodeID] = res.output
		}
	}

	// Check for errors
	if len(errors) > 0 {
		return nil, fmt.Errorf("parallel execution failed: %v", errors)
	}

	e.logger.Debug("parallel execution completed",
		zap.String("node_id", node.ID),
		zap.Int("results_count", len(results)),
	)

	return results, nil
}

// executeSubGraphNode executes a nested subgraph
func (e *DAGExecutor) executeSubGraphNode(ctx context.Context, node *DAGNode, input interface{}) (interface{}, error) {
	if node.SubGraph == nil {
		return nil, fmt.Errorf("subgraph node %s has no subgraph", node.ID)
	}

	e.logger.Debug("executing subgraph", zap.String("node_id", node.ID))

	// Create new executor for subgraph
	subExecutor := NewDAGExecutor(e.checkpointMgr, e.logger)
	subExecutor.threadID = e.threadID

	result, err := subExecutor.Execute(ctx, node.SubGraph, input)
	if err != nil {
		return nil, fmt.Errorf("subgraph execution failed: %w", err)
	}

	return result, nil
}

// executeCheckpointNode creates a checkpoint
func (e *DAGExecutor) executeCheckpointNode(ctx context.Context, node *DAGNode, input interface{}) (interface{}, error) {
	if e.checkpointMgr == nil {
		e.logger.Warn("checkpoint manager not configured, skipping checkpoint",
			zap.String("node_id", node.ID),
		)
		return input, nil
	}

	e.logger.Debug("creating checkpoint", zap.String("node_id", node.ID))

	// Create execution context for checkpoint
	e.mu.RLock()
	execCtx := &ExecutionContext{
		WorkflowID:     e.executionID,
		CurrentNode:    node.ID,
		NodeResults:    make(map[string]interface{}),
		Variables:      make(map[string]interface{}),
		StartTime:      time.Now(),
		LastUpdateTime: time.Now(),
	}

	// Copy node results
	for k, v := range e.nodeResults {
		execCtx.NodeResults[k] = v
	}
	e.mu.RUnlock()

	// Save checkpoint (simplified - actual implementation would need full checkpoint struct)
	if err := e.checkpointMgr.SaveCheckpoint(ctx, execCtx); err != nil {
		e.logger.Error("failed to save checkpoint",
			zap.String("node_id", node.ID),
			zap.Error(err),
		)
		// Don't fail execution on checkpoint error
	}

	return input, nil
}

// resolveNextNodes determines which nodes to execute next based on condition result
func (e *DAGExecutor) resolveNextNodes(ctx context.Context, graph *DAGGraph, node *DAGNode, conditionResult interface{}) ([]*DAGNode, error) {
	// For condition nodes, use metadata to determine routing
	// Expected metadata format:
	// - "on_true": []string - node IDs to execute when condition is true
	// - "on_false": []string - node IDs to execute when condition is false

	var nextNodeIDs []string

	if boolResult, ok := conditionResult.(bool); ok {
		if boolResult {
			// Get on_true nodes from metadata
			if onTrue, exists := node.Metadata["on_true"]; exists {
				if nodeIDs, ok := onTrue.([]string); ok {
					nextNodeIDs = nodeIDs
				}
			}
		} else {
			// Get on_false nodes from metadata
			if onFalse, exists := node.Metadata["on_false"]; exists {
				if nodeIDs, ok := onFalse.([]string); ok {
					nextNodeIDs = nodeIDs
				}
			}
		}
	}

	// If no routing metadata, use default edges
	if len(nextNodeIDs) == 0 {
		nextNodeIDs = graph.GetEdges(node.ID)
	}

	// Resolve node IDs to nodes
	nextNodes := make([]*DAGNode, 0, len(nextNodeIDs))
	for _, nodeID := range nextNodeIDs {
		nextNode, exists := graph.GetNode(nodeID)
		if !exists {
			return nil, fmt.Errorf("next node not found: %s", nodeID)
		}
		nextNodes = append(nextNodes, nextNode)
	}

	return nextNodes, nil
}

// GetNodeResult retrieves the result of a completed node
func (e *DAGExecutor) GetNodeResult(nodeID string) (interface{}, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result, exists := e.nodeResults[nodeID]
	return result, exists
}

// GetExecutionID returns the current execution ID
func (e *DAGExecutor) GetExecutionID() string {
	return e.executionID
}

var executionIDCounter uint64

// generateExecutionID generates a unique execution ID
func generateExecutionID() string {
	counter := atomic.AddUint64(&executionIDCounter, 1)
	return fmt.Sprintf("exec_%d_%d", time.Now().UnixNano(), counter)
}
