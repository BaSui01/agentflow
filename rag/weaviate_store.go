package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// WeaviateConfig configures the Weaviate VectorStore implementation.
//
// Weaviate is an open-source vector database that supports:
// - Vector search with multiple distance metrics
// - BM25 keyword search
// - Hybrid search (combining vector and BM25)
// - GraphQL API for flexible queries
// - Automatic schema management
type WeaviateConfig struct {
	// Connection settings
	Host    string `json:"host"`              // Weaviate host (default: localhost)
	Port    int    `json:"port"`              // Weaviate port (default: 8080)
	Scheme  string `json:"scheme,omitempty"`  // http or https (default: http)
	BaseURL string `json:"base_url,omitempty"` // Full base URL (overrides host/port/scheme)

	// Authentication
	APIKey string `json:"api_key,omitempty"` // API key for authentication

	// Class/Collection settings
	ClassName string `json:"class_name"` // Weaviate class name (required)

	// Schema settings
	AutoCreateSchema bool   `json:"auto_create_schema,omitempty"` // Auto-create class if not exists
	VectorIndexType  string `json:"vector_index_type,omitempty"`  // hnsw (default), flat
	Distance         string `json:"distance,omitempty"`           // cosine (default), dot, l2, hamming, manhattan

	// Vector settings
	VectorSize int `json:"vector_size,omitempty"` // Vector dimension (optional, auto-detected)

	// Hybrid search settings
	HybridAlpha float64 `json:"hybrid_alpha,omitempty"` // Alpha for hybrid search (0=BM25, 1=vector, default: 0.5)

	// Timeout settings
	Timeout time.Duration `json:"timeout,omitempty"` // Request timeout (default: 30s)

	// Property field names
	ContentProperty  string `json:"content_property,omitempty"`  // Property for document content (default: content)
	MetadataProperty string `json:"metadata_property,omitempty"` // Property for document metadata (default: metadata)
	DocIDProperty    string `json:"doc_id_property,omitempty"`   // Property for original document ID (default: docId)
}

// WeaviateStore implements VectorStore using Weaviate's REST and GraphQL APIs.
type WeaviateStore struct {
	cfg WeaviateConfig

	baseURL string
	client  *http.Client
	logger  *zap.Logger

	ensureOnce sync.Once
	ensureErr  error
}

// NewWeaviateStore creates a Weaviate-backed VectorStore.
func NewWeaviateStore(cfg WeaviateConfig, logger *zap.Logger) *WeaviateStore {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Apply defaults
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.Scheme == "" {
		cfg.Scheme = "http"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Distance == "" {
		cfg.Distance = "cosine"
	}
	if cfg.VectorIndexType == "" {
		cfg.VectorIndexType = "hnsw"
	}
	if cfg.HybridAlpha == 0 {
		cfg.HybridAlpha = 0.5
	}
	if cfg.ContentProperty == "" {
		cfg.ContentProperty = "content"
	}
	if cfg.MetadataProperty == "" {
		cfg.MetadataProperty = "metadata"
	}
	if cfg.DocIDProperty == "" {
		cfg.DocIDProperty = "docId"
	}

	// Build base URL
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("%s://%s:%d", cfg.Scheme, cfg.Host, cfg.Port)
	}

	return &WeaviateStore{
		cfg:     cfg,
		baseURL: baseURL,
		client:  &http.Client{Timeout: cfg.Timeout},
		logger:  logger.With(zap.String("component", "weaviate_store")),
	}
}

// weaviateNamespace is used to generate deterministic UUIDs from document IDs.
var weaviateNamespace = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

// weaviateObjectID generates a deterministic UUID from a document ID.
func weaviateObjectID(docID string) string {
	return uuid.NewSHA1(weaviateNamespace, []byte(docID)).String()
}

// applyHeaders sets common headers for Weaviate requests.
func (s *WeaviateStore) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(s.cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	}
}

// doJSON performs a JSON HTTP request to Weaviate.
func (s *WeaviateStore) doJSON(ctx context.Context, method, path string, in any, out any) error {
	endpoint := s.baseURL + path

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("weaviate marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("weaviate create request: %w", err)
	}
	s.applyHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("weaviate request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		return fmt.Errorf("weaviate request failed: method=%s path=%s status=%d body=%s",
			method, path, resp.StatusCode, string(raw))
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("weaviate decode response: %w", err)
	}
	return nil
}

// ensureSchema creates the Weaviate class if it doesn't exist.
func (s *WeaviateStore) ensureSchema(ctx context.Context, vectorSize int) error {
	if !s.cfg.AutoCreateSchema {
		return nil
	}
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return fmt.Errorf("weaviate class_name is required")
	}

	s.ensureOnce.Do(func() {
		// Check if class exists
		checkPath := fmt.Sprintf("/v1/schema/%s", s.cfg.ClassName)
		checkReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+checkPath, nil)
		if err != nil {
			s.ensureErr = err
			return
		}
		s.applyHeaders(checkReq)

		checkResp, err := s.client.Do(checkReq)
		if err != nil {
			s.ensureErr = err
			return
		}
		defer checkResp.Body.Close()

		// Class already exists
		if checkResp.StatusCode == http.StatusOK {
			s.logger.Debug("weaviate class already exists", zap.String("class", s.cfg.ClassName))
			return
		}

		// Create class schema
		schema := s.buildClassSchema(vectorSize)

		var resp any
		if err := s.doJSON(ctx, http.MethodPost, "/v1/schema", schema, &resp); err != nil {
			s.ensureErr = fmt.Errorf("weaviate create schema failed: %w", err)
			return
		}

		s.logger.Info("weaviate class created", zap.String("class", s.cfg.ClassName))
	})

	return s.ensureErr
}

// buildClassSchema builds the Weaviate class schema definition.
func (s *WeaviateStore) buildClassSchema(vectorSize int) map[string]any {
	// Map distance metric to Weaviate format
	distanceMetric := s.cfg.Distance
	switch strings.ToLower(distanceMetric) {
	case "cosine":
		distanceMetric = "cosine"
	case "dot":
		distanceMetric = "dot"
	case "l2", "euclidean":
		distanceMetric = "l2-squared"
	case "hamming":
		distanceMetric = "hamming"
	case "manhattan":
		distanceMetric = "manhattan"
	default:
		distanceMetric = "cosine"
	}

	schema := map[string]any{
		"class":       s.cfg.ClassName,
		"description": "AgentFlow document collection",
		"vectorizer":  "none", // We provide our own vectors
		"vectorIndexConfig": map[string]any{
			"distance": distanceMetric,
		},
		"properties": []map[string]any{
			{
				"name":        s.cfg.DocIDProperty,
				"dataType":    []string{"text"},
				"description": "Original document ID",
				"indexFilterable": true,
				"indexSearchable": true,
			},
			{
				"name":        s.cfg.ContentProperty,
				"dataType":    []string{"text"},
				"description": "Document content",
				"indexFilterable": true,
				"indexSearchable": true,
				"tokenization":    "word",
			},
			{
				"name":        s.cfg.MetadataProperty,
				"dataType":    []string{"text"},
				"description": "Document metadata as JSON",
				"indexFilterable": false,
				"indexSearchable": false,
			},
		},
	}

	// Add inverted index config for BM25 search
	schema["invertedIndexConfig"] = map[string]any{
		"bm25": map[string]any{
			"b":  0.75,
			"k1": 1.2,
		},
		"stopwords": map[string]any{
			"preset": "en",
		},
	}

	return schema
}

// AddDocuments adds documents to the Weaviate store.
func (s *WeaviateStore) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return fmt.Errorf("weaviate class_name is required")
	}

	// Validate documents and determine vector size
	vectorSize := s.cfg.VectorSize
	for i, doc := range docs {
		if doc.ID == "" {
			return fmt.Errorf("document[%d] has empty id", i)
		}
		if len(doc.Embedding) == 0 {
			return fmt.Errorf("document[%d] has no embedding", i)
		}
		if vectorSize == 0 {
			vectorSize = len(doc.Embedding)
		}
		if len(doc.Embedding) != vectorSize {
			return fmt.Errorf("document[%d] embedding dimension mismatch: got=%d want=%d",
				i, len(doc.Embedding), vectorSize)
		}
	}

	// Ensure schema exists
	if err := s.ensureSchema(ctx, vectorSize); err != nil {
		return err
	}

	// Build batch request
	objects := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		// Serialize metadata to JSON
		metadataJSON := "{}"
		if doc.Metadata != nil {
			if b, err := json.Marshal(doc.Metadata); err == nil {
				metadataJSON = string(b)
			}
		}

		obj := map[string]any{
			"class": s.cfg.ClassName,
			"id":    weaviateObjectID(doc.ID),
			"properties": map[string]any{
				s.cfg.DocIDProperty:    doc.ID,
				s.cfg.ContentProperty:  doc.Content,
				s.cfg.MetadataProperty: metadataJSON,
			},
			"vector": doc.Embedding,
		}
		objects = append(objects, obj)
	}

	// Batch upsert
	batchReq := map[string]any{
		"objects": objects,
	}

	var batchResp struct {
		Results []struct {
			ID     string `json:"id"`
			Result struct {
				Errors *struct {
					Error []struct {
						Message string `json:"message"`
					} `json:"error"`
				} `json:"errors"`
			} `json:"result"`
		} `json:"results"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v1/batch/objects", batchReq, &batchResp); err != nil {
		return err
	}

	// Check for errors in batch response
	for _, r := range batchResp.Results {
		if r.Result.Errors != nil && len(r.Result.Errors.Error) > 0 {
			return fmt.Errorf("weaviate batch error for object %s: %s",
				r.ID, r.Result.Errors.Error[0].Message)
		}
	}

	s.logger.Debug("weaviate batch upsert completed", zap.Int("count", len(docs)))
	return nil
}

// Search performs a vector similarity search.
func (s *WeaviateStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return nil, fmt.Errorf("weaviate class_name is required")
	}
	if topK <= 0 {
		return []VectorSearchResult{}, nil
	}
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is required")
	}

	// Build GraphQL query for vector search
	query := s.buildVectorSearchQuery(queryEmbedding, topK)

	return s.executeGraphQLSearch(ctx, query)
}

// HybridSearch performs a hybrid search combining vector similarity and BM25.
func (s *WeaviateStore) HybridSearch(ctx context.Context, queryText string, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return nil, fmt.Errorf("weaviate class_name is required")
	}
	if topK <= 0 {
		return []VectorSearchResult{}, nil
	}

	// Build GraphQL query for hybrid search
	query := s.buildHybridSearchQuery(queryText, queryEmbedding, topK)

	return s.executeGraphQLSearch(ctx, query)
}

// BM25Search performs a keyword-based BM25 search.
func (s *WeaviateStore) BM25Search(ctx context.Context, queryText string, topK int) ([]VectorSearchResult, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return nil, fmt.Errorf("weaviate class_name is required")
	}
	if topK <= 0 {
		return []VectorSearchResult{}, nil
	}
	if strings.TrimSpace(queryText) == "" {
		return nil, fmt.Errorf("query text is required for BM25 search")
	}

	// Build GraphQL query for BM25 search
	query := s.buildBM25SearchQuery(queryText, topK)

	return s.executeGraphQLSearch(ctx, query)
}

// buildVectorSearchQuery builds a GraphQL query for vector search.
func (s *WeaviateStore) buildVectorSearchQuery(vector []float64, topK int) map[string]any {
	vectorStr := formatVector(vector)

	graphql := fmt.Sprintf(`{
		Get {
			%s(
				nearVector: {
					vector: %s
				}
				limit: %d
			) {
				%s
				%s
				%s
				_additional {
					id
					distance
					certainty
				}
			}
		}
	}`, s.cfg.ClassName, vectorStr, topK,
		s.cfg.DocIDProperty, s.cfg.ContentProperty, s.cfg.MetadataProperty)

	return map[string]any{
		"query": graphql,
	}
}

// buildHybridSearchQuery builds a GraphQL query for hybrid search.
func (s *WeaviateStore) buildHybridSearchQuery(queryText string, vector []float64, topK int) map[string]any {
	vectorStr := ""
	if len(vector) > 0 {
		vectorStr = fmt.Sprintf(`, vector: %s`, formatVector(vector))
	}

	// Escape query text for GraphQL
	escapedQuery := escapeGraphQLString(queryText)

	graphql := fmt.Sprintf(`{
		Get {
			%s(
				hybrid: {
					query: "%s"
					alpha: %f
					%s
				}
				limit: %d
			) {
				%s
				%s
				%s
				_additional {
					id
					distance
					score
				}
			}
		}
	}`, s.cfg.ClassName, escapedQuery, s.cfg.HybridAlpha, vectorStr, topK,
		s.cfg.DocIDProperty, s.cfg.ContentProperty, s.cfg.MetadataProperty)

	return map[string]any{
		"query": graphql,
	}
}

// buildBM25SearchQuery builds a GraphQL query for BM25 search.
func (s *WeaviateStore) buildBM25SearchQuery(queryText string, topK int) map[string]any {
	// Escape query text for GraphQL
	escapedQuery := escapeGraphQLString(queryText)

	graphql := fmt.Sprintf(`{
		Get {
			%s(
				bm25: {
					query: "%s"
					properties: ["%s"]
				}
				limit: %d
			) {
				%s
				%s
				%s
				_additional {
					id
					score
				}
			}
		}
	}`, s.cfg.ClassName, escapedQuery, s.cfg.ContentProperty, topK,
		s.cfg.DocIDProperty, s.cfg.ContentProperty, s.cfg.MetadataProperty)

	return map[string]any{
		"query": graphql,
	}
}

// executeGraphQLSearch executes a GraphQL search query and parses results.
func (s *WeaviateStore) executeGraphQLSearch(ctx context.Context, query map[string]any) ([]VectorSearchResult, error) {
	var resp struct {
		Data struct {
			Get map[string][]struct {
				DocID      string `json:"docId"`
				Content    string `json:"content"`
				Metadata   string `json:"metadata"`
				Additional struct {
					ID        string   `json:"id"`
					Distance  *float64 `json:"distance"`
					Certainty *float64 `json:"certainty"`
					Score     *float64 `json:"score"`
				} `json:"_additional"`
			} `json:"Get"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v1/graphql", query, &resp); err != nil {
		return nil, err
	}

	// Check for GraphQL errors
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("weaviate graphql error: %s", resp.Errors[0].Message)
	}

	// Parse results
	results := resp.Data.Get[s.cfg.ClassName]
	out := make([]VectorSearchResult, 0, len(results))

	for _, r := range results {
		doc := Document{
			ID:      r.DocID,
			Content: r.Content,
		}

		// Parse metadata JSON
		if r.Metadata != "" && r.Metadata != "{}" {
			var meta map[string]any
			if err := json.Unmarshal([]byte(r.Metadata), &meta); err == nil {
				doc.Metadata = meta
			}
		}

		// Calculate score and distance
		var score, distance float64
		if r.Additional.Certainty != nil {
			score = *r.Additional.Certainty
			distance = 1.0 - score
		} else if r.Additional.Distance != nil {
			distance = *r.Additional.Distance
			// Convert distance to score (assuming cosine distance)
			score = 1.0 - distance
		} else if r.Additional.Score != nil {
			score = *r.Additional.Score
			distance = 1.0 - score
		}

		out = append(out, VectorSearchResult{
			Document: doc,
			Score:    score,
			Distance: distance,
		})
	}

	return out, nil
}

// DeleteDocuments deletes documents by their IDs.
func (s *WeaviateStore) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return fmt.Errorf("weaviate class_name is required")
	}

	// Delete each document by its Weaviate object ID
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}

		objectID := weaviateObjectID(id)
		path := fmt.Sprintf("/v1/objects/%s/%s", s.cfg.ClassName, objectID)

		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.baseURL+path, nil)
		if err != nil {
			return fmt.Errorf("weaviate create delete request: %w", err)
		}
		s.applyHeaders(req)

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("weaviate delete request failed: %w", err)
		}
		defer resp.Body.Close()

		// 404 is acceptable (object doesn't exist)
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("weaviate delete failed: status=%d", resp.StatusCode)
		}
	}

	s.logger.Debug("weaviate delete completed", zap.Int("count", len(ids)))
	return nil
}

// UpdateDocument updates a single document (upsert).
func (s *WeaviateStore) UpdateDocument(ctx context.Context, doc Document) error {
	return s.AddDocuments(ctx, []Document{doc})
}

// Count returns the total number of documents in the collection.
func (s *WeaviateStore) Count(ctx context.Context) (int, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return 0, fmt.Errorf("weaviate class_name is required")
	}

	// Use GraphQL aggregate query
	graphql := fmt.Sprintf(`{
		Aggregate {
			%s {
				meta {
					count
				}
			}
		}
	}`, s.cfg.ClassName)

	query := map[string]any{
		"query": graphql,
	}

	var resp struct {
		Data struct {
			Aggregate map[string][]struct {
				Meta struct {
					Count int `json:"count"`
				} `json:"meta"`
			} `json:"Aggregate"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v1/graphql", query, &resp); err != nil {
		return 0, err
	}

	if len(resp.Errors) > 0 {
		return 0, fmt.Errorf("weaviate graphql error: %s", resp.Errors[0].Message)
	}

	results := resp.Data.Aggregate[s.cfg.ClassName]
	if len(results) == 0 {
		return 0, nil
	}

	return results[0].Meta.Count, nil
}

// DeleteClass deletes the entire Weaviate class (use with caution).
func (s *WeaviateStore) DeleteClass(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return fmt.Errorf("weaviate class_name is required")
	}

	path := fmt.Sprintf("/v1/schema/%s", s.cfg.ClassName)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("weaviate create delete class request: %w", err)
	}
	s.applyHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("weaviate delete class request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("weaviate delete class failed: status=%d", resp.StatusCode)
	}

	// Reset ensureOnce so schema can be recreated
	s.ensureOnce = sync.Once{}
	s.ensureErr = nil

	s.logger.Info("weaviate class deleted", zap.String("class", s.cfg.ClassName))
	return nil
}

// GetSchema returns the current class schema.
func (s *WeaviateStore) GetSchema(ctx context.Context) (map[string]any, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return nil, fmt.Errorf("weaviate class_name is required")
	}

	path := fmt.Sprintf("/v1/schema/%s", s.cfg.ClassName)
	var schema map[string]any
	if err := s.doJSON(ctx, http.MethodGet, path, nil, &schema); err != nil {
		return nil, err
	}

	return schema, nil
}

// Helper functions

// formatVector formats a float64 slice as a JSON array string.
func formatVector(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// escapeGraphQLString escapes a string for use in GraphQL queries.
func escapeGraphQLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
