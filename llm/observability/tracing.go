// 包可观察性为多回合对话提供了LangSmith风格的追踪.
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

// TraceType定义了追踪的类型.
type TraceType string

const (
	TraceTypeLLM       TraceType = "llm"
	TraceTypeTool      TraceType = "tool"
	TraceTypeChain     TraceType = "chain"
	TraceTypeAgent     TraceType = "agent"
	TraceTypeRetriever TraceType = "retriever"
)

// 追踪代表一个追踪条目。
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

// TraceFeedback代表了用户对一个跟踪的反馈.
type TraceFeedback struct {
	Score   float64 `json:"score"`
	Comment string  `json:"comment,omitempty"`
	UserID  string  `json:"user_id,omitempty"`
}

// 运行代表一个完整的执行运行 。
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

// TokenUsage追踪到象征性消费.
type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// 追踪器提供追踪能力。
type Tracer struct {
	runs      map[string]*Run
	traces    map[string]*Trace
	otelTrace oteltrace.Tracer
	logger    *zap.Logger
	exporter  TraceExporter
	mu        sync.RWMutex
}

// TraceExporter向外部系统输出痕迹.
type TraceExporter interface {
	Export(ctx context.Context, run *Run) error
	ExportTrace(ctx context.Context, trace *Trace) error
}

// TracerConfig 配置了跟踪器.
type TracerConfig struct {
	ServiceName string
	Exporter    TraceExporter
	BufferSize  int
}

// 新追踪器创建了新的追踪器.
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

// StartRun 开始新的追踪运行 。
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

// EndRun结束追踪运行.
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

// 启动 Trace 在运行中开始新的追踪 。
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

	// 创建 OpenTeleometry 跨度
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

// EndTrace结束一个追踪。
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

	// 结束 OpenTeleometry 跨度
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

// 添加Feedback 添加到跟踪中的反馈 。
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

// GetRun 获取了 ID 运行 。
func (t *Tracer) GetRun(runID string) (*Run, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	run, ok := t.runs[runID]
	return run, ok
}

// Get Trace通过身份追踪到线索
func (t *Tracer) GetTrace(traceID string) (*Trace, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	tr, ok := t.traces[traceID]
	return tr, ok
}

// TraceLLMCall追踪一个LLM电话。
func (t *Tracer) TraceLLMCall(ctx context.Context, model string, input any, fn func() (any, error)) (any, error) {
	ctx, tr := t.StartTrace(ctx, TraceTypeLLM, model, input)
	output, err := fn()
	t.EndTrace(ctx, tr.ID, output, err)
	return output, err
}

// TraceTooCall追踪一个工具呼叫.
func (t *Tracer) TraceToolCall(ctx context.Context, toolName string, input any, fn func() (any, error)) (any, error) {
	ctx, tr := t.StartTrace(ctx, TraceTypeTool, toolName, input)
	output, err := fn()
	t.EndTrace(ctx, tr.ID, output, err)
	return output, err
}

// 对话 追踪器追踪多回合对话
type ConversationTracer struct {
	tracer        *Tracer
	conversations map[string]*ConversationTrace
	mu            sync.RWMutex
}

// 对话 Trace代表了追踪的对话.
type ConversationTrace struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id"`
	Turns     []*TurnTrace   `json:"turns"`
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// TurnTrace代表单一对话转会.
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

// ToolCall代表一个回合内的工具呼叫.
type ToolCall struct {
	Name     string        `json:"name"`
	Input    any           `json:"input"`
	Output   any           `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// 新建组合 Tracer创建了新的对话追踪器.
func NewConversationTracer(tracer *Tracer) *ConversationTracer {
	return &ConversationTracer{
		tracer:        tracer,
		conversations: make(map[string]*ConversationTrace),
	}
}

// 开始追踪谈话
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

// TraceTurn追踪到对话的转折.
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

// 尾声结束对话追踪。
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

// Get Conversation retrieved a conversation recording a track. 找寻一个对话的痕迹。
func (c *ConversationTracer) GetConversation(convID string) (*ConversationTrace, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conv, ok := c.conversations[convID]
	return conv, ok
}

// ExportJSON 输出对话追踪为JSON.
func (c *ConversationTracer) ExportJSON(convID string) ([]byte, error) {
	conv, ok := c.GetConversation(convID)
	if !ok {
		return nil, fmt.Errorf("conversation not found: %s", convID)
	}
	return json.MarshalIndent(conv, "", "  ")
}
