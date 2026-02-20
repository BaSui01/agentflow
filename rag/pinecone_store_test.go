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

func TestPineconeStore_BasicFlow(t *testing.T) {
	t.Parallel()

	var upsertCalls atomic.Int64
	var searchCalls atomic.Int64
	var deleteCalls atomic.Int64
	var countCalls atomic.Int64

	mux := http.NewServeMux()

	mux.HandleFunc("/vectors/upsert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Api-Key"); got != "test-key" {
			t.Fatalf("expected Api-Key=test-key, got %q", got)
		}
		upsertCalls.Add(1)

		var req struct {
			Vectors []struct {
				ID       string         `json:"id"`
				Values   []float64      `json:"values"`
				Metadata map[string]any `json:"metadata"`
			} `json:"vectors"`
			Namespace string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upsert: %v", err)
		}
		if len(req.Vectors) != 2 {
			t.Fatalf("expected 2 vectors, got %d", len(req.Vectors))
		}
		for _, v := range req.Vectors {
			if v.ID == "" {
				t.Fatalf("expected non-empty vector id")
			}
			if len(v.Values) == 0 {
				t.Fatalf("expected vector values")
			}
			if _, ok := v.Metadata["content"]; !ok {
				t.Fatalf("expected metadata content field")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upsertedCount":2}`))
	})

	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		searchCalls.Add(1)

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
		deleteCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	mux.HandleFunc("/describe_index_stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		countCalls.Add(1)
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
	if results[0].Score != 0.9 {
		t.Fatalf("expected score=0.9, got %f", results[0].Score)
	}
	if results[0].Distance < 0.09 || results[0].Distance > 0.11 {
		t.Fatalf("expected distance~0.1, got %f", results[0].Distance)
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

	// Verify all endpoints were hit
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

func TestPineconeStore_ListDocumentIDs(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	mux.HandleFunc("/vectors/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Api-Key"); got == "" {
			t.Fatalf("expected Api-Key header")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"vectors": [
				{"id": "doc1"},
				{"id": "doc2"},
				{"id": "doc3"}
			]
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewPineconeStore(PineconeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, zap.NewNop())

	ctx := context.Background()

	// All documents
	ids, err := store.ListDocumentIDs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListDocumentIDs: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "doc1" || ids[1] != "doc2" || ids[2] != "doc3" {
		t.Fatalf("unexpected IDs: %v", ids)
	}

	// With offset
	ids, err = store.ListDocumentIDs(ctx, 2, 1)
	if err != nil {
		t.Fatalf("ListDocumentIDs with offset: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0] != "doc2" || ids[1] != "doc3" {
		t.Fatalf("unexpected IDs with offset: %v", ids)
	}

	// Offset beyond length
	ids, err = store.ListDocumentIDs(ctx, 10, 10)
	if err != nil {
		t.Fatalf("ListDocumentIDs beyond offset: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs, got %d", len(ids))
	}

	// Zero limit
	ids, err = store.ListDocumentIDs(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListDocumentIDs zero limit: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs for zero limit, got %d", len(ids))
	}
}

func TestPineconeStore_ClearAll(t *testing.T) {
	t.Parallel()

	var deleteCalls atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/vectors/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		deleteCalls.Add(1)

		var req struct {
			DeleteAll bool   `json:"deleteAll"`
			Namespace string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode delete request: %v", err)
		}
		if !req.DeleteAll {
			t.Fatalf("expected deleteAll=true")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewPineconeStore(PineconeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, zap.NewNop())

	if err := store.ClearAll(context.Background()); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected 1 delete call, got %d", deleteCalls.Load())
	}
}

func TestPineconeStore_ClearAll_WithNamespace(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/vectors/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeleteAll bool   `json:"deleteAll"`
			Namespace string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Namespace != "my-ns" {
			t.Fatalf("expected namespace=my-ns, got %q", req.Namespace)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewPineconeStore(PineconeConfig{
		APIKey:    "test-key",
		BaseURL:   srv.URL,
		Namespace: "my-ns",
	}, zap.NewNop())

	if err := store.ClearAll(context.Background()); err != nil {
		t.Fatalf("ClearAll with namespace: %v", err)
	}
}

func TestPineconeStore_UpdateDocument(t *testing.T) {
	t.Parallel()

	var upsertCalls atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/vectors/upsert", func(w http.ResponseWriter, r *http.Request) {
		upsertCalls.Add(1)

		var req struct {
			Vectors []struct {
				ID string `json:"id"`
			} `json:"vectors"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upsert: %v", err)
		}
		if len(req.Vectors) != 1 {
			t.Fatalf("expected 1 vector for update, got %d", len(req.Vectors))
		}
		if req.Vectors[0].ID != "doc1" {
			t.Fatalf("expected id=doc1, got %q", req.Vectors[0].ID)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upsertedCount":1}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewPineconeStore(PineconeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, zap.NewNop())

	err := store.UpdateDocument(context.Background(), Document{
		ID:        "doc1",
		Content:   "updated",
		Embedding: []float64{0.3, 0.4},
	})
	if err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}
	if upsertCalls.Load() != 1 {
		t.Fatalf("expected 1 upsert call, got %d", upsertCalls.Load())
	}
}

func TestPineconeStore_Validation(t *testing.T) {
	t.Parallel()

	store := NewPineconeStore(PineconeConfig{
		APIKey:  "test-key",
		BaseURL: "http://localhost:99999",
	}, zap.NewNop())

	ctx := context.Background()

	// Empty docs is a no-op
	if err := store.AddDocuments(ctx, nil); err != nil {
		t.Fatalf("AddDocuments(nil) should succeed: %v", err)
	}

	// Empty ID
	err := store.AddDocuments(ctx, []Document{
		{ID: "", Content: "x", Embedding: []float64{0.1}},
	})
	if err == nil || !strings.Contains(err.Error(), "empty id") {
		t.Fatalf("expected empty id error, got: %v", err)
	}

	// No embedding
	err = store.AddDocuments(ctx, []Document{
		{ID: "doc1", Content: "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "no embedding") {
		t.Fatalf("expected no embedding error, got: %v", err)
	}

	// Empty query embedding
	_, err = store.Search(ctx, nil, 5)
	if err == nil || !strings.Contains(err.Error(), "query embedding is required") {
		t.Fatalf("expected query embedding error, got: %v", err)
	}

	// Zero topK returns empty
	results, err := store.Search(ctx, []float64{0.1}, 0)
	if err != nil {
		t.Fatalf("Search(topK=0) should succeed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for topK=0, got %d", len(results))
	}

	// Empty delete is a no-op
	if err := store.DeleteDocuments(ctx, nil); err != nil {
		t.Fatalf("DeleteDocuments(nil) should succeed: %v", err)
	}
}

func TestPineconeStore_EmptyAPIKey(t *testing.T) {
	t.Parallel()

	store := NewPineconeStore(PineconeConfig{
		APIKey:  "",
		BaseURL: "http://localhost:99999",
	}, zap.NewNop())

	ctx := context.Background()

	err := store.AddDocuments(ctx, []Document{
		{ID: "doc1", Content: "x", Embedding: []float64{0.1}},
	})
	if err == nil || !strings.Contains(err.Error(), "api_key is required") {
		t.Fatalf("expected api_key error, got: %v", err)
	}

	_, err = store.ListDocumentIDs(ctx, 10, 0)
	if err == nil || !strings.Contains(err.Error(), "api_key is required") {
		t.Fatalf("expected api_key error for ListDocumentIDs, got: %v", err)
	}
}

func TestPineconeStore_APIError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/vectors/upsert", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	})
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	})
	mux.HandleFunc("/describe_index_stats", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewPineconeStore(PineconeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, zap.NewNop())

	ctx := context.Background()

	// Upsert error
	err := store.AddDocuments(ctx, []Document{
		{ID: "doc1", Content: "x", Embedding: []float64{0.1}},
	})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}

	// Search error
	_, err = store.Search(ctx, []float64{0.1}, 5)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 error, got: %v", err)
	}

	// Count error
	_, err = store.Count(ctx)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got: %v", err)
	}
}

func TestPineconeStore_ContextCancellation(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/vectors/upsert", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upsertedCount":1}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewPineconeStore(PineconeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := store.AddDocuments(ctx, []Document{
		{ID: "doc1", Content: "x", Embedding: []float64{0.1}},
	})
	if err == nil {
		t.Fatalf("expected error from cancelled context")
	}
}

func TestPineconeStore_NamespaceHandling(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	mux.HandleFunc("/vectors/upsert", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Namespace string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Namespace != "test-ns" {
			t.Fatalf("expected namespace=test-ns, got %q", req.Namespace)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upsertedCount":1}`))
	})

	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Namespace string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Namespace != "test-ns" {
			t.Fatalf("expected namespace=test-ns, got %q", req.Namespace)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"matches":[]}`))
	})

	mux.HandleFunc("/vectors/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Namespace string `json:"namespace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Namespace != "test-ns" {
			t.Fatalf("expected namespace=test-ns, got %q", req.Namespace)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	mux.HandleFunc("/describe_index_stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"totalVectorCount":10,
			"namespaces":{"test-ns":{"vectorCount":5}}
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := NewPineconeStore(PineconeConfig{
		APIKey:    "test-key",
		BaseURL:   srv.URL,
		Namespace: "test-ns",
	}, zap.NewNop())

	ctx := context.Background()

	// Upsert with namespace
	err := store.AddDocuments(ctx, []Document{
		{ID: "doc1", Content: "hello", Embedding: []float64{0.1}},
	})
	if err != nil {
		t.Fatalf("AddDocuments: %v", err)
	}

	// Search with namespace
	_, err = store.Search(ctx, []float64{0.1}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Delete with namespace
	err = store.DeleteDocuments(ctx, []string{"doc1"})
	if err != nil {
		t.Fatalf("DeleteDocuments: %v", err)
	}

	// Count returns namespace-specific count
	n, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected namespace count=5, got %d", n)
	}
}

func TestPineconeStore_Defaults(t *testing.T) {
	t.Parallel()

	store := NewPineconeStore(PineconeConfig{}, nil)

	if store.cfg.Timeout != 30*1e9 {
		t.Fatalf("expected default timeout=30s, got %v", store.cfg.Timeout)
	}
	if store.cfg.ControllerBaseURL != "https://api.pinecone.io" {
		t.Fatalf("expected default controller URL, got %q", store.cfg.ControllerBaseURL)
	}
	if store.cfg.MetadataContentField != "content" {
		t.Fatalf("expected default content field=content, got %q", store.cfg.MetadataContentField)
	}
	if store.logger == nil {
		t.Fatalf("expected non-nil logger with nil input")
	}
}
