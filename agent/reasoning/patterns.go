package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"go.uber.org/zap"
)

// 理由 图案定义了不同推理策略的界面.
type ReasoningPattern interface {
	// 执行运行推理模式并返回最终结果.
	Execute(ctx context.Context, task string) (*ReasoningResult, error)
	// 名称返回图案名称 。
	Name() string
}

// 理由 结果包含一个推理模式的输出.
type ReasoningResult struct {
	Pattern      string          `json:"pattern"`
	Task         string          `json:"task"`
	FinalAnswer  string          `json:"final_answer"`
	Confidence   float64         `json:"confidence,omitempty"`
	Steps        []ReasoningStep `json:"steps"`
	TotalTokens  int             `json:"total_tokens"`
	TotalLatency time.Duration   `json:"total_latency"`
	Metadata     map[string]any  `json:"metadata,omitempty"`
}

// ReasoningStep代表了推理过程中的一阶.
type ReasoningStep struct {
	StepID     string          `json:"step_id"`
	Type       string          `json:"type"` // thought, action, observation, evaluation, backtrack
	Content    string          `json:"content"`
	Score      float64         `json:"score,omitempty"`
	Children   []ReasoningStep `json:"children,omitempty"`
	Duration   time.Duration   `json:"duration"`
	TokensUsed int             `json:"tokens_used,omitempty"`
}

// ============================================================
// 图案登记
// ============================================================

// PatternRegistry 推理模式注册表 - 管理和发现可用的推理模式
type PatternRegistry struct {
	patterns map[string]ReasoningPattern
	mu       sync.RWMutex
}

// NewPatternRegistry 创建推理模式注册表
func NewPatternRegistry() *PatternRegistry {
	return &PatternRegistry{
		patterns: make(map[string]ReasoningPattern),
	}
}

// Register 注册推理模式
func (r *PatternRegistry) Register(pattern ReasoningPattern) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := pattern.Name()
	if _, exists := r.patterns[name]; exists {
		return fmt.Errorf("reasoning pattern %q already registered", name)
	}
	r.patterns[name] = pattern
	return nil
}

// Get 获取推理模式
func (r *PatternRegistry) Get(name string) (ReasoningPattern, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.patterns[name]
	return p, ok
}

// List 列出所有已注册的推理模式名称
func (r *PatternRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.patterns))
	for name := range r.patterns {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Unregister 注销推理模式
func (r *PatternRegistry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.patterns[name]; !exists {
		return false
	}
	delete(r.patterns, name)
	return true
}

// MustGet 获取推理模式，不存在则 panic
func (r *PatternRegistry) MustGet(name string) ReasoningPattern {
	p, ok := r.Get(name)
	if !ok {
		panic(fmt.Sprintf("reasoning pattern %q not registered", name))
	}
	return p
}

// ============================================================
// 思维树图案
// ============================================================

// TreatyOfThoughtConfig 配置"思想之树"推理模式.
type TreeOfThoughtConfig struct {
	BranchingFactor int           // Number of thoughts to generate at each step
	MaxDepth        int           // Maximum depth of the thought tree
	BeamWidth       int           // Number of best paths to keep (beam search)
	EvaluationMode  string        // "self" (LLM self-eval) or "vote" (majority voting)
	PruneThreshold  float64       // Minimum score to keep a branch (0-1)
	Timeout         time.Duration // Overall timeout
	ParallelEval    bool          // Evaluate branches in parallel
}

// 默认TreeOfThoughtConfig 返回合理的默认值 。
func DefaultTreeOfThoughtConfig() TreeOfThoughtConfig {
	return TreeOfThoughtConfig{
		BranchingFactor: 3,
		MaxDepth:        5,
		BeamWidth:       2,
		EvaluationMode:  "self",
		PruneThreshold:  0.3,
		Timeout:         120 * time.Second,
		ParallelEval:    true,
	}
}

// Tree Of Thought 执行思想之树推理模式.
// 它平行地探索多条推理路径,并选择了最好的一条.
type TreeOfThought struct {
	provider     llm.Provider
	toolExecutor tools.ToolExecutor
	config       TreeOfThoughtConfig
	logger       *zap.Logger
}

// NewTreeOfThought创造出"思想理性之树".
func NewTreeOfThought(provider llm.Provider, executor tools.ToolExecutor, config TreeOfThoughtConfig, logger *zap.Logger) *TreeOfThought {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TreeOfThought{
		provider:     provider,
		toolExecutor: executor,
		config:       config,
		logger:       logger,
	}
}

func (t *TreeOfThought) Name() string { return "tree_of_thought" }

// 执行运行"思想之树"推理模式.
func (t *TreeOfThought) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, t.config.Timeout)
	defer cancel()

	result := &ReasoningResult{
		Pattern:  t.Name(),
		Task:     task,
		Metadata: make(map[string]any),
	}

	// 生成初始想法
	thoughts, tokens, err := t.generateThoughts(ctx, task, nil, t.config.BranchingFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to generate initial thoughts: %w", err)
	}
	result.TotalTokens += tokens

	// 用光束搜索构建树
	currentLevel := thoughts
	for depth := 0; depth < t.config.MaxDepth && len(currentLevel) > 0; depth++ {
		t.logger.Debug("ToT depth", zap.Int("depth", depth), zap.Int("branches", len(currentLevel)))

		// 评价当前水平
		evaluated, evalTokens := t.evaluateThoughts(ctx, task, currentLevel)
		result.TotalTokens += evalTokens

		// 倾斜并选择顶端分支
		selected := t.selectTopBranches(evaluated, t.config.BeamWidth)
		if len(selected) == 0 {
			break
		}

		// 检查是否有分支是最后答案
		for _, s := range selected {
			if s.Score >= 0.9 {
				result.FinalAnswer = s.Content
				result.Confidence = s.Score
				result.Steps = append(result.Steps, s)
				result.TotalLatency = time.Since(start)
				return result, nil
			}
		}

		// 生成下一个层次的想法
		var nextLevel []ReasoningStep
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, branch := range selected {
			result.Steps = append(result.Steps, branch)
			wg.Add(1)
			go func(b ReasoningStep) {
				defer wg.Done()
				children, childTokens, err := t.generateThoughts(ctx, task, &b, t.config.BranchingFactor)
				if err != nil {
					t.logger.Warn("failed to generate children", zap.Error(err))
					return
				}
				mu.Lock()
				nextLevel = append(nextLevel, children...)
				result.TotalTokens += childTokens
				mu.Unlock()
			}(branch)
		}
		wg.Wait()
		currentLevel = nextLevel
	}

	// 从剩余分支中选择最佳最后答案
	if len(currentLevel) > 0 {
		best := t.selectTopBranches(currentLevel, 1)
		if len(best) > 0 {
			result.FinalAnswer = best[0].Content
			result.Confidence = best[0].Score
		}
	}

	result.TotalLatency = time.Since(start)
	result.Metadata["max_depth_reached"] = true
	return result, nil
}

func (t *TreeOfThought) generateThoughts(ctx context.Context, task string, parent *ReasoningStep, count int) ([]ReasoningStep, int, error) {
	prompt := fmt.Sprintf(`Task: %s

Generate %d different approaches or next steps to solve this task.
For each approach, provide a clear reasoning path.

Format your response as JSON array:
[{"thought": "approach 1", "reasoning": "why this might work"}, ...]`, task, count)

	if parent != nil {
		prompt = fmt.Sprintf(`Task: %s

Previous step: %s

Generate %d different next steps to continue from the previous step.
Format as JSON array: [{"thought": "next step", "reasoning": "why"}]`, task, parent.Content, count)
	}

	resp, err := t.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.8,
		MaxTokens:   1000,
	})
	if err != nil {
		return nil, 0, err
	}

	tokens := resp.Usage.TotalTokens
	genChoice, err := llm.FirstChoice(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("thought generation returned no choices: %w", err)
	}
	content := genChoice.Message.Content

	// 从反应中解析想法
	var thoughtsData []struct {
		Thought   string `json:"thought"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(content), &thoughtsData); err != nil {
		// 倒计时:将整个反应视为单一想法
		return []ReasoningStep{{
			StepID:  fmt.Sprintf("thought_%d", time.Now().UnixNano()),
			Type:    "thought",
			Content: content,
		}}, tokens, nil
	}

	steps := make([]ReasoningStep, len(thoughtsData))
	for i, td := range thoughtsData {
		steps[i] = ReasoningStep{
			StepID:  fmt.Sprintf("thought_%d_%d", time.Now().UnixNano(), i),
			Type:    "thought",
			Content: td.Thought + " - " + td.Reasoning,
		}
	}
	return steps, tokens, nil
}

func (t *TreeOfThought) evaluateThoughts(ctx context.Context, task string, thoughts []ReasoningStep) ([]ReasoningStep, int) {
	if !t.config.ParallelEval {
		return t.evaluateSequential(ctx, task, thoughts)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	totalTokens := 0

	for i := range thoughts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			score, tokens := t.evaluateSingle(ctx, task, thoughts[idx])
			mu.Lock()
			thoughts[idx].Score = score
			totalTokens += tokens
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	return thoughts, totalTokens
}

func (t *TreeOfThought) evaluateSequential(ctx context.Context, task string, thoughts []ReasoningStep) ([]ReasoningStep, int) {
	totalTokens := 0
	for i := range thoughts {
		score, tokens := t.evaluateSingle(ctx, task, thoughts[i])
		thoughts[i].Score = score
		totalTokens += tokens
	}
	return thoughts, totalTokens
}

func (t *TreeOfThought) evaluateSingle(ctx context.Context, task string, thought ReasoningStep) (float64, int) {
	prompt := fmt.Sprintf(`Task: %s
Proposed approach: %s

Rate this approach on a scale of 0.0 to 1.0 based on:
- Likelihood of leading to correct solution
- Logical soundness
- Completeness

Respond with only a number between 0.0 and 1.0`, task, thought.Content)

	resp, err := t.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   10,
	})
	if err != nil {
		return 0.5, 0
	}

	var score float64
	evalChoice, choiceErr := llm.FirstChoice(resp)
	if choiceErr != nil {
		return 0.5, resp.Usage.TotalTokens
	}
	fmt.Sscanf(evalChoice.Message.Content, "%f", &score)
	if score < 0 || score > 1 {
		score = 0.5
	}
	return score, resp.Usage.TotalTokens
}

func (t *TreeOfThought) selectTopBranches(thoughts []ReasoningStep, n int) []ReasoningStep {
	// 按阈值过滤
	var filtered []ReasoningStep
	for _, th := range thoughts {
		if th.Score >= t.config.PruneThreshold {
			filtered = append(filtered, th)
		}
	}

	// 按分数排序(优化:O(n logn n)而不是O(n2))
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	if len(filtered) > n {
		return filtered[:n]
	}
	return filtered
}
