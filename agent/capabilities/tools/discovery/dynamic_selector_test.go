package discovery

import (
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
)

func TestDynamicToolSemanticSimilarityScoresNameAndDescription(t *testing.T) {
	tool := types.ToolSchema{
		Name:        "web_search",
		Description: "Search the web for current information",
	}

	score := DynamicToolSemanticSimilarity("search current web information", tool)

	assert.Greater(t, score, 0.5)
}

func TestDynamicToolStatsAndTotalScore(t *testing.T) {
	stats := map[string]*DynamicToolStats{}
	DynamicToolUpdateStats(stats, "search", true, 200*time.Millisecond, 0.05)
	DynamicToolUpdateStats(stats, "search", false, 800*time.Millisecond, 0.15)

	entry := stats["search"]
	assert.NotNil(t, entry)
	assert.EqualValues(t, 2, entry.TotalCalls)
	assert.EqualValues(t, 1, entry.SuccessfulCalls)
	assert.EqualValues(t, 1, entry.FailedCalls)
	assert.Equal(t, 500*time.Millisecond, DynamicToolAverageLatency(entry))
	assert.Equal(t, 0.5, DynamicToolReliability(entry))

	cfg := DefaultDynamicToolSelectionConfig()
	total := DynamicToolTotalScore(DynamicToolScore{
		SemanticSimilarity: 0.9,
		EstimatedCost:      entry.AvgCost,
		AvgLatency:         DynamicToolAverageLatency(entry),
		ReliabilityScore:   DynamicToolReliability(entry),
	}, cfg)
	assert.Greater(t, total, 0.0)
	assert.LessOrEqual(t, total, 1.0)
}

func TestDynamicToolParseIndicesRejectsNewlineListWithoutCommas(t *testing.T) {
	assert.Empty(t, DynamicToolParseIndices("1\n2\n3"))
	assert.Equal(t, []int{1, 2, 3}, DynamicToolParseIndices("1, 2,3"))
}
