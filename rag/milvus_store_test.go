package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
)

func TestMilvusStore_BasicFlow(t *testing.T) {
	t.Parallel()

	var hasCollectionCalls atomic.Int64
	var createCollectionCalls atomic.Int64
	var createIndexCalls atomic.Int64
	var loadCollectionCalls atomic.Int64
	var insertCalls atomic.Int64
	var searchCalls atomic.Int64
	var deleteCalls atomic.Int64
	var statsCalls atomic.Int64

	mux := http.NewServeMux()

	// 有收藏端点
	mux.HandleFunc("/v2/vectordb/collections/has", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		hasCollectionCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode has collection request: %v", err)
		}
		if req["collectionName"] != "testcol" {
			t.Fatalf("unexpected collection name: %v", req["collectionName"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"has":false}}`))
	})

	// 创建收藏端点
	mux.HandleFunc("/v2/vectordb/collections/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		createCollectionCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode create collection request: %v", err)
		}
		if req["collectionName"] != "testcol" {
			t.Fatalf("unexpected collection name: %v", req["collectionName"])
		}

		// 校验方案
		schema, ok := req["schema"].(map[string]any)
		if !ok {
			t.Fatalf("expected schema in request")
		}
		fields, ok := schema["fields"].([]any)
		if !ok || len(fields) < 4 {
			t.Fatalf("expected at least 4 fields in schema")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	})

	// 创建索引终点
	mux.HandleFunc("/v2/vectordb/indexes/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		createIndexCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode create index request: %v", err)
		}

		indexParams, ok := req["indexParams"].([]any)
		if !ok || len(indexParams) == 0 {
			t.Fatalf("expected indexParams in request")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	})

	// 装入收藏端点
	mux.HandleFunc("/v2/vectordb/collections/load", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		loadCollectionCalls.Add(1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	})

	// 插入端点
	mux.HandleFunc("/v2/vectordb/entities/insert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		insertCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode insert request: %v", err)
		}

		data, ok := req["data"].([]any)
		if !ok {
			t.Fatalf("expected data array in request")
		}
		if len(data) != 2 {
			t.Fatalf("expected 2 documents, got %d", len(data))
		}

		// 校验文档结构
		for _, d := range data {
			doc := d.(map[string]any)
			if _, ok := doc["id"]; !ok {
				t.Fatalf("expected id field in document")
			}
			if _, ok := doc["vector"]; !ok {
				t.Fatalf("expected vector field in document")
			}
			if _, ok := doc["content"]; !ok {
				t.Fatalf("expected content field in document")
			}
			if _, ok := doc["doc_id"]; !ok {
				t.Fatalf("expected doc_id field in document")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"insertCount":2,"insertIds":["id1","id2"]}}`))
	})

	// 搜索终点
	mux.HandleFunc("/v2/vectordb/entities/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		searchCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode search request: %v", err)
		}

		if req["collectionName"] != "testcol" {
			t.Fatalf("unexpected collection name: %v", req["collectionName"])
		}
		if req["limit"].(float64) != 2 {
			t.Fatalf("unexpected limit: %v", req["limit"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"data": [[
				{"id": "id1", "distance": 0.95, "entity": {"doc_id": "doc1", "content": "hello", "metadata": {"k": "v"}}},
				{"id": "id2", "distance": 0.85, "entity": {"doc_id": "doc2", "content": "world", "metadata": {"k": "v2"}}}
			]]
		}`))
	})

	// 删除端点
	mux.HandleFunc("/v2/vectordb/entities/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		deleteCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode delete request: %v", err)
		}

		filter, ok := req["filter"].(string)
		if !ok || !strings.Contains(filter, "id in") {
			t.Fatalf("expected filter with id in clause, got: %v", filter)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	})

	// 获取数据端点
	mux.HandleFunc("/v2/vectordb/collections/get_stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		statsCalls.Add(1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"rowCount":2}}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewMilvusStore(MilvusConfig{
		BaseURL:              srv.URL,
		Collection:           "testcol",
		AutoCreateCollection: true,
		IndexType:            MilvusIndexIVFFlat,
		MetricType:           MilvusMetricCosine,
	}, logger)

	ctx := context.Background()

	// 测试添加文档
	docs := []Document{
		{ID: "doc1", Content: "hello", Metadata: map[string]any{"k": "v"}, Embedding: []float64{0.1, 0.2}},
		{ID: "doc2", Content: "world", Metadata: map[string]any{"k": "v2"}, Embedding: []float64{0.2, 0.1}},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments: %v", err)
	}

	// 测试搜索
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
	if results[0].Score != 0.95 {
		t.Fatalf("expected score 0.95, got %f", results[0].Score)
	}

	// 测试计数
	n, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected count=2, got %d", n)
	}

	// 测试删除文档
	if err := store.DeleteDocuments(ctx, []string{"doc1", "doc2"}); err != nil {
		t.Fatalf("DeleteDocuments: %v", err)
	}

	// 校验端点呼叫
	if hasCollectionCalls.Load() != 1 {
		t.Fatalf("expected has collection 1 call, got %d", hasCollectionCalls.Load())
	}
	if createCollectionCalls.Load() != 1 {
		t.Fatalf("expected create collection 1 call, got %d", createCollectionCalls.Load())
	}
	if createIndexCalls.Load() != 1 {
		t.Fatalf("expected create index 1 call, got %d", createIndexCalls.Load())
	}
	if loadCollectionCalls.Load() != 1 {
		t.Fatalf("expected load collection 1 call, got %d", loadCollectionCalls.Load())
	}
	if insertCalls.Load() != 1 {
		t.Fatalf("expected insert 1 call, got %d", insertCalls.Load())
	}
	if searchCalls.Load() != 1 {
		t.Fatalf("expected search 1 call, got %d", searchCalls.Load())
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected delete 1 call, got %d", deleteCalls.Load())
	}
	if statsCalls.Load() != 1 {
		t.Fatalf("expected stats 1 call, got %d", statsCalls.Load())
	}
}

func TestMilvusStore_ExistingCollection(t *testing.T) {
	t.Parallel()

	var hasCollectionCalls atomic.Int64
	var createCollectionCalls atomic.Int64
	var insertCalls atomic.Int64

	mux := http.NewServeMux()

	// 收藏端点 - 返回为真( 收藏存在)
	mux.HandleFunc("/v2/vectordb/collections/has", func(w http.ResponseWriter, r *http.Request) {
		hasCollectionCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"has":true}}`))
	})

	// 创建收藏端点 - 不应调用
	mux.HandleFunc("/v2/vectordb/collections/create", func(w http.ResponseWriter, r *http.Request) {
		createCollectionCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	})

	// 插入端点
	mux.HandleFunc("/v2/vectordb/entities/insert", func(w http.ResponseWriter, r *http.Request) {
		insertCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"insertCount":1,"insertIds":["id1"]}}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewMilvusStore(MilvusConfig{
		BaseURL:              srv.URL,
		Collection:           "existing_col",
		AutoCreateCollection: true,
	}, logger)

	ctx := context.Background()

	docs := []Document{
		{ID: "doc1", Content: "test", Embedding: []float64{0.1, 0.2}},
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments: %v", err)
	}

	// 校验创建收藏未调用
	if hasCollectionCalls.Load() != 1 {
		t.Fatalf("expected has collection 1 call, got %d", hasCollectionCalls.Load())
	}
	if createCollectionCalls.Load() != 0 {
		t.Fatalf("expected create collection 0 calls, got %d", createCollectionCalls.Load())
	}
	if insertCalls.Load() != 1 {
		t.Fatalf("expected insert 1 call, got %d", insertCalls.Load())
	}
}

func TestMilvusStore_BatchInsert(t *testing.T) {
	t.Parallel()

	var insertCalls atomic.Int64
	var totalDocs atomic.Int64

	mux := http.NewServeMux()

	mux.HandleFunc("/v2/vectordb/collections/has", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"has":true}}`))
	})

	mux.HandleFunc("/v2/vectordb/entities/insert", func(w http.ResponseWriter, r *http.Request) {
		insertCalls.Add(1)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode insert request: %v", err)
		}

		data := req["data"].([]any)
		totalDocs.Add(int64(len(data)))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"insertCount":` + fmt.Sprintf("%d", len(data)) + `}}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewMilvusStore(MilvusConfig{
		BaseURL:              srv.URL,
		Collection:           "batch_col",
		AutoCreateCollection: true,
		BatchSize:            3, // Small batch size for testing
	}, logger)

	ctx := context.Background()

	// 创建7个文件以测试批量( 应分出3个批: 3+3+1)
	docs := make([]Document, 7)
	for i := 0; i < 7; i++ {
		docs[i] = Document{
			ID:        fmt.Sprintf("doc%d", i),
			Content:   "test",
			Embedding: []float64{0.1, 0.2},
		}
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments: %v", err)
	}

	if insertCalls.Load() != 3 {
		t.Fatalf("expected 3 insert calls for batching, got %d", insertCalls.Load())
	}
	if totalDocs.Load() != 7 {
		t.Fatalf("expected 7 total docs inserted, got %d", totalDocs.Load())
	}
}

func TestMilvusStore_ValidationErrors(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	store := NewMilvusStore(MilvusConfig{
		Collection: "testcol",
	}, logger)

	ctx := context.Background()

	// 测试空文档 ID
	err := store.AddDocuments(ctx, []Document{{ID: "", Embedding: []float64{0.1}}})
	if err == nil || !strings.Contains(err.Error(), "empty id") {
		t.Fatalf("expected empty id error, got: %v", err)
	}

	// 测试缺失嵌入
	err = store.AddDocuments(ctx, []Document{{ID: "doc1", Embedding: nil}})
	if err == nil || !strings.Contains(err.Error(), "no embedding") {
		t.Fatalf("expected no embedding error, got: %v", err)
	}

	// 测试尺寸不匹配
	err = store.AddDocuments(ctx, []Document{
		{ID: "doc1", Embedding: []float64{0.1, 0.2}},
		{ID: "doc2", Embedding: []float64{0.1, 0.2, 0.3}},
	})
	if err == nil || !strings.Contains(err.Error(), "dimension mismatch") {
		t.Fatalf("expected dimension mismatch error, got: %v", err)
	}

	// 测试空查询嵌入
	_, err = store.Search(ctx, []float64{}, 10)
	if err == nil || !strings.Contains(err.Error(), "query embedding is required") {
		t.Fatalf("expected query embedding required error, got: %v", err)
	}

	// 测试丢失的收藏
	storeNoCol := NewMilvusStore(MilvusConfig{}, logger)
	err = storeNoCol.AddDocuments(ctx, []Document{{ID: "doc1", Embedding: []float64{0.1}}})
	if err == nil || !strings.Contains(err.Error(), "collection is required") {
		t.Fatalf("expected collection required error, got: %v", err)
	}
}

func TestMilvusStore_IndexTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		indexType    MilvusIndexType
		expectedNlist bool
		expectedM     bool
		expectedEf    bool
	}{
		{MilvusIndexIVFFlat, true, false, false},
		{MilvusIndexHNSW, false, true, true},
		{MilvusIndexFlat, false, false, false},
		{MilvusIndexIVFSQ8, true, false, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.indexType), func(t *testing.T) {
			params := defaultIndexParams(tc.indexType)
			searchParams := defaultSearchParams(tc.indexType)

			_, hasNlist := params["nlist"]
			_, hasM := params["M"]
			_, hasEf := searchParams["ef"]

			if hasNlist != tc.expectedNlist {
				t.Errorf("nlist: expected %v, got %v", tc.expectedNlist, hasNlist)
			}
			if hasM != tc.expectedM {
				t.Errorf("M: expected %v, got %v", tc.expectedM, hasM)
			}
			if hasEf != tc.expectedEf {
				t.Errorf("ef: expected %v, got %v", tc.expectedEf, hasEf)
			}
		})
	}
}

func TestMilvusStore_DistanceToScore(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		metricType MilvusMetricType
		distance   float64
		expected   float64
	}{
		{MilvusMetricCosine, 0.95, 0.95},
		{MilvusMetricIP, 0.85, 0.85},
		{MilvusMetricL2, 0.0, 1.0},
		{MilvusMetricL2, 1.0, 0.5},
	}

	for _, tc := range testCases {
		t.Run(string(tc.metricType), func(t *testing.T) {
			store := &MilvusStore{
				cfg: MilvusConfig{MetricType: tc.metricType},
			}
			score := store.distanceToScore(tc.distance)
			if score != tc.expected {
				t.Errorf("expected score %f, got %f", tc.expected, score)
			}
		})
	}
}

func TestMilvusStore_UpdateDocument(t *testing.T) {
	t.Parallel()

	var deleteCalls atomic.Int64
	var insertCalls atomic.Int64

	mux := http.NewServeMux()

	mux.HandleFunc("/v2/vectordb/collections/has", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"has":true}}`))
	})

	mux.HandleFunc("/v2/vectordb/entities/delete", func(w http.ResponseWriter, r *http.Request) {
		deleteCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
	})

	mux.HandleFunc("/v2/vectordb/entities/insert", func(w http.ResponseWriter, r *http.Request) {
		insertCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"insertCount":1}}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	logger := zap.NewNop()
	store := NewMilvusStore(MilvusConfig{
		BaseURL:              srv.URL,
		Collection:           "update_col",
		AutoCreateCollection: true,
	}, logger)

	ctx := context.Background()

	doc := Document{
		ID:        "doc1",
		Content:   "updated content",
		Embedding: []float64{0.3, 0.4},
	}

	if err := store.UpdateDocument(ctx, doc); err != nil {
		t.Fatalf("UpdateDocument: %v", err)
	}

	if deleteCalls.Load() != 1 {
		t.Fatalf("expected delete 1 call, got %d", deleteCalls.Load())
	}
	if insertCalls.Load() != 1 {
		t.Fatalf("expected insert 1 call, got %d", insertCalls.Load())
	}
}

func TestMilvusStore_EmptyOperations(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	store := NewMilvusStore(MilvusConfig{
		Collection: "testcol",
	}, logger)

	ctx := context.Background()

	// 空添加文档应成功
	if err := store.AddDocuments(ctx, []Document{}); err != nil {
		t.Fatalf("empty AddDocuments should succeed: %v", err)
	}

	// 空删除文档应成功
	if err := store.DeleteDocuments(ctx, []string{}); err != nil {
		t.Fatalf("empty DeleteDocuments should succeed: %v", err)
	}

	// 0 上K 应返回空结果
	results, err := store.Search(ctx, []float64{0.1}, 0)
	if err != nil {
		t.Fatalf("zero topK Search should succeed: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestMilvusStore_DefaultConfig(t *testing.T) {
	t.Parallel()

	store := NewMilvusStore(MilvusConfig{}, nil)

	if store.cfg.Host != "localhost" {
		t.Errorf("expected default host localhost, got %s", store.cfg.Host)
	}
	if store.cfg.Port != 19530 {
		t.Errorf("expected default port 19530, got %d", store.cfg.Port)
	}
	if store.cfg.Database != "default" {
		t.Errorf("expected default database 'default', got %s", store.cfg.Database)
	}
	if store.cfg.IndexType != MilvusIndexIVFFlat {
		t.Errorf("expected default index type IVF_FLAT, got %s", store.cfg.IndexType)
	}
	if store.cfg.MetricType != MilvusMetricCosine {
		t.Errorf("expected default metric type COSINE, got %s", store.cfg.MetricType)
	}
	if store.cfg.BatchSize != 1000 {
		t.Errorf("expected default batch size 1000, got %d", store.cfg.BatchSize)
	}
}

func TestMilvusStore_HelperFunctions(t *testing.T) {
	t.Parallel()

	// 测试短线
	if truncateString("hello", 10) != "hello" {
		t.Error("truncateString should not truncate short strings")
	}
	if truncateString("hello world", 5) != "hello" {
		t.Error("truncateString should truncate long strings")
	}

	// 测试格式 列表
	result := formatStringList([]string{"a", "b", "c"})
	if result != `"a", "b", "c"` {
		t.Errorf("unexpected formatStringList result: %s", result)
	}

	// 测试 milvusPointID 生成一致的 UUID
	id1 := milvusPointID("doc1")
	id2 := milvusPointID("doc1")
	id3 := milvusPointID("doc2")

	if id1 != id2 {
		t.Error("milvusPointID should generate consistent UUIDs for same input")
	}
	if id1 == id3 {
		t.Error("milvusPointID should generate different UUIDs for different inputs")
	}
}
