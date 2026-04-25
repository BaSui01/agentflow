package multiagent

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// AggregationStrategy defines how sub-agent results are combined.
type AggregationStrategy string

const (
	StrategyMergeAll      AggregationStrategy = "merge_all"
	StrategyBestOfN       AggregationStrategy = "best_of_n"
	StrategyVoteMajority  AggregationStrategy = "vote_majority"
	StrategyWeightedMerge AggregationStrategy = "weighted_merge"
)

// WorkerResult holds the output of a single sub-agent execution.
type WorkerResult struct {
	AgentID      string
	Content      string
	TokensUsed   int
	Cost         float64
	Duration     time.Duration
	Score        float64 // confidence / quality score (used by BestOfN)
	Weight       float64 // agent weight (used by WeightedMerge)
	Err          error
	FinishReason string
	Metadata     map[string]any
}

// AggregatedResult is the final merged output from all sub-agents.
type AggregatedResult struct {
	Content      string
	TokensUsed   int
	Cost         float64
	Duration     time.Duration
	SourceCount  int
	FailedCount  int
	Metadata     map[string]any
	FinishReason string
}

// Aggregator merges multiple WorkerResult into a single AggregatedResult.
type Aggregator struct {
	strategy AggregationStrategy
}

// NewAggregator creates an Aggregator with the given strategy.
func NewAggregator(strategy AggregationStrategy) *Aggregator {
	return &Aggregator{strategy: strategy}
}

// Aggregate applies the configured strategy to the results.
func (a *Aggregator) Aggregate(results []WorkerResult) (*AggregatedResult, error) {
	successful, failed := partitionResults(results)
	if len(successful) == 0 {
		return nil, fmt.Errorf("all %d sub-agents failed", len(results))
	}

	var agg *AggregatedResult
	var err error

	switch a.strategy {
	case StrategyMergeAll:
		agg = mergeAll(successful)
	case StrategyBestOfN:
		agg = bestOfN(successful)
	case StrategyVoteMajority:
		agg = voteMajority(successful)
	case StrategyWeightedMerge:
		agg = weightedMerge(successful)
	default:
		agg = mergeAll(successful)
	}

	if err != nil {
		return nil, err
	}

	agg.SourceCount = len(successful)
	agg.FailedCount = len(failed)
	return agg, nil
}

// --- strategy implementations ---

func partitionResults(results []WorkerResult) (successful, failed []WorkerResult) {
	for _, r := range results {
		if r.Err != nil {
			failed = append(failed, r)
		} else {
			successful = append(successful, r)
		}
	}
	return
}

// mergeAll concatenates all results in order.
func mergeAll(results []WorkerResult) *AggregatedResult {
	agg := &AggregatedResult{Metadata: map[string]any{}}
	var sb strings.Builder
	var maxDur time.Duration
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(r.Content)
		agg.TokensUsed += r.TokensUsed
		agg.Cost += r.Cost
		if r.Duration > maxDur {
			maxDur = r.Duration
		}
	}
	agg.Content = sb.String()
	agg.Duration = maxDur
	agg.FinishReason = "merge_all"
	return agg
}

// bestOfN picks the result with the highest Score.
func bestOfN(results []WorkerResult) *AggregatedResult {
	best := results[0]
	for _, r := range results[1:] {
		if r.Score > best.Score {
			best = r
		}
	}
	return &AggregatedResult{
		Content:      best.Content,
		TokensUsed:   best.TokensUsed,
		Cost:         best.Cost,
		Duration:     best.Duration,
		FinishReason: "best_of_n",
		Metadata: map[string]any{
			"selected_agent": best.AgentID,
			"selected_score": best.Score,
		},
	}
}

// voteMajority selects the most common content (simple majority).
func voteMajority(results []WorkerResult) *AggregatedResult {
	votes := map[string]int{}
	for _, r := range results {
		normalized := strings.TrimSpace(r.Content)
		votes[normalized]++
	}

	type candidate struct {
		content string
		count   int
	}
	candidates := make([]candidate, 0, len(votes))
	for c, n := range votes {
		candidates = append(candidates, candidate{c, n})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count > candidates[j].count
		}
		return candidates[i].content < candidates[j].content // 平局时按字典序确定性排序
	})

	winner := candidates[0].content
	agg := &AggregatedResult{
		Content:      winner,
		FinishReason: "vote_majority",
		Metadata: map[string]any{
			"vote_count":  candidates[0].count,
			"total_votes": len(results),
		},
	}
	var maxDur time.Duration
	for _, r := range results {
		agg.TokensUsed += r.TokensUsed
		agg.Cost += r.Cost
		if r.Duration > maxDur {
			maxDur = r.Duration
		}
	}
	agg.Duration = maxDur
	return agg
}

// weightedMerge combines results weighted by each agent's Weight.
func weightedMerge(results []WorkerResult) *AggregatedResult {
	var totalWeight float64
	for _, r := range results {
		totalWeight += r.Weight
	}
	if totalWeight == 0 {
		return mergeAll(results)
	}

	var sb strings.Builder
	var maxDur time.Duration
	agg := &AggregatedResult{Metadata: map[string]any{}}
	for i, r := range results {
		w := r.Weight / totalWeight
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("[weight=%.2f] %s", w, r.Content))
		agg.TokensUsed += r.TokensUsed
		agg.Cost += r.Cost
		if r.Duration > maxDur {
			maxDur = r.Duration
		}
	}
	agg.Content = sb.String()
	agg.Duration = maxDur
	agg.FinishReason = "weighted_merge"
	return agg
}
