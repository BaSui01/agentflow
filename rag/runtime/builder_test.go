package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// mockEmbeddingProvider 是测试用的 mock embedding provider
type mockEmbeddingProvider struct {
	name string
}

func (m *mockEmbeddingProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return []float64{0.1, 0.2, 0.3}, nil
}

func (m *mockEmbeddingProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return [][]float64{{0.1, 0.2, 0.3}}, nil
}

func (m *mockEmbeddingProvider) Name() string {
	return m.name
}

// mockRerankProvider 是测试用的 mock rerank provider
type mockRerankProvider struct {
	name string
}

func (m *mockRerankProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]types.RerankResult, error) {
	return []types.RerankResult{{Index: 0, RelevanceScore: 0.9}}, nil
}

func (m *mockRerankProvider) Name() string {
	return m.name
}

func TestBuildProviders(t *testing.T) {
	cfg := &config.Config{}

	// 新设计：provider 必须由上层注入
	mockEmbedding := &mockEmbeddingProvider{name: "test-embedding"}
	b := NewBuilder(cfg, zap.NewNop()).WithEmbeddingProvider(mockEmbedding)

	providers, err := b.BuildProviders()
	if err != nil {
		t.Fatalf("BuildProviders failed: %v", err)
	}
	if providers == nil {
		t.Fatal("expected providers, got nil")
	}
	if providers.Embedding == nil {
		t.Fatal("expected embedding provider, got nil")
	}
	if providers.Embedding.Name() != "test-embedding" {
		t.Fatalf("expected embedding name 'test-embedding', got '%s'", providers.Embedding.Name())
	}
}

func TestBuildProvidersWithRerank(t *testing.T) {
	cfg := &config.Config{}

	// 新设计：provider 必须由上层注入
	mockEmbedding := &mockEmbeddingProvider{name: "test-embedding"}
	mockRerank := &mockRerankProvider{name: "test-rerank"}

	b := NewBuilder(cfg, zap.NewNop()).
		WithEmbeddingProvider(mockEmbedding).
		WithRerankProvider(mockRerank)

	providers, err := b.BuildProviders()
	if err != nil {
		t.Fatalf("BuildProviders failed: %v", err)
	}
	if providers == nil {
		t.Fatal("expected providers, got nil")
	}
	if providers.Embedding == nil {
		t.Fatal("expected embedding provider")
	}
	if providers.Rerank == nil {
		t.Fatal("expected rerank provider")
	}
	if providers.Rerank.Name() != "test-rerank" {
		t.Fatalf("expected rerank name 'test-rerank', got '%s'", providers.Rerank.Name())
	}
}

func TestBuildProvidersNoInjection(t *testing.T) {
	cfg := &config.Config{}

	// 新设计：没有注入 provider 时，返回的 providers 中字段为 nil（不再报错）
	b := NewBuilder(cfg, zap.NewNop())
	providers, err := b.BuildProviders()
	if err != nil {
		t.Fatalf("BuildProviders should not fail: %v", err)
	}
	if providers == nil {
		t.Fatal("expected providers struct, got nil")
	}
	// 没有注入时，provider 为 nil 是预期行为
	if providers.Embedding != nil {
		t.Fatal("expected nil embedding provider when not injected")
	}
	if providers.Rerank != nil {
		t.Fatal("expected nil rerank provider when not injected")
	}
}

func TestBuildProvidersNilBuilder(t *testing.T) {
	var b *Builder
	_, err := b.BuildProviders()
	if err == nil {
		t.Fatal("expected error for nil builder")
	}
}

func TestBuildVectorStoreDefault(t *testing.T) {
	cfg := config.DefaultConfig()
	store, err := NewBuilder(cfg, zap.NewNop()).BuildVectorStore()
	if err != nil {
		t.Fatalf("BuildVectorStore failed: %v", err)
	}
	if store == nil {
		t.Fatal("expected vector store")
	}
}

func TestBuildVectorStoreUnsupported(t *testing.T) {
	cfg := config.DefaultConfig()
	_, err := NewBuilder(cfg, zap.NewNop()).
		WithVectorStoreType(core.VectorStoreType("bad-store")).
		BuildVectorStore()
	if err == nil {
		t.Fatal("expected unsupported vector store error")
	}
}

func TestBuildEnhancedRetriever(t *testing.T) {
	cfg := &config.Config{}

	// 新设计：EnhancedRetriever 可在没有外部 provider 的情况下构建（仅使用混合检索）
	retriever, err := NewBuilder(cfg, zap.NewNop()).BuildEnhancedRetriever()
	if err != nil {
		t.Fatalf("BuildEnhancedRetriever failed: %v", err)
	}
	if retriever == nil {
		t.Fatal("expected retriever")
	}
}

func TestBuildHybridRetriever(t *testing.T) {
	cfg := &config.Config{}

	retriever, err := NewBuilder(cfg, zap.NewNop()).BuildHybridRetriever()
	if err != nil {
		t.Fatalf("BuildHybridRetriever failed: %v", err)
	}
	if retriever == nil {
		t.Fatal("expected retriever")
	}
}

func TestBuilderWithVectorStore(t *testing.T) {
	cfg := config.DefaultConfig()

	// 创建自定义向量存储
	customStore := &mockVectorStore{}

	b := NewBuilder(cfg, zap.NewNop()).WithVectorStore(customStore)
	store, err := b.BuildVectorStore()
	if err != nil {
		t.Fatalf("BuildVectorStore failed: %v", err)
	}
	// 检查返回的 store 不是 nil
	if store == nil {
		t.Fatal("expected non-nil vector store")
	}
}

// mockVectorStore 是测试用的 mock vector store
type mockVectorStore struct{}

func (m *mockVectorStore) AddDocuments(ctx context.Context, docs []core.Document) error {
	return nil
}

func (m *mockVectorStore) Search(ctx context.Context, queryEmbedding []float64, topK int) ([]core.VectorSearchResult, error) {
	return []core.VectorSearchResult{}, nil
}

func (m *mockVectorStore) DeleteDocuments(ctx context.Context, ids []string) error {
	return nil
}

func (m *mockVectorStore) UpdateDocument(ctx context.Context, doc core.Document) error {
	return nil
}

func (m *mockVectorStore) Count(ctx context.Context) (int, error) {
	return 0, nil
}
