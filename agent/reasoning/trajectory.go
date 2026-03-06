package reasoning

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// TrajectoryRecord captures a complete execution trajectory for evaluation and replay.
type TrajectoryRecord struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agent_id"`
	Pattern     string          `json:"pattern"`
	Task        string          `json:"task"`
	Steps       []TrajectoryStep `json:"steps"`
	FinalAnswer string          `json:"final_answer"`
	Quality     float64         `json:"quality,omitempty"`
	TotalTokens int             `json:"total_tokens"`
	Duration    time.Duration   `json:"duration"`
	CreatedAt   time.Time       `json:"created_at"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

// TrajectoryStep records a single Thought → Action → Observation cycle.
type TrajectoryStep struct {
	Index       int            `json:"index"`
	Type        string         `json:"type"` // thought, action, observation, reflection, tool_call
	Content     string         `json:"content"`
	ToolName    string         `json:"tool_name,omitempty"`
	ToolArgs    map[string]any `json:"tool_args,omitempty"`
	ToolResult  string         `json:"tool_result,omitempty"`
	TokensUsed  int            `json:"tokens_used,omitempty"`
	Duration    time.Duration  `json:"duration"`
	Timestamp   time.Time      `json:"timestamp"`
}

// TrajectoryStore persists and retrieves trajectory records.
type TrajectoryStore interface {
	Save(ctx context.Context, record *TrajectoryRecord) error
	Load(ctx context.Context, id string) (*TrajectoryRecord, error)
	ListByAgent(ctx context.Context, agentID string, limit int) ([]*TrajectoryRecord, error)
}

// TrajectoryEvaluator scores the quality of an execution trajectory.
type TrajectoryEvaluator interface {
	Evaluate(ctx context.Context, record *TrajectoryRecord) (float64, error)
}

// TrajectoryCollector builds trajectory records incrementally during execution.
// Safe for concurrent use.
type TrajectoryCollector struct {
	mu     sync.Mutex
	record *TrajectoryRecord
	index  int
}

// NewTrajectoryCollector creates a new collector for the given agent and task.
func NewTrajectoryCollector(id, agentID, pattern, task string) *TrajectoryCollector {
	return &TrajectoryCollector{
		record: &TrajectoryRecord{
			ID:        id,
			AgentID:   agentID,
			Pattern:   pattern,
			Task:      task,
			Steps:     make([]TrajectoryStep, 0, 16),
			CreatedAt: time.Now(),
		},
	}
}

// AddStep records a step in the trajectory.
func (c *TrajectoryCollector) AddStep(stepType, content string, opts ...TrajectoryStepOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	step := TrajectoryStep{
		Index:     c.index,
		Type:      stepType,
		Content:   content,
		Timestamp: time.Now(),
	}
	for _, opt := range opts {
		opt(&step)
	}
	c.record.Steps = append(c.record.Steps, step)
	c.index++
}

// Finalize completes the trajectory record with the final answer and total stats.
func (c *TrajectoryCollector) Finalize(answer string, totalTokens int, duration time.Duration) *TrajectoryRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.record.FinalAnswer = answer
	c.record.TotalTokens = totalTokens
	c.record.Duration = duration
	return c.record
}

// Record returns the current trajectory record (may be incomplete).
func (c *TrajectoryCollector) Record() *TrajectoryRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.record
}

// TrajectoryStepOption configures optional fields on a TrajectoryStep.
type TrajectoryStepOption func(*TrajectoryStep)

func WithToolCall(name string, args map[string]any, result string) TrajectoryStepOption {
	return func(s *TrajectoryStep) {
		s.ToolName = name
		s.ToolArgs = args
		s.ToolResult = result
	}
}

func WithStepTokens(tokens int) TrajectoryStepOption {
	return func(s *TrajectoryStep) { s.TokensUsed = tokens }
}

func WithStepDuration(d time.Duration) TrajectoryStepOption {
	return func(s *TrajectoryStep) { s.Duration = d }
}

// FromReasoningResult converts a ReasoningResult into a TrajectoryRecord.
// Returns nil if result is nil.
func FromReasoningResult(id, agentID string, result *ReasoningResult) *TrajectoryRecord {
	if result == nil {
		return nil
	}
	steps := make([]TrajectoryStep, 0, len(result.Steps))
	for i, s := range result.Steps {
		steps = append(steps, TrajectoryStep{
			Index:      i,
			Type:       s.Type,
			Content:    s.Content,
			TokensUsed: s.TokensUsed,
			Duration:   s.Duration,
			Timestamp:  time.Now(),
		})
	}
	return &TrajectoryRecord{
		ID:          id,
		AgentID:     agentID,
		Pattern:     result.Pattern,
		Task:        result.Task,
		Steps:       steps,
		FinalAnswer: result.FinalAnswer,
		TotalTokens: result.TotalTokens,
		Duration:    result.TotalLatency,
		CreatedAt:   time.Now(),
		Metadata:    result.Metadata,
	}
}

// ToJSON serializes a trajectory record to JSON.
func (r *TrajectoryRecord) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// TrajectoryRecordFromJSON deserializes a trajectory record from JSON.
func TrajectoryRecordFromJSON(data []byte) (*TrajectoryRecord, error) {
	var r TrajectoryRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
