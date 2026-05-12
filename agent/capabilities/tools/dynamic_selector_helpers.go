package tools

import (
	"time"

	tooldiscovery "github.com/BaSui01/agentflow/agent/capabilities/tools/discovery"
	"github.com/BaSui01/agentflow/types"
)

type DynamicToolScore = tooldiscovery.DynamicToolScore
type DynamicToolSelectionConfig = tooldiscovery.DynamicToolSelectionConfig
type DynamicToolStats = tooldiscovery.DynamicToolStats

func DefaultDynamicToolSelectionConfig() DynamicToolSelectionConfig {
	return tooldiscovery.DefaultDynamicToolSelectionConfig()
}

func DynamicToolSemanticSimilarity(task string, tool types.ToolSchema) float64 {
	return tooldiscovery.DynamicToolSemanticSimilarity(task, tool)
}

func DynamicToolEstimateCost(tool types.ToolSchema) float64 {
	return tooldiscovery.DynamicToolEstimateCost(tool)
}

func DynamicToolAverageLatency(stats *DynamicToolStats) time.Duration {
	return tooldiscovery.DynamicToolAverageLatency(stats)
}

func DynamicToolReliability(stats *DynamicToolStats) float64 {
	return tooldiscovery.DynamicToolReliability(stats)
}

func DynamicToolTotalScore(score DynamicToolScore, cfg DynamicToolSelectionConfig) float64 {
	return tooldiscovery.DynamicToolTotalScore(score, cfg)
}

func DynamicToolExtractKeywords(text string) []string {
	return tooldiscovery.DynamicToolExtractKeywords(text)
}

func DynamicToolParseIndices(text string) []int {
	return tooldiscovery.DynamicToolParseIndices(text)
}

func DynamicToolUpdateStats(stats map[string]*DynamicToolStats, toolName string, success bool, latency time.Duration, cost float64) {
	tooldiscovery.DynamicToolUpdateStats(stats, toolName, success, latency, cost)
}
