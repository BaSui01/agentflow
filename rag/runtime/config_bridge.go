package runtime

import (
	"fmt"

	"github.com/BaSui01/agentflow/config"
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
		return NewInMemoryVectorStore(logger), nil
	case core.VectorStoreQdrant:
		return NewQdrantStore(mapQdrantConfig(&cfg.Qdrant), logger), nil
	case core.VectorStoreWeaviate:
		return NewWeaviateStore(mapWeaviateConfig(&cfg.Weaviate), logger), nil
	case core.VectorStoreMilvus:
		return NewMilvusStore(mapMilvusConfig(&cfg.Milvus), logger), nil
	case core.VectorStorePinecone:
		return NewPineconeStore(PineconeConfig{}, logger), nil
	default:
		return nil, fmt.Errorf("unsupported vector store type: %s", storeType)
	}
}

func mapQdrantConfig(c *config.QdrantConfig) QdrantConfig {
	return QdrantConfig{
		Host:                 c.Host,
		Port:                 c.Port,
		APIKey:               c.APIKey,
		Collection:           c.Collection,
		AutoCreateCollection: true,
	}
}

func mapWeaviateConfig(c *config.WeaviateConfig) WeaviateConfig {
	return WeaviateConfig{
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

func mapMilvusConfig(c *config.MilvusConfig) MilvusConfig {
	return MilvusConfig{
		Host:                 c.Host,
		Port:                 c.Port,
		Username:             c.Username,
		Password:             c.Password,
		Token:                c.Token,
		Database:             c.Database,
		Collection:           c.Collection,
		VectorDimension:      c.VectorDimension,
		IndexType:            MilvusIndexType(c.IndexType),
		MetricType:           MilvusMetricType(c.MetricType),
		AutoCreateCollection: c.AutoCreateCollection,
		Timeout:              c.Timeout,
		BatchSize:            c.BatchSize,
		ConsistencyLevel:     c.ConsistencyLevel,
	}
}
