package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// RAGService defines the use-case boundary for RAG handler.
type RAGService interface {
	Query(ctx context.Context, query string, topK int, opts RAGQueryOptions) (*RAGQueryResponse, error)
	Index(ctx context.Context, docs []core.Document) error
	SupportedStrategies() []string
}

type RAGQueryOptions struct {
	Strategy string
}

type RAGQueryResponse struct {
	Results           []core.VectorSearchResult
	RequestedStrategy string
	EffectiveStrategy string
}

type ragStrategyExecutor func(ctx context.Context, query string, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error)

type DefaultRAGService struct {
	store     core.VectorStore
	embedding core.EmbeddingProvider

	autoRouter *rag.QueryRouter
	executors  map[string]ragStrategyExecutor
	strategies []string

	hybridRetriever     *rag.HybridRetriever
	bm25Retriever       *rag.HybridRetriever
	contextualRetriever *rag.ContextualRetrieval
}

func NewDefaultRAGService(store core.VectorStore, embedding core.EmbeddingProvider) *DefaultRAGService {
	service := &DefaultRAGService{
		store:     store,
		embedding: embedding,
		executors: make(map[string]ragStrategyExecutor),
	}

	service.bootstrapExecutors()
	return service
}

func (s *DefaultRAGService) Query(ctx context.Context, query string, topK int, opts RAGQueryOptions) (*RAGQueryResponse, error) {
	queryEmbedding, err := s.embedding.EmbedQuery(ctx, query)
	if err != nil {
		return nil, types.NewError(types.ErrUpstreamError, "failed to generate query embedding").WithCause(err)
	}

	requestedStrategy := normalizeRAGStrategy(opts.Strategy)
	effectiveStrategy, err := s.resolveStrategy(ctx, query, requestedStrategy)
	if err != nil {
		return nil, err
	}

	executor, ok := s.executors[effectiveStrategy]
	if !ok {
		return nil, types.NewError(types.ErrInvalidRequest, fmt.Sprintf("unsupported strategy: %s", effectiveStrategy))
	}

	results, err := executor(ctx, query, queryEmbedding, topK)
	if err != nil {
		return nil, types.NewError(types.ErrInternalError, "rag query failed").WithCause(err)
	}

	return &RAGQueryResponse{
		Results:           results,
		RequestedStrategy: requestedStrategy,
		EffectiveStrategy: effectiveStrategy,
	}, nil
}

func (s *DefaultRAGService) Index(ctx context.Context, docs []core.Document) error {
	if len(docs) == 0 {
		return nil
	}
	contents := make([]string, len(docs))
	for i := range docs {
		contents[i] = docs[i].Content
	}

	embeddings, err := s.embedding.EmbedDocuments(ctx, contents)
	if err != nil {
		return types.NewError(types.ErrUpstreamError, "failed to generate embeddings").WithCause(err)
	}
	for i := range docs {
		docs[i].Embedding = embeddings[i]
	}

	if err := s.store.AddDocuments(ctx, docs); err != nil {
		return types.NewError(types.ErrInternalError, "failed to index documents").WithCause(err)
	}

	// Keep strategy-specific retrievers in sync with indexed documents.
	if s.hybridRetriever != nil {
		if err := s.hybridRetriever.IndexDocuments(docs); err != nil {
			return types.NewError(types.ErrInternalError, "failed to index documents for hybrid strategy").WithCause(err)
		}
	}
	if s.bm25Retriever != nil {
		if err := s.bm25Retriever.IndexDocuments(docs); err != nil {
			return types.NewError(types.ErrInternalError, "failed to index documents for bm25 strategy").WithCause(err)
		}
	}
	if s.contextualRetriever != nil {
		if err := s.contextualRetriever.IndexDocumentsWithContext(ctx, docs); err != nil {
			return types.NewError(types.ErrInternalError, "failed to index documents for contextual strategy").WithCause(err)
		}
	}

	return nil
}

func (s *DefaultRAGService) SupportedStrategies() []string {
	out := make([]string, 0, len(s.strategies)+1)
	out = append(out, ragStrategyAuto)
	out = append(out, s.strategies...)
	return out
}

const (
	ragStrategyAuto       = "auto"
	ragStrategyVector     = "vector"
	ragStrategyBM25       = "bm25"
	ragStrategyHybrid     = "hybrid"
	ragStrategyContextual = "contextual"
)

func (s *DefaultRAGService) bootstrapExecutors() {
	if s.store != nil {
		s.executors[ragStrategyVector] = func(ctx context.Context, _ string, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error) {
			return s.store.Search(ctx, queryEmbedding, topK)
		}
	}

	hybridConfig := rag.DefaultHybridRetrievalConfig()
	hybridConfig.TopK = 128
	hybridConfig.MinScore = -1
	hybridConfig.UseReranking = false
	s.hybridRetriever = rag.NewHybridRetriever(hybridConfig, zap.NewNop())

	bm25Config := hybridConfig
	bm25Config.UseVector = false
	bm25Config.UseBM25 = true
	s.bm25Retriever = rag.NewHybridRetriever(bm25Config, zap.NewNop())

	contextualBaseConfig := hybridConfig
	contextualBase := rag.NewHybridRetriever(contextualBaseConfig, zap.NewNop())
	contextualConfig := rag.DefaultContextualRetrievalConfig()
	contextualConfig.UseContextPrefix = false
	s.contextualRetriever = rag.NewContextualRetrieval(
		contextualBase,
		rag.NewSimpleContextProvider(zap.NewNop()),
		contextualConfig,
		zap.NewNop(),
	)

	s.executors[ragStrategyHybrid] = func(ctx context.Context, query string, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error) {
		results, err := s.hybridRetriever.Retrieve(ctx, query, queryEmbedding)
		if err != nil {
			return nil, err
		}
		return convertRetrievalResults(results, topK), nil
	}
	s.executors[ragStrategyBM25] = func(ctx context.Context, query string, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error) {
		results, err := s.bm25Retriever.Retrieve(ctx, query, queryEmbedding)
		if err != nil {
			return nil, err
		}
		return convertRetrievalResults(results, topK), nil
	}
	s.executors[ragStrategyContextual] = func(ctx context.Context, query string, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error) {
		results, err := s.contextualRetriever.Retrieve(ctx, query, queryEmbedding)
		if err != nil {
			return nil, err
		}
		return convertRetrievalResults(results, topK), nil
	}

	s.strategies = make([]string, 0, len(s.executors))
	for strategy := range s.executors {
		s.strategies = append(s.strategies, strategy)
	}
	sort.Strings(s.strategies)

	routerConfig := rag.DefaultQueryRouterConfig()
	routerConfig.EnableLLMRouting = false
	routerConfig.EnableAdaptiveRouting = false
	routerConfig.DefaultStrategy = rag.RetrievalStrategy(s.defaultStrategy())
	routerConfig.Strategies = []rag.StrategyConfig{
		{Strategy: rag.StrategyVector, Enabled: hasStrategy(s.executors, ragStrategyVector), Weight: 1.1, MinScore: 0},
		{Strategy: rag.StrategyBM25, Enabled: hasStrategy(s.executors, ragStrategyBM25), Weight: 0.9, MinScore: 0},
		{Strategy: rag.StrategyHybrid, Enabled: hasStrategy(s.executors, ragStrategyHybrid), Weight: 1.0, MinScore: 0},
		{Strategy: rag.StrategyContextual, Enabled: hasStrategy(s.executors, ragStrategyContextual), Weight: 0.8, MinScore: 0},
	}
	s.autoRouter = rag.NewQueryRouter(routerConfig, nil, nil, zap.NewNop())
}

func (s *DefaultRAGService) resolveStrategy(ctx context.Context, query string, requested string) (string, error) {
	if requested == ragStrategyAuto {
		if s.autoRouter == nil {
			return s.defaultStrategy(), nil
		}
		decision, err := s.autoRouter.Route(ctx, query)
		if err != nil {
			return "", types.NewError(types.ErrInternalError, "failed to route rag strategy").WithCause(err)
		}
		candidate := normalizeRAGStrategy(string(decision.SelectedStrategy))
		if hasStrategy(s.executors, candidate) {
			return candidate, nil
		}
		return s.defaultStrategy(), nil
	}

	if !hasStrategy(s.executors, requested) {
		return "", types.NewError(types.ErrInvalidRequest, fmt.Sprintf("unsupported rag strategy: %s", requested))
	}
	return requested, nil
}

func normalizeRAGStrategy(strategy string) string {
	normalized := strings.ToLower(strings.TrimSpace(strategy))
	switch normalized {
	case "", ragStrategyAuto:
		return ragStrategyAuto
	case ragStrategyVector, "dense":
		return ragStrategyVector
	case ragStrategyBM25, "sparse":
		return ragStrategyBM25
	case ragStrategyHybrid:
		return ragStrategyHybrid
	case ragStrategyContextual:
		return ragStrategyContextual
	default:
		return normalized
	}
}

func convertRetrievalResults(results []rag.RetrievalResult, topK int) []rag.VectorSearchResult {
	if topK <= 0 {
		topK = 5
	}
	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]rag.VectorSearchResult, 0, len(results))
	for i := range results {
		score := results[i].FinalScore
		if score == 0 {
			score = results[i].HybridScore
		}
		if score == 0 {
			score = results[i].VectorScore
		}
		out = append(out, rag.VectorSearchResult{
			Document: results[i].Document,
			Score:    score,
		})
	}
	return out
}

func hasStrategy(executors map[string]ragStrategyExecutor, strategy string) bool {
	_, ok := executors[strategy]
	return ok
}

func (s *DefaultRAGService) defaultStrategy() string {
	candidates := []string{ragStrategyVector, ragStrategyHybrid, ragStrategyBM25, ragStrategyContextual}
	for _, strategy := range candidates {
		if hasStrategy(s.executors, strategy) {
			return strategy
		}
	}
	return ragStrategyVector
}
