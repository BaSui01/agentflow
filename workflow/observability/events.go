package observability

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// NodeEventType 节点事件类型。
type NodeEventType = types.WorkflowNodeEventType

const (
	NodeStart    NodeEventType = types.WorkflowNodeEventStart
	NodeComplete NodeEventType = types.WorkflowNodeEventComplete
	NodeError    NodeEventType = types.WorkflowNodeEventError
)

// NodeEvent 节点级观测事件，统一跨层字段。
type NodeEvent struct {
	Type       NodeEventType `json:"type"`
	TraceID    string        `json:"trace_id,omitempty"`
	RunID      string        `json:"run_id,omitempty"`
	WorkflowID string       `json:"workflow_id,omitempty"`
	NodeID     string        `json:"node_id"`
	NodeType   string        `json:"node_type,omitempty"`
	LatencyMs  int64         `json:"latency_ms,omitempty"`
	Error      string        `json:"error,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

// NodeEventEmitter 节点事件发射回调。
type NodeEventEmitter func(NodeEvent)

type nodeEventEmitterKey struct{}

// WithNodeEventEmitter 将 NodeEventEmitter 注入 context。
func WithNodeEventEmitter(ctx context.Context, emitter NodeEventEmitter) context.Context {
	if emitter == nil {
		return ctx
	}
	return context.WithValue(ctx, nodeEventEmitterKey{}, emitter)
}

// NodeEventEmitterFromContext 从 context 提取 NodeEventEmitter。
func NodeEventEmitterFromContext(ctx context.Context) (NodeEventEmitter, bool) {
	v, ok := ctx.Value(nodeEventEmitterKey{}).(NodeEventEmitter)
	return v, ok && v != nil
}

// EmitNodeStart 发射 node_start 事件。
func EmitNodeStart(ctx context.Context, workflowID, nodeID, nodeType string) {
	emitter, ok := NodeEventEmitterFromContext(ctx)
	if !ok {
		return
	}
	emitter(newNodeEvent(ctx, NodeStart, workflowID, nodeID, nodeType, 0, ""))
}

// EmitNodeComplete 发射 node_complete 事件。
func EmitNodeComplete(ctx context.Context, workflowID, nodeID, nodeType string, latencyMs int64) {
	emitter, ok := NodeEventEmitterFromContext(ctx)
	if !ok {
		return
	}
	emitter(newNodeEvent(ctx, NodeComplete, workflowID, nodeID, nodeType, latencyMs, ""))
}

// EmitNodeError 发射 node_error 事件。
func EmitNodeError(ctx context.Context, workflowID, nodeID, nodeType string, latencyMs int64, err error) {
	emitter, ok := NodeEventEmitterFromContext(ctx)
	if !ok {
		return
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	emitter(newNodeEvent(ctx, NodeError, workflowID, nodeID, nodeType, latencyMs, errMsg))
}

func newNodeEvent(ctx context.Context, typ NodeEventType, workflowID, nodeID, nodeType string, latencyMs int64, errMsg string) NodeEvent {
	ev := NodeEvent{
		Type:       typ,
		WorkflowID: workflowID,
		NodeID:     nodeID,
		NodeType:   nodeType,
		LatencyMs:  latencyMs,
		Error:      errMsg,
		Timestamp:  time.Now(),
	}
	if traceID, ok := types.TraceID(ctx); ok {
		ev.TraceID = traceID
	}
	if runID, ok := types.RunID(ctx); ok {
		ev.RunID = runID
	}
	return ev
}
