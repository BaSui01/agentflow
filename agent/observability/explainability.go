// 软件包可观察性提供了可解释性和推理跟踪能力.
package observability

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// 决定 类型代表所作决定的类型。
type DecisionType string

const (
	DecisionToolSelection  DecisionType = "tool_selection"
	DecisionModelRouting   DecisionType = "model_routing"
	DecisionStrategyChoice DecisionType = "strategy_choice"
	DecisionContentFilter  DecisionType = "content_filter"
	DecisionRetry          DecisionType = "retry"
	DecisionFallback       DecisionType = "fallback"
	DecisionBudgetThrottle DecisionType = "budget_throttle"
)

// 决定是代理人作出的单一决定。
type Decision struct {
	ID           string            `json:"id"`
	Type         DecisionType      `json:"type"`
	Description  string            `json:"description"`
	Input        interface{}       `json:"input,omitempty"`
	Output       interface{}       `json:"output,omitempty"`
	Reasoning    string            `json:"reasoning"`
	Confidence   float64           `json:"confidence,omitempty"`
	Alternatives []Alternative     `json:"alternatives,omitempty"`
	Factors      []Factor          `json:"factors,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
	Duration     time.Duration     `json:"duration,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// 备选案文是经过审议的备选决定。
type Alternative struct {
	Option    string  `json:"option"`
	Score     float64 `json:"score"`
	Reason    string  `json:"reason"`
	WasChosen bool    `json:"was_chosen"`
}

// 因素是一个影响决定的因素。
type Factor struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Weight      float64 `json:"weight"`
	Impact      string  `json:"impact"` // positive, negative, neutral
	Explanation string  `json:"explanation"`
}

// 理性步骤代表了推理过程的一步.
type ReasoningStep struct {
	StepNumber int           `json:"step_number"`
	Type       string        `json:"type"` // thought, action, observation, decision
	Content    string        `json:"content"`
	Decisions  []Decision    `json:"decisions,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration `json:"duration,omitempty"`
}

// 理由 Trace代表了一个完整的推理追踪.
type ReasoningTrace struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	AgentID     string          `json:"agent_id"`
	TaskID      string          `json:"task_id,omitempty"`
	Steps       []ReasoningStep `json:"steps"`
	Decisions   []Decision      `json:"decisions"`
	StartTime   time.Time       `json:"start_time"`
	EndTime     time.Time       `json:"end_time,omitempty"`
	Duration    time.Duration   `json:"duration,omitempty"`
	Success     bool            `json:"success"`
	FinalOutput string          `json:"final_output,omitempty"`
	Error       string          `json:"error,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

// 可解释性 Config 配置可解释性系统.
type ExplainabilityConfig struct {
	Enabled            bool          `json:"enabled"`
	DetailLevel        string        `json:"detail_level"` // minimal, standard, verbose
	MaxTraceAge        time.Duration `json:"max_trace_age"`
	MaxTracesPerAgent  int           `json:"max_traces_per_agent"`
	RecordAlternatives bool          `json:"record_alternatives"`
	RecordFactors      bool          `json:"record_factors"`
}

// 默认解释性 Config 返回明智的默认 。
func DefaultExplainabilityConfig() ExplainabilityConfig {
	return ExplainabilityConfig{
		Enabled:            true,
		DetailLevel:        "standard",
		MaxTraceAge:        24 * time.Hour,
		MaxTracesPerAgent:  100,
		RecordAlternatives: true,
		RecordFactors:      true,
	}
}

// 可解释性 追踪器追踪和存储推理痕迹.
type ExplainabilityTracker struct {
	config       ExplainabilityConfig
	traces       map[string]*ReasoningTrace
	agentTraces  map[string][]string // agentID -> traceIDs
	mu           sync.RWMutex
	traceCounter int64
}

// 新建解释性 Tracker创建了新的可解释性跟踪器.
func NewExplainabilityTracker(config ExplainabilityConfig) *ExplainabilityTracker {
	return &ExplainabilityTracker{
		config:      config,
		traces:      make(map[string]*ReasoningTrace),
		agentTraces: make(map[string][]string),
	}
}

// 启动 Trace 开始新的推理追踪 。
func (t *ExplainabilityTracker) StartTrace(sessionID, agentID string) *ReasoningTrace {
	if !t.config.Enabled {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.traceCounter++
	trace := &ReasoningTrace{
		ID:        fmt.Sprintf("trace_%d_%d", time.Now().UnixNano(), t.traceCounter),
		SessionID: sessionID,
		AgentID:   agentID,
		Steps:     make([]ReasoningStep, 0),
		Decisions: make([]Decision, 0),
		StartTime: time.Now(),
		Metadata:  make(map[string]any),
	}

	t.traces[trace.ID] = trace
	t.agentTraces[agentID] = append(t.agentTraces[agentID], trace.ID)

	// 清理旧的痕迹
	t.cleanupOldTraces(agentID)

	return trace
}

// 添加Step为跟踪添加了推理步骤.
func (t *ExplainabilityTracker) AddStep(traceID string, step ReasoningStep) {
	if !t.config.Enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	trace, ok := t.traces[traceID]
	if !ok {
		return
	}

	step.StepNumber = len(trace.Steps) + 1
	step.Timestamp = time.Now()
	trace.Steps = append(trace.Steps, step)
}

// 记录决定记录在一处。
func (t *ExplainabilityTracker) RecordDecision(traceID string, decision Decision) {
	if !t.config.Enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	trace, ok := t.traces[traceID]
	if !ok {
		return
	}

	decision.Timestamp = time.Now()
	if decision.ID == "" {
		decision.ID = fmt.Sprintf("decision_%d", len(trace.Decisions)+1)
	}

	// 基于配置的过滤
	if !t.config.RecordAlternatives {
		decision.Alternatives = nil
	}
	if !t.config.RecordFactors {
		decision.Factors = nil
	}

	trace.Decisions = append(trace.Decisions, decision)
}

// EndTrace结束推理追踪.
func (t *ExplainabilityTracker) EndTrace(traceID string, success bool, output, errorMsg string) {
	if !t.config.Enabled {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	trace, ok := t.traces[traceID]
	if !ok {
		return
	}

	trace.EndTime = time.Now()
	trace.Duration = trace.EndTime.Sub(trace.StartTime)
	trace.Success = success
	trace.FinalOutput = output
	trace.Error = errorMsg
}

// Get Trace通过身份追踪到线索
func (t *ExplainabilityTracker) GetTrace(traceID string) *ReasoningTrace {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.traces[traceID]
}

// Get AgentTraces为特工检索所有痕迹.
func (t *ExplainabilityTracker) GetAgentTraces(agentID string) []*ReasoningTrace {
	t.mu.RLock()
	defer t.mu.RUnlock()

	traceIDs := t.agentTraces[agentID]
	traces := make([]*ReasoningTrace, 0, len(traceIDs))
	for _, id := range traceIDs {
		if trace, ok := t.traces[id]; ok {
			traces = append(traces, trace)
		}
	}
	return traces
}

// 解释决定为决定产生人能读取的解释.
func (t *ExplainabilityTracker) ExplainDecision(decision Decision) string {
	explanation := fmt.Sprintf("Decision: %s\n", decision.Description)
	explanation += fmt.Sprintf("Type: %s\n", decision.Type)
	explanation += fmt.Sprintf("Reasoning: %s\n", decision.Reasoning)

	if decision.Confidence > 0 {
		explanation += fmt.Sprintf("Confidence: %.2f%%\n", decision.Confidence*100)
	}

	if len(decision.Factors) > 0 {
		explanation += "\nFactors considered:\n"
		for _, f := range decision.Factors {
			explanation += fmt.Sprintf("  - %s (weight: %.2f, impact: %s): %s\n",
				f.Name, f.Weight, f.Impact, f.Explanation)
		}
	}

	if len(decision.Alternatives) > 0 {
		explanation += "\nAlternatives considered:\n"
		for _, a := range decision.Alternatives {
			chosen := ""
			if a.WasChosen {
				chosen = " [CHOSEN]"
			}
			explanation += fmt.Sprintf("  - %s (score: %.2f)%s: %s\n",
				a.Option, a.Score, chosen, a.Reason)
		}
	}

	return explanation
}

// 生成审计报告以进行追踪。
func (t *ExplainabilityTracker) GenerateAuditReport(traceID string) (*AuditReport, error) {
	trace := t.GetTrace(traceID)
	if trace == nil {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	report := &AuditReport{
		TraceID:         trace.ID,
		SessionID:       trace.SessionID,
		AgentID:         trace.AgentID,
		StartTime:       trace.StartTime,
		EndTime:         trace.EndTime,
		Duration:        trace.Duration,
		Success:         trace.Success,
		TotalSteps:      len(trace.Steps),
		TotalDecisions:  len(trace.Decisions),
		DecisionSummary: make(map[DecisionType]int),
	}

	for _, d := range trace.Decisions {
		report.DecisionSummary[d.Type]++
	}

	// 生成时间表
	for _, step := range trace.Steps {
		report.Timeline = append(report.Timeline, TimelineEvent{
			Timestamp:   step.Timestamp,
			Type:        "step",
			Description: step.Content,
		})
	}
	for _, decision := range trace.Decisions {
		report.Timeline = append(report.Timeline, TimelineEvent{
			Timestamp:   decision.Timestamp,
			Type:        "decision",
			Description: decision.Description,
		})
	}

	return report, nil
}

func (t *ExplainabilityTracker) cleanupOldTraces(agentID string) {
	cutoff := time.Now().Add(-t.config.MaxTraceAge)
	traceIDs := t.agentTraces[agentID]

	var validIDs []string
	for _, id := range traceIDs {
		trace, ok := t.traces[id]
		if !ok {
			continue
		}
		if trace.StartTime.After(cutoff) {
			validIDs = append(validIDs, id)
		} else {
			delete(t.traces, id)
		}
	}

	// 限制每个剂的痕量
	if len(validIDs) > t.config.MaxTracesPerAgent {
		for _, id := range validIDs[:len(validIDs)-t.config.MaxTracesPerAgent] {
			delete(t.traces, id)
		}
		validIDs = validIDs[len(validIDs)-t.config.MaxTracesPerAgent:]
	}

	t.agentTraces[agentID] = validIDs
}

// 审计报告是一份跟踪审计报告。
type AuditReport struct {
	TraceID         string               `json:"trace_id"`
	SessionID       string               `json:"session_id"`
	AgentID         string               `json:"agent_id"`
	StartTime       time.Time            `json:"start_time"`
	EndTime         time.Time            `json:"end_time"`
	Duration        time.Duration        `json:"duration"`
	Success         bool                 `json:"success"`
	TotalSteps      int                  `json:"total_steps"`
	TotalDecisions  int                  `json:"total_decisions"`
	DecisionSummary map[DecisionType]int `json:"decision_summary"`
	Timeline        []TimelineEvent      `json:"timeline"`
}

// 时间线Event代表审计时间表中的一个事件.
type TimelineEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
}

// 将审计报告出口给JSON。
func (r *AuditReport) Export() ([]byte, error) {
	return json.Marshal(r)
}
