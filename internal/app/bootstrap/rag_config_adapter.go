package bootstrap

import (
	ragruntime "github.com/BaSui01/agentflow/rag/runtime"
	"github.com/BaSui01/agentflow/config"
)

// StoreConfigFromApp 将全局 config.Config 中向量存储相关的字段
// 映射为 rag/runtime 自包含的 StoreConfig，解除 rag 层对 config 包的直接依赖。
func StoreConfigFromApp(cfg *config.Config) *ragruntime.StoreConfig {
	if cfg == nil {
		return nil
	}
	return &ragruntime.StoreConfig{
		Qdrant: ragruntime.QdrantStoreConfig{
			Host:               cfg.Qdrant.Host,
			Port:               cfg.Qdrant.Port,
			APIKey:             cfg.Qdrant.APIKey,
			Collection:         cfg.Qdrant.Collection,
			AutoCreateCollection: true,
		},
		Weaviate: ragruntime.WeaviateStoreConfig{
			Host:             cfg.Weaviate.Host,
			Port:             cfg.Weaviate.Port,
			Scheme:           cfg.Weaviate.Scheme,
			APIKey:           cfg.Weaviate.APIKey,
			ClassName:        cfg.Weaviate.ClassName,
			AutoCreateSchema: cfg.Weaviate.AutoCreateSchema,
			Distance:         cfg.Weaviate.Distance,
			HybridAlpha:      cfg.Weaviate.HybridAlpha,
			Timeout:          cfg.Weaviate.Timeout,
		},
		Milvus: ragruntime.MilvusStoreConfig{
			Host:                 cfg.Milvus.Host,
			Port:                 cfg.Milvus.Port,
			Username:             cfg.Milvus.Username,
			Password:             cfg.Milvus.Password,
			Token:                cfg.Milvus.Token,
			Database:             cfg.Milvus.Database,
			Collection:           cfg.Milvus.Collection,
			VectorDimension:      cfg.Milvus.VectorDimension,
			IndexType:            ragruntime.MilvusIndexType(cfg.Milvus.IndexType),
			MetricType:           ragruntime.MilvusMetricType(cfg.Milvus.MetricType),
			AutoCreateCollection: cfg.Milvus.AutoCreateCollection,
			Timeout:              cfg.Milvus.Timeout,
			BatchSize:            cfg.Milvus.BatchSize,
			ConsistencyLevel:     cfg.Milvus.ConsistencyLevel,
		},
		Pinecone: ragruntime.PineconeStoreConfig{
			APIKey:    cfg.Pinecone.APIKey,
			Index:     cfg.Pinecone.Index,
			BaseURL:   cfg.Pinecone.BaseURL,
			Namespace: cfg.Pinecone.Namespace,
			Timeout:   cfg.Pinecone.Timeout,
		},
	}
}
