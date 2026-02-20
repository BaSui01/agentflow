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

// MilvusIndexType定义了Milvus向量搜索的索引类型.
type MilvusIndexType string

const (
	// MilvusIndexIVFFlat是IVF FLAT指数类型(速度和准确性的良好平衡).
	MilvusIndexIVFFlat MilvusIndexType = "IVF_FLAT"
	// MilvusIndexHNSW是HNSW指数类型(高精度,多为内存).
	MilvusIndexHNSW MilvusIndexType = "HNSW"
	// MilvusIndexFlat是FLAT指数类型(Brute force,最高精度).
	MilvusIndexFlat MilvusIndexType = "FLAT"
	// MilvusIndexIVFSQ8是IVF SQ8指数类型(压缩,速度快但准确度更低).
	MilvusIndexIVFSQ8 MilvusIndexType = "IVF_SQ8"
	// MilvusIndexIVFPQ是IVF PQ指数类型(高度压缩,速度最快但最不准确).
	MilvusIndexIVFPQ MilvusIndexType = "IVF_PQ"
)

// MilvusMetricType定义了Milvus向量搜索的距离度量.
type MilvusMetricType string

const (
	// MilvusMetricL2是欧几里得相距度量衡.
	MilvusMetricL2 MilvusMetricType = "L2"
	// MilvusMetricIP是内产物(宇宙相似性)的度量衡.
	MilvusMetricIP MilvusMetricType = "IP"
	// 密尔武斯Metric Cosine是克辛类似度量衡.
	MilvusMetricCosine MilvusMetricType = "COSINE"
)

// MilvusConfig配置了Milvus矢量Store执行.
type MilvusConfig struct {
	// 连接设置
	Host    string `json:"host"`
	Port    int    `json:"port"`
	BaseURL string `json:"base_url,omitempty"` // Override host:port if set

	// 认证
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"` // For Zilliz Cloud

	// 收藏设置
	Collection string `json:"collection"`
	Database   string `json:"database,omitempty"` // Default: "default"

	// Schema 设置
	VectorDimension int    `json:"vector_dimension,omitempty"` // Required for auto-create
	PrimaryField    string `json:"primary_field,omitempty"`    // Default: "id"
	VectorField     string `json:"vector_field,omitempty"`     // Default: "vector"
	ContentField    string `json:"content_field,omitempty"`    // Default: "content"
	MetadataField   string `json:"metadata_field,omitempty"`   // Default: "metadata"

	// 索引设置
	IndexType   MilvusIndexType  `json:"index_type,omitempty"`   // Default: IVF_FLAT
	MetricType  MilvusMetricType `json:"metric_type,omitempty"`  // Default: COSINE
	IndexParams map[string]any   `json:"index_params,omitempty"` // Index-specific params

	// 搜索设置
	SearchParams map[string]any `json:"search_params,omitempty"` // Search-specific params

	// 行为设置
	AutoCreateCollection bool          `json:"auto_create_collection,omitempty"`
	Timeout              time.Duration `json:"timeout,omitempty"`
	BatchSize            int           `json:"batch_size,omitempty"` // For batch operations

	// 一致性水平:强、会、会、会、终
	ConsistencyLevel string `json:"consistency_level,omitempty"`
}

// 米尔武斯斯托尔执行矢量Store使用米尔武斯REST API(v2).
type MilvusStore struct {
	cfg     MilvusConfig
	baseURL string
	client  *http.Client
	logger  *zap.Logger

	ensureOnce sync.Once
	ensureErr  error
}

// NewMilvusStore 创建了由米尔武斯支撑的"矢量".
func NewMilvusStore(cfg MilvusConfig, logger *zap.Logger) *MilvusStore {
	if logger == nil {
		logger = zap.NewNop()
	}

	// 应用默认
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

	// 根据索引类型设定默认索引参数
	if cfg.IndexParams == nil {
		cfg.IndexParams = defaultIndexParams(cfg.IndexType)
	}
	if cfg.SearchParams == nil {
		cfg.SearchParams = defaultSearchParams(cfg.IndexType)
	}

	// 构建基础 URL
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}

	return &MilvusStore{
		cfg:     cfg,
		baseURL: baseURL,
		client:  tlsutil.SecureHTTPClient(cfg.Timeout),
		logger:  logger.With(zap.String("component", "milvus_store")),
	}
}

// 默认 IndexParams 返回给定索引类型的默认索引参数。
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

// 默认SearchParams返回给定索引类型的默认搜索参数。
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

// milvusNamespace用于从文档ID生成稳定的UUID.
var milvusNamespace = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

// milvusPointID从文档ID生成稳定的UUID.
func milvusPointID(docID string) string {
	return uuid.NewSHA1(milvusNamespace, []byte(docID)).String()
}

// 应用程序标题为请求添加认证和内容类型标题。
func (s *MilvusStore) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// 基于托肯的认证( Zilliz Cloud)
	if s.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.Token)
	}

	// 基本认证
	if s.cfg.Username != "" && s.cfg.Password != "" {
		req.SetBasicAuth(s.cfg.Username, s.cfg.Password)
	}
}

// doJSON执行JSON HTTP请求并解码响应.
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

	// Milvus REST API 返回 200 甚至是错误, 请检查响应体
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

// 如果收藏不存在,则确保收藏创建。
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

// 创建 Collection If NotExists 创建了图集和索引。
func (s *MilvusStore) createCollectionIfNotExists(ctx context.Context, vectorDim int) error {
	// 检查收藏是否存在
	exists, err := s.collectionExists(ctx)
	if err != nil {
		s.logger.Warn("failed to check collection existence", zap.Error(err))
		// 继续尝试创建
	}
	if exists {
		s.logger.Debug("collection already exists", zap.String("collection", s.cfg.Collection))
		return nil
	}

	// 用计划创建收藏
	if err := s.createCollection(ctx, vectorDim); err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	// 在向量字段创建索引
	if err := s.createIndex(ctx); err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	// 将收藏装入内存
	if err := s.loadCollection(ctx); err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	s.logger.Info("collection created and loaded",
		zap.String("collection", s.cfg.Collection),
		zap.Int("dimension", vectorDim),
		zap.String("index_type", string(s.cfg.IndexType)))

	return nil
}

// 收藏 Exists 检查收藏是否存在 。
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
		return false, fmt.Errorf("check collection existence: %w", err)
	}

	return resp.Data.HasCollection, nil
}

// 创建 Collection 以指定的方案创建新收藏。
func (s *MilvusStore) createCollection(ctx context.Context, vectorDim int) error {
	// 构建计划
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
		return fmt.Errorf("create collection %s: %w", s.cfg.Collection, err)
	}

	return nil
}

// 创建 Index 在向量字段上创建索引。
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
		return fmt.Errorf("create index on %s: %w", s.cfg.VectorField, err)
	}

	return nil
}

// 加载 Collection 将收藏装入内存以进行搜索。
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
		return fmt.Errorf("load collection %s: %w", s.cfg.Collection, err)
	}

	return nil
}

// 添加文档将文档添加到米尔武斯收藏中 。
func (s *MilvusStore) AddDocuments(ctx context.Context, docs []Document) error {
	if len(docs) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("milvus collection is required")
	}

	// 验证嵌入和确定矢量维度
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

	// 确保收藏存在
	if err := s.ensureCollection(ctx, vectorDim); err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}

	// 分批处理
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

// 插入批次文档。
func (s *MilvusStore) insertBatch(ctx context.Context, docs []Document) error {
	// 构建数据数组
	data := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		// 将元数据序列化为 JSON
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
		return fmt.Errorf("insert entities: %w", err)
	}

	return nil
}

// 在 Milvus 收藏中搜索类似的文档 。
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

	// 构建搜索请求
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
		return nil, fmt.Errorf("search entities: %w", err)
	}

	// 转换结果
	results := make([]VectorSearchResult, 0)
	if len(resp.Data) > 0 {
		for _, hit := range resp.Data[0] {
			doc := Document{
				ID: hit.ID,
			}

			// 从实体提取字段
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

			// 根据公制类型将距离转换为分数
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

// 距离 ToScore将Milvus的距离转换成相似的分数。
func (s *MilvusStore) distanceToScore(distance float64) float64 {
	switch s.cfg.MetricType {
	case MilvusMetricIP, MilvusMetricCosine:
		// 对IP和Cosine来说,更高更好,距离已经很相似
		return distance
	case MilvusMetricL2:
		// 对于L2, 下调更好, 转换为相似性
		return 1.0 / (1.0 + distance)
	default:
		return 1.0 - distance
	}
}

// 删除文档删除米尔武斯收藏中的文档。
func (s *MilvusStore) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return fmt.Errorf("milvus collection is required")
	}

	// 将文档ID转换为指向ID
	pointIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			pointIDs = append(pointIDs, milvusPointID(id))
		}
	}

	if len(pointIDs) == 0 {
		return nil
	}

	// 构建过滤表达式
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
		return fmt.Errorf("delete entities: %w", err)
	}

	s.logger.Debug("milvus delete completed", zap.Int("count", len(pointIDs)))
	return nil
}

// 更新文档更新米尔武斯收藏中的文档。
func (s *MilvusStore) UpdateDocument(ctx context.Context, doc Document) error {
	// Milvus没有本地更新 所以我们删除并重新插入
	if err := s.DeleteDocuments(ctx, []string{doc.ID}); err != nil {
		s.logger.Warn("failed to delete document for update", zap.Error(err))
		// 无论如何继续插入
	}
	return s.AddDocuments(ctx, []Document{doc})
}

// 计数返回 Milvus 收藏中的文档数 。
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
		return 0, fmt.Errorf("get collection stats: %w", err)
	}

	return resp.Data.RowCount, nil
}

// Drop Collection 将收藏放下( 谨慎使用) 。
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
		return fmt.Errorf("drop collection %s: %w", s.cfg.Collection, err)
	}

	s.logger.Info("collection dropped", zap.String("collection", s.cfg.Collection))
	return nil
}

// Flush冲出收集器,以确保数据的持久性.
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

	// 注:Milvus 2.x REST API可能没有直接冲出端点
	// 这是可供使用的占位符
	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/collections/flush", req, &resp); err != nil {
		// 忽略冲出错误, 因为可能不被支持
		s.logger.Debug("flush may not be supported", zap.Error(err))
	}

	return nil
}

// ListDocumentIDs returns a paginated list of document IDs stored in the Milvus collection.
// It uses the query API with an output field of "doc_id" to retrieve original document IDs.
func (s *MilvusStore) ListDocumentIDs(ctx context.Context, limit int, offset int) ([]string, error) {
	if strings.TrimSpace(s.cfg.Collection) == "" {
		return nil, fmt.Errorf("milvus collection is required")
	}
	if limit <= 0 {
		return []string{}, nil
	}

	req := map[string]any{
		"dbName":         s.cfg.Database,
		"collectionName": s.cfg.Collection,
		"outputFields":   []string{"doc_id"},
		"limit":          limit,
		"offset":         offset,
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			DocID string `json:"doc_id"`
		} `json:"data"`
	}

	if err := s.doJSON(ctx, http.MethodPost, "/v2/vectordb/entities/query", req, &resp); err != nil {
		return nil, fmt.Errorf("milvus query entities: %w", err)
	}

	ids := make([]string, 0, len(resp.Data))
	for _, item := range resp.Data {
		if item.DocID != "" {
			ids = append(ids, item.DocID)
		}
	}

	return ids, nil
}

// ClearAll drops and recreates the Milvus collection, effectively removing all data.
// The collection schema and index will be recreated on the next AddDocuments call.
func (s *MilvusStore) ClearAll(ctx context.Context) error {
	if err := s.DropCollection(ctx); err != nil {
		return fmt.Errorf("milvus clear all: %w", err)
	}
	// Reset the ensureOnce so the collection can be recreated.
	s.ensureOnce = sync.Once{}
	s.ensureErr = nil
	s.logger.Info("milvus collection cleared", zap.String("collection", s.cfg.Collection))
	return nil
}

// 辅助功能

// 将字符串切入指定的最大长度。
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// 格式化StringList格式是Milvus过滤表达式的字符串列表。
func formatStringList(strs []string) string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf(`"%s"`, s)
	}
	return strings.Join(quoted, ", ")
}
