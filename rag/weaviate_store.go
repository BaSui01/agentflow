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

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// WeaviateConfig配置了Weaviate矢量Store执行.
//
// Weaviate是一个开源向量数据库,支持:
// - 具有多距离测量标准的矢量搜索
// - BM25关键字搜索
// - 混合搜索(合并向量和BM25)
// - 用于灵活查询的图表QL API
// - 自动计划管理
type WeaviateConfig struct {
	// 连接设置
	Host    string `json:"host"`              // Weaviate host (default: localhost)
	Port    int    `json:"port"`              // Weaviate port (default: 8080)
	Scheme  string `json:"scheme,omitempty"`  // http or https (default: http)
	BaseURL string `json:"base_url,omitempty"` // Full base URL (overrides host/port/scheme)

	// 认证
	APIKey string `json:"api_key,omitempty"` // API key for authentication

	// 类/收集设置
	ClassName string `json:"class_name"` // Weaviate class name (required)

	// Schema 设置
	AutoCreateSchema bool   `json:"auto_create_schema,omitempty"` // Auto-create class if not exists
	VectorIndexType  string `json:"vector_index_type,omitempty"`  // hnsw (default), flat
	Distance         string `json:"distance,omitempty"`           // cosine (default), dot, l2, hamming, manhattan

	// 矢量设置
	VectorSize int `json:"vector_size,omitempty"` // Vector dimension (optional, auto-detected)

	// 混合搜索设置
	HybridAlpha float64 `json:"hybrid_alpha,omitempty"` // Alpha for hybrid search (0=BM25, 1=vector, default: 0.5)

	// 超时设置
	Timeout time.Duration `json:"timeout,omitempty"` // Request timeout (default: 30s)

	// 属性字段名称
	ContentProperty  string `json:"content_property,omitempty"`  // Property for document content (default: content)
	MetadataProperty string `json:"metadata_property,omitempty"` // Property for document metadata (default: metadata)
	DocIDProperty    string `json:"doc_id_property,omitempty"`   // Property for original document ID (default: docId)
}

// Weaviate Store 使用 Weaviate 的 REST 和 GraphQL API 执行 VectorStore 。
type WeaviateStore struct {
	cfg WeaviateConfig

	baseURL string
	client  *http.Client
	logger  *zap.Logger

	ensureOnce sync.Once
	ensureErr  error
}

// NewWeaviate Store创建了由Weaviate支撑的"矢量".
func NewWeaviateStore(cfg WeaviateConfig, logger *zap.Logger) *WeaviateStore {
	if logger == nil {
		logger = zap.NewNop()
	}

	// 应用默认
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

	// 构建基础 URL
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("%s://%s:%d", cfg.Scheme, cfg.Host, cfg.Port)
	}

	return &WeaviateStore{
		cfg:     cfg,
		baseURL: baseURL,
		client:  tlsutil.SecureHTTPClient(cfg.Timeout),
		logger:  logger.With(zap.String("component", "weaviate_store")),
	}
}

// 用于从文档ID中生成决定性的UUID。
var weaviateNamespace = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

// weaviate ObjectID从文档ID生成一个决定性的UUID.
func weaviateObjectID(docID string) string {
	return uuid.NewSHA1(weaviateNamespace, []byte(docID)).String()
}

// 应用程序标题为Weaviate请求设置常见标题。
func (s *WeaviateStore) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(s.cfg.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	}
}

// doJSON执行JSON HTTP请求去织造.
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

// 保证Schema创建Weaviate类 如果它不存在。
func (s *WeaviateStore) ensureSchema(ctx context.Context, vectorSize int) error {
	if !s.cfg.AutoCreateSchema {
		return nil
	}
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return fmt.Errorf("weaviate class_name is required")
	}

	s.ensureOnce.Do(func() {
		// 检查类是否存在
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

		// 类已存在
		if checkResp.StatusCode == http.StatusOK {
			s.logger.Debug("weaviate class already exists", zap.String("class", s.cfg.ClassName))
			return
		}

		// 创建类计划
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

// BuildClassSchema 构建了Weaviate类计划定义.
func (s *WeaviateStore) buildClassSchema(vectorSize int) map[string]any {
	// 向 Weaviate 格式映射距离度量尺
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

	// 为 BM25 搜索添加倒数索引配置
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

// 添加文档将文档添加到 Weaviate 商店 。
func (s *WeaviateStore) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return fmt.Errorf("weaviate class_name is required")
	}

	// 校验文档并确定向量大小
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

	// 确保存在计划
	if err := s.ensureSchema(ctx, vectorSize); err != nil {
		return err
	}

	// 构建批量请求
	objects := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		// 将元数据序列化为 JSON
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

	// 批量升级
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

	// 检查批量回复中的错误
	for _, r := range batchResp.Results {
		if r.Result.Errors != nil && len(r.Result.Errors.Error) > 0 {
			return fmt.Errorf("weaviate batch error for object %s: %s",
				r.ID, r.Result.Errors.Error[0].Message)
		}
	}

	s.logger.Debug("weaviate batch upsert completed", zap.Int("count", len(docs)))
	return nil
}

// 搜索执行向量相似性搜索 。
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

	// 构建矢量搜索的图QL查询
	query := s.buildVectorSearchQuery(queryEmbedding, topK)

	return s.executeGraphQLSearch(ctx, query)
}

// HybridSearch)进行混合搜索,结合了矢量相似性和BM25.
func (s *WeaviateStore) HybridSearch(ctx context.Context, queryText string, queryEmbedding []float64, topK int) ([]VectorSearchResult, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return nil, fmt.Errorf("weaviate class_name is required")
	}
	if topK <= 0 {
		return []VectorSearchResult{}, nil
	}

	// 构建用于混合搜索的图形QL查询
	query := s.buildHybridSearchQuery(queryText, queryEmbedding, topK)

	return s.executeGraphQLSearch(ctx, query)
}

// BM25Search执行基于关键词的BM25搜索.
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

	// 为 BM25 搜索构建图形QL 查询
	query := s.buildBM25SearchQuery(queryText, topK)

	return s.executeGraphQLSearch(ctx, query)
}

// 构建 VectorSearchQuery 构建用于向量搜索的 GraphQL 查询 。
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

// 构建 HybridSearchQuery 构建用于混合搜索的 GraphQL 查询 。
func (s *WeaviateStore) buildHybridSearchQuery(queryText string, vector []float64, topK int) map[string]any {
	vectorStr := ""
	if len(vector) > 0 {
		vectorStr = fmt.Sprintf(`, vector: %s`, formatVector(vector))
	}

	// GraphQL 的逃逸查询文本
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

// 建设BM25SearchQuery为BM25搜索构建了GraphQL查询.
func (s *WeaviateStore) buildBM25SearchQuery(queryText string, topK int) map[string]any {
	// GraphQL 的逃逸查询文本
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

// 执行 GraphQLSearch 执行 GraphQL 搜索查询和剖析结果。
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

	// 检查图QL出错
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("weaviate graphql error: %s", resp.Errors[0].Message)
	}

	// 分析结果
	results := resp.Data.Get[s.cfg.ClassName]
	out := make([]VectorSearchResult, 0, len(results))

	for _, r := range results {
		doc := Document{
			ID:      r.DocID,
			Content: r.Content,
		}

		// 解析元数据 JSON
		if r.Metadata != "" && r.Metadata != "{}" {
			var meta map[string]any
			if err := json.Unmarshal([]byte(r.Metadata), &meta); err == nil {
				doc.Metadata = meta
			}
		}

		// 计算得分和距离
		var score, distance float64
		if r.Additional.Certainty != nil {
			score = *r.Additional.Certainty
			distance = 1.0 - score
		} else if r.Additional.Distance != nil {
			distance = *r.Additional.Distance
			// 将距离转换为分数( 假设余弦距离)
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

// 删除文档用其标识删除文档。
func (s *WeaviateStore) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return fmt.Errorf("weaviate class_name is required")
	}

	// 删除每个文档的 Weaviate 对象ID
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

		// 404是可以接受的(对象不存在)
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("weaviate delete failed: status=%d", resp.StatusCode)
		}
	}

	s.logger.Debug("weaviate delete completed", zap.Int("count", len(ids)))
	return nil
}

// 更新文档更新一个文档(upsert).
func (s *WeaviateStore) UpdateDocument(ctx context.Context, doc Document) error {
	return s.AddDocuments(ctx, []Document{doc})
}

// 计数返回收藏中的文档总数。
func (s *WeaviateStore) Count(ctx context.Context) (int, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return 0, fmt.Errorf("weaviate class_name is required")
	}

	// 使用 GraphQL 汇总查询
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

// 删除Class删除整个Weviate类(谨慎使用).
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

	// 重置保证, 这样可以重新创建计划
	s.ensureOnce = sync.Once{}
	s.ensureErr = nil

	s.logger.Info("weaviate class deleted", zap.String("class", s.cfg.ClassName))
	return nil
}

// GetSchema 返回当前类计划 。
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

// ListDocumentIDs returns a paginated list of document IDs stored in the Weaviate class.
// It uses a GraphQL query to retrieve the docId property with limit and offset.
func (s *WeaviateStore) ListDocumentIDs(ctx context.Context, limit int, offset int) ([]string, error) {
	if strings.TrimSpace(s.cfg.ClassName) == "" {
		return nil, fmt.Errorf("weaviate class_name is required")
	}
	if limit <= 0 {
		return []string{}, nil
	}

	graphql := fmt.Sprintf(`{
		Get {
			%s(
				limit: %d
				offset: %d
			) {
				%s
			}
		}
	}`, s.cfg.ClassName, limit, offset, s.cfg.DocIDProperty)

	query := map[string]any{
		"query": graphql,
	}

	var resp struct {
		Data struct {
			Get map[string][]map[string]any `json:"Get"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v1/graphql", query, &resp); err != nil {
		return nil, fmt.Errorf("weaviate list document IDs: %w", err)
	}

	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("weaviate graphql error: %s", resp.Errors[0].Message)
	}

	results := resp.Data.Get[s.cfg.ClassName]
	ids := make([]string, 0, len(results))
	for _, r := range results {
		if docID, ok := r[s.cfg.DocIDProperty].(string); ok && docID != "" {
			ids = append(ids, docID)
		}
	}

	return ids, nil
}

// ClearAll deletes the entire Weaviate class and resets the schema guard so it
// can be recreated on the next AddDocuments call.
func (s *WeaviateStore) ClearAll(ctx context.Context) error {
	if err := s.DeleteClass(ctx); err != nil {
		return fmt.Errorf("weaviate clear all: %w", err)
	}
	s.logger.Info("weaviate class cleared", zap.String("class", s.cfg.ClassName))
	return nil
}

// 辅助功能

// 格式Vector将一个浮点64切片作为JSON数组字符串。
func formatVector(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// 逃出 GraphQLSTring 摆脱了用于 GraphQL 查询的字符串。
func escapeGraphQLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}
