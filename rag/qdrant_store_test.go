package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

func TestQdrantStore_BasicFlow(t *testing.T) {
	t.Parallel()

	var createCollectionCalls atomic.Int64
	var upsertCalls atomic.Int64
	var searchCalls atomic.Int64
	var deleteCalls atomic.Int64
	var countCalls atomic.Int64

	mux := http.NewServeMux()

	mux.HandleFunc("/collections/testcol", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		createCollectionCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{}}`))
	})

	mux.HandleFunc("/collections/testcol/points", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "wait=true") {
			t.Fatalf("expected wait=true query, got: %q", r.URL.RawQuery)
		}
		upsertCalls.Add(1)

		var req struct {
			Points []struct {
				ID      string                 `json:"id"`
				Vector  []float64              `json:"vector"`
				Payload map[string]any `json:"payload"`
			} `json:"points"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upsert: %v", err)
		}
		if len(req.Points) != 2 {
			t.Fatalf("expected 2 points, got %d", len(req.Points))
		}
		for _, p := range req.Points {
			if p.ID == "" {
				t.Fatalf("expected non-empty point id")
			}
			if len(p.Vector) == 0 {
				t.Fatalf("expected vector values")
			}
			if _, ok := p.Payload["doc_id"]; !ok {
				t.Fatalf("expected payload doc_id")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"operation_id":1}}`))
	})

	mux.HandleFunc("/collections/testcol/points/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		searchCalls.Add(1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"ok",
			"result":[
				{"id":"00000000-0000-0000-0000-000000000001","score":0.9,"payload":{"doc_id":"doc1","content":"hello","metadata":{"k":"v"}}},
				{"id":"00000000-0000-0000-0000-000000000002","score":0.8,"payload":{"doc_id":"doc2","content":"world","metadata":{"k":"v2"}}}
			]
		}`))
	})

	mux.HandleFunc("/collections/testcol/points/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if !strings.Contains(r.URL.RawQuery, "wait=true") {
			t.Fatalf("expected wait=true query, got: %q", r.URL.RawQuery)
		}
		deleteCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"operation_id":2}}`))
	})

	mux.HandleFunc("/collections/testcol/points/count", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		countCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","result":{"count":2}}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewQdrantStore(QdrantConfig{
		BaseURL:              srv.URL,
		Collection:           "testcol",
		AutoCreateCollection: true,
	}, logger)

	ctx := context.Background()

	docs := []Document{
		{ID: "doc1", Content: "hello", Metadata: map[string]any{"k": "v"}, Embedding: []float64{0.1, 0.2}},
		{ID: "doc2", Content: "world", Metadata: map[string]any{"k": "v2"}, Embedding: []float64{0.2, 0.1}},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments: %v", err)
	}

	results, err := store.Search(ctx, []float64{0.1, 0.2}, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Document.ID != "doc1" || results[0].Document.Content != "hello" {
		t.Fatalf("unexpected result[0]: %+v", results[0].Document)
	}

	n, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected count=2, got %d", n)
	}

	if err := store.DeleteDocuments(ctx, []string{"doc1", "doc2"}); err != nil {
		t.Fatalf("DeleteDocuments: %v", err)
	}

	// 确保终点被击中。
	if createCollectionCalls.Load() != 1 {
		t.Fatalf("expected create collection 1 call, got %d", createCollectionCalls.Load())
	}
	if upsertCalls.Load() != 1 {
		t.Fatalf("expected upsert 1 call, got %d", upsertCalls.Load())
	}
	if searchCalls.Load() != 1 {
		t.Fatalf("expected search 1 call, got %d", searchCalls.Load())
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected delete 1 call, got %d", deleteCalls.Load())
	}
	if countCalls.Load() != 1 {
		t.Fatalf("expected count 1 call, got %d", countCalls.Load())
	}
}
