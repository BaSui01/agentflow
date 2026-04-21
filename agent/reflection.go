package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/reasoning"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// 反射执行器配置
type ReflectionExecutorConfig struct {
	Enabled       bool    `json:"enabled"`
	MaxIterations int     `json:"max_iterations"` // Maximum reflection iterations
	MinQuality    float64 `json:"min_quality"`    // Minimum quality threshold (0-1)
	CriticPrompt  string  `json:"critic_prompt"`  // Critic prompt template
}

// reflectionRunnerAdapter wraps *ReflectionExecutor to satisfy ReflectionRunner.
type reflectionRunnerAdapter struct {
	executor *ReflectionExecutor
}

func (a *reflectionRunnerAdapter) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	result, err := a.executor.ExecuteWithReflection(ctx, input)
	if err != nil {
		return nil, err
	}
	return result.FinalOutput, nil
}

func (a *reflectionRunnerAdapter) ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	return a.executor.ReflectStep(ctx, input, output, state)
}

// AsReflectionRunner wraps a *ReflectionExecutor as a ReflectionRunner.
func AsReflectionRunner(executor *ReflectionExecutor) ReflectionRunner {
	return &reflectionRunnerAdapter{executor: executor}
}

// promptEnhancerRunnerAdapter wraps *PromptEnhancer to satisfy PromptEnhancerRunner.
type promptEnhancerRunnerAdapter struct {
	enhancer *PromptEnhancer
}

func (a *promptEnhancerRunnerAdapter) EnhanceUserPrompt(prompt, context string) (string, error) {
	return a.enhancer.EnhanceUserPrompt(prompt, context), nil
}

// AsPromptEnhancerRunner wraps a *PromptEnhancer as a PromptEnhancerRunner.
func AsPromptEnhancerRunner(enhancer *PromptEnhancer) PromptEnhancerRunner {
	return &promptEnhancerRunnerAdapter{enhancer: enhancer}
}

// 默认反射 Config 返回默认反射配置
func DefaultReflectionConfig() *ReflectionExecutorConfig {
	config := DefaultReflectionExecutorConfig()
	return &config
}

// 默认反射 ExecutorConfig 返回默认反射配置
func DefaultReflectionExecutorConfig() ReflectionExecutorConfig {
	return ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
		CriticPrompt: `你是一个严格的评审专家。请评估以下任务执行结果的质量。

任务：{{.Task}}

执行结果：
{{.Output}}

请从以下维度评估（0-10分）：
1. 准确性：结果是否准确回答了问题
2. 完整性：是否涵盖了所有必要信息
3. 清晰度：表达是否清晰易懂
4. 相关性：是否紧扣主题

输出格式：
评分：[总分]/10
问题：[具体问题列表]
改进建议：[具体改进建议]`,
	}
}

// Critique 评审结果
type Critique struct {
	Score       float64  `json:"score"`        // 0-1 分数
	IsGood      bool     `json:"is_good"`      // 是否达标
	Issues      []string `json:"issues"`       // 问题列表
	Suggestions []string `json:"suggestions"`  // 改进建议
	RawFeedback string   `json:"raw_feedback"` // 原始反馈
}

// ReflectionResult Reflection 执行结果
type ReflectionResult struct {
	FinalOutput          *Output       `json:"final_output"`
	Iterations           int           `json:"iterations"`
	Critiques            []Critique    `json:"critiques"`
	TotalDuration        time.Duration `json:"total_duration"`
	ImprovedByReflection bool          `json:"improved_by_reflection"`
}

// ReflectionExecutor Reflection 执行器
type ReflectionExecutor struct {
	agent  *BaseAgent
	config ReflectionExecutorConfig
	logger *zap.Logger
}

// NewReflectionExecutor 创建 Reflection 执行器
func NewReflectionExecutor(agent *BaseAgent, config ReflectionExecutorConfig) *ReflectionExecutor {
	policyConfig := reflectionExecutorConfigFromPolicy(agent.loopControlPolicy())
	if config.MaxIterations <= 0 {
		config.MaxIterations = policyConfig.MaxIterations
	}
	if config.MinQuality <= 0 {
		config.MinQuality = policyConfig.MinQuality
	}
	if strings.TrimSpace(config.CriticPrompt) == "" {
		config.CriticPrompt = policyConfig.CriticPrompt
	}

	return &ReflectionExecutor{
		agent:  agent,
		config: config,
		logger: agent.Logger().With(zap.String("component", "reflection")),
	}
}

// ExecuteWithReflection 执行任务并进行 Reflection
func (r *ReflectionExecutor) ExecuteWithReflection(ctx context.Context, input *Input) (*ReflectionResult, error) {
	startTime := time.Now()

	if !r.config.Enabled {
		output, err := r.agent.executeCore(ctx, input)
		if err != nil {
			return nil, err
		}
		return &ReflectionResult{
			FinalOutput:          output,
			Iterations:           1,
			TotalDuration:        time.Since(startTime),
			ImprovedByReflection: false,
		}, nil
	}

	r.logger.Info("starting reflection execution", zap.String("trace_id", input.TraceID), zap.Int("max_iterations", r.config.MaxIterations))
	executor := &LoopExecutor{
		MaxIterations: r.config.MaxIterations,
		StepExecutor: func(ctx context.Context, input *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return r.agent.executeCore(ctx, input)
		},
		Selector: reasoningModeSelectorFunc(func(_ context.Context, _ *Input, _ *LoopState, _ *reasoning.PatternRegistry, _ bool) ReasoningSelection {
			return ReasoningSelection{Mode: ReasoningModeReflection}
		}),
		Judge:             newReflectionCompletionJudge(r.config.MinQuality, r.critique),
		ReflectionStep:    r.ReflectStep,
		ReflectionEnabled: true,
		CheckpointManager: r.agent.checkpointManager,
		AgentID:           r.agent.ID(),
		Logger:            r.logger,
	}
	output, err := executor.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]any, 4)
	}
	output.Metadata["reflection_iteration_budget"] = r.config.MaxIterations
	output.Metadata["reflection_quality_threshold"] = r.config.MinQuality
	output.Metadata["reflection_budget_scope"] = internalBudgetScope

	duration := time.Since(startTime)
	critiques := outputReflectionCritiques(output)
	improved := len(critiques) > 1
	iterations := output.IterationCount
	if iterations == 0 {
		iterations = 1
	}
	r.logger.Info("reflection execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Int("iterations", iterations),
		zap.Duration("total_duration", duration),
		zap.Bool("improved", improved))

	return &ReflectionResult{
		FinalOutput:          output,
		Iterations:           iterations,
		Critiques:            critiques,
		TotalDuration:        duration,
		ImprovedByReflection: improved,
	}, nil
}

func (r *ReflectionExecutor) ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	if !r.config.Enabled || input == nil || output == nil {
		return nil, nil
	}

	critique := outputReflectionCritique(output)
	if critique == nil {
		var err error
		critique, err = r.critique(ctx, input.Content, output.Content)
		if err != nil {
			return nil, err
		}
	}

	observation := &LoopObservation{
		Stage:     LoopStageDecideNext,
		Content:   "reflection_completed",
		Iteration: state.Iteration,
		Metadata: map[string]any{
			"reflection_critique": *critique,
			"reflection_score":    critique.Score,
			"reflection_is_good":  critique.IsGood,
		},
	}
	if critique.IsGood || state.Iteration >= state.MaxIterations {
		return &LoopReflectionResult{
			Critique:    critique,
			Observation: observation,
		}, nil
	}

	return &LoopReflectionResult{
		NextInput:   r.refineInput(input, critique),
		Critique:    critique,
		Observation: observation,
	}, nil
}

// critique 评审输出质量
func (r *ReflectionExecutor) critique(ctx context.Context, task, output string) (*Critique, error) {
	// 构建评审提示词
	prompt := r.config.CriticPrompt
	prompt = strings.ReplaceAll(prompt, "{{.Task}}", task)
	prompt = strings.ReplaceAll(prompt, "{{.Output}}", output)

	messages := []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: "你是一个专业的质量评审专家，擅长发现问题并提供建设性建议。",
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	// 调用 LLM 进行评审
	resp, err := r.agent.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("critique LLM call failed: %w", err)
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return nil, fmt.Errorf("critique LLM returned no choices: %w", err)
	}

	feedback := choice.Message.Content

	// 解析评审结果
	critique := r.parseCritique(feedback)
	critique.RawFeedback = feedback

	return critique, nil
}

// parseCritique 解析评审反馈
func (r *ReflectionExecutor) parseCritique(feedback string) *Critique {
	critique := &Critique{
		Score:       0.5, // 默认中等分数
		Issues:      []string{},
		Suggestions: []string{},
	}

	lines := strings.Split(feedback, "\n")
	var currentSection string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 提取分数
		if strings.Contains(line, "评分") || strings.Contains(line, "Score") {
			score := r.extractScore(line)
			if score > 0 {
				critique.Score = score / 10.0 // 转换为 0-1
			}
		}

		// 识别章节
		if strings.Contains(line, "问题") || strings.Contains(line, "Issues") {
			currentSection = "issues"
			continue
		}
		if strings.Contains(line, "改进建议") || strings.Contains(line, "Suggestions") {
			currentSection = "suggestions"
			continue
		}

		// 提取列表项
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•") ||
			(len(line) > 2 && line[0] >= '0' && line[0] <= '9' && line[1] == '.') {
			item := strings.TrimLeft(line, "-•0123456789. ")
			if item != "" {
				switch currentSection {
				case "issues":
					critique.Issues = append(critique.Issues, item)
				case "suggestions":
					critique.Suggestions = append(critique.Suggestions, item)
				}
			}
		}
	}

	// 判断是否达标
	critique.IsGood = critique.Score >= r.config.MinQuality

	return critique
}

// 从文本中提取分数
func (r *ReflectionExecutor) extractScore(text string) float64 {
	// 尝试提取“ X/ 10” 格式
	if idx := strings.Index(text, "/"); idx > 0 {
		// 提取“ /” 之前的部分
		beforeSlash := strings.TrimSpace(text[:idx])
		// 从结尾删除非数字字符
		numStr := ""
		for i := len(beforeSlash) - 1; i >= 0; i-- {
			ch := beforeSlash[i]
			if (ch >= '0' && ch <= '9') || ch == '.' {
				numStr = string(ch) + numStr
			} else if numStr != "" {
				break
			}
		}
		if numStr != "" {
			var score float64
			if _, err := fmt.Sscanf(numStr, "%f", &score); err == nil {
				return score
			}
		}
	}

	// 尝试提取纯数
	var score float64
	if _, err := fmt.Sscanf(text, "%f", &score); err == nil {
		return score
	}

	return 0
}

// refineInput 基于评审反馈改进输入
func (r *ReflectionExecutor) refineInput(original *Input, critique *Critique) *Input {
	// 构建改进提示
	refinementPrompt := fmt.Sprintf(`原始任务：
%s

之前的执行存在以下问题：
%s

改进建议：
%s

请重新执行任务，注意避免上述问题，并采纳改进建议。`,
		original.Content,
		strings.Join(critique.Issues, "\n- "),
		strings.Join(critique.Suggestions, "\n- "),
	)

	// 创建新的输入
	refined := &Input{
		TraceID:   original.TraceID,
		TenantID:  original.TenantID,
		UserID:    original.UserID,
		ChannelID: original.ChannelID,
		Content:   refinementPrompt,
		Context:   original.Context,
		Variables: original.Variables,
	}

	// 在 Context 中记录 Reflection 历史
	if refined.Context == nil {
		refined.Context = make(map[string]any)
	}
	refined.Context["reflection_feedback"] = critique

	return refined
}

func outputReflectionCritiques(output *Output) []Critique {
	if output == nil || output.Metadata == nil {
		return nil
	}
	critiques := make([]Critique, 0, 2)
	if rawCritiques, ok := output.Metadata["reflection_critiques"]; ok {
		storedCritiques, ok := rawCritiques.([]Critique)
		if ok {
			critiques = append(critiques, storedCritiques...)
		}
	}
	if critique := outputReflectionCritique(output); critique != nil {
		critiques = append(critiques, *critique)
	}
	if len(critiques) == 0 {
		return nil
	}
	return critiques
}

func outputReflectionCritique(output *Output) *Critique {
	if output == nil || output.Metadata == nil {
		return nil
	}
	rawCritique, ok := output.Metadata["reflection_critique"]
	if !ok {
		return nil
	}
	critique, ok := rawCritique.(Critique)
	if !ok {
		return nil
	}
	copied := critique
	return &copied
}

type reflectionCompletionJudge struct {
	minQuality float64
	fallback   CompletionJudge
	critiqueFn func(context.Context, string, string) (*Critique, error)
}

func newReflectionCompletionJudge(minQuality float64, critiqueFn func(context.Context, string, string) (*Critique, error)) CompletionJudge {
	if minQuality <= 0 {
		minQuality = 0.7
	}
	return &reflectionCompletionJudge{
		minQuality: minQuality,
		fallback:   NewDefaultCompletionJudge(),
		critiqueFn: critiqueFn,
	}
}

func (j *reflectionCompletionJudge) Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error) {
	decision, judgeErr := j.fallback.Judge(ctx, state, output, err)
	if judgeErr != nil || decision == nil || err != nil || output == nil || strings.TrimSpace(output.Content) == "" {
		return decision, judgeErr
	}

	critique, critiqueErr := j.critiqueFn(ctx, state.Goal, output.Content)
	if critiqueErr != nil {
		return nil, critiqueErr
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]any, 4)
	}
	output.Metadata["reflection_critique"] = *critique
	output.Metadata["reflection_score"] = critique.Score
	output.Metadata["reflection_is_good"] = critique.IsGood
	output.Metadata["reflection_quality_threshold"] = j.minQuality

	if critique.IsGood || critique.Score >= j.minQuality {
		decision.Decision = LoopDecisionDone
		decision.Solved = true
		decision.StopReason = StopReasonSolved
		decision.Confidence = critique.Score
		decision.Reason = "reflection quality acceptable"
		return decision, nil
	}

	if state != nil && state.Iteration >= state.MaxIterations {
		decision.Decision = LoopDecisionDone
		decision.StopReason = StopReasonBlocked
		decision.Confidence = critique.Score
		decision.Reason = "reflection iteration budget exhausted"
		if output.Metadata == nil {
			output.Metadata = make(map[string]any, 4)
		}
		output.Metadata["internal_stop_cause"] = "reflection_iteration_budget_exhausted"
		return decision, nil
	}

	return &CompletionDecision{
		Decision:       LoopDecisionReflect,
		NeedReflection: true,
		StopReason:     StopReasonBlocked,
		Confidence:     critique.Score,
		Reason:         "reflection requested another iteration",
	}, nil
}

func reflectionCritiquesFromObservations(observations []LoopObservation) []Critique {
	critiques := make([]Critique, 0, len(observations))
	for _, observation := range observations {
		if observation.Metadata == nil {
			continue
		}
		raw, ok := observation.Metadata["reflection_critique"]
		if !ok {
			continue
		}
		critique, ok := raw.(Critique)
		if ok {
			critiques = append(critiques, critique)
		}
	}
	return critiques
}
