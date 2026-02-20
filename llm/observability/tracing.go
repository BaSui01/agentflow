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

// TraceType 定义追踪的类型.
type TraceType string

const (
	TraceTypeLLM       TraceType = "llm"
	TraceTypeTool      TraceType = "tool"
	TraceTypeChain     TraceType = "chain"
	TraceTypeAgent     TraceType = "agent"
	TraceTypeRetriever TraceType = "retriever"
)

// Trace 表示一个追踪条目.
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

// TraceFeedback 表示用户对追踪的反馈.
type TraceFeedback struct {
	Score   float64 `json:"score"`
	Comment string  `json:"comment,omitempty"`
	UserID  string  `json:"user_id,omitempty"`
}

// Run 表示一个完整的执行运行.
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

// TokenUsage 追踪 Token 消耗.
type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// Tracer 提供追踪能力.
type Tracer struct {
	runs      map[string]*Run
	traces    map[string]*Trace
	otelTrace oteltrace.Tracer
	logger    *zap.Logger
	exporter  TraceExporter
	mu        sync.RWMutex
}

// TraceExporter 向外部系统导出追踪.
type TraceExporter interface {
	Export(ctx context.Context, run *Run) error
	ExportTrace(ctx context.Context, trace *Trace) error
}

// TracerConfig 配置追踪器.
type TracerConfig struct {
	ServiceName string
	Exporter    TraceExporter
	BufferSize  int
}

// NewTracer 创建新的追踪器.
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

// StartRun 开始新的追踪运行.
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

// EndRun 结束追踪运行.
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

// StartTrace 在运行中开始新的追踪.
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

	// 创建 OpenTelemetry span
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

// EndTrace 结束一个追踪.
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

	// 结束 OpenTelemetry span
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

// AddFeedback 为追踪添加反馈.
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

// GetRun 根据 ID 获取运行.
func (t *Tracer) GetRun(runID string) (*Run, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	run, ok := t.runs[runID]
	return run, ok
}

// GetTrace 根据 ID 获取追踪.
func (t *Tracer) GetTrace(traceID string) (*Trace, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	tr, ok := t.traces[traceID]
	return tr, ok
}

// TraceLLMCall 追踪一次 LLM 调用.
func (t *Tracer) TraceLLMCall(ctx context.Context, model string, input any, fn func() (any, error)) (any, error) {
	ctx, tr := t.StartTrace(ctx, TraceTypeLLM, model, input)
	output, err := fn()
	t.EndTrace(ctx, tr.ID, output, err)
	return output, err
}

// TraceToolCall 追踪一次工具调用.
func (t *Tracer) TraceToolCall(ctx context.Context, toolName string, input any, fn func() (any, error)) (any, error) {
	ctx, tr := t.StartTrace(ctx, TraceTypeTool, toolName, input)
	output, err := fn()
	t.EndTrace(ctx, tr.ID, output, err)
	return output, err
}

// ConversationTracer 追踪多回合对话.
type ConversationTracer struct {
	tracer        *Tracer
	conversations map[string]*ConversationTrace
	mu            sync.RWMutex
}

// ConversationTrace 表示追踪的对话.
type ConversationTrace struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	Turns     []*TurnTrace   `json:"turns"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TurnTrace 表示单次对话回合.
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

// ToolCall 表示回合内的工具调用.
type ToolCall struct {
	Name     string        `json:"name"`
	Input    any           `json:"input"`
	Output   any           `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// NewConversationTracer 创建新的对话追踪器.
func NewConversationTracer(tracer *Tracer) *ConversationTracer {
	return &ConversationTracer{
		tracer:        tracer,
		conversations: make(map[string]*ConversationTrace),
	}
}

// StartConversation 开始追踪对话.
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

// TraceTurn 追踪对话的一个回合.
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

// EndConversation 结束对话追踪.
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

// GetConversation 获取对话追踪.
func (c *ConversationTracer) GetConversation(convID string) (*ConversationTrace, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conv, ok := c.conversations[convID]
	return conv, ok
}

// ExportJSON 将对话追踪导出为 JSON.
func (c *ConversationTracer) ExportJSON(convID string) ([]byte, error) {
	conv, ok := c.GetConversation(convID)
	if !ok {
		return nil, fmt.Errorf("conversation not found: %s", convID)
	}
	return json.MarshalIndent(conv, "", "  ")
}
