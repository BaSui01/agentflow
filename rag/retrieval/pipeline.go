package retrieval

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/rag"
)

// QueryTransformer transforms the incoming query before retrieval.
type QueryTransformer interface {
	Transform(ctx context.Context, query string) (string, error)
}

// Retriever fetches candidates for the transformed query.
type Retriever interface {
	Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error)
}

// Reranker re-orders retrieval candidates.
type Reranker interface {
	Rerank(ctx context.Context, query string, results []rag.RetrievalResult) ([]rag.RetrievalResult, error)
}

// Composer builds context text from final candidates.
type Composer interface {
	Compose(ctx context.Context, query string, results []rag.RetrievalResult) (string, error)
}

// PipelineInput is the request payload for retrieval pipeline execution.
type PipelineInput struct {
	Query          string
	QueryEmbedding []float64
}

// PipelineOutput is the normalized output of the retrieval pipeline.
type PipelineOutput struct {
	TransformedQuery string
	Results          []rag.RetrievalResult
	Context          string
}

// PipelineConfig controls candidate limits for retrieval and rerank phases.
type PipelineConfig struct {
	RetrieveTopK int
	RerankTopK   int
}

// DefaultPipelineConfig returns conservative defaults for production flow.
func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		RetrieveTopK: 100,
		RerankTopK:   10,
	}
}

// Pipeline implements unified execution path:
// query transform -> retrieve -> rerank(optional) -> compose context.
type Pipeline struct {
	config      PipelineConfig
	transformer QueryTransformer
	retriever   Retriever
	reranker    Reranker
	composer    Composer
}

// NewPipeline creates a pipeline with required retriever and optional stages.
func NewPipeline(
	config PipelineConfig,
	transformer QueryTransformer,
	retriever Retriever,
	reranker Reranker,
	composer Composer,
) *Pipeline {
	if config.RetrieveTopK <= 0 {
		config.RetrieveTopK = DefaultPipelineConfig().RetrieveTopK
	}
	if config.RerankTopK <= 0 {
		config.RerankTopK = DefaultPipelineConfig().RerankTopK
	}
	return &Pipeline{
		config:      config,
		transformer: transformer,
		retriever:   retriever,
		reranker:    reranker,
		composer:    composer,
	}
}

// Execute runs the unified retrieval pipeline.
func (p *Pipeline) Execute(ctx context.Context, in PipelineInput) (*PipelineOutput, error) {
	if p == nil || p.retriever == nil {
		return nil, fmt.Errorf("retrieval pipeline retriever is not configured")
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, fmt.Errorf("query is empty")
	}

	query := in.Query
	if p.transformer != nil {
		transformed, err := p.transformer.Transform(ctx, in.Query)
		if err != nil {
			return nil, fmt.Errorf("transform query: %w", err)
		}
		if strings.TrimSpace(transformed) != "" {
			query = transformed
		}
	}

	results, err := p.retriever.Retrieve(ctx, query, in.QueryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}
	results = clipTopK(results, p.config.RetrieveTopK)

	if p.reranker != nil && len(results) > 0 {
		reranked, rerankErr := p.reranker.Rerank(ctx, query, results)
		if rerankErr != nil {
			return nil, fmt.Errorf("rerank: %w", rerankErr)
		}
		results = clipTopK(reranked, p.config.RerankTopK)
	}

	contextText := defaultCompose(results)
	if p.composer != nil {
		composed, composeErr := p.composer.Compose(ctx, query, results)
		if composeErr != nil {
			return nil, fmt.Errorf("compose context: %w", composeErr)
		}
		contextText = composed
	}

	return &PipelineOutput{
		TransformedQuery: query,
		Results:          results,
		Context:          contextText,
	}, nil
}

func clipTopK(results []rag.RetrievalResult, topK int) []rag.RetrievalResult {
	if topK <= 0 || len(results) <= topK {
		return results
	}
	return results[:topK]
}

func defaultCompose(results []rag.RetrievalResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(results[i].Document.Content)
	}
	return b.String()
}
