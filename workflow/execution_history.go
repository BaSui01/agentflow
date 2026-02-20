package workflow

import (
	"sync"
	"time"
)

// ExecutionStatus represents the status of an execution
type ExecutionStatus string

const (
	// ExecutionStatusRunning indicates the execution is in progress
	ExecutionStatusRunning ExecutionStatus = "running"
	// ExecutionStatusCompleted indicates the execution completed successfully
	ExecutionStatusCompleted ExecutionStatus = "completed"
	// ExecutionStatusFailed indicates the execution failed
	ExecutionStatusFailed ExecutionStatus = "failed"
)

// NodeExecution records the execution of a single node
type NodeExecution struct {
	NodeID    string          `json:"node_id"`
	NodeType  NodeType        `json:"node_type"`
	StartTime time.Time       `json:"start_time"`
	EndTime   time.Time       `json:"end_time"`
	Duration  time.Duration   `json:"duration"`
	Status    ExecutionStatus `json:"status"`
	Input     any     `json:"input,omitempty"`
	Output    any     `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// ExecutionHistory records the complete execution path of a workflow
type ExecutionHistory struct {
	ExecutionID string           `json:"execution_id"`
	WorkflowID  string           `json:"workflow_id"`
	StartTime   time.Time        `json:"start_time"`
	EndTime     time.Time        `json:"end_time"`
	Duration    time.Duration    `json:"duration"`
	Status      ExecutionStatus  `json:"status"`
	Nodes       []*NodeExecution `json:"nodes"`
	Error       string           `json:"error,omitempty"`
	Metadata    map[string]any   `json:"metadata,omitempty"`
	mu          sync.RWMutex
}

// NewExecutionHistory creates a new execution history
func NewExecutionHistory(executionID, workflowID string) *ExecutionHistory {
	return &ExecutionHistory{
		ExecutionID: executionID,
		WorkflowID:  workflowID,
		StartTime:   time.Now(),
		Status:      ExecutionStatusRunning,
		Nodes:       make([]*NodeExecution, 0),
		Metadata:    make(map[string]any),
	}
}

// RecordNodeStart records the start of a node execution
func (h *ExecutionHistory) RecordNodeStart(nodeID string, nodeType NodeType, input any) *NodeExecution {
	h.mu.Lock()
	defer h.mu.Unlock()

	node := &NodeExecution{
		NodeID:    nodeID,
		NodeType:  nodeType,
		StartTime: time.Now(),
		Status:    ExecutionStatusRunning,
		Input:     input,
	}
	h.Nodes = append(h.Nodes, node)
	return node
}

// RecordNodeEnd records the end of a node execution
func (h *ExecutionHistory) RecordNodeEnd(node *NodeExecution, output any, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	node.EndTime = time.Now()
	node.Duration = node.EndTime.Sub(node.StartTime)
	node.Output = output

	if err != nil {
		node.Status = ExecutionStatusFailed
		node.Error = err.Error()
	} else {
		node.Status = ExecutionStatusCompleted
	}
}

// Complete marks the execution as completed
func (h *ExecutionHistory) Complete(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.EndTime = time.Now()
	h.Duration = h.EndTime.Sub(h.StartTime)

	if err != nil {
		h.Status = ExecutionStatusFailed
		h.Error = err.Error()
	} else {
		h.Status = ExecutionStatusCompleted
	}
}

// GetNodes returns a copy of the node executions
func (h *ExecutionHistory) GetNodes() []*NodeExecution {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nodes := make([]*NodeExecution, len(h.Nodes))
	copy(nodes, h.Nodes)
	return nodes
}

// GetNodeByID returns the execution record for a specific node
func (h *ExecutionHistory) GetNodeByID(nodeID string) *NodeExecution {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, node := range h.Nodes {
		if node.NodeID == nodeID {
			return node
		}
	}
	return nil
}

// ExecutionHistoryStore stores and queries execution histories
type ExecutionHistoryStore struct {
	histories map[string]*ExecutionHistory
	mu        sync.RWMutex
}

// NewExecutionHistoryStore creates a new execution history store
func NewExecutionHistoryStore() *ExecutionHistoryStore {
	return &ExecutionHistoryStore{
		histories: make(map[string]*ExecutionHistory),
	}
}

// Save saves an execution history
func (s *ExecutionHistoryStore) Save(history *ExecutionHistory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.histories[history.ExecutionID] = history
}

// Get retrieves an execution history by ID
func (s *ExecutionHistoryStore) Get(executionID string) (*ExecutionHistory, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.histories[executionID]
	return h, ok
}

// ListByWorkflow returns all executions for a workflow
func (s *ExecutionHistoryStore) ListByWorkflow(workflowID string) []*ExecutionHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ExecutionHistory
	for _, h := range s.histories {
		if h.WorkflowID == workflowID {
			result = append(result, h)
		}
	}
	return result
}

// ListByTimeRange returns executions within a time range
func (s *ExecutionHistoryStore) ListByTimeRange(start, end time.Time) []*ExecutionHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ExecutionHistory
	for _, h := range s.histories {
		if !h.StartTime.Before(start) && !h.StartTime.After(end) {
			result = append(result, h)
		}
	}
	return result
}

// ListByStatus returns executions with a specific status
func (s *ExecutionHistoryStore) ListByStatus(status ExecutionStatus) []*ExecutionHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ExecutionHistory
	for _, h := range s.histories {
		if h.Status == status {
			result = append(result, h)
		}
	}
	return result
}
