// 软件包审议提供了CrewAI风格的自主推理模式.
package deliberation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 审议 模式控制代理在行动前的理性.
type DeliberationMode string

const (
	ModeImmediate  DeliberationMode = "immediate"  // Direct tool execution
	ModeDeliberate DeliberationMode = "deliberate" // Full reasoning cycle
	ModeAdaptive   DeliberationMode = "adaptive"   // Context-dependent
)

// Deconfig 配置审议引擎 。
type DeliberationConfig struct {
	Mode               DeliberationMode `json:"mode"`
	MaxThinkingTime    time.Duration    `json:"max_thinking_time"`
	MinConfidence      float64          `json:"min_confidence"`
	EnableSelfCritique bool             `json:"enable_self_critique"`
	MaxIterations      int              `json:"max_iterations"`
}

// 默认De ReleaseConfig 返回默认配置 。
func DefaultDeliberationConfig() DeliberationConfig {
	return DeliberationConfig{
		Mode:               ModeDeliberate,
		MaxThinkingTime:    10 * time.Second,
		MinConfidence:      0.7,
		EnableSelfCritique: true,
		MaxIterations:      3,
	}
}

// ThoughtProcess代表了单一的推理步骤.
type ThoughtProcess struct {
	ID         string    `json:"id"`
	Step       int       `json:"step"`
	Type       string    `json:"type"` // understand, evaluate, plan, critique
	Content    string    `json:"content"`
	Confidence float64   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
}

// 审议结果载有审议结果。
type DeliberationResult struct {
	TaskID          string           `json:"task_id"`
	Thoughts        []ThoughtProcess `json:"thoughts"`
	Decision        *Decision        `json:"decision"`
	TotalTime       time.Duration    `json:"total_time"`
	Iterations      int              `json:"iterations"`
	FinalConfidence float64          `json:"final_confidence"`
}

// 决定是审议后的最后决定。
type Decision struct {
	Action     string         `json:"action"`
	Tool       string         `json:"tool,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
	Reasoning  string         `json:"reasoning"`
	Confidence float64        `json:"confidence"`
}

// 基于LLM的推理的理性界面.
type Reasoner interface {
	Think(ctx context.Context, prompt string) (string, float64, error)
}

// 发动机提供考虑能力.
type Engine struct {
	config   DeliberationConfig
	reasoner Reasoner
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewEngine创造了一个新的审议引擎.
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

// 故意在行动前进行推理。
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

		// 步骤1:了解任务
		thought, err := e.understand(ctx, task, i)
		if err != nil {
			return result, err
		}
		result.Thoughts = append(result.Thoughts, *thought)

		// 步骤2:评估选项
		evalThought, err := e.evaluate(ctx, task, result.Thoughts, i)
		if err != nil {
			return result, err
		}
		result.Thoughts = append(result.Thoughts, *evalThought)

		// 步骤3:计划行动
		decision, planThought, err := e.plan(ctx, task, result.Thoughts, i)
		if err != nil {
			return result, err
		}
		result.Thoughts = append(result.Thoughts, *planThought)
		confidence = decision.Confidence

		// 第4步:自律(可选)
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

// 设置模式在运行时更改审议模式 。
func (e *Engine) SetMode(mode DeliberationMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config.Mode = mode
	e.logger.Info("deliberation mode changed", zap.String("mode", string(mode)))
}

// GetMode 返回当前审议模式 。
func (e *Engine) GetMode() DeliberationMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Mode
}

// 任务是一项需要审议的任务。
type Task struct {
	ID             string         `json:"id"`
	Description    string         `json:"description"`
	Goal           string         `json:"goal"`
	Context        map[string]any `json:"context,omitempty"`
	AvailableTools []string       `json:"available_tools,omitempty"`
	SuggestedTool  string         `json:"suggested_tool,omitempty"`
	Parameters     map[string]any `json:"parameters,omitempty"`
}
