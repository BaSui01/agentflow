package runtime

import "time"

// StoreConfig 聚合了 RAG 运行时所需的全部向量存储后端配置。
// 本结构体是 config.Config 中向量存储相关字段的自包含副本，
// 使 rag/runtime 不再直接依赖 config 包。
// 上层调用方负责将 config.Config 映射到本结构体（参见 internal 适配层）。
type StoreConfig struct {
	Qdrant   QdrantStoreConfig
	Weaviate WeaviateStoreConfig
	Milvus   MilvusStoreConfig
	Pinecone PineconeStoreConfig
}

// QdrantStoreConfig Qdrant 向量存储配置
type QdrantStoreConfig struct {
	Host               string
	Port               int
	APIKey             string
	Collection         string
	AutoCreateCollection bool
}

// WeaviateStoreConfig Weaviate 向量存储配置
type WeaviateStoreConfig struct {
	Host             string
	Port             int
	Scheme           string
	APIKey           string
	ClassName        string
	AutoCreateSchema bool
	Distance         string
	HybridAlpha      float64
	Timeout          time.Duration
}

// MilvusStoreConfig Milvus 向量存储配置
type MilvusStoreConfig struct {
	Host                 string
	Port                 int
	Username             string
	Password             string
	Token                string
	Database             string
	Collection           string
	VectorDimension      int
	IndexType            MilvusIndexType
	MetricType           MilvusMetricType
	AutoCreateCollection bool
	Timeout              time.Duration
	BatchSize            int
	ConsistencyLevel     string
}

// PineconeStoreConfig Pinecone 向量存储配置
type PineconeStoreConfig struct {
	APIKey    string
	Index     string
	BaseURL   string
	Namespace string
	Timeout   time.Duration
}
