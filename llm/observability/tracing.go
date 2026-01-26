// Package observability provides LangSmith-style tracing for multi-turn conversations.
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// TraceType defines the type of trace.
type TraceType string

const (
	TraceTypeLLM       TraceType = "llm"
	TraceTypeTool      TraceType = "tool"
	TraceTypeChain     TraceType = "chain"
	TraceTypeAgent     TraceType = "agent"
	TraceTypeRetriever TraceType = "retriever"
)

// Trace represents a single trace entry.
type Trace struct {
	ID        string         `json:"id"`
	ParentID  string         `json:"parent_id,omitempty"`
	RunID     string         `json:"run_id"`
	Type      TraceType      `json:"type"`
	Name      string         `json:"name"`
	Input     any            `json:"input"`
	Output    any            `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time,omitempty"`
	Duration  time.Duration  `json:"duration,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Feedback  *TraceFeedback `json:"feedback,omitempty"`
}

// TraceFeedback represents user feedback on a trace.
type TraceFeedback struct {
	Score   float64 `json:"score"`
	Comment string  `json:"comment,omitempty"`
	UserID  string  `json:"user_id,omitempty"`
}

// Run represents a complete execution run.
type Run struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Traces    []*Trace       `json:"traces"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time,omitempty"`
	Status    string         `json:"status"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Tokens    TokenUsage     `json:"tokens"`
	Cost      float64        `json:"cost"`
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// Tracer provides tracing capabilities.
type Tracer struct {
	runs      map[string]*Run
	traces    map[string]*Trace
	otelTrace oteltrace.Tracer
	logger    *zap.Logger
	exporter  TraceExporter
	mu        sync.RWMutex
}

// TraceExporter exports traces to external systems.
type TraceExporter interface {
	Export(ctx context.Context, run *Run) error
	ExportTrace(ctx context.Context, trace *Trace) error
}

// TracerConfig configures the tracer.
type TracerConfig struct {
	ServiceName string
	Exporter    TraceExporter
	BufferSize  int
}

// NewTracer creates a new tracer.
func NewTracer(config TracerConfig, otelTracer oteltrace.Tracer, logger *zap.Logger) *Tracer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Tracer{
		runs:      make(map[string]*Run),
		traces:    make(map[string]*Trace),
		otelTrace: otelTracer,
		logger:    logger.With(zap.String("component", "tracer")),
		exporter:  config.Exporter,
	}
}

// StartRun starts a new tracing run.
func (t *Tracer) StartRun(ctx context.Context, name string) (context.Context, *Run) {
	run := &Run{
		ID:        fmt.Sprintf("run_%d", time.Now().UnixNano()),
		Name:      name,
		Traces:    make([]*Trace, 0),
		StartTime: time.Now(),
		Status:    "running",
		Metadata:  make(map[string]any),
	}

	t.mu.Lock()
	t.runs[run.ID] = run
	t.mu.Unlock()

	ctx = context.WithValue(ctx, runIDKey, run.ID)
	t.logger.Debug("run started", zap.String("run_id", run.ID), zap.String("name", name))
	return ctx, run
}

// EndRun ends a tracing run.
func (t *Tracer) EndRun(ctx context.Context, runID string, status string) error {
	t.mu.Lock()
	run, ok := t.runs[runID]
	if !ok {
		t.mu.Unlock()
		return fmt.Errorf("run not found: %s", runID)
	}
	run.EndTime = time.Now()
	run.Status = status
	t.mu.Unlock()

	if t.exporter != nil {
		if err := t.exporter.Export(ctx, run); err != nil {
			t.logger.Error("failed to export run", zap.Error(err))
		}
	}

	t.logger.Debug("run ended", zap.String("run_id", runID), zap.String("status", status))
	return nil
}

type contextKey string

const runIDKey contextKey = "run_id"

// StartTrace starts a new trace within a run.
func (t *Tracer) StartTrace(ctx context.Context, traceType TraceType, name string, input any) (context.Context, *Trace) {
	runID, _ := ctx.Value(runIDKey).(string)

	tr := &Trace{
		ID:        fmt.Sprintf("trace_%d", time.Now().UnixNano()),
		RunID:     runID,
		Type:      traceType,
		Name:      name,
		Input:     input,
		StartTime: time.Now(),
		Metadata:  make(map[string]any),
	}

	// Create OpenTelemetry span
	var span oteltrace.Span
	if t.otelTrace != nil {
		ctx, span = t.otelTrace.Start(ctx, name)
		span.SetAttributes(
			attribute.String("trace.type", string(traceType)),
			attribute.String("trace.id", tr.ID),
		)
	}

	t.mu.Lock()
	t.traces[tr.ID] = tr
	if run, ok := t.runs[runID]; ok {
		run.Traces = append(run.Traces, tr)
	}
	t.mu.Unlock()

	ctx = context.WithValue(ctx, traceIDKey, tr.ID)
	if span != nil {
		ctx = context.WithValue(ctx, spanKey, span)
	}

	return ctx, tr
}

const traceIDKey contextKey = "trace_id"
const spanKey contextKey = "span"

// EndTrace ends a trace.
func (t *Tracer) EndTrace(ctx context.Context, traceID string, output any, err error) {
	t.mu.Lock()
	tr, ok := t.traces[traceID]
	if !ok {
		t.mu.Unlock()
		return
	}
	tr.EndTime = time.Now()
	tr.Duration = tr.EndTime.Sub(tr.StartTime)
	tr.Output = output
	if err != nil {
		tr.Error = err.Error()
	}
	t.mu.Unlock()

	// End OpenTelemetry span
	if span, ok := ctx.Value(spanKey).(oteltrace.Span); ok {
		if err != nil {
			span.SetAttributes(attribute.String("error", err.Error()))
		}
		span.End()
	}

	if t.exporter != nil {
		t.exporter.ExportTrace(ctx, tr)
	}
}

// AddFeedback adds feedback to a trace.
func (t *Tracer) AddFeedback(traceID string, feedback TraceFeedback) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	tr, ok := t.traces[traceID]
	if !ok {
		return fmt.Errorf("trace not found: %s", traceID)
	}
	tr.Feedback = &feedback
	return nil
}

// GetRun retrieves a run by ID.
func (t *Tracer) GetRun(runID string) (*Run, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	run, ok := t.runs[runID]
	return run, ok
}

// GetTrace retrieves a trace by ID.
func (t *Tracer) GetTrace(traceID string) (*Trace, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	tr, ok := t.traces[traceID]
	return tr, ok
}

// TraceLLMCall traces an LLM call.
func (t *Tracer) TraceLLMCall(ctx context.Context, model string, input any, fn func() (any, error)) (any, error) {
	ctx, tr := t.StartTrace(ctx, TraceTypeLLM, model, input)
	output, err := fn()
	t.EndTrace(ctx, tr.ID, output, err)
	return output, err
}

// TraceToolCall traces a tool call.
func (t *Tracer) TraceToolCall(ctx context.Context, toolName string, input any, fn func() (any, error)) (any, error) {
	ctx, tr := t.StartTrace(ctx, TraceTypeTool, toolName, input)
	output, err := fn()
	t.EndTrace(ctx, tr.ID, output, err)
	return output, err
}

// ConversationTracer traces multi-turn conversations.
type ConversationTracer struct {
	tracer        *Tracer
	conversations map[string]*ConversationTrace
	mu            sync.RWMutex
}

// ConversationTrace represents a traced conversation.
type ConversationTrace struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	Turns     []*TurnTrace   `json:"turns"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TurnTrace represents a single conversation turn.
type TurnTrace struct {
	ID         string        `json:"id"`
	TurnNumber int           `json:"turn_number"`
	UserInput  string        `json:"user_input"`
	Response   string        `json:"response"`
	Model      string        `json:"model"`
	Tokens     TokenUsage    `json:"tokens"`
	Latency    time.Duration `json:"latency"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

// ToolCall represents a tool call within a turn.
type ToolCall struct {
	Name     string        `json:"name"`
	Input    any           `json:"input"`
	Output   any           `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// NewConversationTracer creates a new conversation tracer.
func NewConversationTracer(tracer *Tracer) *ConversationTracer {
	return &ConversationTracer{
		tracer:        tracer,
		conversations: make(map[string]*ConversationTrace),
	}
}

// StartConversation starts tracing a conversation.
func (c *ConversationTracer) StartConversation(ctx context.Context, name string) (context.Context, *ConversationTrace) {
	ctx, run := c.tracer.StartRun(ctx, name)

	conv := &ConversationTrace{
		ID:        fmt.Sprintf("conv_%d", time.Now().UnixNano()),
		RunID:     run.ID,
		Turns:     make([]*TurnTrace, 0),
		StartTime: time.Now(),
		Metadata:  make(map[string]any),
	}

	c.mu.Lock()
	c.conversations[conv.ID] = conv
	c.mu.Unlock()

	ctx = context.WithValue(ctx, convIDKey, conv.ID)
	return ctx, conv
}

const convIDKey contextKey = "conv_id"

// TraceTurn traces a conversation turn.
func (c *ConversationTracer) TraceTurn(ctx context.Context, userInput string, fn func() (string, TokenUsage, error)) (*TurnTrace, error) {
	convID, _ := ctx.Value(convIDKey).(string)

	c.mu.RLock()
	conv, ok := c.conversations[convID]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("conversation not found: %s", convID)
	}

	turn := &TurnTrace{
		ID:         fmt.Sprintf("turn_%d", time.Now().UnixNano()),
		TurnNumber: len(conv.Turns) + 1,
		UserInput:  userInput,
		Timestamp:  time.Now(),
	}

	start := time.Now()
	response, tokens, err := fn()
	turn.Latency = time.Since(start)
	turn.Response = response
	turn.Tokens = tokens

	if err != nil {
		return turn, err
	}

	c.mu.Lock()
	conv.Turns = append(conv.Turns, turn)
	c.mu.Unlock()

	return turn, nil
}

// EndConversation ends conversation tracing.
func (c *ConversationTracer) EndConversation(ctx context.Context, convID string) error {
	c.mu.Lock()
	conv, ok := c.conversations[convID]
	if ok {
		conv.EndTime = time.Now()
	}
	c.mu.Unlock()

	if !ok {
		return fmt.Errorf("conversation not found: %s", convID)
	}

	return c.tracer.EndRun(ctx, conv.RunID, "completed")
}

// GetConversation retrieves a conversation trace.
func (c *ConversationTracer) GetConversation(convID string) (*ConversationTrace, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conv, ok := c.conversations[convID]
	return conv, ok
}

// ExportJSON exports a conversation trace as JSON.
func (c *ConversationTracer) ExportJSON(convID string) ([]byte, error) {
	conv, ok := c.GetConversation(convID)
	if !ok {
		return nil, fmt.Errorf("conversation not found: %s", convID)
	}
	return json.MarshalIndent(conv, "", "  ")
}
