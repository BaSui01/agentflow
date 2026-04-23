package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	rag "github.com/BaSui01/agentflow/rag/runtime"
)

// StrategyKind identifies retrieval strategy nodes mounted into unified pipeline.
type StrategyKind string

const (
	StrategyHybrid     StrategyKind = "hybrid"
	StrategyBM25       StrategyKind = "bm25"
	StrategyVector     StrategyKind = "vector"
	StrategyGraph      StrategyKind = "graph"
	StrategyContextual StrategyKind = "contextual"
	StrategyMultiHop   StrategyKind = "multi_hop"
)

// MultiHopReasoner is the minimal contract required by multi-hop strategy node.
type MultiHopReasoner interface {
	Reason(ctx context.Context, query string) (*rag.ReasoningChain, error)
}

// GraphRetriever is the minimal contract required by graph strategy node.
type GraphRetriever interface {
	Retrieve(ctx context.Context, query string) ([]rag.GraphRetrievalResult, error)
}

// StrategyNodes groups available strategy implementations.
type StrategyNodes struct {
	Hybrid     *rag.HybridRetriever
	BM25       *rag.HybridRetriever
	Vector     *rag.HybridRetriever
	Graph      GraphRetriever
	Contextual *rag.ContextualRetrieval
	MultiHop   MultiHopReasoner
}

// NewStrategyNode builds a retriever strategy node for unified pipeline.
func NewStrategyNode(kind StrategyKind, nodes StrategyNodes) (Retriever, error) {
	switch kind {
	case StrategyHybrid:
		if nodes.Hybrid == nil {
			return nil, fmt.Errorf("hybrid strategy retriever is not configured")
		}
		return &hybridStrategyNode{retriever: nodes.Hybrid}, nil
	case StrategyBM25:
		if nodes.BM25 == nil {
			return nil, fmt.Errorf("bm25 strategy retriever is not configured")
		}
		return &bm25StrategyNode{retriever: nodes.BM25}, nil
	case StrategyVector:
		if nodes.Vector == nil {
			return nil, fmt.Errorf("vector strategy retriever is not configured")
		}
		return &vectorStrategyNode{retriever: nodes.Vector}, nil
	case StrategyGraph:
		if nodes.Graph == nil {
			return nil, fmt.Errorf("graph strategy retriever is not configured")
		}
		return &graphStrategyNode{retriever: nodes.Graph}, nil
	case StrategyContextual:
		if nodes.Contextual == nil {
			return nil, fmt.Errorf("contextual strategy retriever is not configured")
		}
		return &contextualStrategyNode{retriever: nodes.Contextual}, nil
	case StrategyMultiHop:
		if nodes.MultiHop == nil {
			return nil, fmt.Errorf("multi-hop strategy reasoner is not configured")
		}
		return &multiHopStrategyNode{reasoner: nodes.MultiHop}, nil
	default:
		return nil, fmt.Errorf("unsupported retrieval strategy: %s", kind)
	}
}

type hybridStrategyNode struct {
	retriever *rag.HybridRetriever
}

func (n *hybridStrategyNode) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	return n.retriever.Retrieve(ctx, query, queryEmbedding)
}

type bm25StrategyNode struct {
	retriever *rag.HybridRetriever
}

func (n *bm25StrategyNode) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	return n.retriever.Retrieve(ctx, query, queryEmbedding)
}

type vectorStrategyNode struct {
	retriever *rag.HybridRetriever
}

func (n *vectorStrategyNode) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	return n.retriever.Retrieve(ctx, query, queryEmbedding)
}

type graphStrategyNode struct {
	retriever GraphRetriever
}

func (n *graphStrategyNode) Retrieve(ctx context.Context, query string, _ []float64) ([]rag.RetrievalResult, error) {
	graphResults, err := n.retriever.Retrieve(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]rag.RetrievalResult, 0, len(graphResults))
	for i := range graphResults {
		out = append(out, rag.RetrievalResult{
			Document: rag.Document{
				ID:       graphResults[i].ID,
				Content:  graphResults[i].Content,
				Metadata: graphResults[i].Metadata,
			},
			VectorScore: graphResults[i].VectorScore,
			FinalScore:  graphResults[i].Score,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FinalScore > out[j].FinalScore
	})
	return out, nil
}

type contextualStrategyNode struct {
	retriever *rag.ContextualRetrieval
}

func (n *contextualStrategyNode) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	return n.retriever.Retrieve(ctx, query, queryEmbedding)
}

type multiHopStrategyNode struct {
	reasoner MultiHopReasoner
}

func (n *multiHopStrategyNode) Retrieve(ctx context.Context, query string, _ []float64) ([]rag.RetrievalResult, error) {
	chain, err := n.reasoner.Reason(ctx, query)
	if err != nil {
		return nil, err
	}
	return aggregateReasoningResults(chain), nil
}

func aggregateReasoningResults(chain *rag.ReasoningChain) []rag.RetrievalResult {
	if chain == nil {
		return nil
	}
	merged := make(map[string]rag.RetrievalResult)
	for _, hop := range chain.Hops {
		for _, item := range hop.Results {
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
	return out
}
