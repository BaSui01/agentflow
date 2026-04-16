package multiagent

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/rag"
)

type stubDecomposer struct {
	parts []string
	err   error
}

func (s stubDecomposer) Decompose(context.Context, string) ([]string, error) {
	return s.parts, s.err
}

type stubRetrievalWorker struct {
	data map[string][]rag.RetrievalResult
	err  error
}

func (s stubRetrievalWorker) Retrieve(_ context.Context, query string) ([]rag.RetrievalResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data[query], nil
}

func TestRetrievalSupervisor_Retrieve(t *testing.T) {
	workers := []RetrievalWorker{
		stubRetrievalWorker{data: map[string][]rag.RetrievalResult{
			"golang concurrency": {{Document: rag.Document{ID: "d1"}, FinalScore: 0.7}},
			"goroutine sync":     {{Document: rag.Document{ID: "d2"}, FinalScore: 0.8}},
		}},
	}
	sup := NewRetrievalSupervisor(
		stubDecomposer{parts: []string{"golang concurrency", "goroutine sync"}},
		workers,
		nil,
		nil,
	)

	out, err := sup.Retrieve(context.Background(), "golang")
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 merged results, got %d", len(out))
	}
	if out[0].Document.ID != "d2" {
		t.Fatalf("expected highest score first, got %s", out[0].Document.ID)
	}
}

func TestRetrievalSupervisor_Dedup(t *testing.T) {
	worker := stubRetrievalWorker{data: map[string][]rag.RetrievalResult{
		"q1": {{Document: rag.Document{ID: "dup"}, FinalScore: 0.4}},
		"q2": {{Document: rag.Document{ID: "dup"}, FinalScore: 0.9}},
	}}
	sup := NewRetrievalSupervisor(
		stubDecomposer{parts: []string{"q1", "q2"}},
		[]RetrievalWorker{worker},
		NewDedupResultAggregator(),
		nil,
	)
	out, err := sup.Retrieve(context.Background(), "root")
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	if len(out) != 1 || out[0].FinalScore != 0.9 {
		t.Fatalf("expected dedup highest score result, got %#v", out)
	}
}

func TestRetrievalSupervisor_ErrorPath(t *testing.T) {
	sup := NewRetrievalSupervisor(stubDecomposer{err: errors.New("decompose")}, []RetrievalWorker{stubRetrievalWorker{}}, nil, nil)
	if _, err := sup.Retrieve(context.Background(), "q"); err == nil {
		t.Fatal("expected decompose error")
	}

	sup = NewRetrievalSupervisor(stubDecomposer{parts: []string{"q"}}, []RetrievalWorker{stubRetrievalWorker{err: errors.New("worker")}}, nil, nil)
	if _, err := sup.Retrieve(context.Background(), "q"); err == nil {
		t.Fatal("expected worker error")
	}

	sup = NewRetrievalSupervisor(stubDecomposer{parts: []string{"q"}}, nil, nil, nil)
	if _, err := sup.Retrieve(context.Background(), "q"); err == nil {
		t.Fatal("expected empty workers error")
	}
}
