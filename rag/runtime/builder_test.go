package runtime

import (
	"testing"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

func TestBuildProviders(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLM.APIKey = "test-key"
	cfg.LLM.DefaultProvider = "openai"

	b := NewBuilder(cfg, zap.NewNop())
	providers, err := b.BuildProviders()
	if err != nil {
		t.Fatalf("BuildProviders failed: %v", err)
	}
	if providers == nil || providers.Embedding == nil {
		t.Fatalf("expected embedding provider, got %#v", providers)
	}
}

func TestBuildProvidersWithRerank(t *testing.T) {
	cfg := &config.Config{}
	cfg.LLM.APIKey = "test-key"
	cfg.LLM.DefaultProvider = "openai"

	b := NewBuilder(cfg, zap.NewNop()).
		WithEmbeddingType(core.EmbeddingProviderType("openai")).
		WithRerankType(core.RerankProviderType("cohere"))

	providers, err := b.BuildProviders()
	if err != nil {
		t.Fatalf("BuildProviders failed: %v", err)
	}
	if providers.Rerank == nil {
		t.Fatal("expected rerank provider")
	}
}

func TestBuildProvidersNoAPIKey(t *testing.T) {
	cfg := &config.Config{}
	_, err := NewBuilder(cfg, zap.NewNop()).BuildProviders()
	if err == nil {
		t.Fatal("expected error for empty api key")
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
	cfg.LLM.APIKey = "test-key"
	cfg.LLM.DefaultProvider = "openai"

	retriever, err := NewBuilder(cfg, zap.NewNop()).BuildEnhancedRetriever()
	if err != nil {
		t.Fatalf("BuildEnhancedRetriever failed: %v", err)
	}
	if retriever == nil {
		t.Fatal("expected retriever")
	}
}
