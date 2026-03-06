package observability

import (
	"context"
	"sync"
	"time"
)

type CostRecord struct {
	Provider     string
	Model        string
	AgentID      string
	SessionID    string
	ToolName     string
	InputTokens  int
	OutputTokens int
	Cost         float64
	Timestamp    time.Time
}

type CostTracker struct {
	calculator *CostCalculator
	records    []CostRecord
	mu         sync.RWMutex
}

func NewCostTracker(calculator *CostCalculator) *CostTracker {
	return &CostTracker{
		calculator: calculator,
		records:    make([]CostRecord, 0),
	}
}

func (t *CostTracker) Record(provider, model, agentID string, inputTokens, outputTokens int, opts ...RecordOption) {
	cost := t.calculator.Calculate(provider, model, inputTokens, outputTokens)
	rec := CostRecord{
		Provider:     provider,
		Model:        model,
		AgentID:      agentID,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Cost:         cost,
		Timestamp:    time.Now(),
	}
	for _, opt := range opts {
		opt(&rec)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = append(t.records, rec)
}

// RecordOption configures optional fields on a CostRecord.
type RecordOption func(*CostRecord)

// WithSessionID attaches a session identifier to the cost record.
func WithSessionID(id string) RecordOption {
	return func(r *CostRecord) { r.SessionID = id }
}

// WithToolName attaches a tool name to the cost record.
func WithToolName(name string) RecordOption {
	return func(r *CostRecord) { r.ToolName = name }
}

func (t *CostTracker) TotalCost() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var total float64
	for _, r := range t.records {
		total += r.Cost
	}
	return total
}

func (t *CostTracker) CostByProvider() map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := make(map[string]float64)
	for _, r := range t.records {
		m[r.Provider] += r.Cost
	}
	return m
}

func (t *CostTracker) CostByModel() map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := make(map[string]float64)
	for _, r := range t.records {
		key := r.Provider + ":" + r.Model
		m[key] += r.Cost
	}
	return m
}

func (t *CostTracker) CostByAgent() map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := make(map[string]float64)
	for _, r := range t.records {
		m[r.AgentID] += r.Cost
	}
	return m
}

func (t *CostTracker) CostBySession() map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := make(map[string]float64)
	for _, r := range t.records {
		if r.SessionID != "" {
			m[r.SessionID] += r.Cost
		}
	}
	return m
}

func (t *CostTracker) CostByTool() map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := make(map[string]float64)
	for _, r := range t.records {
		if r.ToolName != "" {
			m[r.ToolName] += r.Cost
		}
	}
	return m
}

func (t *CostTracker) Records() []CostRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]CostRecord, len(t.records))
	copy(out, t.records)
	return out
}

func (t *CostTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = nil
}

type CostTrackerLedger struct {
	tracker *CostTracker
}

func NewCostTrackerLedger(tracker *CostTracker) *CostTrackerLedger {
	return &CostTrackerLedger{tracker: tracker}
}

func (l *CostTrackerLedger) Record(ctx context.Context, entry LedgerEntry) error {
	agentID := ""
	if entry.Metadata != nil {
		agentID = entry.Metadata["agent_id"]
	}
	l.tracker.Record(entry.Provider, entry.Model, agentID, entry.Usage.PromptTokens, entry.Usage.CompletionTokens)
	return nil
}
