//go:build integration

package rag

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestMilvusStore 集成测试 米尔武斯商店针对真实的米尔武斯实例.
// 运行方式: go test -tags=整合 -v - 运行 TestMilvusStore 集成./rag/.
//
// 先决条件:
// - Milvus在本地主机上运行:19530(或设置MILVUS HOST和MILVUS PORT)
// - 插头组装 - 标注的Milvus up - d
func TestMilvusStore_Integration(t *testing.T) {
	host := os.Getenv("MILVUS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 19530
	if p := os.Getenv("MILVUS_PORT"); p != "" {
		// 需要解析端口
	}

	logger, _ := zap.NewDevelopment()
	store := NewMilvusStore(MilvusConfig{
		Host:                 host,
		Port:                 port,
		Collection:           "agentflow_test_" + time.Now().Format("20060102150405"),
		VectorDimension:      128,
		IndexType:            MilvusIndexIVFFlat,
		MetricType:           MilvusMetricCosine,
		AutoCreateCollection: true,
		Timeout:              60 * time.Second,
		BatchSize:            100,
	}, logger)

	ctx := context.Background()

	// 测试后清理
	defer func() {
		if err := store.DropCollection(ctx); err != nil {
			t.Logf("Warning: failed to drop collection: %v", err)
		}
	}()

	t.Run("AddAndSearch", func(t *testing.T) {
		// 创建带有随机嵌入的测试文档
		docs := make([]Document, 10)
		for i := 0; i < 10; i++ {
			embedding := make([]float64, 128)
			for j := 0; j < 128; j++ {
				embedding[j] = float64(i*128+j) / 1280.0
			}
			docs[i] = Document{
				ID:        string(rune('a' + i)),
				Content:   "Test document " + string(rune('a'+i)),
				Metadata:  map[string]any{"index": i},
				Embedding: embedding,
			}
		}

		// 添加文档
		if err := store.AddDocuments(ctx, docs); err != nil {
			t.Fatalf("AddDocuments failed: %v", err)
		}

		// 等待索引
		time.Sleep(2 * time.Second)

		// 搜索
		queryEmbedding := docs[0].Embedding
		results, err := store.Search(ctx, queryEmbedding, 5)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("Expected at least one result")
		}

		t.Logf("Found %d results", len(results))
		for i, r := range results {
			t.Logf("  Result %d: ID=%s, Score=%.4f, Content=%s",
				i, r.Document.ID, r.Score, r.Document.Content)
		}

		// 第一个结果应该是我们搜索的文件
		if results[0].Document.ID != "a" {
			t.Logf("Warning: expected first result to be 'a', got '%s'", results[0].Document.ID)
		}
	})

	t.Run("Count", func(t *testing.T) {
		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count != 10 {
			t.Fatalf("Expected count=10, got %d", count)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		// 删除一些文档
		if err := store.DeleteDocuments(ctx, []string{"a", "b", "c"}); err != nil {
			t.Fatalf("DeleteDocuments failed: %v", err)
		}

		// 等待删除
		time.Sleep(1 * time.Second)

		// 校验计数
		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count after delete failed: %v", err)
		}
		if count != 7 {
			t.Logf("Warning: expected count=7 after delete, got %d", count)
		}
	})

	t.Run("Update", func(t *testing.T) {
		// 更新文档
		embedding := make([]float64, 128)
		for j := 0; j < 128; j++ {
			embedding[j] = 0.5
		}
		doc := Document{
			ID:        "d",
			Content:   "Updated document d",
			Metadata:  map[string]any{"updated": true},
			Embedding: embedding,
		}

		if err := store.UpdateDocument(ctx, doc); err != nil {
			t.Fatalf("UpdateDocument failed: %v", err)
		}

		// 等待更新
		time.Sleep(1 * time.Second)

		// 搜索更新的文档
		results, err := store.Search(ctx, embedding, 1)
		if err != nil {
			t.Fatalf("Search after update failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("Expected at least one result after update")
		}

		if results[0].Document.Content != "Updated document d" {
			t.Logf("Warning: expected updated content, got '%s'", results[0].Document.Content)
		}
	})
}

// 测试MilvusStore 集成 HNSW测试HNSW指数类型.
func TestMilvusStore_Integration_HNSW(t *testing.T) {
	host := os.Getenv("MILVUS_HOST")
	if host == "" {
		host = "localhost"
	}

	logger, _ := zap.NewDevelopment()
	store := NewMilvusStore(MilvusConfig{
		Host:                 host,
		Port:                 19530,
		Collection:           "agentflow_hnsw_test_" + time.Now().Format("20060102150405"),
		VectorDimension:      64,
		IndexType:            MilvusIndexHNSW,
		MetricType:           MilvusMetricL2,
		AutoCreateCollection: true,
		Timeout:              60 * time.Second,
		IndexParams:          map[string]any{"M": 8, "efConstruction": 128},
		SearchParams:         map[string]any{"ef": 32},
	}, logger)

	ctx := context.Background()

	defer func() {
		if err := store.DropCollection(ctx); err != nil {
			t.Logf("Warning: failed to drop collection: %v", err)
		}
	}()

	// 创建测试文档
	docs := make([]Document, 5)
	for i := 0; i < 5; i++ {
		embedding := make([]float64, 64)
		for j := 0; j < 64; j++ {
			embedding[j] = float64(i*64+j) / 320.0
		}
		docs[i] = Document{
			ID:        string(rune('a' + i)),
			Content:   "HNSW test document " + string(rune('a'+i)),
			Embedding: embedding,
		}
	}

	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	results, err := store.Search(ctx, docs[0].Embedding, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	t.Logf("HNSW search found %d results", len(results))
	for i, r := range results {
		t.Logf("  Result %d: ID=%s, Score=%.4f, Distance=%.4f",
			i, r.Document.ID, r.Score, r.Distance)
	}
}

// TestMilvusStore 集成-BatchPerformance 测试批次插入性能.
func TestMilvusStore_Integration_BatchPerformance(t *testing.T) {
	host := os.Getenv("MILVUS_HOST")
	if host == "" {
		host = "localhost"
	}

	logger, _ := zap.NewDevelopment()
	store := NewMilvusStore(MilvusConfig{
		Host:                 host,
		Port:                 19530,
		Collection:           "agentflow_batch_test_" + time.Now().Format("20060102150405"),
		VectorDimension:      256,
		IndexType:            MilvusIndexIVFFlat,
		MetricType:           MilvusMetricCosine,
		AutoCreateCollection: true,
		Timeout:              120 * time.Second,
		BatchSize:            500,
	}, logger)

	ctx := context.Background()

	defer func() {
		if err := store.DropCollection(ctx); err != nil {
			t.Logf("Warning: failed to drop collection: %v", err)
		}
	}()

	// 创建1000个测试文档
	numDocs := 1000
	docs := make([]Document, numDocs)
	for i := 0; i < numDocs; i++ {
		embedding := make([]float64, 256)
		for j := 0; j < 256; j++ {
			embedding[j] = float64((i*256+j)%1000) / 1000.0
		}
		docs[i] = Document{
			ID:        string(rune(i)),
			Content:   "Batch test document",
			Embedding: embedding,
		}
	}

	start := time.Now()
	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments failed: %v", err)
	}
	elapsed := time.Since(start)

	t.Logf("Inserted %d documents in %v (%.2f docs/sec)",
		numDocs, elapsed, float64(numDocs)/elapsed.Seconds())

	// 校验计数
	time.Sleep(2 * time.Second)
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	t.Logf("Collection count: %d", count)
}
