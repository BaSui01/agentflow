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

// MilvusIndexType defines the index type for Milvus vector search.
type MilvusIndexType string

const (
	// MilvusIndexIVFFlat is the IVF_FLAT index type (good balance of speed and accuracy).
	MilvusIndexIVFFlat MilvusIndexType = "IVF_FLAT"
	// MilvusIndexHNSW is the HNSW index type (high accuracy, more memory).
	MilvusIndexHNSW MilvusIndexType = "HNSW"
	// MilvusIndexFlat is the FLAT index type (brute force, highest accuracy).
	MilvusIndexFlat MilvusIndexType = "FLAT"
	// MilvusIndexIVFSQ8 is the IVF_SQ8 index type (compressed, faster but less accurate).
	MilvusIndexIVFSQ8 MilvusIndexType = "IVF_SQ8"
	// MilvusIndexIVFPQ is the IVF_PQ index type (highly compressed, fastest but least accurate).
	MilvusIndexIVFPQ MilvusIndexType = "IVF_PQ"
)

// MilvusMetricType defines the distance metric for Milvus vector search.
type MilvusMetricType string

const (
	// MilvusMetricL2 is the Euclidean distance metric.
	MilvusMetricL2 MilvusMetricType = "L2"
	// MilvusMetricIP is the Inner Product (cosine similarity) metric.
	MilvusMetricIP MilvusMetricType = "IP"
	// MilvusMetricCosine is the Cosine similarity metric.
	MilvusMetricCosine MilvusMetricType = "COSINE"
)

// MilvusConfig configures the Milvus VectorStore implementation.
type MilvusConfig struct {
	// Connection settings
	Host    string `json:"host"`
	Port    int    `json:"port"`
	BaseURL string `json:"base_url,omitempty"` // Override host:port if set

	// Authentication
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"` // For Zilliz Cloud

	// Collection settings
	Collection string `json:"collection"`
	Database   string `json:"database,omitempty"` // Default: "default"

	// Schema settings
	VectorDimension int    `json:"vector_dimension,omitempty"` // Required for auto-create
	PrimaryField    string `json:"primary_field,omitempty"`    // Default: "id"
	VectorField     string `json:"vector_field,omitempty"`     // Default: "vector"
	ContentField    string `json:"content_field,omitempty"`    // Default: "content"
	MetadataField   string `json:"metadata_field,omitempty"`   // Default: "metadata"

	// Index settings
	IndexType   MilvusIndexType  `json:"index_type,omitempty"`   // Default: IVF_FLAT
	MetricType  MilvusMetricType `json:"metric_type,omitempty"`  // Default: COSINE
	IndexParams map[string]any   `json:"index_params,omitempty"` // Index-specific params

	// Search settings
	SearchParams map[string]any `json:"search_params,omitempty"` // Search-specific params

	// Behavior settings
	AutoCreateCollection bool          `json:"auto_create_collection,omitempty"`
	Timeout              time.Duration `json:"timeout,omitempty"`
	BatchSize            int           `json:"batch_size,omitempty"` // For batch operations

	// Consistency level: Strong, Session, Bounded, Eventually
	ConsistencyLevel string `json:"consistency_level,omitempty"`
}

// MilvusStore implements VectorStore using Milvus REST API (v2).
type MilvusStore struct {
	cfg     MilvusConfig
	baseURL string
	client  *http.Client
	logger  *zap.Logger

	ensureOnce sync.Once
	ensureErr  error
}

// NewMilvusStore creates a Milvus-backed VectorStore.
func NewMilvusStore(cfg MilvusConfig, logger *zap.Logger) *MilvusStore {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Apply defaults
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 19530
	}
	if cfg.Database == "" {
		cfg.Database = "default"
	}
	if cfg.PrimaryField == "" {
		cfg.PrimaryField = "id"
	}
	if cfg.VectorField == "" {
		cfg.VectorField = "vector"
	}
	if cfg.ContentField == "" {
		cfg.ContentField = "content"
	}
	if cfg.MetadataField == "" {
		cfg.MetadataField = "metadata"
	}
	if cfg.IndexType == "" {
		cfg.IndexType = MilvusIndexIVFFlat
	}
	if cfg.MetricType == "" {
		cfg.MetricType = MilvusMetricCosine
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 1000
	}
	if cfg.ConsistencyLevel == "" {
		cfg.ConsistencyLevel = "Strong"
	}

	// Set default index params based on index type
	if cfg.IndexParams == nil {
		cfg.IndexParams = defaultIndexParams(cfg.IndexType)
	}
	if cfg.SearchParams == nil {
		cfg.SearchParams = defaultSearchParams(cfg.IndexType)
	}

	// Build base URL
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}

	return &MilvusStore{
		cfg:     cfg,
		baseURL: baseURL,
		client:  &http.Client{Timeout: cfg.Timeout},
		logger:  logger.With(zap.String("component", "milvus_store")),
	}
}

// defaultIndexParams returns default index parameters for the given index type.
func defaultIndexParams(indexType MilvusIndexType) map[string]any {
	switch indexType {
	case MilvusIndexIVFFlat:
		return map[string]any{"nlist": 1024}
	case MilvusIndexHNSW:
		return map[string]any{"M": 16, "efConstruction": 256}
	case MilvusIndexIVFSQ8:
		return map[string]any{"nlist": 1024}
	case MilvusIndexIVFPQ:
		return map[string]any{"nlist": 1024, "m": 8, "nbits": 8}
	case MilvusIndexFlat:
		return map[string]any{}
	default:
		return map[string]any{"nlist": 1024}
	}
}

// defaultSearchParams returns default search parameters for the given index type.
func defaultSearchParams(indexType MilvusIndexType) map[string]any {
	switch indexType {
	case MilvusIndexIVFFlat, MilvusIndexIVFSQ8, MilvusIndexIVFPQ:
		return map[string]any{"nprobe": 16}
	case MilvusIndexHNSW:
		return map[string]any{"ef": 64}
	case MilvusIndexFlat:
		return map[string]any{}
	default:
		return map[string]any{"nprobe": 16}
	}
}

// milvusNamespace is used to generate stable UUIDs from document IDs.
var milvusNamespace = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

// milvusPointID generates a stable UUID from a document ID.
func milvusPointID(docID string) string {
	return uuid.NewSHA1(milvusNamespace, []byte(docID)).String()
}

// applyHeaders adds authentication and content-type headers to a request.
func (s *MilvusStore) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Token-based auth (Zilliz Cloud)
	if s.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	}

	// Basic auth
	if s.cfg.Username != "" && s.cfg.Password != "" {
		req.SetBasicAuth(s.cfg.Username, s.cfg.Password)
	}
}

// doJSON performs a JSON HTTP request and decodes the response.
func (s *MilvusStore) doJSON(ctx context.Context, method, path string, in any, out any) error {
	endpoint := s.baseURL + path

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
		s.logger.Debug("milvus request", zap.String("method", method), zap.String("path", path), zap.String("body", string(b)))
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	s.applyHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	s.logger.Debug("milvus response", zap.Int("status", resp.StatusCode), zap.String("body", string(respBody)))

	// Milvus REST API returns 200 even for errors, check the response body
	var baseResp struct {
		Code    int    `json:"code"`
		Message string `json:"message,omitempty"`
	}
	if err := json.Unmarshal(respBody, &baseResp); err == nil {
		if baseResp.Code != 0 {
			return fmt.Errorf("milvus error: code=%d message=%s", baseResp.Code, baseResp.Message)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("milvus request failed: method=%s path=%s status=%d body=%s",
			method, path, resp.StatusCode, string(respBody))
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// ensureCollection creates the collection if it doesn't exist.
func (s *MilvusStore) ensureCollection(ctx context.Context, vectorDim int) error {
	if !s.cfg.AutoCreateCollection {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("milvus collection name is required")
	}
	if vectorDim <= 0 {
		return fmt.Errorf("milvus vector dimension must be > 0")
	}

	s.ensureOnce.Do(func() {
		s.ensureErr = s.createCollectionIfNotExists(ctx, vectorDim)
	})

	return s.ensureErr
}

// createCollectionIfNotExists creates the collection with schema and index.
func (s *MilvusStore) createCollectionIfNotExists(ctx context.Context, vectorDim int) error {
	// Check if collection exists
	exists, err := s.collectionExists(ctx)
	if err != nil {
		s.logger.Warn("failed to check collection existence", zap.Error(err))
		// Continue to try creating
	}
	if exists {
		s.logger.Debug("collection already exists", zap.String("collection", s.cfg.Collection))
		return nil
	}

	// Create collection with schema
	if err := s.createCollection(ctx, vectorDim); err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	// Create index on vector field
	if err := s.createIndex(ctx); err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	// Load collection into memory
	if err := s.loadCollection(ctx); err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	s.logger.Info("collection created and loaded",
		zap.String("collection", s.cfg.Collection),
		zap.Int("dimension", vectorDim),
		zap.String("index_type", string(s.cfg.IndexType)))

	return nil
}

// collectionExists checks if the collection exists.
func (s *MilvusStore) collectionExists(ctx context.Context) (bool, error) {
	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			HasCollection bool `json:"has"`
		} `json:"data"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/collections/has", req, &resp); err != nil {
		return false, err
	}

	return resp.Data.HasCollection, nil
}

// createCollection creates a new collection with the specified schema.
func (s *MilvusStore) createCollection(ctx context.Context, vectorDim int) error {
	// Build schema
	schema := map[string]any{
		"autoId": false,
		"fields": []map[string]any{
			{
				"fieldName":   s.cfg.PrimaryField,
				"dataType":    "VarChar",
				"isPrimary":   true,
				"elementTypeParams": map[string]any{
					"max_length": 128,
				},
			},
			{
				"fieldName": s.cfg.VectorField,
				"dataType":  "FloatVector",
				"elementTypeParams": map[string]any{
					"dim": vectorDim,
				},
			},
			{
				"fieldName": s.cfg.ContentField,
				"dataType":  "VarChar",
				"elementTypeParams": map[string]any{
					"max_length": 65535,
				},
			},
			{
				"fieldName": s.cfg.MetadataField,
				"dataType":  "JSON",
			},
			{
				"fieldName": "doc_id",
				"dataType":  "VarChar",
				"elementTypeParams": map[string]any{
					"max_length": 256,
				},
			},
		},
	}

	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
		"schema":         schema,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/collections/create", req, &resp); err != nil {
		return err
	}

	return nil
}

// createIndex creates an index on the vector field.
func (s *MilvusStore) createIndex(ctx context.Context) error {
	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
		"indexParams": []map[string]any{
			{
				"fieldName":  s.cfg.VectorField,
				"indexName":  s.cfg.VectorField + "_idx",
				"metricType": string(s.cfg.MetricType),
				"indexType":  string(s.cfg.IndexType),
				"params":     s.cfg.IndexParams,
			},
		},
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/indexes/create", req, &resp); err != nil {
		return err
	}

	return nil
}

// loadCollection loads the collection into memory for searching.
func (s *MilvusStore) loadCollection(ctx context.Context) error {
	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/collections/load", req, &resp); err != nil {
		return err
	}

	return nil
}

// AddDocuments adds documents to the Milvus collection.
func (s *MilvusStore) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("milvus collection is required")
	}

	// Validate embeddings and determine vector dimension
	vectorDim := s.cfg.VectorDimension
	for i, doc := range docs {
		if doc.ID == "" {
			return fmt.Errorf("document[%d] has empty id", i)
		}
		if len(doc.Embedding) == 0 {
			return fmt.Errorf("document[%d] has no embedding", i)
		}
		if vectorDim == 0 {
			vectorDim = len(doc.Embedding)
		}
		if len(doc.Embedding) != vectorDim {
			return fmt.Errorf("document[%d] embedding dimension mismatch: got=%d want=%d",
				i, len(doc.Embedding), vectorDim)
		}
	}

	// Ensure collection exists
	if err := s.ensureCollection(ctx, vectorDim); err != nil {
		return err
	}

	// Process in batches
	batchSize := s.cfg.BatchSize
	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[i:end]

		if err := s.insertBatch(ctx, batch); err != nil {
			return fmt.Errorf("insert batch %d-%d: %w", i, end, err)
		}
	}

	s.logger.Debug("milvus upsert completed", zap.Int("count", len(docs)))
	return nil
}

// insertBatch inserts a batch of documents.
func (s *MilvusStore) insertBatch(ctx context.Context, docs []Document) error {
	// Build data array
	data := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		// Serialize metadata to JSON
		metadata := doc.Metadata
		if metadata == nil {
			metadata = make(map[string]any)
		}

		row := map[string]any{
			s.cfg.PrimaryField:  milvusPointID(doc.ID),
			s.cfg.VectorField:   doc.Embedding,
			s.cfg.ContentField:  truncateString(doc.Content, 65535),
			s.cfg.MetadataField: metadata,
			"doc_id":            doc.ID,
		}
		data = append(data, row)
	}

	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
		"data":           data,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			InsertCount int      `json:"insertCount"`
			InsertIds   []string `json:"insertIds"`
		} `json:"data"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/entities/insert", req, &resp); err != nil {
		return err
	}

	return nil
}

// Search searches for similar documents in the Milvus collection.
func (s *MilvusStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return nil, fmt.Errorf("milvus collection is required")
	}
	if topK <= 0 {
		return []VectorSearchResult{}, nil
	}
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is required")
	}

	// Build search request
	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
		"data":           [][]float64{queryEmbedding},
		"annsField":      s.cfg.VectorField,
		"limit":          topK,
		"outputFields":   []string{s.cfg.PrimaryField, s.cfg.ContentField, s.cfg.MetadataField, "doc_id"},
		"searchParams":   s.cfg.SearchParams,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    [][]struct {
			ID       string         `json:"id"`
			Distance float64        `json:"distance"`
			Entity   map[string]any `json:"entity"`
		} `json:"data"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/entities/search", req, &resp); err != nil {
		return nil, err
	}

	// Convert results
	results := make([]VectorSearchResult, 0)
	if len(resp.Data) > 0 {
		for _, hit := range resp.Data[0] {
			doc := Document{
				ID: hit.ID,
			}

			// Extract fields from entity
			if hit.Entity != nil {
				if docID, ok := hit.Entity["doc_id"].(string); ok {
					doc.ID = docID
				}
				if content, ok := hit.Entity[s.cfg.ContentField].(string); ok {
					doc.Content = content
				}
				if metadata, ok := hit.Entity[s.cfg.MetadataField].(map[string]any); ok {
					doc.Metadata = metadata
				}
			}

			// Convert distance to score based on metric type
			score := s.distanceToScore(hit.Distance)

			results = append(results, VectorSearchResult{
				Document: doc,
				Score:    score,
				Distance: hit.Distance,
			})
		}
	}

	return results, nil
}

// distanceToScore converts Milvus distance to a similarity score.
func (s *MilvusStore) distanceToScore(distance float64) float64 {
	switch s.cfg.MetricType {
	case MilvusMetricIP, MilvusMetricCosine:
		// For IP and Cosine, higher is better, distance is already similarity
		return distance
	case MilvusMetricL2:
		// For L2, lower is better, convert to similarity
		return 1.0 / (1.0 + distance)
	default:
		return 1.0 - distance
	}
}

// DeleteDocuments deletes documents from the Milvus collection.
func (s *MilvusStore) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("milvus collection is required")
	}

	// Convert document IDs to point IDs
	pointIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			pointIDs = append(pointIDs, milvusPointID(id))
		}
	}

	if len(pointIDs) == 0 {
		return nil
	}

	// Build filter expression
	filter := fmt.Sprintf("%s in [%s]", s.cfg.PrimaryField, formatStringList(pointIDs))

	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
		"filter":         filter,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/entities/delete", req, &resp); err != nil {
		return err
	}

	s.logger.Debug("milvus delete completed", zap.Int("count", len(pointIDs)))
	return nil
}

// UpdateDocument updates a document in the Milvus collection.
func (s *MilvusStore) UpdateDocument(ctx context.Context, doc Document) error {
	// Milvus doesn't have native update, so we delete and re-insert
	if err := s.DeleteDocuments(ctx, []string{doc.ID}); err != nil {
		s.logger.Warn("failed to delete document for update", zap.Error(err))
		// Continue with insert anyway
	}
	return s.AddDocuments(ctx, []Document{doc})
}

// Count returns the number of documents in the Milvus collection.
func (s *MilvusStore) Count(ctx context.Context) (int, error) {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return 0, fmt.Errorf("milvus collection is required")
	}

	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			RowCount int `json:"rowCount"`
		} `json:"data"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/collections/get_stats", req, &resp); err != nil {
		return 0, err
	}

	return resp.Data.RowCount, nil
}

// DropCollection drops the collection (use with caution).
func (s *MilvusStore) DropCollection(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("milvus collection is required")
	}

	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/collections/drop", req, &resp); err != nil {
		return err
	}

	s.logger.Info("collection dropped", zap.String("collection", s.cfg.Collection))
	return nil
}

// Flush flushes the collection to ensure data persistence.
func (s *MilvusStore) Flush(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("milvus collection is required")
	}

	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	// Note: Milvus 2.x REST API may not have a direct flush endpoint
	// This is a placeholder for when it becomes available
	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/collections/flush", req, &resp); err != nil {
		// Ignore flush errors as it may not be supported
		s.logger.Debug("flush may not be supported", zap.Error(err))
	}

	return nil
}

// Helper functions

// truncateString truncates a string to the specified maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// formatStringList formats a list of strings for Milvus filter expression.
func formatStringList(strs []string) string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return strings.Join(quoted, ", ")
}
