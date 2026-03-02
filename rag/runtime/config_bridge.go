package runtime

import (
	"fmt"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

func newVectorStoreFromConfig(cfg *config.Config, storeType core.VectorStoreType, logger *zap.Logger) (core.VectorStore, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	switch storeType {
	case core.VectorStoreMemory, "":
		return rag.NewInMemoryVectorStore(logger), nil
	case core.VectorStoreQdrant:
		return rag.NewQdrantStore(mapQdrantConfig(&cfg.Qdrant), logger), nil
	case core.VectorStoreWeaviate:
		return rag.NewWeaviateStore(mapWeaviateConfig(&cfg.Weaviate), logger), nil
	case core.VectorStoreMilvus:
		return rag.NewMilvusStore(mapMilvusConfig(&cfg.Milvus), logger), nil
	case core.VectorStorePinecone:
		return rag.NewPineconeStore(rag.PineconeConfig{}, logger), nil
	default:
		return nil, fmt.Errorf("unsupported vector store type: %s", storeType)
	}
}

func mapQdrantConfig(c *config.QdrantConfig) rag.QdrantConfig {
	return rag.QdrantConfig{
		Host:                 c.Host,
		Port:                 c.Port,
		APIKey:               c.APIKey,
		Collection:           c.Collection,
		AutoCreateCollection: true,
	}
}

func mapWeaviateConfig(c *config.WeaviateConfig) rag.WeaviateConfig {
	return rag.WeaviateConfig{
		Host:             c.Host,
		Port:             c.Port,
		Scheme:           c.Scheme,
		APIKey:           c.APIKey,
		ClassName:        c.ClassName,
		AutoCreateSchema: c.AutoCreateSchema,
		Distance:         c.Distance,
		HybridAlpha:      c.HybridAlpha,
		Timeout:          c.Timeout,
	}
}

func mapMilvusConfig(c *config.MilvusConfig) rag.MilvusConfig {
	return rag.MilvusConfig{
		Host:                 c.Host,
		Port:                 c.Port,
		Username:             c.Username,
		Password:             c.Password,
		Token:                c.Token,
		Database:             c.Database,
		Collection:           c.Collection,
		VectorDimension:      c.VectorDimension,
		IndexType:            rag.MilvusIndexType(c.IndexType),
		MetricType:           rag.MilvusMetricType(c.MetricType),
		AutoCreateCollection: c.AutoCreateCollection,
		Timeout:              c.Timeout,
		BatchSize:            c.BatchSize,
		ConsistencyLevel:     c.ConsistencyLevel,
	}
}
