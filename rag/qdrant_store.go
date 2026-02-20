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

// QdrantConfig 配置了 Qdrant 矢量Store 执行 。
//
// 注释:
// - Qdrant点ID是UUID;AgentFlow从文档中获得稳定的UUID. 身份证
// - 文件内容/元数据储存在有效载荷中(最佳JSON)。
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

// QdrantStore使用Qdrant的REST API执行矢量Store.
type QdrantStore struct {
	cfg QdrantConfig

	baseURL string
	client  *http.Client
	logger  *zap.Logger

	ensureOnce sync.Once
	ensureErr  error
}

// 新克德兰特斯多尔创建了克德兰特后卫矢量斯多尔.
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
	// 从文档 ID (支持任意字符串输入) 得到的稳定 UUID 。
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

		// 如果收藏存在, Qdrant 返回 409 。
		if resp.StatusCode == http.StatusConflict {
			s.ensureErr = nil
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				s.ensureErr = fmt.Errorf("qdrant create collection failed: status=%d (failed to read body: %v)", resp.StatusCode, err)
				return
			}
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
		// 克德兰特公约。
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
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
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

	// 验证嵌入并确定矢量大小。
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

		// 从有效载荷(首选)中恢复原始的 doc ID.
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
			// 如果有效载荷不包括 doc id,则返回到指向ID.
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

// ListDocumentIDs returns a paginated list of document IDs stored in the Qdrant collection.
// It uses the scroll API to retrieve points and extracts the original document ID from the payload.
func (s *QdrantStore) ListDocumentIDs(ctx context.Context, limit int, offset int) ([]string, error) {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return nil, fmt.Errorf("qdrant collection is required")
	}
	if limit <= 0 {
		return []string{}, nil
	}

	// Qdrant scroll API uses an offset point ID for pagination.
	// We fetch offset+limit points and discard the first `offset`.
	fetchLimit := offset + limit

	req := struct {
		Limit       int  `json:"limit"`
		WithPayload any  `json:"with_payload"`
		WithVector  bool `json:"with_vector"`
	}{
		Limit: fetchLimit,
		WithPayload: map[string]any{
			"include": []string{s.cfg.PayloadIDField},
		},
		WithVector: false,
	}

	var resp struct {
		Result struct {
			Points []struct {
				ID      any            `json:"id"`
				Payload map[string]any `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}

	path := fmt.Sprintf("/collections/%s/points/scroll", url.PathEscape(s.cfg.Collection))
	if err := s.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, fmt.Errorf("qdrant scroll points: %w", err)
	}

	points := resp.Result.Points
	if offset >= len(points) {
		return []string{}, nil
	}

	end := offset + limit
	if end > len(points) {
		end = len(points)
	}

	ids := make([]string, 0, end-offset)
	for _, p := range points[offset:end] {
		if p.Payload != nil {
			if docID, ok := p.Payload[s.cfg.PayloadIDField].(string); ok && docID != "" {
				ids = append(ids, docID)
				continue
			}
		}
		// Fallback to point ID
		ids = append(ids, fmt.Sprint(p.ID))
	}

	return ids, nil
}

// ClearAll deletes all points from the Qdrant collection.
// It uses the delete-by-filter API with a match-all filter to remove all points
// while preserving the collection schema.
func (s *QdrantStore) ClearAll(ctx context.Context) error {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("qdrant collection is required")
	}

	// Delete all points using a filter that matches everything.
	req := struct {
		Filter map[string]any `json:"filter"`
	}{
		Filter: map[string]any{
			"must": []map[string]any{},
		},
	}

	path := fmt.Sprintf("/collections/%s/points/delete", url.PathEscape(s.cfg.Collection))
	if s.cfg.Wait == nil || *s.cfg.Wait {
		path += "?wait=true"
	}

	var resp any
	if err := s.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return fmt.Errorf("qdrant clear all points: %w", err)
	}

	s.logger.Info("all points cleared from collection", zap.String("collection", s.cfg.Collection))
	return nil
}
