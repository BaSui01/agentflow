// Package deliberation provides CrewAI-style autonomous reasoning mode.
package deliberation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DeliberationMode controls how agents reason before acting.
type DeliberationMode string

const (
	ModeImmediate  DeliberationMode = "immediate"  // Direct tool execution
	ModeDeliberate DeliberationMode = "deliberate" // Full reasoning cycle
	ModeAdaptive   DeliberationMode = "adaptive"   // Context-dependent
)

// DeliberationConfig configures the deliberation engine.
type DeliberationConfig struct {
	Mode               DeliberationMode `json:"mode"`
	MaxThinkingTime    time.Duration    `json:"max_thinking_time"`
	MinConfidence      float64          `json:"min_confidence"`
	EnableSelfCritique bool             `json:"enable_self_critique"`
	MaxIterations      int              `json:"max_iterations"`
}

// DefaultDeliberationConfig returns default configuration.
func DefaultDeliberationConfig() DeliberationConfig {
	return DeliberationConfig{
		Mode:               ModeDeliberate,
		MaxThinkingTime:    10 * time.Second,
		MinConfidence:      0.7,
		EnableSelfCritique: true,
		MaxIterations:      3,
	}
}

// ThoughtProcess represents a single reasoning step.
type ThoughtProcess struct {
	ID         string    `json:"id"`
	Step       int       `json:"step"`
	Type       string    `json:"type"` // understand, evaluate, plan, critique
	Content    string    `json:"content"`
	Confidence float64   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
}

// DeliberationResult contains the outcome of deliberation.
type DeliberationResult struct {
	TaskID          string           `json:"task_id"`
	Thoughts        []ThoughtProcess `json:"thoughts"`
	Decision        *Decision        `json:"decision"`
	TotalTime       time.Duration    `json:"total_time"`
	Iterations      int              `json:"iterations"`
	FinalConfidence float64          `json:"final_confidence"`
}

// Decision represents the final decision after deliberation.
type Decision struct {
	Action     string         `json:"action"`
	Tool       string         `json:"tool,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Reasoning  string         `json:"reasoning"`
	Confidence float64        `json:"confidence"`
}

// Reasoner interface for LLM-based reasoning.
type Reasoner interface {
	Think(ctx context.Context, prompt string) (string, float64, error)
}

// Engine provides deliberation capabilities.
type Engine struct {
	config   DeliberationConfig
	reasoner Reasoner
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewEngine creates a new deliberation engine.
func NewEngine(config DeliberationConfig, reasoner Reasoner, logger *zap.Logger) *Engine {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Engine{
		config:   config,
		reasoner: reasoner,
		logger:   logger.With(zap.String("component", "deliberation")),
	}
}

// Deliberate performs reasoning before action.
func (e *Engine) Deliberate(ctx context.Context, task Task) (*DeliberationResult, error) {
	if e.config.Mode == ModeImmediate {
		return e.immediateDecision(task)
	}

	start := time.Now()
	result := &DeliberationResult{
		TaskID:   task.ID,
		Thoughts: make([]ThoughtProcess, 0),
	}

	ctx, cancel := context.WithTimeout(ctx, e.config.MaxThinkingTime)
	defer cancel()

	var finalDecision *Decision
	var confidence float64

	for i := 0; i < e.config.MaxIterations; i++ {
		result.Iterations = i + 1

		// Step 1: Understand the task
		thought, err := e.understand(ctx, task, i)
		if err != nil {
			return result, err
		}
		result.Thoughts = append(result.Thoughts, *thought)

		// Step 2: Evaluate options
		evalThought, err := e.evaluate(ctx, task, result.Thoughts, i)
		if err != nil {
			return result, err
		}
		result.Thoughts = append(result.Thoughts, *evalThought)

		// Step 3: Plan action
		decision, planThought, err := e.plan(ctx, task, result.Thoughts, i)
		if err != nil {
			return result, err
		}
		result.Thoughts = append(result.Thoughts, *planThought)
		confidence = decision.Confidence

		// Step 4: Self-critique (optional)
		if e.config.EnableSelfCritique && confidence < e.config.MinConfidence {
			critiqueThought, err := e.critique(ctx, decision, i)
			if err != nil {
				return result, err
			}
			result.Thoughts = append(result.Thoughts, *critiqueThought)
			continue // Another iteration
		}

		finalDecision = decision
		break
	}

	result.Decision = finalDecision
	result.TotalTime = time.Since(start)
	result.FinalConfidence = confidence

	e.logger.Info("deliberation completed",
		zap.String("task_id", task.ID),
		zap.Duration("time", result.TotalTime),
		zap.Int("iterations", result.Iterations),
		zap.Float64("confidence", confidence),
	)

	return result, nil
}

func (e *Engine) immediateDecision(task Task) (*DeliberationResult, error) {
	return &DeliberationResult{
		TaskID: task.ID,
		Decision: &Decision{
			Action:     "execute",
			Tool:       task.SuggestedTool,
			Parameters: task.Parameters,
			Reasoning:  "Immediate mode - direct execution",
			Confidence: 1.0,
		},
		TotalTime:       0,
		Iterations:      0,
		FinalConfidence: 1.0,
	}, nil
}

func (e *Engine) understand(ctx context.Context, task Task, step int) (*ThoughtProcess, error) {
	prompt := fmt.Sprintf("Understand this task: %s\nGoal: %s\nContext: %v",
		task.Description, task.Goal, task.Context)

	content, confidence, err := e.reasoner.Think(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return &ThoughtProcess{
		ID:         fmt.Sprintf("thought_%d_%d", step, 1),
		Step:       step,
		Type:       "understand",
		Content:    content,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}, nil
}

func (e *Engine) evaluate(ctx context.Context, task Task, thoughts []ThoughtProcess, step int) (*ThoughtProcess, error) {
	prompt := fmt.Sprintf("Evaluate options for task: %s\nAvailable tools: %v\nPrevious thoughts: %d",
		task.Description, task.AvailableTools, len(thoughts))

	content, confidence, err := e.reasoner.Think(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return &ThoughtProcess{
		ID:         fmt.Sprintf("thought_%d_%d", step, 2),
		Step:       step,
		Type:       "evaluate",
		Content:    content,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}, nil
}

func (e *Engine) plan(ctx context.Context, task Task, thoughts []ThoughtProcess, step int) (*Decision, *ThoughtProcess, error) {
	prompt := fmt.Sprintf("Plan action for: %s\nBased on %d thoughts", task.Description, len(thoughts))

	content, confidence, err := e.reasoner.Think(ctx, prompt)
	if err != nil {
		return nil, nil, err
	}

	thought := &ThoughtProcess{
		ID:         fmt.Sprintf("thought_%d_%d", step, 3),
		Step:       step,
		Type:       "plan",
		Content:    content,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}

	decision := &Decision{
		Action:     "execute",
		Tool:       task.SuggestedTool,
		Parameters: task.Parameters,
		Reasoning:  content,
		Confidence: confidence,
	}

	return decision, thought, nil
}

func (e *Engine) critique(ctx context.Context, decision *Decision, step int) (*ThoughtProcess, error) {
	prompt := fmt.Sprintf("Critique this decision: %s\nReasoning: %s\nConfidence: %.2f",
		decision.Action, decision.Reasoning, decision.Confidence)

	content, confidence, err := e.reasoner.Think(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return &ThoughtProcess{
		ID:         fmt.Sprintf("thought_%d_%d", step, 4),
		Step:       step,
		Type:       "critique",
		Content:    content,
		Confidence: confidence,
		Timestamp:  time.Now(),
	}, nil
}

// SetMode changes the deliberation mode at runtime.
func (e *Engine) SetMode(mode DeliberationMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.Mode = mode
	e.logger.Info("deliberation mode changed", zap.String("mode", string(mode)))
}

// GetMode returns the current deliberation mode.
func (e *Engine) GetMode() DeliberationMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Mode
}

// Task represents a task for deliberation.
type Task struct {
	ID             string         `json:"id"`
	Description    string         `json:"description"`
	Goal           string         `json:"goal"`
	Context        map[string]any `json:"context,omitempty"`
	AvailableTools []string       `json:"available_tools,omitempty"`
	SuggestedTool  string         `json:"suggested_tool,omitempty"`
	Parameters     map[string]any `json:"parameters,omitempty"`
}
