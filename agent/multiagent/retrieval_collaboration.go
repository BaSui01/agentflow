package multiagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/rag"
	"go.uber.org/zap"
)

// QueryDecomposer splits a query into sub-queries for parallel retrieval.
type QueryDecomposer interface {
	Decompose(ctx context.Context, query string) ([]string, error)
}

// RetrievalWorker executes retrieval for a sub-query.
type RetrievalWorker interface {
	Retrieve(ctx context.Context, query string) ([]rag.RetrievalResult, error)
}

// RetrievalResultAggregator merges worker retrieval outputs.
type RetrievalResultAggregator interface {
	Aggregate(ctx context.Context, resultsByQuery map[string][]rag.RetrievalResult) ([]rag.RetrievalResult, error)
}

// RetrievalSupervisor orchestrates query decomposition -> parallel worker retrieval -> dedup aggregation.
type RetrievalSupervisor struct {
	decomposer QueryDecomposer
	workers    []RetrievalWorker
	aggregator RetrievalResultAggregator
	logger     *zap.Logger
}

// NewRetrievalSupervisor creates a retrieval collaboration supervisor.
func NewRetrievalSupervisor(
	decomposer QueryDecomposer,
	workers []RetrievalWorker,
	aggregator RetrievalResultAggregator,
	logger *zap.Logger,
) *RetrievalSupervisor {
	if logger == nil {
		logger = zap.NewNop()
	}
	if aggregator == nil {
		aggregator = NewDedupResultAggregator()
	}
	return &RetrievalSupervisor{
		decomposer: decomposer,
		workers:    workers,
		aggregator: aggregator,
		logger:     logger.With(zap.String("component", "retrieval_supervisor")),
	}
}

// Retrieve runs the collaboration pipeline.
func (s *RetrievalSupervisor) Retrieve(ctx context.Context, query string) ([]rag.RetrievalResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	if s.decomposer == nil {
		return nil, fmt.Errorf("query decomposer is nil")
	}
	if len(s.workers) == 0 {
		return nil, fmt.Errorf("retrieval workers are empty")
	}

	subQueries, err := s.decomposer.Decompose(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("decompose query: %w", err)
	}
	if len(subQueries) == 0 {
		subQueries = []string{query}
	}

	resultsByQuery := make(map[string][]rag.RetrievalResult, len(subQueries))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, len(subQueries))

	for i, sq := range subQueries {
		worker := s.workers[i%len(s.workers)]
		subQuery := strings.TrimSpace(sq)
		if subQuery == "" {
			continue
		}
		wg.Add(1)
		go func(q string, w RetrievalWorker) {
			defer wg.Done()
			out, retrieveErr := w.Retrieve(ctx, q)
			if retrieveErr != nil {
				errCh <- fmt.Errorf("worker retrieve %q: %w", q, retrieveErr)
				return
			}
			mu.Lock()
			resultsByQuery[q] = out
			mu.Unlock()
		}(subQuery, worker)
	}

	wg.Wait()
	close(errCh)

	for e := range errCh {
		if e != nil {
			return nil, e
		}
	}

	merged, err := s.aggregator.Aggregate(ctx, resultsByQuery)
	if err != nil {
		return nil, fmt.Errorf("aggregate retrieval results: %w", err)
	}
	return merged, nil
}

// DedupResultAggregator merges retrieval results and keeps highest-score doc per key.
type DedupResultAggregator struct{}

// NewDedupResultAggregator creates default dedup aggregator.
func NewDedupResultAggregator() *DedupResultAggregator { return &DedupResultAggregator{} }

// Aggregate merges and deduplicates by document ID (fallback: content).
func (a *DedupResultAggregator) Aggregate(_ context.Context, resultsByQuery map[string][]rag.RetrievalResult) ([]rag.RetrievalResult, error) {
	merged := make(map[string]rag.RetrievalResult)
	for _, results := range resultsByQuery {
		for _, item := range results {
			key := strings.TrimSpace(item.Document.ID)
			if key == "" {
				key = strings.TrimSpace(item.Document.Content)
			}
			if key == "" {
				continue
			}
			if current, ok := merged[key]; ok {
				if item.FinalScore > current.FinalScore {
					merged[key] = item
				}
				continue
			}
			merged[key] = item
		}
	}

	out := make([]rag.RetrievalResult, 0, len(merged))
	for _, v := range merged {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FinalScore > out[j].FinalScore
	})
	return out, nil
}
