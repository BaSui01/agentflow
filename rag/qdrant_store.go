package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// QdrantConfig configures the Qdrant VectorStore implementation.
//
// Notes:
// - Qdrant point IDs are UUIDs; AgentFlow derives a stable UUID from Document.ID.
// - Document content/metadata are stored in payload (best-effort JSON).
type QdrantConfig struct {
	Host       string        `json:"host"`
	Port       int           `json:"port"`
	BaseURL    string        `json:"base_url,omitempty"`
	APIKey     string        `json:"api_key,omitempty"`
	Collection string        `json:"collection"`
	Timeout    time.Duration `json:"timeout,omitempty"`

	AutoCreateCollection bool   `json:"auto_create_collection,omitempty"`
	Distance             string `json:"distance,omitempty"`     // Cosine (default), Dot, Euclid
	VectorSize           int    `json:"vector_size,omitempty"`  // Optional override; defaults to len(embedding)
	Wait                 *bool  `json:"wait,omitempty"`         // Wait for operation completion (default true)
	PayloadContentField  string `json:"payload_content_field"`  // Payload key for document content (default "content")
	PayloadMetadataField string `json:"payload_metadata_field"` // Payload key for document metadata (default "metadata")
	PayloadIDField       string `json:"payload_id_field"`       // Payload key for original document ID (default "doc_id")
}

// QdrantStore implements VectorStore using Qdrant's REST API.
type QdrantStore struct {
	cfg QdrantConfig

	baseURL string
	client  *http.Client
	logger  *zap.Logger

	ensureOnce sync.Once
	ensureErr  error
}

// NewQdrantStore creates a Qdrant-backed VectorStore.
func NewQdrantStore(cfg QdrantConfig, logger *zap.Logger) *QdrantStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 6333
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Distance == "" {
		cfg.Distance = "Cosine"
	}
	if cfg.PayloadContentField == "" {
		cfg.PayloadContentField = "content"
	}
	if cfg.PayloadMetadataField == "" {
		cfg.PayloadMetadataField = "metadata"
	}
	if cfg.PayloadIDField == "" {
		cfg.PayloadIDField = "doc_id"
	}
	if cfg.Wait == nil {
		wait := true
		cfg.Wait = &wait
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}

	return &QdrantStore{
		cfg:     cfg,
		baseURL: baseURL,
		client:  &http.Client{Timeout: cfg.Timeout},
		logger:  logger.With(zap.String("component", "qdrant_store")),
	}
}

var qdrantNamespace = uuid.MustParse("d9bde6d4-4f3a-4e6b-8f7a-5d8d2f3b4c1a")

func qdrantPointID(docID string) string {
	// Stable UUID derived from document ID (supports any string input).
	return uuid.NewSHA1(qdrantNamespace, []byte(docID)).String()
}

func (s *QdrantStore) ensureCollection(ctx context.Context, vectorSize int) error {
	if !s.cfg.AutoCreateCollection {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("qdrant collection is required")
	}
	if vectorSize <= 0 {
		return fmt.Errorf("qdrant vector size must be > 0")
	}

	s.ensureOnce.Do(func() {
		body := map[string]any{
			"vectors": map[string]any{
				"size":     vectorSize,
				"distance": s.cfg.Distance,
			},
		}

		endpoint := fmt.Sprintf("%s/collections/%s", s.baseURL, url.PathEscape(s.cfg.Collection))
		reqBody, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(reqBody))
		if err != nil {
			s.ensureErr = err
			return
		}
		s.applyHeaders(req)

		resp, err := s.client.Do(req)
		if err != nil {
			s.ensureErr = err
			return
		}
		defer resp.Body.Close()

		// Qdrant returns 409 if collection exists.
		if resp.StatusCode == http.StatusConflict {
			s.ensureErr = nil
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, _ := io.ReadAll(resp.Body)
			s.ensureErr = fmt.Errorf("qdrant create collection failed: status=%d body=%s", resp.StatusCode, string(raw))
			return
		}
		s.ensureErr = nil
	})

	return s.ensureErr
}

func (s *QdrantStore) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(s.cfg.APIKey) != "" {
		// Qdrant convention.
		req.Header.Set("api-key", s.cfg.APIKey)
	}
}

func (s *QdrantStore) doJSON(ctx context.Context, method, path string, in any, out any) error {
	endpoint := s.baseURL + path

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	s.applyHeaders(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant request failed: method=%s path=%s status=%d body=%s", method, path, resp.StatusCode, string(raw))
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}

func (s *QdrantStore) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("qdrant collection is required")
	}

	// Validate embeddings and determine vector size.
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
			return fmt.Errorf("document[%d] embedding dimension mismatch: got=%d want=%d", i, len(doc.Embedding), vectorSize)
		}
	}

	if err := s.ensureCollection(ctx, vectorSize); err != nil {
		return err
	}

	type point struct {
		ID      string         `json:"id"`
		Vector  []float64      `json:"vector"`
		Payload map[string]any `json:"payload,omitempty"`
	}

	points := make([]point, 0, len(docs))
	for _, doc := range docs {
		payload := map[string]any{
			s.cfg.PayloadIDField:       doc.ID,
			s.cfg.PayloadContentField:  doc.Content,
			s.cfg.PayloadMetadataField: doc.Metadata,
		}
		points = append(points, point{
			ID:      qdrantPointID(doc.ID),
			Vector:  doc.Embedding,
			Payload: payload,
		})
	}

	req := struct {
		Points []point `json:"points"`
	}{
		Points: points,
	}

	path := fmt.Sprintf("/collections/%s/points", url.PathEscape(s.cfg.Collection))
	if s.cfg.Wait == nil || *s.cfg.Wait {
		path += "?wait=true"
	}

	var resp any
	if err := s.doJSON(ctx, http.MethodPut, path, req, &resp); err != nil {
		return err
	}

	s.logger.Debug("qdrant upsert completed", zap.Int("count", len(docs)))
	return nil
}

func (s *QdrantStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return nil, fmt.Errorf("qdrant collection is required")
	}
	if topK <= 0 {
		return []VectorSearchResult{}, nil
	}
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is required")
	}

	req := struct {
		Vector      []float64 `json:"vector"`
		Limit       int       `json:"limit"`
		WithPayload bool      `json:"with_payload"`
		WithVector  bool      `json:"with_vector"`
	}{
		Vector:      queryEmbedding,
		Limit:       topK,
		WithPayload: true,
		WithVector:  false,
	}

	type qdrantResult struct {
		ID      any            `json:"id"`
		Score   float64        `json:"score"`
		Payload map[string]any `json:"payload"`
	}
	var resp struct {
		Result []qdrantResult `json:"result"`
		Status string         `json:"status"`
	}

	path := fmt.Sprintf("/collections/%s/points/search", url.PathEscape(s.cfg.Collection))
	if err := s.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}

	out := make([]VectorSearchResult, 0, len(resp.Result))
	for _, r := range resp.Result {
		doc := Document{}

		// Recover original doc ID from payload (preferred).
		if r.Payload != nil {
			if v, ok := r.Payload[s.cfg.PayloadIDField]; ok {
				if s, ok := v.(string); ok {
					doc.ID = s
				}
			}
			if v, ok := r.Payload[s.cfg.PayloadContentField]; ok {
				if s, ok := v.(string); ok {
					doc.Content = s
				}
			}
			if v, ok := r.Payload[s.cfg.PayloadMetadataField]; ok {
				if m, ok := v.(map[string]any); ok {
					doc.Metadata = m
				}
			}
		}

		if doc.ID == "" {
			// Fallback to point ID if payload does not include doc_id.
			doc.ID = fmt.Sprint(r.ID)
		}

		score := r.Score
		out = append(out, VectorSearchResult{
			Document:  doc,
			Score:     score,
			Distance:  1.0 - score,
		})
	}

	return out, nil
}

func (s *QdrantStore) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("qdrant collection is required")
	}

	points := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		points = append(points, qdrantPointID(id))
	}

	req := struct {
		Points []string `json:"points"`
	}{
		Points: points,
	}

	path := fmt.Sprintf("/collections/%s/points/delete", url.PathEscape(s.cfg.Collection))
	if s.cfg.Wait == nil || *s.cfg.Wait {
		path += "?wait=true"
	}

	var resp any
	if err := s.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return err
	}
	return nil
}

func (s *QdrantStore) UpdateDocument(ctx context.Context, doc Document) error {
	return s.AddDocuments(ctx, []Document{doc})
}

func (s *QdrantStore) Count(ctx context.Context) (int, error) {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return 0, fmt.Errorf("qdrant collection is required")
	}

	req := struct {
		Exact bool `json:"exact"`
	}{
		Exact: true,
	}

	var resp struct {
		Result struct {
			Count int `json:"count"`
		} `json:"result"`
	}

	path := fmt.Sprintf("/collections/%s/points/count", url.PathEscape(s.cfg.Collection))
	if err := s.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return 0, err
	}

	return resp.Result.Count, nil
}
