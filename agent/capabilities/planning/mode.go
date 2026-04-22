package planning

import (
	"context"
	"fmt"
	"strings"
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

// Engine provides deliberation capabilities.
type Engine struct {
	config     DeliberationConfig
	reasoner   Reasoner
	logger     *zap.Logger
	mu         sync.RWMutex
	OnThought  func(ThoughtProcess)
	OnDecision func(Decision)
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

// Deliberate performs reasoning before action.
func (e *Engine) Deliberate(ctx context.Context, task Task) (*DeliberationResult, error) {
	mode := e.GetMode()

	// Adaptive mode: decide between immediate and deliberate based on task complexity.
	if mode == ModeAdaptive {
		mode = e.selectAdaptiveMode(task)
	}

	if mode == ModeImmediate {
		result := e.immediateDecision(task)
		return result, nil
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
		decision, iterationConfidence, done, err := e.runDeliberationIteration(ctx, task, result, i)
		if err != nil {
			return result, err
		}
		result.Iterations = i + 1
		confidence = iterationConfidence
		if !done {
			continue
		}
		finalDecision = decision
		break
	}

	if finalDecision != nil {
		e.emitDecision(*finalDecision)
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

func (e *Engine) runDeliberationIteration(ctx context.Context, task Task, result *DeliberationResult, step int) (decision *Decision, confidence float64, done bool, err error) {
	if checkErr := ensureDeliberationContext(ctx, "understand"); checkErr != nil {
		return nil, 0, false, checkErr
	}
	thought, err := e.understand(ctx, task, step)
	if err != nil {
		return nil, 0, false, err
	}
	appendThought(result, thought, e.emitThought)

	if checkErr := ensureDeliberationContext(ctx, "evaluate"); checkErr != nil {
		return nil, 0, false, checkErr
	}
	evalThought, err := e.evaluate(ctx, task, result.Thoughts, step)
	if err != nil {
		return nil, 0, false, err
	}
	appendThought(result, evalThought, e.emitThought)

	if checkErr := ensureDeliberationContext(ctx, "plan"); checkErr != nil {
		return nil, 0, false, checkErr
	}
	decision, planThought, err := e.plan(ctx, task, result.Thoughts, step)
	if err != nil {
		return nil, 0, false, err
	}
	appendThought(result, planThought, e.emitThought)

	if e.config.EnableSelfCritique && decision.Confidence < e.config.MinConfidence {
		if checkErr := ensureDeliberationContext(ctx, "critique"); checkErr != nil {
			return nil, 0, false, checkErr
		}
		critiqueThought, err := e.critique(ctx, decision, step)
		if err != nil {
			return nil, 0, false, err
		}
		appendThought(result, critiqueThought, e.emitThought)
		return nil, decision.Confidence, false, nil
	}

	return decision, decision.Confidence, true, nil
}

func ensureDeliberationContext(ctx context.Context, stage string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("deliberation canceled before %s: %w", stage, err)
	}
	return nil
}

func appendThought(result *DeliberationResult, thought *ThoughtProcess, emit func(ThoughtProcess)) {
	if result == nil || thought == nil {
		return
	}
	result.Thoughts = append(result.Thoughts, *thought)
	if emit != nil {
		emit(*thought)
	}
}

func (e *Engine) immediateDecision(task Task) *DeliberationResult {
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
	}
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
	prompt := fmt.Sprintf(
		"Plan action for: %s\nBased on %d thoughts\nAvailable tools: %v\nSuggested tool: %s\n\nSelect the best tool from the available tools list. "+
			"Include a line in your response: TOOL: <tool_name>",
		task.Description, len(thoughts), task.AvailableTools, task.SuggestedTool)

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

	selectedTool := parseToolSelection(content, task.AvailableTools, task.SuggestedTool)

	decision := &Decision{
		Action:     "execute",
		Tool:       selectedTool,
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

// GetMode returns the current deliberation mode.
func (e *Engine) GetMode() DeliberationMode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Mode
}

// selectAdaptiveMode estimates task complexity and returns the appropriate mode.
// Tasks with many tools, large context, or long goals use deliberate mode.
func (e *Engine) selectAdaptiveMode(task Task) DeliberationMode {
	complexity := 0

	// More available tools means more options to reason about.
	complexity += len(task.AvailableTools)

	// Large context maps add complexity.
	complexity += len(task.Context)

	// Long goal descriptions suggest complex tasks.
	if len(task.Goal) > 100 {
		complexity += 2
	}

	// Threshold: 3 or more complexity points triggers deliberate mode.
	if complexity >= 3 {
		e.logger.Debug("adaptive mode selected deliberate",
			zap.Int("complexity", complexity),
			zap.String("task_id", task.ID),
		)
		return ModeDeliberate
	}

	e.logger.Debug("adaptive mode selected immediate",
		zap.Int("complexity", complexity),
		zap.String("task_id", task.ID),
	)
	return ModeImmediate
}

// parseToolSelection extracts a tool name from the LLM response content.
// It looks for a line like "TOOL: <name>" and validates it against available tools.
// Falls back to suggestedTool if no valid tool is found.
func parseToolSelection(content string, availableTools []string, suggestedTool string) string {
	toolSet := make(map[string]bool, len(availableTools))
	for _, t := range availableTools {
		toolSet[strings.ToLower(t)] = true
	}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "tool:") {
			candidate := strings.TrimSpace(line[len("tool:"):])
			if toolSet[strings.ToLower(candidate)] {
				// Return the original-case version from available tools.
				for _, t := range availableTools {
					if strings.EqualFold(t, candidate) {
						return t
					}
				}
			}
		}
	}
	return suggestedTool
}

func (e *Engine) emitThought(t ThoughtProcess) {
	if e.OnThought != nil {
		e.OnThought(t)
	}
}

func (e *Engine) emitDecision(d Decision) {
	if e.OnDecision != nil {
		e.OnDecision(d)
	}
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
