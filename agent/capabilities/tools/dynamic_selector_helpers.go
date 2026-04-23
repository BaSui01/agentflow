package tools

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"
)

type DynamicToolScore struct {
	Tool               types.ToolSchema `json:"tool"`
	SemanticSimilarity float64          `json:"semantic_similarity"`
	EstimatedCost      float64          `json:"estimated_cost"`
	AvgLatency         time.Duration    `json:"avg_latency"`
	ReliabilityScore   float64          `json:"reliability_score"`
	TotalScore         float64          `json:"total_score"`
}

type DynamicToolSelectionConfig struct {
	Enabled           bool    `json:"enabled"`
	SemanticWeight    float64 `json:"semantic_weight"`
	CostWeight        float64 `json:"cost_weight"`
	LatencyWeight     float64 `json:"latency_weight"`
	ReliabilityWeight float64 `json:"reliability_weight"`
	MaxTools          int     `json:"max_tools"`
	MinScore          float64 `json:"min_score"`
	UseLLMRanking     bool    `json:"use_llm_ranking"`
}

type DynamicToolStats struct {
	Name            string
	TotalCalls      int64
	SuccessfulCalls int64
	FailedCalls     int64
	TotalLatency    time.Duration
	AvgCost         float64
}

func DefaultDynamicToolSelectionConfig() DynamicToolSelectionConfig {
	return DynamicToolSelectionConfig{
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

func DynamicToolSemanticSimilarity(task string, tool types.ToolSchema) float64 {
	taskLower := strings.ToLower(task)
	toolDesc := strings.ToLower(tool.Description)
	toolName := strings.ToLower(tool.Name)
	keywords := DynamicToolExtractKeywords(taskLower)

	matchCount := 0
	for _, keyword := range keywords {
		if strings.Contains(toolDesc, keyword) || strings.Contains(toolName, keyword) {
			matchCount++
		}
	}
	if len(keywords) == 0 {
		return 0.5
	}

	similarity := float64(matchCount) / float64(len(keywords))
	for _, keyword := range keywords {
		if strings.Contains(toolName, keyword) {
			similarity = math.Min(1.0, similarity+0.2)
		}
	}
	return similarity
}

func DynamicToolEstimateCost(tool types.ToolSchema) float64 {
	name := strings.ToLower(tool.Name)
	switch {
	case strings.Contains(name, "api") || strings.Contains(name, "external"):
		return 0.1
	case strings.Contains(name, "search") || strings.Contains(name, "query"):
		return 0.05
	default:
		return 0.01
	}
}

func DynamicToolAverageLatency(stats *DynamicToolStats) time.Duration {
	if stats != nil && stats.TotalCalls > 0 {
		return stats.TotalLatency / time.Duration(stats.TotalCalls)
	}
	return 500 * time.Millisecond
}

func DynamicToolReliability(stats *DynamicToolStats) float64 {
	if stats != nil && stats.TotalCalls > 0 {
		return float64(stats.SuccessfulCalls) / float64(stats.TotalCalls)
	}
	return 0.8
}

func DynamicToolTotalScore(score DynamicToolScore, cfg DynamicToolSelectionConfig) float64 {
	semanticScore := score.SemanticSimilarity
	costScore := 1.0 - math.Min(1.0, score.EstimatedCost*10)
	latencyScore := 1.0 - math.Min(1.0, float64(score.AvgLatency)/float64(5*time.Second))
	reliabilityScore := score.ReliabilityScore

	return semanticScore*cfg.SemanticWeight +
		costScore*cfg.CostWeight +
		latencyScore*cfg.LatencyWeight +
		reliabilityScore*cfg.ReliabilityWeight
}

func DynamicToolExtractKeywords(text string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"是": true, "的": true, "了": true, "在": true, "和": true,
		"与": true, "或": true, "但": true, "对": true, "从": true,
	}

	words := strings.Fields(text)
	keywords := make([]string, 0, len(words))
	punctuation := `,.!?;:"'()[]{}，。！？；：（）【】`

	for _, word := range words {
		word = strings.Trim(word, punctuation)
		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

func DynamicToolParseIndices(text string) []int {
	indices := []int{}
	if strings.Contains(text, "\n") && !strings.Contains(text, ",") {
		return indices
	}
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "\n", "")
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

func DynamicToolUpdateStats(stats map[string]*DynamicToolStats, toolName string, success bool, latency time.Duration, cost float64) {
	if stats[toolName] == nil {
		stats[toolName] = &DynamicToolStats{Name: toolName}
	}
	entry := stats[toolName]
	entry.TotalCalls++
	if success {
		entry.SuccessfulCalls++
	} else {
		entry.FailedCalls++
	}
	entry.TotalLatency += latency
	if entry.TotalCalls == 1 {
		entry.AvgCost = cost
	} else {
		entry.AvgCost = (entry.AvgCost*float64(entry.TotalCalls-1) + cost) / float64(entry.TotalCalls)
	}
}
