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

func TestWeaviateStore_BasicFlow(t *testing.T) {
	t.Parallel()

	var schemaCheckCalls atomic.Int64
	var schemaCreateCalls atomic.Int64
	var batchCalls atomic.Int64
	var graphqlCalls atomic.Int64
	var deleteCalls atomic.Int64

	mux := http.NewServeMux()

	// Schema check endpoint
	mux.HandleFunc("/v1/schema/TestClass", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			schemaCheckCalls.Add(1)
			// Return 404 to trigger schema creation
			if schemaCheckCalls.Load() == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"class":"TestClass"}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method for schema endpoint: %s", r.Method)
		}
	})

	// Schema create endpoint
	mux.HandleFunc("/v1/schema", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		schemaCreateCalls.Add(1)

		var schema map[string]any
		if err := json.NewDecoder(r.Body).Decode(&schema); err != nil {
			t.Fatalf("decode schema: %v", err)
		}

		if schema["class"] != "TestClass" {
			t.Fatalf("expected class TestClass, got %v", schema["class"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"class":"TestClass"}`))
	})

	// Batch objects endpoint
	mux.HandleFunc("/v1/batch/objects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		batchCalls.Add(1)

		var req struct {
			Objects []struct {
				Class      string         `json:"class"`
				ID         string         `json:"id"`
				Properties map[string]any `json:"properties"`
				Vector     []float64      `json:"vector"`
			} `json:"objects"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode batch: %v", err)
		}

		if len(req.Objects) != 2 {
			t.Fatalf("expected 2 objects, got %d", len(req.Objects))
		}

		for _, obj := range req.Objects {
			if obj.Class != "TestClass" {
				t.Fatalf("expected class TestClass, got %s", obj.Class)
			}
			if obj.ID == "" {
				t.Fatalf("expected non-empty object id")
			}
			if len(obj.Vector) == 0 {
				t.Fatalf("expected vector values")
			}
			if _, ok := obj.Properties["docId"]; !ok {
				t.Fatalf("expected docId property")
			}
		}

		// Return success response
		results := make([]map[string]any, len(req.Objects))
		for i, obj := range req.Objects {
			results[i] = map[string]any{
				"id":     obj.ID,
				"result": map[string]any{},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		resp, _ := json.Marshal(map[string]any{"results": results})
		_, _ = w.Write(resp)
	})

	// GraphQL endpoint
	mux.HandleFunc("/v1/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		graphqlCalls.Add(1)

		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode graphql: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")

		// Check if it's an aggregate query (for Count)
		if strings.Contains(req.Query, "Aggregate") {
			_, _ = w.Write([]byte(`{
				"data": {
					"Aggregate": {
						"TestClass": [{"meta": {"count": 2}}]
					}
				}
			}`))
			return
		}

		// Search query response
		_, _ = w.Write([]byte(`{
			"data": {
				"Get": {
					"TestClass": [
						{
							"docId": "doc1",
							"content": "hello world",
							"metadata": "{\"key\":\"value1\"}",
							"_additional": {"id": "uuid1", "distance": 0.1, "certainty": 0.9}
						},
						{
							"docId": "doc2",
							"content": "goodbye world",
							"metadata": "{\"key\":\"value2\"}",
							"_additional": {"id": "uuid2", "distance": 0.2, "certainty": 0.8}
						}
					]
				}
			}
		}`))
	})

	// Delete object endpoint
	mux.HandleFunc("/v1/objects/TestClass/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		deleteCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewWeaviateStore(WeaviateConfig{
		BaseURL:          srv.URL,
		ClassName:        "TestClass",
		AutoCreateSchema: true,
	}, logger)

	ctx := context.Background()

	// Test AddDocuments
	docs := []Document{
		{ID: "doc1", Content: "hello world", Metadata: map[string]any{"key": "value1"}, Embedding: []float64{0.1, 0.2, 0.3}},
		{ID: "doc2", Content: "goodbye world", Metadata: map[string]any{"key": "value2"}, Embedding: []float64{0.3, 0.2, 0.1}},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments: %v", err)
	}

	// Test Search
	results, err := store.Search(ctx, []float64{0.1, 0.2, 0.3}, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Document.ID != "doc1" || results[0].Document.Content != "hello world" {
		t.Fatalf("unexpected result[0]: %+v", results[0].Document)
	}
	if results[0].Score != 0.9 {
		t.Fatalf("expected score 0.9, got %f", results[0].Score)
	}

	// Test Count
	n, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected count=2, got %d", n)
	}

	// Test DeleteDocuments
	if err := store.DeleteDocuments(ctx, []string{"doc1", "doc2"}); err != nil {
		t.Fatalf("DeleteDocuments: %v", err)
	}

	// Verify endpoint calls
	if schemaCheckCalls.Load() < 1 {
		t.Fatalf("expected at least 1 schema check call, got %d", schemaCheckCalls.Load())
	}
	if schemaCreateCalls.Load() != 1 {
		t.Fatalf("expected 1 schema create call, got %d", schemaCreateCalls.Load())
	}
	if batchCalls.Load() != 1 {
		t.Fatalf("expected 1 batch call, got %d", batchCalls.Load())
	}
	if graphqlCalls.Load() != 2 { // 1 search + 1 count
		t.Fatalf("expected 2 graphql calls, got %d", graphqlCalls.Load())
	}
	if deleteCalls.Load() != 2 {
		t.Fatalf("expected 2 delete calls, got %d", deleteCalls.Load())
	}
}

func TestWeaviateStore_HybridSearch(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode graphql: %v", err)
		}

		// Verify hybrid search query
		if !strings.Contains(req.Query, "hybrid") {
			t.Fatalf("expected hybrid query, got: %s", req.Query)
		}
		if !strings.Contains(req.Query, "alpha") {
			t.Fatalf("expected alpha parameter in hybrid query")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"Get": {
					"TestClass": [
						{
							"docId": "doc1",
							"content": "hello world",
							"metadata": "{}",
							"_additional": {"id": "uuid1", "score": 0.95}
						}
					]
				}
			}
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewWeaviateStore(WeaviateConfig{
		BaseURL:     srv.URL,
		ClassName:   "TestClass",
		HybridAlpha: 0.7,
	}, logger)

	ctx := context.Background()

	results, err := store.HybridSearch(ctx, "hello", []float64{0.1, 0.2}, 5)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Document.ID != "doc1" {
		t.Fatalf("unexpected document ID: %s", results[0].Document.ID)
	}
	if results[0].Score != 0.95 {
		t.Fatalf("expected score 0.95, got %f", results[0].Score)
	}
}

func TestWeaviateStore_BM25Search(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode graphql: %v", err)
		}

		// Verify BM25 search query
		if !strings.Contains(req.Query, "bm25") {
			t.Fatalf("expected bm25 query, got: %s", req.Query)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"Get": {
					"TestClass": [
						{
							"docId": "doc1",
							"content": "hello world",
							"metadata": "{}",
							"_additional": {"id": "uuid1", "score": 0.85}
						}
					]
				}
			}
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewWeaviateStore(WeaviateConfig{
		BaseURL:   srv.URL,
		ClassName: "TestClass",
	}, logger)

	ctx := context.Background()

	results, err := store.BM25Search(ctx, "hello world", 5)
	if err != nil {
		t.Fatalf("BM25Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 0.85 {
		t.Fatalf("expected score 0.85, got %f", results[0].Score)
	}
}

func TestWeaviateStore_ValidationErrors(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()

	t.Run("empty class name", func(t *testing.T) {
		store := NewWeaviateStore(WeaviateConfig{
			BaseURL: "http://localhost:8080",
		}, logger)

		ctx := context.Background()

		_, err := store.Search(ctx, []float64{0.1}, 5)
		if err == nil || !strings.Contains(err.Error(), "class_name is required") {
			t.Fatalf("expected class_name error, got: %v", err)
		}
	})

	t.Run("empty document ID", func(t *testing.T) {
		store := NewWeaviateStore(WeaviateConfig{
			BaseURL:   "http://localhost:8080",
			ClassName: "TestClass",
		}, logger)

		ctx := context.Background()

		err := store.AddDocuments(ctx, []Document{
			{ID: "", Content: "test", Embedding: []float64{0.1}},
		})
		if err == nil || !strings.Contains(err.Error(), "empty id") {
			t.Fatalf("expected empty id error, got: %v", err)
		}
	})

	t.Run("missing embedding", func(t *testing.T) {
		store := NewWeaviateStore(WeaviateConfig{
			BaseURL:   "http://localhost:8080",
			ClassName: "TestClass",
		}, logger)

		ctx := context.Background()

		err := store.AddDocuments(ctx, []Document{
			{ID: "doc1", Content: "test", Embedding: nil},
		})
		if err == nil || !strings.Contains(err.Error(), "no embedding") {
			t.Fatalf("expected no embedding error, got: %v", err)
		}
	})

	t.Run("dimension mismatch", func(t *testing.T) {
		store := NewWeaviateStore(WeaviateConfig{
			BaseURL:   "http://localhost:8080",
			ClassName: "TestClass",
		}, logger)

		ctx := context.Background()

		err := store.AddDocuments(ctx, []Document{
			{ID: "doc1", Content: "test1", Embedding: []float64{0.1, 0.2}},
			{ID: "doc2", Content: "test2", Embedding: []float64{0.1, 0.2, 0.3}},
		})
		if err == nil || !strings.Contains(err.Error(), "dimension mismatch") {
			t.Fatalf("expected dimension mismatch error, got: %v", err)
		}
	})

	t.Run("empty query embedding", func(t *testing.T) {
		store := NewWeaviateStore(WeaviateConfig{
			BaseURL:   "http://localhost:8080",
			ClassName: "TestClass",
		}, logger)

		ctx := context.Background()

		_, err := store.Search(ctx, []float64{}, 5)
		if err == nil || !strings.Contains(err.Error(), "query embedding is required") {
			t.Fatalf("expected query embedding error, got: %v", err)
		}
	})

	t.Run("empty BM25 query", func(t *testing.T) {
		store := NewWeaviateStore(WeaviateConfig{
			BaseURL:   "http://localhost:8080",
			ClassName: "TestClass",
		}, logger)

		ctx := context.Background()

		_, err := store.BM25Search(ctx, "", 5)
		if err == nil || !strings.Contains(err.Error(), "query text is required") {
			t.Fatalf("expected query text error, got: %v", err)
		}
	})
}

func TestWeaviateStore_DefaultConfig(t *testing.T) {
	t.Parallel()

	store := NewWeaviateStore(WeaviateConfig{
		ClassName: "TestClass",
	}, nil)

	// Verify defaults
	if store.cfg.Host != "localhost" {
		t.Fatalf("expected default host localhost, got %s", store.cfg.Host)
	}
	if store.cfg.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", store.cfg.Port)
	}
	if store.cfg.Scheme != "http" {
		t.Fatalf("expected default scheme http, got %s", store.cfg.Scheme)
	}
	if store.cfg.Distance != "cosine" {
		t.Fatalf("expected default distance cosine, got %s", store.cfg.Distance)
	}
	if store.cfg.HybridAlpha != 0.5 {
		t.Fatalf("expected default hybrid alpha 0.5, got %f", store.cfg.HybridAlpha)
	}
	if store.cfg.ContentProperty != "content" {
		t.Fatalf("expected default content property 'content', got %s", store.cfg.ContentProperty)
	}
	if store.baseURL != "http://localhost:8080" {
		t.Fatalf("expected base URL http://localhost:8080, got %s", store.baseURL)
	}
}

func TestWeaviateStore_UpdateDocument(t *testing.T) {
	t.Parallel()

	var batchCalls atomic.Int64

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/batch/objects", func(w http.ResponseWriter, r *http.Request) {
		batchCalls.Add(1)

		var req struct {
			Objects []struct {
				ID string `json:"id"`
			} `json:"objects"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode batch: %v", err)
		}

		if len(req.Objects) != 1 {
			t.Fatalf("expected 1 object for update, got %d", len(req.Objects))
		}

		results := []map[string]any{
			{"id": req.Objects[0].ID, "result": map[string]any{}},
		}

		w.Header().Set("Content-Type", "application/json")
		resp, _ := json.Marshal(map[string]any{"results": results})
		_, _ = w.Write(resp)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewWeaviateStore(WeaviateConfig{
		BaseURL:   srv.URL,
		ClassName: "TestClass",
	}, logger)

	ctx := context.Background()

	err := store.UpdateDocument(ctx, Document{
		ID:        "doc1",
		Content:   "updated content",
		Embedding: []float64{0.1, 0.2, 0.3},
	})
	if err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}

	if batchCalls.Load() != 1 {
		t.Fatalf("expected 1 batch call, got %d", batchCalls.Load())
	}
}

func TestWeaviateStore_EmptyResults(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"Get": {
					"TestClass": []
				}
			}
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewWeaviateStore(WeaviateConfig{
		BaseURL:   srv.URL,
		ClassName: "TestClass",
	}, logger)

	ctx := context.Background()

	results, err := store.Search(ctx, []float64{0.1, 0.2}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestWeaviateStore_GraphQLError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/graphql", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": null,
			"errors": [{"message": "Class TestClass does not exist"}]
		}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewWeaviateStore(WeaviateConfig{
		BaseURL:   srv.URL,
		ClassName: "TestClass",
	}, logger)

	ctx := context.Background()

	_, err := store.Search(ctx, []float64{0.1, 0.2}, 5)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected graphql error, got: %v", err)
	}
}

func TestWeaviateObjectID(t *testing.T) {
	t.Parallel()

	// Test deterministic UUID generation
	id1 := weaviateObjectID("doc1")
	id2 := weaviateObjectID("doc1")
	id3 := weaviateObjectID("doc2")

	if id1 != id2 {
		t.Fatalf("expected same UUID for same input, got %s and %s", id1, id2)
	}
	if id1 == id3 {
		t.Fatalf("expected different UUIDs for different inputs")
	}

	// Verify it's a valid UUID format
	if len(id1) != 36 {
		t.Fatalf("expected UUID length 36, got %d", len(id1))
	}
}

func TestEscapeGraphQLString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello world", "hello world"},
		{`hello "world"`, `hello \"world\"`},
		{"hello\nworld", `hello\nworld`},
		{"hello\tworld", `hello\tworld`},
		{`path\to\file`, `path\\to\\file`},
	}

	for _, tt := range tests {
		result := escapeGraphQLString(tt.input)
		if result != tt.expected {
			t.Errorf("escapeGraphQLString(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatVector(t *testing.T) {
	t.Parallel()

	result := formatVector([]float64{0.1, 0.2, 0.3})
	if !strings.HasPrefix(result, "[") || !strings.HasSuffix(result, "]") {
		t.Fatalf("expected array format, got %s", result)
	}
	if !strings.Contains(result, "0.1") {
		t.Fatalf("expected vector values in output, got %s", result)
	}
}
