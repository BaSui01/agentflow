package runtime

import (
	"fmt"

	"github.com/BaSui01/agentflow/rag/core"
	"go.uber.org/zap"
)

func newVectorStoreFromConfig(cfg *StoreConfig, storeType core.VectorStoreType, logger *zap.Logger) (core.VectorStore, error) {
	if cfg == nil {
		return nil, fmt.Errorf("store config is nil")
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
		return NewPineconeStore(mapPineconeConfig(&cfg.Pinecone), logger), nil
	default:
		return nil, fmt.Errorf("unsupported vector store type: %s", storeType)
	}
}

func mapQdrantConfig(c *QdrantStoreConfig) QdrantConfig {
	return QdrantConfig{
		Host:               c.Host,
		Port:               c.Port,
		APIKey:             c.APIKey,
		Collection:         c.Collection,
		AutoCreateCollection: c.AutoCreateCollection,
	}
}

func mapWeaviateConfig(c *WeaviateStoreConfig) WeaviateConfig {
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

func mapMilvusConfig(c *MilvusStoreConfig) MilvusConfig {
	return MilvusConfig{
		Host:                 c.Host,
		Port:                 c.Port,
		Username:             c.Username,
		Password:             c.Password,
		Token:                c.Token,
		Database:             c.Database,
		Collection:           c.Collection,
		VectorDimension:      c.VectorDimension,
		IndexType:            c.IndexType,
		MetricType:           c.MetricType,
		AutoCreateCollection: c.AutoCreateCollection,
		Timeout:              c.Timeout,
		BatchSize:            c.BatchSize,
		ConsistencyLevel:     c.ConsistencyLevel,
	}
}

func mapPineconeConfig(c *PineconeStoreConfig) PineconeConfig {
	return PineconeConfig{
		APIKey:    c.APIKey,
		Index:     c.Index,
		BaseURL:   c.BaseURL,
		Namespace: c.Namespace,
		Timeout:   c.Timeout,
	}
}
