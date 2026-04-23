package retrieval

import (
	"context"
	"errors"
	"strings"
	"testing"

	rag "github.com/BaSui01/agentflow/rag/runtime"
)

type stubTransformer struct {
	out string
	err error
}

func (s stubTransformer) Transform(context.Context, string) (string, error) {
	return s.out, s.err
}

type stubRetriever struct {
	results []rag.RetrievalResult
	err     error
	seenQ   string
}

func (s *stubRetriever) Retrieve(_ context.Context, q string, _ []float64) ([]rag.RetrievalResult, error) {
	s.seenQ = q
	return s.results, s.err
}

type stubReranker struct {
	results []rag.RetrievalResult
	err     error
	seenQ   string
}

func (s *stubReranker) Rerank(_ context.Context, q string, _ []rag.RetrievalResult) ([]rag.RetrievalResult, error) {
	s.seenQ = q
	return s.results, s.err
}

type stubComposer struct {
	out   string
	err   error
	seenQ string
}

func (s *stubComposer) Compose(_ context.Context, q string, _ []rag.RetrievalResult) (string, error) {
	s.seenQ = q
	return s.out, s.err
}

func mkResults(contents ...string) []rag.RetrievalResult {
	out := make([]rag.RetrievalResult, 0, len(contents))
	for i, c := range contents {
		out = append(out, rag.RetrievalResult{
			Document: rag.Document{
				ID:      string(rune('a' + i)),
				Content: c,
			},
		})
	}
	return out
}

func TestPipelineExecute_OrderAndTopK(t *testing.T) {
	retriever := &stubRetriever{results: mkResults("d1", "d2", "d3", "d4")}
	reranker := &stubReranker{results: mkResults("r1", "r2", "r3")}

	p := NewPipeline(
		PipelineConfig{RetrieveTopK: 3, RerankTopK: 2},
		stubTransformer{out: "transformed"},
		retriever,
		reranker,
		nil,
	)

	out, err := p.Execute(context.Background(), PipelineInput{Query: "original"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retriever.seenQ != "transformed" {
		t.Fatalf("expected retriever query transformed, got %q", retriever.seenQ)
	}
	if reranker.seenQ != "transformed" {
		t.Fatalf("expected reranker query transformed, got %q", reranker.seenQ)
	}
	if out.TransformedQuery != "transformed" {
		t.Fatalf("unexpected transformed query: %q", out.TransformedQuery)
	}
	if len(out.Results) != 2 {
		t.Fatalf("expected rerank topK 2, got %d", len(out.Results))
	}
	if !strings.Contains(out.Context, "r1") || !strings.Contains(out.Context, "r2") {
		t.Fatalf("default context should contain reranked docs, got %q", out.Context)
	}
}

func TestPipelineExecute_CustomComposer(t *testing.T) {
	retriever := &stubRetriever{results: mkResults("a", "b")}
	composer := &stubComposer{out: "custom-context"}

	p := NewPipeline(DefaultPipelineConfig(), nil, retriever, nil, composer)
	out, err := p.Execute(context.Background(), PipelineInput{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Context != "custom-context" {
		t.Fatalf("expected custom context, got %q", out.Context)
	}
	if composer.seenQ != "q" {
		t.Fatalf("expected composer seen query q, got %q", composer.seenQ)
	}
}

func TestPipelineExecute_Errors(t *testing.T) {
	tests := []struct {
		name string
		p    *Pipeline
		in   PipelineInput
	}{
		{
			name: "nil retriever",
			p:    NewPipeline(DefaultPipelineConfig(), nil, nil, nil, nil),
			in:   PipelineInput{Query: "q"},
		},
		{
			name: "empty query",
			p:    NewPipeline(DefaultPipelineConfig(), nil, &stubRetriever{}, nil, nil),
			in:   PipelineInput{Query: "  "},
		},
		{
			name: "transform error",
			p: NewPipeline(
				DefaultPipelineConfig(),
				stubTransformer{err: errors.New("xform")},
				&stubRetriever{},
				nil,
				nil,
			),
			in: PipelineInput{Query: "q"},
		},
		{
			name: "retrieve error",
			p: NewPipeline(
				DefaultPipelineConfig(),
				nil,
				&stubRetriever{err: errors.New("retrieve")},
				nil,
				nil,
			),
			in: PipelineInput{Query: "q"},
		},
		{
			name: "rerank error",
			p: NewPipeline(
				DefaultPipelineConfig(),
				nil,
				&stubRetriever{results: mkResults("a")},
				&stubReranker{err: errors.New("rerank")},
				nil,
			),
			in: PipelineInput{Query: "q"},
		},
		{
			name: "compose error",
			p: NewPipeline(
				DefaultPipelineConfig(),
				nil,
				&stubRetriever{results: mkResults("a")},
				nil,
				&stubComposer{err: errors.New("compose")},
			),
			in: PipelineInput{Query: "q"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.p.Execute(context.Background(), tt.in)
			if err == nil {
				t.Fatalf("expected error for case %q", tt.name)
			}
		})
	}
}
