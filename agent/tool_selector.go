package agent

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ToolScore 工具评分
type ToolScore struct {
	Tool               llm.ToolSchema `json:"tool"`
	SemanticSimilarity float64        `json:"semantic_similarity"` // Semantic similarity (0-1)
	EstimatedCost      float64        `json:"estimated_cost"`      // Estimated cost
	AvgLatency         time.Duration  `json:"avg_latency"`         // Average latency
	ReliabilityScore   float64        `json:"reliability_score"`   // Reliability (0-1)
	TotalScore         float64        `json:"total_score"`         // Total score (0-1)
}

// ToolSelectionConfig 工具选择配置
type ToolSelectionConfig struct {
	Enabled bool `json:"enabled"`

	// Scoring weights
	SemanticWeight    float64 `json:"semantic_weight"`    // Semantic similarity weight
	CostWeight        float64 `json:"cost_weight"`        // Cost weight
	LatencyWeight     float64 `json:"latency_weight"`     // Latency weight
	ReliabilityWeight float64 `json:"reliability_weight"` // Reliability weight

	// Selection strategy
	MaxTools      int     `json:"max_tools"`       // Maximum number of tools to select
	MinScore      float64 `json:"min_score"`       // Minimum score threshold
	UseLLMRanking bool    `json:"use_llm_ranking"` // Whether to use LLM-assisted ranking
}

// DefaultToolSelectionConfig returns default tool selection configuration
func DefaultToolSelectionConfig() *ToolSelectionConfig {
	config := defaultToolSelectionConfigValue()
	return &config
}

// defaultToolSelectionConfigValue returns default tool selection configuration value
func defaultToolSelectionConfigValue() ToolSelectionConfig {
	return ToolSelectionConfig{
		Enabled:           true,
		SemanticWeight:    0.5,
		CostWeight:        0.2,
		LatencyWeight:     0.15,
		ReliabilityWeight: 0.15,
		MaxTools:          5,
		MinScore:          0.3,
		UseLLMRanking:     true,
	}
}

// ToolSelector 工具选择器接口
type ToolSelector interface {
	// SelectTools 基于任务选择最佳工具
	SelectTools(ctx context.Context, task string, availableTools []llm.ToolSchema) ([]llm.ToolSchema, error)

	// ScoreTools 对工具进行评分
	ScoreTools(ctx context.Context, task string, tools []llm.ToolSchema) ([]ToolScore, error)
}

// DynamicToolSelector 动态工具选择器
type DynamicToolSelector struct {
	agent  *BaseAgent
	config ToolSelectionConfig

	// Tool statistics (can be loaded from database)
	toolStats map[string]*ToolStats

	logger *zap.Logger
}

// ToolStats 工具统计信息
type ToolStats struct {
	Name            string
	TotalCalls      int64
	SuccessfulCalls int64
	FailedCalls     int64
	TotalLatency    time.Duration
	AvgCost         float64
}

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
func (s *DynamicToolSelector) SelectTools(ctx context.Context, task string, availableTools []llm.ToolSchema) ([]llm.ToolSchema, error) {
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
	selected := []llm.ToolSchema{}
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

// ScoreTools 对工具进行评分
func (s *DynamicToolSelector) ScoreTools(ctx context.Context, task string, tools []llm.ToolSchema) ([]ToolScore, error) {
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

// calculateSemanticSimilarity calculates semantic similarity between task and tool
func (s *DynamicToolSelector) calculateSemanticSimilarity(task string, tool llm.ToolSchema) float64 {
	// Simplified version: keyword-based matching
	// Production should use vector embeddings + cosine similarity

	taskLower := strings.ToLower(task)
	toolDesc := strings.ToLower(tool.Description)
	toolName := strings.ToLower(tool.Name)

	// Extract task keywords
	keywords := extractKeywords(taskLower)

	matchCount := 0
	for _, keyword := range keywords {
		if strings.Contains(toolDesc, keyword) || strings.Contains(toolName, keyword) {
			matchCount++
		}
	}

	if len(keywords) == 0 {
		return 0.5 // Default medium similarity
	}

	similarity := float64(matchCount) / float64(len(keywords))

	// Bonus for exact name match
	for _, keyword := range keywords {
		if strings.Contains(toolName, keyword) {
			similarity = math.Min(1.0, similarity+0.2)
		}
	}

	return similarity
}

// estimateCost estimates tool execution cost
func (s *DynamicToolSelector) estimateCost(tool llm.ToolSchema) float64 {
	// Simplified version: estimate based on tool type
	// Production should use historical data statistics

	name := strings.ToLower(tool.Name)

	// High-cost tools
	if strings.Contains(name, "api") || strings.Contains(name, "external") {
		return 0.1
	}

	// Medium-cost tools
	if strings.Contains(name, "search") || strings.Contains(name, "query") {
		return 0.05
	}

	// Low-cost tools
	return 0.01
}

// getAvgLatency 获取平均延迟
func (s *DynamicToolSelector) getAvgLatency(toolName string) time.Duration {
	if stats, ok := s.toolStats[toolName]; ok && stats.TotalCalls > 0 {
		return stats.TotalLatency / time.Duration(stats.TotalCalls)
	}

	// Default latency estimate
	return 500 * time.Millisecond
}

// getReliability 获取可靠性分数
func (s *DynamicToolSelector) getReliability(toolName string) float64 {
	if stats, ok := s.toolStats[toolName]; ok && stats.TotalCalls > 0 {
		return float64(stats.SuccessfulCalls) / float64(stats.TotalCalls)
	}

	// Default reliability for new tools
	return 0.8
}

// calculateTotalScore calculates the total weighted score
func (s *DynamicToolSelector) calculateTotalScore(score ToolScore) float64 {
	// Normalize each metric
	semanticScore := score.SemanticSimilarity

	// Lower cost is better (inverse)
	costScore := 1.0 - math.Min(1.0, score.EstimatedCost*10)

	// Lower latency is better (inverse, assuming 5s is worst)
	latencyScore := 1.0 - math.Min(1.0, float64(score.AvgLatency)/float64(5*time.Second))

	reliabilityScore := score.ReliabilityScore

	// Weighted sum
	total := semanticScore*s.config.SemanticWeight +
		costScore*s.config.CostWeight +
		latencyScore*s.config.LatencyWeight +
		reliabilityScore*s.config.ReliabilityWeight

	return total
}

// llmRanking uses LLM for secondary ranking
func (s *DynamicToolSelector) llmRanking(ctx context.Context, task string, scores []ToolScore) ([]ToolScore, error) {
	// Build tool list description
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

	messages := []llm.Message{
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

	// Parse tool indices returned by LLM
	selected := parseToolIndices(resp.Choices[0].Message.Content)

	// Reorder tools
	reordered := []ToolScore{}
	for _, idx := range selected {
		if idx > 0 && idx <= len(scores) {
			reordered = append(reordered, scores[idx-1])
		}
	}

	// Add remaining tools
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

// UpdateToolStats 更新工具统计信息
func (s *DynamicToolSelector) UpdateToolStats(toolName string, success bool, latency time.Duration, cost float64) {
	if s.toolStats[toolName] == nil {
		s.toolStats[toolName] = &ToolStats{
			Name: toolName,
		}
	}

	stats := s.toolStats[toolName]
	stats.TotalCalls++
	if success {
		stats.SuccessfulCalls++
	} else {
		stats.FailedCalls++
	}
	stats.TotalLatency += latency

	// Update average cost (moving average)
	if stats.TotalCalls == 1 {
		stats.AvgCost = cost
	} else {
		stats.AvgCost = (stats.AvgCost*float64(stats.TotalCalls-1) + cost) / float64(stats.TotalCalls)
	}
}

// extractKeywords extracts keywords from text (simplified version)
func extractKeywords(text string) []string {
	// Remove common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"是": true, "的": true, "了": true, "在": true, "和": true,
		"与": true, "或": true, "但": true, "对": true, "从": true,
	}

	words := strings.Fields(text)
	keywords := []string{}

	// Define punctuation to remove (using raw string to avoid escaping)
	punctuation := `,.!?;:"'()[]{}，。！？；：""''（）【】`

	for _, word := range words {
		word = strings.Trim(word, punctuation)
		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// parseToolIndices parses tool indices from text
// Only parses comma-separated format, returns empty for newline-separated
func parseToolIndices(text string) []int {
	indices := []int{}

	// Check if text contains newlines without commas - return empty
	if strings.Contains(text, "\n") && !strings.Contains(text, ",") {
		return indices
	}

	// Remove all spaces and newlines
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "\n", "")

	// Split by comma
	parts := strings.Split(text, ",")

	for _, part := range parts {
		if part == "" {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(part, "%d", &idx); err == nil {
			indices = append(indices, idx)
		}
	}

	return indices
}
