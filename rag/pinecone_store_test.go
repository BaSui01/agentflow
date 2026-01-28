package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestPineconeStore_BasicFlow(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	mux.HandleFunc("/vectors/upsert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Api-Key"); got == "" {
			t.Fatalf("expected Api-Key header")
		}

		var req struct {
			Vectors []struct {
				ID     string    `json:"id"`
				Values []float64 `json:"values"`
			} `json:"vectors"`
			Namespace string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upsert: %v", err)
		}
		if len(req.Vectors) != 2 {
			t.Fatalf("expected 2 vectors, got %d", len(req.Vectors))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upsertedCount":2}`))
	})

	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"matches":[
				{"id":"doc1","score":0.9,"metadata":{"content":"hello"}},
				{"id":"doc2","score":0.8,"metadata":{"content":"world"}}
			]
		}`))
	})

	mux.HandleFunc("/vectors/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	mux.HandleFunc("/describe_index_stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalVectorCount":2}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewPineconeStore(PineconeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, logger)

	ctx := context.Background()

	docs := []Document{
		{ID: "doc1", Content: "hello", Embedding: []float64{0.1, 0.2}},
		{ID: "doc2", Content: "world", Embedding: []float64{0.2, 0.1}},
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

	if err := store.DeleteDocuments(ctx, []string{"doc1"}); err != nil {
		t.Fatalf("DeleteDocuments: %v", err)
	}
}

