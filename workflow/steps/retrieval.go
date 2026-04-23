package steps

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	rag "github.com/BaSui01/agentflow/rag/runtime"
	"github.com/BaSui01/agentflow/types"
	"github.com/BaSui01/agentflow/workflow/core"
)

// HybridRetriever abstracts hybrid retrieval execution.
// Uses types.RetrievalRecord to avoid coupling to rag layer implementation.
type HybridRetriever interface {
	Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]types.RetrievalRecord, error)
}

// MultiHopReasoner abstracts multi-hop retrieval reasoning.
// Note: ReasoningChain is rag-layer specific and kept as-is.
type MultiHopReasoner interface {
	Reason(ctx context.Context, query string) (*rag.ReasoningChain, error)
}

// RetrievalReranker abstracts retrieval rerank behavior.
// Uses types.RetrievalRecord to avoid coupling to rag layer implementation.
type RetrievalReranker interface {
	Rerank(ctx context.Context, query string, results []types.RetrievalRecord) ([]types.RetrievalRecord, error)
}

// HybridRetrieveStep executes a hybrid retrieval node in DAG.
type HybridRetrieveStep struct {
	id        string
	Query     string
	Retriever HybridRetriever
}

func NewHybridRetrieveStep(id string, retriever HybridRetriever) *HybridRetrieveStep {
	return &HybridRetrieveStep{id: id, Retriever: retriever}
}

func (s *HybridRetrieveStep) ID() string          { return s.id }
func (s *HybridRetrieveStep) Type() core.StepType { return core.StepTypeHybridRetrieve }

func (s *HybridRetrieveStep) Validate() error {
	if s.Retriever == nil {
		return core.NewStepError(s.id, core.StepTypeHybridRetrieve, core.ErrStepNotConfigured)
	}
	return nil
}

func (s *HybridRetrieveStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Retriever == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeHybridRetrieve, core.ErrStepNotConfigured)
	}
	query := resolveQuery(s.Query, input.Data)
	if query == "" {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeHybridRetrieve, fmt.Errorf("%w: query is empty", core.ErrStepValidation))
	}

	start := time.Now()
	results, err := s.Retriever.Retrieve(ctx, query, nil)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeHybridRetrieve, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}
	return core.StepOutput{
		Data: map[string]any{
			"query":   query,
			"results": results,
		},
		Latency: time.Since(start),
	}, nil
}

// MultiHopRetrieveStep executes multi-hop retrieval and flattens hop results.
type MultiHopRetrieveStep struct {
	id       string
	Query    string
	Reasoner MultiHopReasoner
}

func NewMultiHopRetrieveStep(id string, reasoner MultiHopReasoner) *MultiHopRetrieveStep {
	return &MultiHopRetrieveStep{id: id, Reasoner: reasoner}
}

func (s *MultiHopRetrieveStep) ID() string          { return s.id }
func (s *MultiHopRetrieveStep) Type() core.StepType { return core.StepTypeMultiHopRetrieve }

func (s *MultiHopRetrieveStep) Validate() error {
	if s.Reasoner == nil {
		return core.NewStepError(s.id, core.StepTypeMultiHopRetrieve, core.ErrStepNotConfigured)
	}
	return nil
}

func (s *MultiHopRetrieveStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Reasoner == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeMultiHopRetrieve, core.ErrStepNotConfigured)
	}
	query := resolveQuery(s.Query, input.Data)
	if query == "" {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeMultiHopRetrieve, fmt.Errorf("%w: query is empty", core.ErrStepValidation))
	}

	start := time.Now()
	chain, err := s.Reasoner.Reason(ctx, query)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeMultiHopRetrieve, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}
	results := flattenReasoningChain(chain)
	return core.StepOutput{
		Data: map[string]any{
			"query":   query,
			"chain":   chain,
			"results": results,
		},
		Latency: time.Since(start),
	}, nil
}

// RerankStep re-orders retrieval results in DAG.
type RerankStep struct {
	id       string
	Query    string
	Reranker RetrievalReranker
}

func NewRerankStep(id string, reranker RetrievalReranker) *RerankStep {
	return &RerankStep{id: id, Reranker: reranker}
}

func (s *RerankStep) ID() string          { return s.id }
func (s *RerankStep) Type() core.StepType { return core.StepTypeRerank }

func (s *RerankStep) Validate() error {
	if s.Reranker == nil {
		return core.NewStepError(s.id, core.StepTypeRerank, core.ErrStepNotConfigured)
	}
	return nil
}

func (s *RerankStep) Execute(ctx context.Context, input core.StepInput) (core.StepOutput, error) {
	if s.Reranker == nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeRerank, core.ErrStepNotConfigured)
	}
	query := resolveQuery(s.Query, input.Data)
	if query == "" {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeRerank, fmt.Errorf("%w: query is empty", core.ErrStepValidation))
	}
	rawResults, ok := input.Data["results"].([]types.RetrievalRecord)
	if !ok {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeRerank, fmt.Errorf("%w: results is not []types.RetrievalRecord", core.ErrStepValidation))
	}

	start := time.Now()
	out, err := s.Reranker.Rerank(ctx, query, rawResults)
	if err != nil {
		return core.StepOutput{}, core.NewStepError(s.id, core.StepTypeRerank, fmt.Errorf("%w: %w", core.ErrStepExecution, err))
	}
	return core.StepOutput{
		Data: map[string]any{
			"query":   query,
			"results": out,
		},
		Latency: time.Since(start),
	}, nil
}

func resolveQuery(defaultQuery string, data map[string]any) string {
	if q := strings.TrimSpace(defaultQuery); q != "" {
		return q
	}
	if data == nil {
		return ""
	}
	if q, ok := data["query"].(string); ok {
		return strings.TrimSpace(q)
	}
	return ""
}

func flattenReasoningChain(chain *rag.ReasoningChain) []types.RetrievalRecord {
	if chain == nil {
		return nil
	}
	merged := make(map[string]types.RetrievalRecord)
	for _, hop := range chain.Hops {
		for _, item := range hop.Results {
			key := strings.TrimSpace(item.Document.ID)
			if key == "" {
				key = strings.TrimSpace(item.Document.Content)
			}
			if key == "" {
				continue
			}
			// Convert rag.RetrievalResult to types.RetrievalRecord
			record := convertRetrievalResultToRecord(item)
			if current, ok := merged[key]; ok {
				if record.Score > current.Score {
					merged[key] = record
				}
				continue
			}
			merged[key] = record
		}
	}
	out := make([]types.RetrievalRecord, 0, len(merged))
	for _, v := range merged {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}

// convertRetrievalResultToRecord converts rag.RetrievalResult to types.RetrievalRecord.
func convertRetrievalResultToRecord(r rag.RetrievalResult) types.RetrievalRecord {
	source := ""
	if r.Document.Metadata != nil {
		if v, ok := r.Document.Metadata["source"].(string); ok {
			source = v
		}
	}
	return types.RetrievalRecord{
		DocID:   r.Document.ID,
		Content: r.Document.Content,
		Source:  source,
		Score:   r.FinalScore,
	}
}
