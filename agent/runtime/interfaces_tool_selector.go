package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	skills "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	executionloop "github.com/BaSui01/agentflow/agent/execution/loop"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"time"
)

// Merged from tool_selector.go.

// ToolScore 工具评分
type ToolScore = skills.DynamicToolScore

// ToolSelectionConfig 工具选择配置
type ToolSelectionConfig = skills.DynamicToolSelectionConfig

// 默认工具SecutConfig 返回默认工具选择配置
func DefaultToolSelectionConfig() *ToolSelectionConfig {
	config := defaultToolSelectionConfigValue()
	return &config
}

// 默认工具Secution ConfigValue 返回默认工具选择配置值
func defaultToolSelectionConfigValue() ToolSelectionConfig {
	return skills.DefaultDynamicToolSelectionConfig()
}

// ToolSelector 工具选择器接口
type ToolSelector interface {
	// SelectTools 基于任务选择最佳工具
	SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error)

	// ScoreTools 对工具进行评分
	ScoreTools(ctx context.Context, task string, tools []types.ToolSchema) ([]ToolScore, error)
}

// ReasoningExposureLevel controls which non-default reasoning patterns are
// registered into the runtime. The official execution path remains react,
// with reflection as an opt-in quality enhancement outside the registry.
type ReasoningExposureLevel string

const (
	ReasoningExposureOfficial ReasoningExposureLevel = "official"
	ReasoningExposureAdvanced ReasoningExposureLevel = "advanced"
	ReasoningExposureAll      ReasoningExposureLevel = "all"
)

func normalizeReasoningExposureLevel(level ReasoningExposureLevel) ReasoningExposureLevel {
	switch level {
	case ReasoningExposureAdvanced, ReasoningExposureAll:
		return level
	default:
		return ReasoningExposureOfficial
	}
}

const (
	ReasoningModeReact          = executionloop.ReasoningModeReact
	ReasoningModeReflection     = executionloop.ReasoningModeReflection
	ReasoningModeReWOO          = executionloop.ReasoningModeReWOO
	ReasoningModePlanAndExecute = executionloop.ReasoningModePlanAndExecute
	ReasoningModeDynamicPlanner = executionloop.ReasoningModeDynamicPlanner
	ReasoningModeTreeOfThought  = executionloop.ReasoningModeTreeOfThought
)

type ReasoningSelection = executionloop.ReasoningSelection

type ReasoningModeSelector interface {
	Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection
}

type reasoningModeSelectorFunc func(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection

func (f reasoningModeSelectorFunc) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	return f(ctx, input, state, registry, reflectionEnabled)
}

type DefaultReasoningModeSelector struct{}

func NewDefaultReasoningModeSelector() ReasoningModeSelector { return DefaultReasoningModeSelector{} }

func (DefaultReasoningModeSelector) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	selection := executionloop.DefaultReasoningModeSelector{}.Select(ctx, loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry, reflectionEnabled)
	return ReasoningSelection(selection)
}

func runtimeSelectResumedReasoningMode(state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) (ReasoningSelection, bool) {
	return executionloop.SelectResumedReasoningMode(loopExecutionStateFromRoot(state), registry, reflectionEnabled)
}

func runtimeBuildReasoningSelectionWithFallback(mode string, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	return executionloop.BuildReasoningSelectionWithFallback(mode, registry, reflectionEnabled)
}

func runtimeBuildReasoningSelection(mode string, registry *reasoning.PatternRegistry) ReasoningSelection {
	return executionloop.BuildReasoningSelection(mode, registry)
}

func runtimeNormalizeReasoningMode(value string) string {
	return executionloop.NormalizeReasoningMode(value)
}

func runtimeShouldUseReflection(input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) bool {
	return executionloop.ShouldUseReflection(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry, reflectionEnabled)
}

func runtimeShouldUseReWOO(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUseReWOO(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func runtimeShouldUsePlanAndExecute(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUsePlanAndExecute(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func runtimeShouldUseDynamicPlanner(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUseDynamicPlanner(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func runtimeShouldUseTreeOfThought(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUseTreeOfThought(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func hasReasoningPattern(registry *reasoning.PatternRegistry, mode string) bool {
	return executionloop.HasReasoningPattern(registry, mode)
}

// DynamicToolSelector 动态工具选择器
type DynamicToolSelector struct {
	agent  *BaseAgent
	config ToolSelectionConfig

	// 工具统计(可以从数据库中加载)
	toolStats map[string]*ToolStats

	logger *zap.Logger
}

// ToolStats 工具统计信息
type ToolStats = skills.DynamicToolStats

// NewDynamicToolSelector 创建动态工具选择器
func NewDynamicToolSelector(agent *BaseAgent, config ToolSelectionConfig) *DynamicToolSelector {
	if config.MaxTools <= 0 {
		config.MaxTools = 5
	}
	if config.MinScore <= 0 {
		config.MinScore = 0.3
	}

	return &DynamicToolSelector{
		agent:     agent,
		config:    config,
		toolStats: make(map[string]*ToolStats),
		logger:    agent.Logger().With(zap.String("component", "tool_selector")),
	}
}

// SelectTools 选择最佳工具
func (s *DynamicToolSelector) SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error) {
	if !s.config.Enabled || len(availableTools) == 0 {
		return availableTools, nil
	}

	s.logger.Debug("selecting tools",
		zap.String("task", task),
		zap.Int("available_tools", len(availableTools)),
	)

	// 1. 对所有工具评分
	scores, err := s.ScoreTools(ctx, task, availableTools)
	if err != nil {
		s.logger.Warn("tool scoring failed, using all tools", zap.Error(err))
		return availableTools, nil
	}

	// 2. 按分数排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})

	// 3. 可选：使用 LLM 进行二次排序
	if s.config.UseLLMRanking && len(scores) > s.config.MaxTools {
		scores, err = s.llmRanking(ctx, task, scores)
		if err != nil {
			s.logger.Warn("LLM ranking failed, using score-based ranking", zap.Error(err))
		}
	}

	// 4. 选择 Top-K 工具
	selected := []types.ToolSchema{}
	for i, score := range scores {
		if i >= s.config.MaxTools {
			break
		}
		if score.TotalScore < s.config.MinScore {
			break
		}
		selected = append(selected, score.Tool)
	}

	s.logger.Info("tools selected",
		zap.Int("selected", len(selected)),
		zap.Int("total", len(availableTools)),
	)

	return selected, nil
}

func (b *BaseAgent) toolSelectionMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		b.logger.Debug("selecting tools dynamically", zap.String("trace_id", input.TraceID))
		availableTools := b.toolManager.GetAllowedTools(b.ID())
		selected, err := b.extensions.ToolSelector().SelectTools(ctx, input.Content, availableTools)
		if err != nil {
			b.logger.Warn("tool selection failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			toolNames := make([]string, 0, len(selected))
			for _, tool := range selected {
				name := strings.TrimSpace(tool.Name)
				if name == "" {
					continue
				}
				toolNames = append(toolNames, name)
			}

			override := &RunConfig{}
			if len(toolNames) == 0 {
				override.DisableTools = true
			} else {
				override.ToolWhitelist = toolNames
			}
			ctx = WithRunConfig(ctx, MergeRunConfig(GetRunConfig(ctx), override))

			b.logger.Info("tools selected dynamically",
				zap.String("trace_id", input.TraceID),
				zap.Strings("selected_tools", toolNames),
				zap.Bool("tools_disabled", len(toolNames) == 0),
			)
		}
		return next(ctx, input)
	}
}

// ScoreTools 对工具进行评分
func (s *DynamicToolSelector) ScoreTools(ctx context.Context, task string, tools []types.ToolSchema) ([]ToolScore, error) {
	scores := make([]ToolScore, len(tools))

	for i, tool := range tools {
		score := ToolScore{
			Tool: tool,
		}

		// 1. 语义相似度（基于描述匹配）
		score.SemanticSimilarity = s.calculateSemanticSimilarity(task, tool)

		// 2. 成本评估
		score.EstimatedCost = s.estimateCost(tool)

		// 3. 延迟评估
		score.AvgLatency = s.getAvgLatency(tool.Name)

		// 4. 可靠性评估
		score.ReliabilityScore = s.getReliability(tool.Name)

		// 5. 计算综合得分
		score.TotalScore = s.calculateTotalScore(score)

		scores[i] = score
	}

	return scores, nil
}

// 计算任务和工具之间的语义相似性
func (s *DynamicToolSelector) calculateSemanticSimilarity(task string, tool types.ToolSchema) float64 {
	return skills.DynamicToolSemanticSimilarity(task, tool)
}

// 成本估计工具执行费用
func (s *DynamicToolSelector) estimateCost(tool types.ToolSchema) float64 {
	return skills.DynamicToolEstimateCost(tool)
}

// getAvgLatency 获取平均延迟
func (s *DynamicToolSelector) getAvgLatency(toolName string) time.Duration {
	return skills.DynamicToolAverageLatency(s.toolStats[toolName])
}

// getReliability 获取可靠性分数
func (s *DynamicToolSelector) getReliability(toolName string) float64 {
	return skills.DynamicToolReliability(s.toolStats[toolName])
}

// 计算总加权分数
func (s *DynamicToolSelector) calculateTotalScore(score ToolScore) float64 {
	return skills.DynamicToolTotalScore(score, s.config)
}

// llmRanking 使用 LLM 进行二级排名
func (s *DynamicToolSelector) llmRanking(ctx context.Context, task string, scores []ToolScore) ([]ToolScore, error) {
	// 构建工具列表描述
	toolList := []string{}
	for i, score := range scores {
		if i >= s.config.MaxTools*2 { // Only let LLM rank top 2*MaxTools
			break
		}
		toolList = append(toolList, fmt.Sprintf("%d. %s: %s (Score: %.2f)",
			i+1, score.Tool.Name, score.Tool.Description, score.TotalScore))
	}

	prompt := fmt.Sprintf(`任务：%s

可用工具列表：
%s

请从上述工具中选择最适合完成任务的 %d 个工具，按优先级排序。
只输出工具编号，用逗号分隔，例如：1,3,5`,
		task,
		strings.Join(toolList, "\n"),
		s.config.MaxTools,
	)

	messages := []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: "你是一个工具选择专家，擅长为任务选择最合适的工具。",
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	resp, err := s.agent.ChatCompletion(ctx, messages)
	if err != nil {
		return scores, err
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return scores, err
	}

	// LLM 返回的解析工具索引
	selected := parseToolIndices(choice.Message.Content)

	// 重排工具
	reordered := []ToolScore{}
	for _, idx := range selected {
		if idx > 0 && idx <= len(scores) {
			reordered = append(reordered, scores[idx-1])
		}
	}

	// 添加剩余工具
	usedIndices := make(map[int]bool)
	for _, idx := range selected {
		usedIndices[idx] = true
	}
	for i := range scores {
		if !usedIndices[i+1] {
			reordered = append(reordered, scores[i])
		}
	}

	return reordered, nil
}

// AsToolSelectorRunner wraps a *DynamicToolSelector as a DynamicToolSelectorRunner.
// Since the interface now uses concrete types, this is a direct cast.
func AsToolSelectorRunner(selector *DynamicToolSelector) DynamicToolSelectorRunner {
	return selector
}

// UpdateToolStats 更新工具统计信息
func (s *DynamicToolSelector) UpdateToolStats(toolName string, success bool, latency time.Duration, cost float64) {
	skills.DynamicToolUpdateStats(s.toolStats, toolName, success, latency, cost)
}

// 取出关键字从文本中取出关键字(简化版)
func extractKeywords(text string) []string {
	return skills.DynamicToolExtractKeywords(text)
}

// 解析工具索引
// 只解析逗号分隔格式, 返回新行分隔为空
func parseToolIndices(text string) []int {
	return skills.DynamicToolParseIndices(text)
}
