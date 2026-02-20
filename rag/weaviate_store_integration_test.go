// +build integration

package rag

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// 测试WeaviateStore 集成 测试Weaviate商店与真正的Weaviate案例.
// 运行与: go test -tags=整合 - 运行的测试WeaviateStore 集成. /rag/.
//
// 先决条件:
// - 本地主机:8080(或设置WEAVIATE HOST和WEAVIATE PORT)
// - 涂鸦编织 - 标注式编织 - d
func TestWeaviateStore_Integration(t *testing.T) {
	host := os.Getenv("WEAVIATE_HOST")
	if host == "" {
		host = "localhost"
	}
	port := 8080
	if p := os.Getenv("WEAVIATE_PORT"); p != "" {
		// 需要解析端口
		port = 8080
	}

	// 无法使用 Weaviate 时跳过
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger, _ := zap.NewDevelopment()
	store := NewWeaviateStore(WeaviateConfig{
		Host:             host,
		Port:             port,
		ClassName:        "IntegrationTest",
		AutoCreateSchema: true,
		Distance:         "cosine",
		HybridAlpha:      0.5,
	}, logger)

	// 测试前后清理
	_ = store.DeleteClass(ctx)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = store.DeleteClass(cleanupCtx)
	}()

	// 测试文档
	docs := []Document{
		{
			ID:        "doc1",
			Content:   "The quick brown fox jumps over the lazy dog",
			Metadata:  map[string]any{"category": "animals", "language": "en"},
			Embedding: generateTestEmbedding(128, 0.1),
		},
		{
			ID:        "doc2",
			Content:   "Machine learning is a subset of artificial intelligence",
			Metadata:  map[string]any{"category": "technology", "language": "en"},
			Embedding: generateTestEmbedding(128, 0.2),
		},
		{
			ID:        "doc3",
			Content:   "Natural language processing enables computers to understand human language",
			Metadata:  map[string]any{"category": "technology", "language": "en"},
			Embedding: generateTestEmbedding(128, 0.3),
		},
	}

	// 测试添加文档
	t.Run("AddDocuments", func(t *testing.T) {
		if err := store.AddDocuments(ctx, docs); err != nil {
			t.Fatalf("AddDocuments failed: %v", err)
		}
	})

	// 等待索引
	time.Sleep(500 * time.Millisecond)

	// 测试计数
	t.Run("Count", func(t *testing.T) {
		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count failed: %v", err)
		}
		if count != 3 {
			t.Fatalf("expected count 3, got %d", count)
		}
	})

	// 测试矢量搜索
	t.Run("VectorSearch", func(t *testing.T) {
		results, err := store.Search(ctx, generateTestEmbedding(128, 0.15), 2)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		// 第一个结果应最接近查询嵌入
		t.Logf("Vector search results: %+v", results)
	})

	// 测试混合搜索
	t.Run("HybridSearch", func(t *testing.T) {
		results, err := store.HybridSearch(ctx, "machine learning artificial intelligence", generateTestEmbedding(128, 0.2), 2)
		if err != nil {
			t.Fatalf("HybridSearch failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatalf("expected at least 1 result")
		}
		t.Logf("Hybrid search results: %+v", results)
	})

	// 测试 BM25 搜索
	t.Run("BM25Search", func(t *testing.T) {
		results, err := store.BM25Search(ctx, "fox dog", 2)
		if err != nil {
			t.Fatalf("BM25Search failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatalf("expected at least 1 result")
		}
		// 应该找到关于狐狸和狗的文件
		found := false
		for _, r := range results {
			if r.Document.ID == "doc1" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected to find doc1 in BM25 results")
		}
		t.Logf("BM25 search results: %+v", results)
	})

	// 测试更新文档
	t.Run("UpdateDocument", func(t *testing.T) {
		updatedDoc := Document{
			ID:        "doc1",
			Content:   "The quick brown fox jumps over the lazy dog - updated",
			Metadata:  map[string]any{"category": "animals", "language": "en", "updated": true},
			Embedding: generateTestEmbedding(128, 0.15),
		}
		if err := store.UpdateDocument(ctx, updatedDoc); err != nil {
			t.Fatalf("UpdateDocument failed: %v", err)
		}

		// 等待更新索引
		time.Sleep(500 * time.Millisecond)

		// 通过搜索验证更新
		results, err := store.BM25Search(ctx, "updated", 1)
		if err != nil {
			t.Fatalf("Search after update failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatalf("expected to find updated document")
		}
	})

	// 测试删除文档
	t.Run("DeleteDocuments", func(t *testing.T) {
		if err := store.DeleteDocuments(ctx, []string{"doc1"}); err != nil {
			t.Fatalf("DeleteDocuments failed: %v", err)
		}

		// 等待删除
		time.Sleep(500 * time.Millisecond)

		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count after delete failed: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected count 2 after delete, got %d", count)
		}
	})

	// 测试 GetSchema
	t.Run("GetSchema", func(t *testing.T) {
		schema, err := store.GetSchema(ctx)
		if err != nil {
			t.Fatalf("GetSchema failed: %v", err)
		}
		if schema["class"] != "IntegrationTest" {
			t.Fatalf("unexpected class name in schema: %v", schema["class"])
		}
		t.Logf("Schema: %+v", schema)
	})
}

// 测试WeaviateStore Large Scale 用更大的数据集测试Weaviate商店.
func TestWeaviateStore_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large scale test in short mode")
	}

	host := os.Getenv("WEAVIATE_HOST")
	if host == "" {
		host = "localhost"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger, _ := zap.NewDevelopment()
	store := NewWeaviateStore(WeaviateConfig{
		Host:             host,
		Port:             8080,
		ClassName:        "LargeScaleTest",
		AutoCreateSchema: true,
	}, logger)

	// 清理
	_ = store.DeleteClass(ctx)
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = store.DeleteClass(cleanupCtx)
	}()

	// 生成 1000 个文档
	numDocs := 1000
	docs := make([]Document, numDocs)
	for i := 0; i < numDocs; i++ {
		docs[i] = Document{
			ID:        generateDocID(i),
			Content:   generateContent(i),
			Metadata:  map[string]any{"index": i},
			Embedding: generateTestEmbedding(128, float64(i)/float64(numDocs)),
		}
	}

	// 批次插入
	batchSize := 100
	start := time.Now()
	for i := 0; i < numDocs; i += batchSize {
		end := i + batchSize
		if end > numDocs {
			end = numDocs
		}
		if err := store.AddDocuments(ctx, docs[i:end]); err != nil {
			t.Fatalf("AddDocuments batch %d failed: %v", i/batchSize, err)
		}
	}
	t.Logf("Inserted %d documents in %v", numDocs, time.Since(start))

	// 等待索引
	time.Sleep(2 * time.Second)

	// 校验计数
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != numDocs {
		t.Fatalf("expected count %d, got %d", numDocs, count)
	}

	// 测试搜索性能
	start = time.Now()
	for i := 0; i < 100; i++ {
		_, err := store.Search(ctx, generateTestEmbedding(128, 0.5), 10)
		if err != nil {
			t.Fatalf("Search %d failed: %v", i, err)
		}
	}
	t.Logf("100 searches completed in %v (avg: %v)", time.Since(start), time.Since(start)/100)
}

// 辅助功能

func generateTestEmbedding(dim int, seed float64) []float64 {
	embedding := make([]float64, dim)
	for i := 0; i < dim; i++ {
		// 基于种子生成确定值
		embedding[i] = (seed + float64(i)/float64(dim)) / 2.0
	}
	return embedding
}

func generateDocID(index int) string {
	return "doc_" + string(rune('a'+index%26)) + "_" + string(rune('0'+index%10))
}

func generateContent(index int) string {
	topics := []string{
		"machine learning",
		"artificial intelligence",
		"natural language processing",
		"computer vision",
		"deep learning",
		"neural networks",
		"data science",
		"big data",
		"cloud computing",
		"distributed systems",
	}
	return "Document about " + topics[index%len(topics)] + " with index " + string(rune('0'+index%10))
}
