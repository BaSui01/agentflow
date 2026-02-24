// 默认配置定义与默认值构造函数。
package config

import "time"

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server:    DefaultServerConfig(),
		Agent:     DefaultAgentConfig(),
		Redis:     DefaultRedisConfig(),
		Database:  DefaultDatabaseConfig(),
		Qdrant:    DefaultQdrantConfig(),
		Weaviate:  DefaultWeaviateConfig(),
		Milvus:    DefaultMilvusConfig(),
		MongoDB:   DefaultMongoDBConfig(),
		LLM:       DefaultLLMConfig(),
		Log:       DefaultLogConfig(),
		Telemetry: DefaultTelemetryConfig(),
		Tools:     DefaultToolsConfig(),
	}
}

// DefaultServerConfig 返回默认服务器配置
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		HTTPPort:         8080,
		GRPCPort:         9090,
		MetricsPort:      9091,
		ReadTimeout:      30 * time.Second,
		WriteTimeout:     30 * time.Second,
		ShutdownTimeout:  15 * time.Second,
		AllowQueryAPIKey: false,
		RateLimitRPS:     100,
		RateLimitBurst:   200,
		TenantRateLimitRPS:   50,
		TenantRateLimitBurst: 100,
	}
}

// DefaultAgentConfig 返回默认 Agent 配置
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Name:          "default-agent",
		Description:   "Default AgentFlow agent",
		Model:         "gpt-4",
		SystemPrompt:  "You are a helpful AI assistant.",
		MaxIterations: 10,
		Temperature:   0.7,
		MaxTokens:     4096,
		Timeout:       5 * time.Minute,
		StreamEnabled: true,
		Memory: MemoryConfig{
			Enabled:     true,
			Type:        "buffer",
			MaxMessages: 100,
			TokenLimit:  8000,
		},
	}
}

// DefaultRedisConfig 返回默认 Redis 配置
func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
	}
}

// DefaultDatabaseConfig 返回默认数据库配置
func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Driver:          "postgres",
		Host:            "localhost",
		Port:            5432,
		User:            "agentflow",
		Password:        "",
		Name:            "agentflow",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// DefaultQdrantConfig 返回默认 Qdrant 配置
func DefaultQdrantConfig() QdrantConfig {
	return QdrantConfig{
		Host:       "localhost",
		Port:       6334,
		APIKey:     "",
		Collection: "agentflow_vectors",
	}
}

// DefaultWeaviateConfig 返回默认 Weaviate 配置
func DefaultWeaviateConfig() WeaviateConfig {
	return WeaviateConfig{
		Host:             "localhost",
		Port:             8080,
		Scheme:           "http",
		APIKey:           "",
		ClassName:        "AgentFlowDocuments",
		AutoCreateSchema: true,
		Distance:         "cosine",
		HybridAlpha:      0.5,
		Timeout:          30 * time.Second,
	}
}

// DefaultMilvusConfig 返回默认 Milvus 配置
func DefaultMilvusConfig() MilvusConfig {
	return MilvusConfig{
		Host:                 "localhost",
		Port:                 19530,
		Username:             "",
		Password:             "",
		Token:                "",
		Database:             "default",
		Collection:           "agentflow_vectors",
		VectorDimension:      1536, // OpenAI embedding dimension
		IndexType:            "IVF_FLAT",
		MetricType:           "COSINE",
		AutoCreateCollection: true,
		Timeout:              30 * time.Second,
		BatchSize:            1000,
		ConsistencyLevel:     "Strong",
	}
}

// DefaultMongoDBConfig 返回默认 MongoDB 配置
func DefaultMongoDBConfig() MongoDBConfig {
	return MongoDBConfig{
		Host:                "localhost",
		Port:                27017,
		Database:            "agentflow",
		AuthSource:          "admin",
		MaxPoolSize:         100,
		MinPoolSize:         5,
		ConnectTimeout:      10 * time.Second,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}
}

// DefaultLLMConfig 返回默认 LLM 配置
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		DefaultProvider: "openai",
		APIKey:          "",
		BaseURL:         "",
		Timeout:         2 * time.Minute,
		MaxRetries:      3,
	}
}

// DefaultLogConfig 返回默认日志配置
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:            "info",
		Format:           "json",
		OutputPaths:      []string{"stdout"},
		EnableCaller:     true,
		EnableStacktrace: false,
	}
}

// DefaultTelemetryConfig 返回默认遥测配置
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		Enabled:      false,
		OTLPEndpoint: "localhost:4317",
		OTLPInsecure: false,
		ServiceName:  "agentflow",
		SampleRate:   0.1,
	}
}

// DefaultToolsConfig 返回默认工具提供者配置
func DefaultToolsConfig() ToolsConfig {
	return ToolsConfig{
		Tavily: TavilyToolConfig{
			BaseURL: "https://api.tavily.com",
			Timeout: 15 * time.Second,
		},
		Jina: JinaToolConfig{
			BaseURL: "https://r.jina.ai",
			Timeout: 30 * time.Second,
		},
		Firecrawl: FirecrawlToolConfig{
			BaseURL: "https://api.firecrawl.dev",
			Timeout: 30 * time.Second,
		},
		DuckDuckGo: DuckDuckGoToolConfig{
			Timeout: 15 * time.Second,
		},
		SearXNG: SearXNGToolConfig{
			Timeout: 15 * time.Second,
		},
		HTTPScrape: HTTPScrapeToolConfig{
			UserAgent: "Mozilla/5.0 (compatible; AgentFlow/1.0)",
			Timeout:   30 * time.Second,
		},
	}
}
