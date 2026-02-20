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

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"go.uber.org/zap"
)

// PineconeConfig 配置 Pinecone 矢量执行 。
//
// 要使用Pinecone,你也需要:
// - BaseURL(数据-飞机主机,例如https://<index>-<project>.svc.<region>.pinecone.io),或
// - Index,在这种情况下,商店将通过控制器API解决主机.
type PineconeConfig struct {
	APIKey     string        `json:"api_key"`
	Index      string        `json:"index,omitempty"`     // Used to resolve BaseURL if BaseURL is empty
	BaseURL    string        `json:"base_url,omitempty"`  // Data-plane base URL (preferred if known)
	Namespace  string        `json:"namespace,omitempty"`
	Timeout    time.Duration `json:"timeout,omitempty"`

	ControllerBaseURL string `json:"controller_base_url,omitempty"` // Default: https://api.pinecone.io

	// 存储在元数据中的有效载荷字段 。
	MetadataContentField string `json:"metadata_content_field,omitempty"` // Default: "content"
}

// PineconeStore使用Pinecone的REST API执行矢量Store.
type PineconeStore struct {
	cfg    PineconeConfig
	logger *zap.Logger
	client *http.Client

	mu      sync.RWMutex
	baseURL string
}

// 新PineconeStore创建了皮内可酮后置矢量Store.
func NewPineconeStore(cfg PineconeConfig, logger *zap.Logger) *PineconeStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.ControllerBaseURL == "" {
		cfg.ControllerBaseURL = "https://api.pinecone.io"
	}
	if cfg.MetadataContentField == "" {
		cfg.MetadataContentField = "content"
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")

	return &PineconeStore{
		cfg:     cfg,
		logger:  logger.With(zap.String("component", "pinecone_store")),
		client:  tlsutil.SecureHTTPClient(cfg.Timeout),
		baseURL: baseURL,
	}
}

func (s *PineconeStore) ensureBaseURL(ctx context.Context) error {
	s.mu.RLock()
	if s.baseURL != "" {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	if strings.TrimSpace(s.cfg.Index) == "" {
		return fmt.Errorf("pinecone base_url is required when index is empty")
	}
	if strings.TrimSpace(s.cfg.APIKey) == "" {
		return fmt.Errorf("pinecone api_key is required")
	}

	// 通过控制器 API 解析主机: get/indexs/{index}
	controller := strings.TrimRight(strings.TrimSpace(s.cfg.ControllerBaseURL), "/")
	endpoint := fmt.Sprintf("%s/indexes/%s", controller, url.PathEscape(s.cfg.Index))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create describe-index request: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("Api-Key", s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("describe index %s: %w", s.cfg.Index, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		return fmt.Errorf("pinecone describe index failed: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var describe struct {
		Host string `json:"host"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&describe); err != nil {
		return fmt.Errorf("decode describe-index response: %w", err)
	}
	host := strings.TrimSpace(describe.Host)
	if host == "" {
		return fmt.Errorf("pinecone controller returned empty host for index %q", s.cfg.Index)
	}

	baseURL := host
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	s.mu.Lock()
	s.baseURL = baseURL
	s.mu.Unlock()

	return nil
}

func (s *PineconeStore) doJSON(ctx context.Context, method, path string, in any, out any) error {
	if err := s.ensureBaseURL(ctx); err != nil {
		return fmt.Errorf("resolve pinecone base URL: %w", err)
	}

	s.mu.RLock()
	baseURL := s.baseURL
	s.mu.RUnlock()
	endpoint := baseURL + path

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request %s %s: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
		return fmt.Errorf("pinecone request failed: method=%s path=%s status=%d body=%s", method, path, resp.StatusCode, string(raw))
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response for %s %s: %w", method, path, err)
	}
	return nil
}

func (s *PineconeStore) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.APIKey) == "" {
		return fmt.Errorf("pinecone api_key is required")
	}

	type vector struct {
		ID       string         `json:"id"`
		Values   []float64      `json:"values"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}

	vectors := make([]vector, 0, len(docs))
	for i, doc := range docs {
		if doc.ID == "" {
			return fmt.Errorf("document[%d] has empty id", i)
		}
		if len(doc.Embedding) == 0 {
			return fmt.Errorf("document[%d] has no embedding", i)
		}

		meta := make(map[string]any)
		for k, v := range doc.Metadata {
			meta[k] = v
		}
		if doc.Content != "" {
			// 最佳效果:在元数据中存储内容.
			if _, exists := meta[s.cfg.MetadataContentField]; !exists {
				meta[s.cfg.MetadataContentField] = doc.Content
			}
		}

		vectors = append(vectors, vector{
			ID:       doc.ID,
			Values:   doc.Embedding,
			Metadata: meta,
		})
	}

	req := struct {
		Vectors   []vector `json:"vectors"`
		Namespace string   `json:"namespace,omitempty"`
	}{
		Vectors:   vectors,
		Namespace: strings.TrimSpace(s.cfg.Namespace),
	}

	var resp any
	return s.doJSON(ctx, http.MethodPost, "/vectors/upsert", req, &resp)
}

func (s *PineconeStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	if topK <= 0 {
		return []VectorSearchResult{}, nil
	}
	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is required")
	}

	req := struct {
		Vector          []float64 `json:"vector"`
		TopK            int       `json:"topK"`
		Namespace       string    `json:"namespace,omitempty"`
		IncludeMetadata bool      `json:"includeMetadata"`
	}{
		Vector:          queryEmbedding,
		TopK:            topK,
		Namespace:       strings.TrimSpace(s.cfg.Namespace),
		IncludeMetadata: true,
	}

	var resp struct {
		Matches []struct {
			ID       string         `json:"id"`
			Score    float64        `json:"score"`
			Metadata map[string]any `json:"metadata,omitempty"`
		} `json:"matches"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/query", req, &resp); err != nil {
		return nil, fmt.Errorf("query vectors: %w", err)
	}

	out := make([]VectorSearchResult, 0, len(resp.Matches))
	for _, m := range resp.Matches {
		doc := Document{
			ID:       m.ID,
			Metadata: m.Metadata,
		}
		if m.Metadata != nil {
			if v, ok := m.Metadata[s.cfg.MetadataContentField]; ok {
				if s, ok := v.(string); ok {
					doc.Content = s
				}
			}
		}
		out = append(out, VectorSearchResult{
			Document:  doc,
			Score:     m.Score,
			Distance:  1.0 - m.Score,
		})
	}

	return out, nil
}

func (s *PineconeStore) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	req := struct {
		IDs       []string `json:"ids"`
		Namespace string   `json:"namespace,omitempty"`
	}{
		IDs:       ids,
		Namespace: strings.TrimSpace(s.cfg.Namespace),
	}

	var resp any
	return s.doJSON(ctx, http.MethodPost, "/vectors/delete", req, &resp)
}

func (s *PineconeStore) UpdateDocument(ctx context.Context, doc Document) error {
	return s.AddDocuments(ctx, []Document{doc})
}

func (s *PineconeStore) Count(ctx context.Context) (int, error) {
	req := struct {
		Namespace string `json:"namespace,omitempty"`
	}{
		Namespace: strings.TrimSpace(s.cfg.Namespace),
	}

	var resp struct {
		TotalVectorCount int `json:"totalVectorCount"`
		Namespaces       map[string]struct {
			VectorCount int `json:"vectorCount"`
		} `json:"namespaces"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/describe_index_stats", req, &resp); err != nil {
		return 0, fmt.Errorf("describe index stats: %w", err)
	}

	if ns := strings.TrimSpace(s.cfg.Namespace); ns != "" && resp.Namespaces != nil {
		if st, ok := resp.Namespaces[ns]; ok {
			return st.VectorCount, nil
		}
	}
	return resp.TotalVectorCount, nil
}

// ListDocumentIDs returns a paginated list of vector IDs stored in the Pinecone index.
// It uses the list vectors API endpoint. The offset parameter is used to skip results
// by fetching offset+limit IDs and discarding the first offset entries.
func (s *PineconeStore) ListDocumentIDs(ctx context.Context, limit int, offset int) ([]string, error) {
	if strings.TrimSpace(s.cfg.APIKey) == "" {
		return nil, fmt.Errorf("pinecone api_key is required")
	}
	if limit <= 0 {
		return []string{}, nil
	}

	// Pinecone list endpoint returns vector IDs with pagination via paginationToken.
	// We fetch offset+limit IDs and skip the first offset.
	fetchLimit := offset + limit

	path := fmt.Sprintf("/vectors/list?limit=%d", fetchLimit)
	if ns := strings.TrimSpace(s.cfg.Namespace); ns != "" {
		path += "&namespace=" + url.QueryEscape(ns)
	}

	var resp struct {
		Vectors []struct {
			ID string `json:"id"`
		} `json:"vectors"`
	}

	if err := s.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, fmt.Errorf("pinecone list vectors: %w", err)
	}

	if offset >= len(resp.Vectors) {
		return []string{}, nil
	}

	end := offset + limit
	if end > len(resp.Vectors) {
		end = len(resp.Vectors)
	}

	ids := make([]string, 0, end-offset)
	for _, v := range resp.Vectors[offset:end] {
		ids = append(ids, v.ID)
	}

	return ids, nil
}

// ClearAll deletes all vectors from the Pinecone index (or namespace).
func (s *PineconeStore) ClearAll(ctx context.Context) error {
	req := struct {
		DeleteAll bool   `json:"deleteAll"`
		Namespace string `json:"namespace,omitempty"`
	}{
		DeleteAll: true,
		Namespace: strings.TrimSpace(s.cfg.Namespace),
	}

	var resp any
	if err := s.doJSON(ctx, http.MethodPost, "/vectors/delete", req, &resp); err != nil {
		return fmt.Errorf("pinecone delete all: %w", err)
	}

	s.logger.Info("all vectors cleared from index")
	return nil
}
