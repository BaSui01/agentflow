package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// 反射执行器配置
type ReflectionExecutorConfig struct {
	Enabled       bool    `json:"enabled"`
	MaxIterations int     `json:"max_iterations"` // Maximum reflection iterations
	MinQuality    float64 `json:"min_quality"`    // Minimum quality threshold (0-1)
	CriticPrompt  string  `json:"critic_prompt"`  // Critic prompt template
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
	if config.MaxIterations <= 0 {
		config.MaxIterations = 3
	}
	if config.MinQuality <= 0 {
		config.MinQuality = 0.7
	}
	if config.CriticPrompt == "" {
		config = DefaultReflectionExecutorConfig()
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
		// Reflection 未启用，直接执行
		output, err := r.agent.Execute(ctx, input)
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

	r.logger.Info("starting reflection execution",
		zap.String("trace_id", input.TraceID),
		zap.Int("max_iterations", r.config.MaxIterations),
	)

	var (
		currentInput  = input
		currentOutput *Output
		critiques     []Critique
		improved      = false
	)

	// Reflection 循环
	for i := 0; i < r.config.MaxIterations; i++ {
		r.logger.Debug("reflection iteration",
			zap.Int("iteration", i+1),
			zap.String("trace_id", input.TraceID),
		)

		// 1. 执行任务
		output, err := r.agent.Execute(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("execution failed at iteration %d: %w", i+1, err)
		}
		currentOutput = output

		// 2. 评审结果
		critique, err := r.critique(ctx, input.Content, output.Content)
		if err != nil {
			r.logger.Warn("critique failed, using current output",
				zap.Error(err),
				zap.Int("iteration", i+1),
			)
			break
		}
		critiques = append(critiques, *critique)

		r.logger.Info("critique completed",
			zap.Int("iteration", i+1),
			zap.Float64("score", critique.Score),
			zap.Bool("is_good", critique.IsGood),
		)

		// 3. 检查是否达标
		if critique.IsGood {
			r.logger.Info("output quality acceptable",
				zap.Int("iteration", i+1),
				zap.Float64("score", critique.Score),
			)
			if i > 0 {
				improved = true
			}
			break
		}

		// 4. 最后一次迭代，不再改进
		if i == r.config.MaxIterations-1 {
			r.logger.Warn("max iterations reached, using current output",
				zap.Float64("final_score", critique.Score),
			)
			break
		}

		// 5. 基于反馈改进输入
		currentInput = r.refineInput(input, critique)
		improved = true
	}

	duration := time.Since(startTime)

	r.logger.Info("reflection execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Int("iterations", len(critiques)),
		zap.Duration("total_duration", duration),
		zap.Bool("improved", improved),
	)

	return &ReflectionResult{
		FinalOutput:          currentOutput,
		Iterations:           len(critiques),
		Critiques:            critiques,
		TotalDuration:        duration,
		ImprovedByReflection: improved,
	}, nil
}

// critique 评审输出质量
func (r *ReflectionExecutor) critique(ctx context.Context, task, output string) (*Critique, error) {
	// 构建评审提示词
	prompt := r.config.CriticPrompt
	prompt = strings.ReplaceAll(prompt, "{{.Task}}", task)
	prompt = strings.ReplaceAll(prompt, "{{.Output}}", output)

	messages := []llm.Message{
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

	feedback := resp.Choices[0].Message.Content

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
