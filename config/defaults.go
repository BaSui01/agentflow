// 默认配置定义与默认值构造函数。
package config

import "time"

// 默认服务地址常量，替代硬编码
const (
	DefaultRedisAddr    = "localhost:6379"
	DefaultPostgresHost = "localhost"
	DefaultPostgresPort = 5432
	DefaultQdrantHost   = "localhost"
	DefaultQdrantPort   = 6334
	DefaultWeaviateHost = "localhost"
	DefaultWeaviatePort = 8080
	DefaultMilvusHost   = "localhost"
	DefaultMilvusPort   = 19530
	DefaultMongoDBHost  = "localhost"
	DefaultMongoDBPort  = 27017
)

type HostedToolsConfig struct {
	FileOps FileOpsToolConfig `yaml:"file_ops" env:"FILE_OPS"`
	Shell   ShellToolConfig   `yaml:"shell" env:"SHELL"`
	MCP     MCPToolConfig     `yaml:"mcp" env:"MCP"`
}

type FileOpsToolConfig struct {
	Enabled      bool     `yaml:"enabled" env:"ENABLED"`
	AllowedPaths []string `yaml:"allowed_paths" env:"ALLOWED_PATHS"`
	MaxFileSize  int64    `yaml:"max_file_size" env:"MAX_FILE_SIZE"`
}

type ShellToolConfig struct {
	Enabled     bool          `yaml:"enabled" env:"ENABLED"`
	Timeout     time.Duration `yaml:"timeout" env:"TIMEOUT"`
	BlockedCmds []string      `yaml:"blocked_cmds" env:"BLOCKED_CMDS"`
}

type MCPToolConfig struct {
	Enabled bool     `yaml:"enabled" env:"ENABLED"`
	Command string   `yaml:"command" env:"COMMAND"`
	Args    []string `yaml:"args" env:"ARGS"`
	BaseURL string   `yaml:"base_url" env:"BASE_URL"`
}

type WorkflowCheckpointConfig struct {
	Backend string `yaml:"backend" env:"BACKEND"`
}

func DefaultHostedToolsConfig() HostedToolsConfig {
	return HostedToolsConfig{
		FileOps: FileOpsToolConfig{
			Enabled:     false,
			MaxFileSize: 10 << 20,
		},
		Shell: ShellToolConfig{
			Enabled: false,
			Timeout: 30 * time.Second,
		},
	}
}

func DefaultWorkflowCheckpointConfig() WorkflowCheckpointConfig {
	return WorkflowCheckpointConfig{
		Backend: "memory",
	}
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server:             DefaultServerConfig(),
		Agent:              DefaultAgentConfig(),
		Redis:              DefaultRedisConfig(),
		Database:           DefaultDatabaseConfig(),
		Qdrant:             DefaultQdrantConfig(),
		Weaviate:           DefaultWeaviateConfig(),
		Milvus:             DefaultMilvusConfig(),
		MongoDB:            DefaultMongoDBConfig(),
		LLM:                DefaultLLMConfig(),
		Multimodal:         DefaultMultimodalConfig(),
		Log:                DefaultLogConfig(),
		Telemetry:          DefaultTelemetryConfig(),
		Tools:              DefaultToolsConfig(),
		Cache:              DefaultCacheConfig(),
		Budget:             DefaultBudgetConfig(),
		HostedTools:        DefaultHostedToolsConfig(),
		WorkflowCheckpoint: DefaultWorkflowCheckpointConfig(),
	}
}

// DefaultServerConfig 返回默认服务器配置
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		HTTPPort:             8080,
		GRPCPort:             9090,
		MetricsPort:          9091,
		ReadTimeout:          30 * time.Second,
		WriteTimeout:         30 * time.Second,
		ShutdownTimeout:      15 * time.Second,
		RateLimitRPS:         100,
		RateLimitBurst:       200,
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
		ToolModel:     "",
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
		Checkpoint: CheckpointConfig{
			Enabled:     true,
			Backend:     StorageTypeFile,
			FilePath:    "./checkpoints",
			RedisPrefix: "agentflow:checkpoint",
			RedisTTL:    24 * time.Hour,
		},
	}
}

// DefaultRedisConfig 返回默认 Redis 配置
func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:         DefaultRedisAddr,
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
	}
}

// DefaultDatabaseConfig 返回默认数据库配置
func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Driver:          StorageTypePostgres,
		Host:            DefaultPostgresHost,
		Port:            DefaultPostgresPort,
		User:            "agentflow",
		Password:        "",
		Name:            "agentflow",
		SSLMode:         "require",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// DefaultQdrantConfig 返回默认 Qdrant 配置
func DefaultQdrantConfig() QdrantConfig {
	return QdrantConfig{
		Host:       DefaultQdrantHost,
		Port:       DefaultQdrantPort,
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
		Host:                 DefaultMilvusHost,
		Port:                 DefaultMilvusPort,
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
		Host:                DefaultMongoDBHost,
		Port:                DefaultMongoDBPort,
		Database:            "agentflow",
		AuthSource:          "admin",
		MaxPoolSize:         100,
		MinPoolSize:         5,
		ConnectTimeout:      10 * time.Second,
		Timeout:             30 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}
}

// DefaultLLMConfig 返回默认 LLM 连接配置
func DefaultLLMConfig() LLMConnectionConfig {
	return LLMConnectionConfig{
		MainProviderMode: LLMMainProviderModeLegacy,
		DefaultProvider:  "openai",
		ToolProvider:     "",
		APIKey:           "",
		ToolAPIKey:       "",
		BaseURL:          "",
		ToolBaseURL:      "",
		Timeout:          2 * time.Minute,
		ToolTimeout:      0,
		MaxRetries:       3,
		ToolMaxRetries:   0,
	}
}

// DefaultMultimodalConfig 返回默认多模态框架配置
func DefaultMultimodalConfig() MultimodalConfig {
	return MultimodalConfig{
		Enabled:                 true,
		ReferenceMaxSizeBytes:   8 << 20, // 8MB
		ReferenceTTL:            2 * time.Hour,
		ReferenceStoreBackend:   StorageTypeRedis,
		ReferenceStoreKeyPrefix: "agentflow:mm:ref",
		DefaultImageProvider:    "",
		DefaultVideoProvider:    "",
		DefaultChatModel:        "",
		Image: MultimodalImageConfig{
			OpenAIAPIKey:     "",
			OpenAIBaseURL:    "",
			GeminiAPIKey:     "",
			FluxAPIKey:       "",
			FluxBaseURL:      "https://api.bfl.ai",
			StabilityAPIKey:  "",
			StabilityBaseURL: "https://api.stability.ai",
			IdeogramAPIKey:   "",
			IdeogramBaseURL:  "https://api.ideogram.ai",
			TongyiAPIKey:     "",
			TongyiBaseURL:    "https://dashscope.aliyuncs.com",
			ZhipuAPIKey:      "",
			ZhipuBaseURL:     "https://open.bigmodel.cn",
			BaiduAPIKey:      "",
			BaiduSecretKey:   "",
			BaiduBaseURL:     "https://aip.baidubce.com",
			DoubaoAPIKey:     "",
			DoubaoBaseURL:    "https://ark.cn-beijing.volces.com",
			TencentSecretId:  "",
			TencentSecretKey: "",
			TencentBaseURL:   "https://aiart.tencentcloudapi.com",
		},
		Video: MultimodalVideoConfig{
			RunwayAPIKey:    "",
			RunwayBaseURL:   "https://api.runwayml.com",
			VeoAPIKey:       "",
			VeoBaseURL:      "https://generativelanguage.googleapis.com",
			GoogleAPIKey:    "",
			GoogleBaseURL:   "https://generativelanguage.googleapis.com",
			SoraAPIKey:      "",
			SoraBaseURL:     "https://api.openai.com",
			KlingAPIKey:     "",
			KlingBaseURL:    "https://api.klingai.com",
			LumaAPIKey:      "",
			LumaBaseURL:     "https://api.lumalabs.ai",
			MiniMaxAPIKey:   "",
			MiniMaxBaseURL:  "https://api.minimax.chat",
			SeedanceAPIKey:  "",
			SeedanceBaseURL: "https://api.seedance.ai",
		},
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
			UserAgent: "Mozilla/5.0 (compatible; AgentFlow/1.6)",
			Timeout:   30 * time.Second,
		},
	}
}

// DefaultCacheConfig 返回默认缓存配置
// 与 cache.DefaultCacheConfig() 对齐
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:      true,
		LocalMaxSize: 1000,
		LocalTTL:     5 * time.Minute,
		EnableRedis:  false,
		RedisTTL:     1 * time.Hour,
		KeyStrategy:  "hash",
	}
}

// DefaultBudgetConfig 返回默认预算配置
// 与 budget.DefaultBudgetConfig() 对齐
func DefaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		Enabled:             true,
		MaxTokensPerRequest: 100000,
		MaxTokensPerMinute:  500000,
		MaxTokensPerHour:    5000000,
		MaxTokensPerDay:     50000000,
		MaxCostPerRequest:   10.0,
		MaxCostPerDay:       1000.0,
		AlertThreshold:      0.8,
		AutoThrottle:        true,
		ThrottleDelay:       time.Second,
	}
}
