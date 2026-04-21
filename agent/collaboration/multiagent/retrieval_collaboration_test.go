package multiagent

import (
	"context"
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/types"
)

type stubDecomposer struct {
	parts []string
	err   error
}

func (s stubDecomposer) Decompose(context.Context, string) ([]string, error) {
	return s.parts, s.err
}

type stubRetrievalWorker struct {
	data map[string][]types.RetrievalRecord
	err  error
}

func (s stubRetrievalWorker) Retrieve(_ context.Context, query string) ([]types.RetrievalRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data[query], nil
}

func TestRetrievalSupervisor_Retrieve(t *testing.T) {
	workers := []RetrievalWorker{
		stubRetrievalWorker{data: map[string][]types.RetrievalRecord{
			"golang concurrency": {{DocID: "d1", Score: 0.7}},
			"goroutine sync":     {{DocID: "d2", Score: 0.8}},
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
	if out[0].DocID != "d2" {
		t.Fatalf("expected highest score first, got %s", out[0].DocID)
	}
}

func TestRetrievalSupervisor_Dedup(t *testing.T) {
	worker := stubRetrievalWorker{data: map[string][]types.RetrievalRecord{
		"q1": {{DocID: "dup", Score: 0.4}},
		"q2": {{DocID: "dup", Score: 0.9}},
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
	if len(out) != 1 || out[0].Score != 0.9 {
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
