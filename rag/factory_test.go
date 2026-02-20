package rag

import (
	"fmt"
	"testing"

	"github.com/BaSui01/agentflow/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// NewVectorStoreFromConfig
// ---------------------------------------------------------------------------

func TestNewVectorStoreFromConfig(t *testing.T) {
	logger := zap.NewNop()
	cfg := config.DefaultConfig()

	tests := []struct {
		name      string
		storeType VectorStoreType
		wantType  string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "empty type defaults to InMemory",
			storeType: "",
			wantType:  "*rag.InMemoryVectorStore",
		},
		{
			name:      "explicit memory type",
			storeType: VectorStoreMemory,
			wantType:  "*rag.InMemoryVectorStore",
		},
		{
			name:      "qdrant type",
			storeType: VectorStoreQdrant,
			wantType:  "*rag.QdrantStore",
		},
		{
			name:      "weaviate type",
			storeType: VectorStoreWeaviate,
			wantType:  "*rag.WeaviateStore",
		},
		{
			name:      "milvus type",
			storeType: VectorStoreMilvus,
			wantType:  "*rag.MilvusStore",
		},
		{
			name:      "pinecone type",
			storeType: VectorStorePinecone,
			wantType:  "*rag.PineconeStore",
		},
		{
			name:      "unsupported type returns error",
			storeType: VectorStoreType("redis"),
			wantErr:   true,
			errMsg:    "unsupported vector store type: redis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewVectorStoreFromConfig(cfg, tt.storeType, logger)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, store)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, store)
			assert.Contains(t, typeName(store), tt.wantType)
		})
	}
}

func TestNewVectorStoreFromConfig_NilConfig(t *testing.T) {
	_, err := NewVectorStoreFromConfig(nil, VectorStoreMemory, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
}

func TestNewVectorStoreFromConfig_NilLogger(t *testing.T) {
	cfg := config.DefaultConfig()
	store, err := NewVectorStoreFromConfig(cfg, VectorStoreMemory, nil)
	require.NoError(t, err)
	require.NotNil(t, store)
}

// ---------------------------------------------------------------------------
// Config mapping correctness
// ---------------------------------------------------------------------------

func TestMapQdrantConfig(t *testing.T) {
	src := &config.QdrantConfig{
		Host:       "qdrant-host",
		Port:       6334,
		APIKey:     "test-key",
		Collection: "my_collection",
	}
	got := mapQdrantConfig(src)
	assert.Equal(t, "qdrant-host", got.Host)
	assert.Equal(t, 6334, got.Port)
	assert.Equal(t, "test-key", got.APIKey)
	assert.Equal(t, "my_collection", got.Collection)
	assert.True(t, got.AutoCreateCollection)
}

func TestMapWeaviateConfig(t *testing.T) {
	src := &config.WeaviateConfig{
		Host:             "weaviate-host",
		Port:             8080,
		Scheme:           "https",
		APIKey:           "wv-key",
		ClassName:        "Documents",
		AutoCreateSchema: true,
		Distance:         "dot",
		HybridAlpha:      0.7,
	}
	got := mapWeaviateConfig(src)
	assert.Equal(t, "weaviate-host", got.Host)
	assert.Equal(t, 8080, got.Port)
	assert.Equal(t, "https", got.Scheme)
	assert.Equal(t, "wv-key", got.APIKey)
	assert.Equal(t, "Documents", got.ClassName)
	assert.True(t, got.AutoCreateSchema)
	assert.Equal(t, "dot", got.Distance)
	assert.InDelta(t, 0.7, got.HybridAlpha, 0.001)
}

func TestMapMilvusConfig(t *testing.T) {
	src := &config.MilvusConfig{
		Host:                 "milvus-host",
		Port:                 19530,
		Username:             "user",
		Password:             "pass",
		Token:                "tok",
		Database:             "mydb",
		Collection:           "vectors",
		VectorDimension:      768,
		IndexType:            "HNSW",
		MetricType:           "L2",
		AutoCreateCollection: true,
		BatchSize:            500,
		ConsistencyLevel:     "Session",
	}
	got := mapMilvusConfig(src)
	assert.Equal(t, "milvus-host", got.Host)
	assert.Equal(t, 19530, got.Port)
	assert.Equal(t, "user", got.Username)
	assert.Equal(t, "pass", got.Password)
	assert.Equal(t, "tok", got.Token)
	assert.Equal(t, "mydb", got.Database)
	assert.Equal(t, "vectors", got.Collection)
	assert.Equal(t, 768, got.VectorDimension)
	assert.Equal(t, MilvusIndexHNSW, got.IndexType)
	assert.Equal(t, MilvusMetricL2, got.MetricType)
	assert.True(t, got.AutoCreateCollection)
	assert.Equal(t, 500, got.BatchSize)
	assert.Equal(t, "Session", got.ConsistencyLevel)
}

// ---------------------------------------------------------------------------
// NewEmbeddingProviderFromConfig
// ---------------------------------------------------------------------------

func TestNewEmbeddingProviderFromConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.APIKey = "test-api-key"

	tests := []struct {
		name         string
		providerType EmbeddingProviderType
		wantName     string
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "empty type uses default provider from config",
			providerType: "",
			wantName:     "openai-embedding",
		},
		{
			name:         "explicit openai",
			providerType: EmbeddingOpenAI,
			wantName:     "openai-embedding",
		},
		{
			name:         "cohere",
			providerType: EmbeddingCohere,
			wantName:     "cohere-embedding",
		},
		{
			name:         "voyage",
			providerType: EmbeddingVoyage,
			wantName:     "voyage-embedding",
		},
		{
			name:         "jina",
			providerType: EmbeddingJina,
			wantName:     "jina-embedding",
		},
		{
			name:         "unsupported type",
			providerType: EmbeddingProviderType("unknown"),
			wantErr:      true,
			errMsg:       "unsupported embedding provider type: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, err := NewEmbeddingProviderFromConfig(cfg, tt.providerType)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, prov)
			assert.Equal(t, tt.wantName, prov.Name())
		})
	}
}

func TestNewEmbeddingProviderFromConfig_NilConfig(t *testing.T) {
	_, err := NewEmbeddingProviderFromConfig(nil, EmbeddingOpenAI)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
}

// ---------------------------------------------------------------------------
// NewRetrieverFromConfig
// ---------------------------------------------------------------------------

func TestNewRetrieverFromConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.APIKey = "test-key"

	retriever, err := NewRetrieverFromConfig(cfg, WithLogger(zap.NewNop()))
	require.NoError(t, err)
	require.NotNil(t, retriever)
	// EnhancedRetriever embeds HybridRetriever
	assert.NotNil(t, retriever.HybridRetriever)
	assert.NotNil(t, retriever.embeddingProvider)
}

func TestNewRetrieverFromConfig_WithRerank(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.APIKey = "test-key"

	retriever, err := NewRetrieverFromConfig(cfg,
		WithLogger(zap.NewNop()),
		WithEmbeddingType(EmbeddingCohere),
		WithRerankType(RerankCohere),
	)
	require.NoError(t, err)
	require.NotNil(t, retriever)
	assert.NotNil(t, retriever.rerankProvider)
	assert.Equal(t, "cohere-rerank", retriever.rerankProvider.Name())
}

func TestNewRetrieverFromConfig_NilConfig(t *testing.T) {
	_, err := NewRetrieverFromConfig(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is nil")
}

func TestNewRetrieverFromConfig_InvalidEmbedding(t *testing.T) {
	cfg := config.DefaultConfig()
	_, err := NewRetrieverFromConfig(cfg, WithEmbeddingType("bad"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported embedding provider type")
}

func TestNewRetrieverFromConfig_InvalidRerank(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.APIKey = "key"
	_, err := NewRetrieverFromConfig(cfg, WithRerankType("bad"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported rerank provider type")
}

// ---------------------------------------------------------------------------
// NewPineconeVectorStore convenience function
// ---------------------------------------------------------------------------

func TestNewPineconeVectorStore(t *testing.T) {
	store := NewPineconeVectorStore(PineconeConfig{
		APIKey: "test-key",
		Index:  "my-index",
	}, zap.NewNop())
	require.NotNil(t, store)
	assert.Contains(t, typeName(store), "*rag.PineconeStore")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func typeName(v any) string {
	return fmt.Sprintf("%T", v)
}
