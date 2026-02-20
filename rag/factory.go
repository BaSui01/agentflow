// Config → RAG 桥接层。
//
// 提供工厂函数，将全局 config.Config 转换为 rag 包的运行时实例，
// 消除 config 包和 rag 包之间的手动配置映射。
package rag

import (
	"fmt"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm/embedding"
	"github.com/BaSui01/agentflow/llm/rerank"
	"go.uber.org/zap"
)

// VectorStoreType 标识要创建的向量存储后端。
type VectorStoreType string

const (
	VectorStoreMemory   VectorStoreType = "memory"
	VectorStoreQdrant   VectorStoreType = "qdrant"
	VectorStoreWeaviate VectorStoreType = "weaviate"
	VectorStoreMilvus   VectorStoreType = "milvus"
)

// NewVectorStoreFromConfig 根据指定的后端类型和全局配置创建 VectorStore。
// 当 storeType 为空字符串时，默认使用 InMemory 后端。
func NewVectorStoreFromConfig(cfg *config.Config, storeType VectorStoreType, logger *zap.Logger) (VectorStore, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	switch storeType {
	case VectorStoreMemory, "":
		return NewInMemoryVectorStore(logger), nil

	case VectorStoreQdrant:
		ragCfg := mapQdrantConfig(&cfg.Qdrant)
		return NewQdrantStore(ragCfg, logger), nil

	case VectorStoreWeaviate:
		ragCfg := mapWeaviateConfig(&cfg.Weaviate)
		return NewWeaviateStore(ragCfg, logger), nil

	case VectorStoreMilvus:
		ragCfg := mapMilvusConfig(&cfg.Milvus)
		return NewMilvusStore(ragCfg, logger), nil

	default:
		return nil, fmt.Errorf("unsupported vector store type: %s", storeType)
	}
}

// EmbeddingProviderType 标识要创建的嵌入提供者。
type EmbeddingProviderType string

const (
	EmbeddingOpenAI EmbeddingProviderType = "openai"
	EmbeddingCohere EmbeddingProviderType = "cohere"
	EmbeddingVoyage EmbeddingProviderType = "voyage"
	EmbeddingJina   EmbeddingProviderType = "jina"
	EmbeddingGemini EmbeddingProviderType = "gemini"
)

// NewEmbeddingProviderFromConfig 根据 LLM 配置创建 embedding.Provider。
// providerType 指定嵌入提供者类型；为空时默认使用 "openai"。
func NewEmbeddingProviderFromConfig(cfg *config.Config, providerType EmbeddingProviderType) (embedding.Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	apiKey := cfg.LLM.APIKey
	if providerType == "" {
		providerType = EmbeddingProviderType(cfg.LLM.DefaultProvider)
	}
	if providerType == "" {
		providerType = EmbeddingOpenAI
	}

	switch providerType {
	case EmbeddingOpenAI:
		embCfg := embedding.OpenAIConfig{APIKey: apiKey}
		if cfg.LLM.BaseURL != "" {
			embCfg.BaseURL = cfg.LLM.BaseURL
		}
		return embedding.NewOpenAIProvider(embCfg), nil

	case EmbeddingCohere:
		return embedding.NewCohereProvider(embedding.CohereConfig{APIKey: apiKey}), nil

	case EmbeddingVoyage:
		return embedding.NewVoyageProvider(embedding.VoyageConfig{APIKey: apiKey}), nil

	case EmbeddingJina:
		return embedding.NewJinaProvider(embedding.JinaConfig{APIKey: apiKey}), nil

	case EmbeddingGemini:
		return embedding.NewGeminiProvider(embedding.GeminiConfig{APIKey: apiKey}), nil

	default:
		return nil, fmt.Errorf("unsupported embedding provider type: %s", providerType)
	}
}

// NewRetrieverFromConfig 一键创建完整的 EnhancedRetriever。
// 它组装向量存储、嵌入提供者和可选的重排提供者。
func NewRetrieverFromConfig(cfg *config.Config, opts ...RetrieverOption) (*EnhancedRetriever, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	o := defaultRetrieverOptions()
	for _, opt := range opts {
		opt(&o)
	}

	logger := o.logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// 创建嵌入提供者
	embProvider, err := NewEmbeddingProviderFromConfig(cfg, o.embeddingType)
	if err != nil {
		return nil, fmt.Errorf("create embedding provider: %w", err)
	}

	// 创建重排提供者（可选）
	var rerankProv RerankProvider
	if o.rerankType != "" {
		rerankProv, err = newRerankProvider(cfg, o.rerankType)
		if err != nil {
			return nil, fmt.Errorf("create rerank provider: %w", err)
		}
	}

	return NewEnhancedRetriever(EnhancedRetrieverConfig{
		HybridConfig:      DefaultHybridRetrievalConfig(),
		EmbeddingProvider: embProvider,
		RerankProvider:    rerankProv,
	}, logger), nil
}

// RerankProviderType 标识要创建的重排提供者。
type RerankProviderType string

const (
	RerankCohere RerankProviderType = "cohere"
	RerankVoyage RerankProviderType = "voyage"
	RerankJina   RerankProviderType = "jina"
)

// RetrieverOption 配置 NewRetrieverFromConfig 的可选参数。
type RetrieverOption func(*retrieverOptions)

type retrieverOptions struct {
	logger        *zap.Logger
	embeddingType EmbeddingProviderType
	rerankType    RerankProviderType
}

func defaultRetrieverOptions() retrieverOptions {
	return retrieverOptions{}
}

// WithLogger 设置日志记录器。
func WithLogger(l *zap.Logger) RetrieverOption {
	return func(o *retrieverOptions) { o.logger = l }
}

// WithEmbeddingType 指定嵌入提供者类型。
func WithEmbeddingType(t EmbeddingProviderType) RetrieverOption {
	return func(o *retrieverOptions) { o.embeddingType = t }
}

// WithRerankType 指定重排提供者类型。
func WithRerankType(t RerankProviderType) RetrieverOption {
	return func(o *retrieverOptions) { o.rerankType = t }
}

// --- 内部配置映射函数 ---

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

func newRerankProvider(cfg *config.Config, t RerankProviderType) (RerankProvider, error) {
	apiKey := cfg.LLM.APIKey
	switch t {
	case RerankCohere:
		return rerank.NewCohereProvider(rerank.CohereConfig{APIKey: apiKey}), nil
	case RerankVoyage:
		return rerank.NewVoyageProvider(rerank.VoyageConfig{APIKey: apiKey}), nil
	case RerankJina:
		return rerank.NewJinaProvider(rerank.JinaConfig{APIKey: apiKey}), nil
	default:
		return nil, fmt.Errorf("unsupported rerank provider type: %s", t)
	}
}
