// 配置加载器实现。
//
// 支持默认值、YAML 文件与环境变量覆盖，并按优先级合并配置。
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// 配置校验边界常量
const (
	validateHTTPPortMax      = 65535
	validateMaxIterationsMax = 10000
	validateTemperatureMax   = 2
	validateMaxTokensMax     = 128000
)

// Checkpoint 存储类型常量，替代 magic string
const (
	StorageTypeFile     = "file"
	StorageTypeRedis    = "redis"
	StorageTypePostgres = "postgres"
)

// LLM 主入口模式常量。
const (
	LLMMainProviderModeLegacy        = "legacy"
	LLMMainProviderModeChannelRouted = "channel_routed"
)

// --- 核心配置结构 ---

// Config 是 AgentFlow 的完整配置结构
type Config struct {
	// Server 服务器配置
	Server ServerConfig `yaml:"server" env:"SERVER"`

	// Agent 默认 Agent 配置
	Agent AgentConfig `yaml:"agent" env:"AGENT"`

	// Redis 配置（缓存与可选的多模态引用存储复用）。
	Redis RedisConfig `yaml:"redis" env:"REDIS"`

	// Database 数据库配置
	Database DatabaseConfig `yaml:"database" env:"DATABASE"`

	// Qdrant 向量存储配置
	Qdrant QdrantConfig `yaml:"qdrant" env:"QDRANT"`

	// Weaviate 向量存储配置
	Weaviate WeaviateConfig `yaml:"weaviate" env:"WEAVIATE"`

	// Milvus 向量存储配置
	Milvus MilvusConfig `yaml:"milvus" env:"MILVUS"`

	// MongoDB 文档型数据存储配置
	MongoDB MongoDBConfig `yaml:"mongodb" env:"MONGODB"`

	// LLM 大语言模型配置
	LLM LLMConnectionConfig `yaml:"llm" env:"LLM"`

	// Multimodal 多模态框架能力配置
	Multimodal MultimodalConfig `yaml:"multimodal" env:"MULTIMODAL"`

	// Log 日志配置
	Log LogConfig `yaml:"log" env:"LOG"`

	// Telemetry 遥测配置
	Telemetry TelemetryConfig `yaml:"telemetry" env:"TELEMETRY"`

	// Tools 工具提供者配置
	Tools ToolsConfig `yaml:"tools" env:"TOOLS"`

	// Cache LLM 缓存配置
	Cache CacheConfig `yaml:"cache" env:"CACHE"`

	// Budget Token 预算管理配置
	Budget BudgetConfig `yaml:"budget" env:"BUDGET"`

	// HostedTools Hosted 工具配置
	HostedTools HostedToolsConfig `yaml:"hosted_tools" env:"HOSTED_TOOLS"`

	// WorkflowCheckpoint Workflow 检查点配置
	WorkflowCheckpoint WorkflowCheckpointConfig `yaml:"workflow_checkpoint" env:"WORKFLOW_CHECKPOINT"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	// HTTP 端口
	HTTPPort int `yaml:"http_port" env:"HTTP_PORT"`
	// gRPC 端口 — Reserved for future gRPC support, not currently used.
	GRPCPort int `yaml:"grpc_port" env:"GRPC_PORT"`
	// Metrics 端口
	MetricsPort int `yaml:"metrics_port" env:"METRICS_PORT"`
	// 读取超时
	ReadTimeout time.Duration `yaml:"read_timeout" env:"READ_TIMEOUT"`
	// 写入超时
	WriteTimeout time.Duration `yaml:"write_timeout" env:"WRITE_TIMEOUT"`
	// 优雅关闭超时
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`
	// CORS 允许的源
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins" json:"cors_allowed_origins,omitempty"`
	// API 密钥（json:"-" 防止在 JSON 响应中暴露）
	// X-006: 安全建议 — 生产环境中应通过环境变量设置 API 密钥，
	// 避免在 YAML 配置文件中明文存储。
	APIKeys []string `yaml:"api_keys" json:"-"`
	// 限流 RPS，默认 100
	RateLimitRPS int `yaml:"rate_limit_rps" json:"rate_limit_rps,omitempty"`
	// 限流 Burst，默认 200
	RateLimitBurst int `yaml:"rate_limit_burst" json:"rate_limit_burst,omitempty"`
	// JWT 认证配置
	JWT JWTConfig `yaml:"jwt" json:"jwt,omitempty"`
	// 租户级限流 RPS，默认 50
	TenantRateLimitRPS int `yaml:"tenant_rate_limit_rps" json:"tenant_rate_limit_rps,omitempty"`
	// 租户级限流 Burst，默认 100
	TenantRateLimitBurst int `yaml:"tenant_rate_limit_burst" json:"tenant_rate_limit_burst,omitempty"`
	// AllowNoAuth 允许在无认证配置时启动（默认 false）。
	// 生产环境必须显式设置为 true 才能在无 JWT/API Key 配置时启动。
	AllowNoAuth bool `yaml:"allow_no_auth" env:"ALLOW_NO_AUTH" json:"allow_no_auth,omitempty"`
}

// JWTConfig JWT 认证配置
type JWTConfig struct {
	// HMAC 签名密钥
	Secret string `yaml:"secret" env:"SECRET" json:"-"`
	// RSA 公钥（PEM 格式）
	PublicKey string `yaml:"public_key" env:"PUBLIC_KEY" json:"-"`
	// 期望的签发者
	Issuer string `yaml:"issuer" env:"ISSUER" json:"issuer,omitempty"`
	// 期望的受众
	Audience string `yaml:"audience" env:"AUDIENCE" json:"audience,omitempty"`
	// X-012: 默认 token 有效期，未设置时使用 1h
	Expiration time.Duration `yaml:"expiration" env:"EXPIRATION" json:"expiration,omitempty"`
}

// AgentConfig Agent 配置，用于 YAML/环境变量加载（扁平结构）。
// 运行时构建链路统一转换为 types.AgentConfig，并交给 runtime.Builder。
type AgentConfig struct {
	// 名称
	Name string `yaml:"name" env:"NAME"`
	// 描述
	Description string `yaml:"description" env:"DESCRIPTION"`
	// 模型名称
	Model string `yaml:"model" env:"MODEL"`
	// 工具调用阶段模型名称（可选，未设置时回退使用 model）
	ToolModel string `yaml:"tool_model" env:"TOOL_MODEL"`
	// 系统提示词
	SystemPrompt string `yaml:"system_prompt" env:"SYSTEM_PROMPT"`
	// 最大迭代次数
	MaxIterations int `yaml:"max_iterations" env:"MAX_ITERATIONS"`
	// 温度参数
	Temperature float64 `yaml:"temperature" env:"TEMPERATURE"`
	// 最大 Token 数
	MaxTokens int `yaml:"max_tokens" env:"MAX_TOKENS"`
	// 超时时间
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
	// 是否启用流式输出
	StreamEnabled bool `yaml:"stream_enabled" env:"STREAM_ENABLED"`
	// 记忆配置
	Memory MemoryConfig `yaml:"memory" env:"MEMORY"`
	// 检查点配置
	Checkpoint CheckpointConfig `yaml:"checkpoint" env:"CHECKPOINT"`
}

// CheckpointConfig Agent 检查点存储配置。
type CheckpointConfig struct {
	// 是否启用检查点持久化
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// 后端类型: file, redis, postgres
	Backend string `yaml:"backend" env:"BACKEND"`
	// 文件后端目录
	FilePath string `yaml:"file_path" env:"FILE_PATH"`
	// Redis 键前缀
	RedisPrefix string `yaml:"redis_prefix" env:"REDIS_PREFIX"`
	// Redis TTL
	RedisTTL time.Duration `yaml:"redis_ttl" env:"REDIS_TTL"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	// 是否启用
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// 类型: buffer, summary, vector
	Type string `yaml:"type" env:"TYPE"`
	// 最大消息数
	MaxMessages int `yaml:"max_messages" env:"MAX_MESSAGES"`
	// Token 限制
	TokenLimit int `yaml:"token_limit" env:"TOKEN_LIMIT"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	// 地址
	Addr string `yaml:"addr" env:"ADDR"`
	// 密码
	Password string `yaml:"password" env:"PASSWORD"`
	// 数据库编号
	DB int `yaml:"db" env:"DB"`
	// 连接池大小
	PoolSize int `yaml:"pool_size" env:"POOL_SIZE"`
	// 最小空闲连接
	MinIdleConns int `yaml:"min_idle_conns" env:"MIN_IDLE_CONNS"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	// 驱动类型: postgres, mysql, sqlite
	Driver string `yaml:"driver" env:"DRIVER"`
	// 主机
	Host string `yaml:"host" env:"HOST"`
	// 端口
	Port int `yaml:"port" env:"PORT"`
	// 用户名
	User string `yaml:"user" env:"USER"`
	// 密码
	Password string `yaml:"password" env:"PASSWORD"`
	// 数据库名
	Name string `yaml:"name" env:"NAME"`
	// SSL 模式
	SSLMode string `yaml:"ssl_mode" env:"SSL_MODE"`
	// 最大连接数
	MaxOpenConns int `yaml:"max_open_conns" env:"MAX_OPEN_CONNS"`
	// 最大空闲连接
	MaxIdleConns int `yaml:"max_idle_conns" env:"MAX_IDLE_CONNS"`
	// 连接最大生命周期
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"CONN_MAX_LIFETIME"`
}

// QdrantConfig Qdrant 向量存储配置
type QdrantConfig struct {
	// 主机
	Host string `yaml:"host" env:"HOST"`
	// gRPC 端口
	Port int `yaml:"port" env:"PORT"`
	// API Key（可选）
	APIKey string `yaml:"api_key" env:"API_KEY"`
	// 默认集合名
	Collection string `yaml:"collection" env:"COLLECTION"`
}

// WeaviateConfig Weaviate 向量存储配置
type WeaviateConfig struct {
	// 主机
	Host string `yaml:"host" env:"HOST"`
	// HTTP 端口
	Port int `yaml:"port" env:"PORT"`
	// 协议: http 或 https
	Scheme string `yaml:"scheme" env:"SCHEME"`
	// API Key（可选）
	APIKey string `yaml:"api_key" env:"API_KEY"`
	// 默认类名（集合名）
	ClassName string `yaml:"class_name" env:"CLASS_NAME"`
	// 是否自动创建 Schema
	AutoCreateSchema bool `yaml:"auto_create_schema" env:"AUTO_CREATE_SCHEMA"`
	// 距离度量: cosine, dot, l2
	Distance string `yaml:"distance" env:"DISTANCE"`
	// 混合搜索 Alpha 值 (0=BM25, 1=向量)
	HybridAlpha float64 `yaml:"hybrid_alpha" env:"HYBRID_ALPHA"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// MilvusConfig Milvus 向量存储配置
type MilvusConfig struct {
	// 主机
	Host string `yaml:"host" env:"HOST"`
	// gRPC 端口
	Port int `yaml:"port" env:"PORT"`
	// 用户名（可选）
	Username string `yaml:"username" env:"USERNAME"`
	// 密码（可选）
	Password string `yaml:"password" env:"PASSWORD"`
	// Token（用于 Zilliz Cloud）
	Token string `yaml:"token" env:"TOKEN"`
	// 数据库名
	Database string `yaml:"database" env:"DATABASE"`
	// 默认集合名
	Collection string `yaml:"collection" env:"COLLECTION"`
	// 向量维度
	VectorDimension int `yaml:"vector_dimension" env:"VECTOR_DIMENSION"`
	// 索引类型: IVF_FLAT, HNSW, FLAT, IVF_SQ8, IVF_PQ
	IndexType string `yaml:"index_type" env:"INDEX_TYPE"`
	// 距离度量: L2, IP, COSINE
	MetricType string `yaml:"metric_type" env:"METRIC_TYPE"`
	// 是否自动创建集合
	AutoCreateCollection bool `yaml:"auto_create_collection" env:"AUTO_CREATE_COLLECTION"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
	// 批量操作大小
	BatchSize int `yaml:"batch_size" env:"BATCH_SIZE"`
	// 一致性级别: Strong, Session, Bounded, Eventually
	ConsistencyLevel string `yaml:"consistency_level" env:"CONSISTENCY_LEVEL"`
}

// MongoDBConfig MongoDB 文档型数据存储配置
type MongoDBConfig struct {
	// 连接 URI（优先级最高，设置后忽略 Host/Port/User/Password）
	URI string `yaml:"uri" env:"URI"`
	// 主机
	Host string `yaml:"host" env:"HOST"`
	// 端口
	Port int `yaml:"port" env:"PORT"`
	// 用户名
	User string `yaml:"user" env:"USER"`
	// 密码
	Password string `yaml:"password" env:"PASSWORD"`
	// 数据库名
	Database string `yaml:"database" env:"DATABASE"`
	// 认证数据库
	AuthSource string `yaml:"auth_source" env:"AUTH_SOURCE"`
	// 副本集名称（可选）
	ReplicaSet string `yaml:"replica_set" env:"REPLICA_SET"`
	// 最大连接池大小
	MaxPoolSize int `yaml:"max_pool_size" env:"MAX_POOL_SIZE"`
	// 最小连接池大小
	MinPoolSize int `yaml:"min_pool_size" env:"MIN_POOL_SIZE"`
	// 连接超时
	ConnectTimeout time.Duration `yaml:"connect_timeout" env:"CONNECT_TIMEOUT"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
	// 健康检查间隔
	HealthCheckInterval time.Duration `yaml:"health_check_interval" env:"HEALTH_CHECK_INTERVAL"`
	// 是否启用 TLS
	TLSEnabled bool `yaml:"tls_enabled" env:"TLS_ENABLED"`
	// TLS CA 证书路径
	TLSCAFile string `yaml:"tls_ca_file" env:"TLS_CA_FILE"`
	// TLS 客户端证书路径
	TLSCertFile string `yaml:"tls_cert_file" env:"TLS_CERT_FILE"`
	// TLS 客户端密钥路径
	TLSKeyFile string `yaml:"tls_key_file" env:"TLS_KEY_FILE"`
}

// LLMConnectionConfig 连接级 LLM 配置（YAML 反序列化用）。
// 注意：与 types.LLMConfig（运行时 Agent 配置）和 llm/config.LLMConfig（路由/降级配置）不同，
// 此结构体仅用于应用启动时的连接参数加载。
type LLMConnectionConfig struct {
	// 主文本 Provider 入口模式：legacy 或 channel_routed。
	// 空值按 legacy 处理；自定义模式保留给 bootstrap builder registry 扩展。
	MainProviderMode string `yaml:"main_provider_mode" env:"MAIN_PROVIDER_MODE"`
	// 默认 Provider
	DefaultProvider string `yaml:"default_provider" env:"DEFAULT_PROVIDER"`
	// 工具调用阶段 Provider（可选，未设置时回退 default_provider）
	ToolProvider string `yaml:"tool_provider" env:"TOOL_PROVIDER"`
	// API Key（通用）
	// X-006: 安全建议 — 生产环境中应通过环境变量 AGENTFLOW_LLM_API_KEY 设置，
	// 避免在 YAML 配置文件中明文存储 API Key。
	APIKey string `yaml:"api_key" env:"API_KEY"`
	// 工具调用阶段 API Key（可选，未设置时回退 api_key）
	ToolAPIKey string `yaml:"tool_api_key" env:"TOOL_API_KEY"`
	// 基础 URL（可选）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 工具调用阶段基础 URL（可选，未设置时回退 base_url）
	ToolBaseURL string `yaml:"tool_base_url" env:"TOOL_BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
	// 工具调用阶段请求超时（可选，未设置时回退 timeout）
	ToolTimeout time.Duration `yaml:"tool_timeout" env:"TOOL_TIMEOUT"`
	// 最大重试次数
	MaxRetries int `yaml:"max_retries" env:"MAX_RETRIES"`
	// 工具调用阶段最大重试次数（可选，未设置时回退 max_retries）
	ToolMaxRetries int `yaml:"tool_max_retries" env:"TOOL_MAX_RETRIES"`
}

// NormalizeLLMMainProviderMode canonicalizes configured main provider mode.
func NormalizeLLMMainProviderMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", LLMMainProviderModeLegacy:
		return LLMMainProviderModeLegacy
	case "channel", "channel-routed", LLMMainProviderModeChannelRouted:
		return LLMMainProviderModeChannelRouted
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// MultimodalConfig 多模态框架配置（能力层，不绑定具体业务）。
type MultimodalConfig struct {
	// 是否启用多模态 API 路由
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// 引用图上传的最大字节数
	ReferenceMaxSizeBytes int64 `yaml:"reference_max_size_bytes" env:"REFERENCE_MAX_SIZE_BYTES"`
	// 引用图默认存活时长
	ReferenceTTL time.Duration `yaml:"reference_ttl" env:"REFERENCE_TTL"`
	// 引用图存储后端（仅支持 redis）
	ReferenceStoreBackend string `yaml:"reference_store_backend" env:"REFERENCE_STORE_BACKEND"`
	// 引用图存储 key 前缀（Redis 后端使用）
	ReferenceStoreKeyPrefix string `yaml:"reference_store_key_prefix" env:"REFERENCE_STORE_KEY_PREFIX"`
	// 默认图像提供商标识（openai/gemini 等）
	DefaultImageProvider string `yaml:"default_image_provider" env:"DEFAULT_IMAGE_PROVIDER"`
	// 默认视频提供商标识（runway/veo 等）
	DefaultVideoProvider string `yaml:"default_video_provider" env:"DEFAULT_VIDEO_PROVIDER"`
	// 默认对话模型（多模态 chat 未传 model 时使用；空则回退 agent.model）
	DefaultChatModel string `yaml:"default_chat_model" env:"DEFAULT_CHAT_MODEL"`
	// 图像提供商配置
	Image MultimodalImageConfig `yaml:"image" env:"IMAGE"`
	// 视频提供商配置
	Video MultimodalVideoConfig `yaml:"video" env:"VIDEO"`
}

type MultimodalImageConfig struct {
	OpenAIAPIKey     string `yaml:"openai_api_key" env:"OPENAI_API_KEY" json:"-"`
	OpenAIBaseURL    string `yaml:"openai_base_url" env:"OPENAI_BASE_URL"`
	GeminiAPIKey     string `yaml:"gemini_api_key" env:"GEMINI_API_KEY" json:"-"`
	FluxAPIKey       string `yaml:"flux_api_key" env:"FLUX_API_KEY" json:"-"`
	FluxBaseURL      string `yaml:"flux_base_url" env:"FLUX_BASE_URL"`
	StabilityAPIKey  string `yaml:"stability_api_key" env:"STABILITY_API_KEY" json:"-"`
	StabilityBaseURL string `yaml:"stability_base_url" env:"STABILITY_BASE_URL"`
	IdeogramAPIKey   string `yaml:"ideogram_api_key" env:"IDEOGRAM_API_KEY" json:"-"`
	IdeogramBaseURL  string `yaml:"ideogram_base_url" env:"IDEOGRAM_BASE_URL"`
	TongyiAPIKey     string `yaml:"tongyi_api_key" env:"TONGYI_API_KEY" json:"-"`
	TongyiBaseURL    string `yaml:"tongyi_base_url" env:"TONGYI_BASE_URL"`
	ZhipuAPIKey      string `yaml:"zhipu_api_key" env:"ZHIPU_API_KEY" json:"-"`
	ZhipuBaseURL     string `yaml:"zhipu_base_url" env:"ZHIPU_BASE_URL"`
	BaiduAPIKey      string `yaml:"baidu_api_key" env:"BAIDU_API_KEY" json:"-"`
	BaiduSecretKey   string `yaml:"baidu_secret_key" env:"BAIDU_SECRET_KEY" json:"-"`
	BaiduBaseURL     string `yaml:"baidu_base_url" env:"BAIDU_BASE_URL"`
	DoubaoAPIKey     string `yaml:"doubao_api_key" env:"DOUBAO_API_KEY" json:"-"`
	DoubaoBaseURL    string `yaml:"doubao_base_url" env:"DOUBAO_BASE_URL"`
	TencentSecretId  string `yaml:"tencent_secret_id" env:"TENCENT_SECRET_ID" json:"-"`
	TencentSecretKey string `yaml:"tencent_secret_key" env:"TENCENT_SECRET_KEY" json:"-"`
	TencentBaseURL   string `yaml:"tencent_base_url" env:"TENCENT_BASE_URL"`
}

type MultimodalVideoConfig struct {
	RunwayAPIKey    string `yaml:"runway_api_key" env:"RUNWAY_API_KEY" json:"-"`
	RunwayBaseURL   string `yaml:"runway_base_url" env:"RUNWAY_BASE_URL"`
	VeoAPIKey       string `yaml:"veo_api_key" env:"VEO_API_KEY" json:"-"`
	VeoBaseURL      string `yaml:"veo_base_url" env:"VEO_BASE_URL"`
	GoogleAPIKey    string `yaml:"google_api_key" env:"GOOGLE_API_KEY" json:"-"`
	GoogleBaseURL   string `yaml:"google_base_url" env:"GOOGLE_BASE_URL"`
	SoraAPIKey      string `yaml:"sora_api_key" env:"SORA_API_KEY" json:"-"`
	SoraBaseURL     string `yaml:"sora_base_url" env:"SORA_BASE_URL"`
	KlingAPIKey     string `yaml:"kling_api_key" env:"KLING_API_KEY" json:"-"`
	KlingBaseURL    string `yaml:"kling_base_url" env:"KLING_BASE_URL"`
	LumaAPIKey      string `yaml:"luma_api_key" env:"LUMA_API_KEY" json:"-"`
	LumaBaseURL     string `yaml:"luma_base_url" env:"LUMA_BASE_URL"`
	MiniMaxAPIKey   string `yaml:"minimax_api_key" env:"MINIMAX_API_KEY" json:"-"`
	MiniMaxBaseURL  string `yaml:"minimax_base_url" env:"MINIMAX_BASE_URL"`
	SeedanceAPIKey  string `yaml:"seedance_api_key" env:"SEEDANCE_API_KEY" json:"-"`
	SeedanceBaseURL string `yaml:"seedance_base_url" env:"SEEDANCE_BASE_URL"`
}

// LogConfig 日志配置
type LogConfig struct {
	// 日志级别: debug, info, warn, error
	Level string `yaml:"level" env:"LEVEL"`
	// 输出格式: json, console
	Format string `yaml:"format" env:"FORMAT"`
	// 输出路径
	OutputPaths []string `yaml:"output_paths" env:"OUTPUT_PATHS"`
	// 是否启用调用者信息
	EnableCaller bool `yaml:"enable_caller" env:"ENABLE_CALLER"`
	// 是否启用堆栈跟踪
	EnableStacktrace bool `yaml:"enable_stacktrace" env:"ENABLE_STACKTRACE"`
}

// TelemetryConfig 遥测配置
type TelemetryConfig struct {
	// 是否启用
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// OTLP 端点
	OTLPEndpoint string `yaml:"otlp_endpoint" env:"OTLP_ENDPOINT"`
	// 是否使用非加密连接（仅用于开发/测试环境）
	OTLPInsecure bool `yaml:"otlp_insecure" env:"OTLP_INSECURE"`
	// 服务名称
	ServiceName string `yaml:"service_name" env:"SERVICE_NAME"`
	// 采样率
	SampleRate float64 `yaml:"sample_rate" env:"SAMPLE_RATE"`
}

// ToolsConfig 工具提供者配置
type ToolsConfig struct {
	// Tavily 搜索配置（需要 API Key）
	Tavily TavilyToolConfig `yaml:"tavily" env:"TAVILY"`
	// Jina Reader 抓取配置（可选 API Key，免费可用）
	Jina JinaToolConfig `yaml:"jina" env:"JINA"`
	// Firecrawl 搜索+抓取配置（需要 API Key）
	Firecrawl FirecrawlToolConfig `yaml:"firecrawl" env:"FIRECRAWL"`
	// DuckDuckGo 搜索配置（完全免费，无需 API Key）
	DuckDuckGo DuckDuckGoToolConfig `yaml:"duckduckgo" env:"DUCKDUCKGO"`
	// SearXNG 搜索配置（自托管，无需 API Key）
	SearXNG SearXNGToolConfig `yaml:"searxng" env:"SEARXNG"`
	// HTTP 抓取配置（纯 HTTP，无需 API Key）
	HTTPScrape HTTPScrapeToolConfig `yaml:"http_scrape" env:"HTTP_SCRAPE"`
}

// TavilyToolConfig Tavily 搜索工具配置
type TavilyToolConfig struct {
	// API Key（建议使用环境变量 AGENTFLOW_TOOLS_TAVILY_API_KEY）
	APIKey string `yaml:"api_key" env:"API_KEY" json:"-"`
	// 基础 URL（可选，默认 https://api.tavily.com）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// JinaToolConfig Jina Reader 工具配置
type JinaToolConfig struct {
	// API Key（建议使用环境变量 AGENTFLOW_TOOLS_JINA_API_KEY）
	APIKey string `yaml:"api_key" env:"API_KEY" json:"-"`
	// 基础 URL（可选，默认 https://r.jina.ai）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// FirecrawlToolConfig Firecrawl 工具配置
type FirecrawlToolConfig struct {
	// API Key（建议使用环境变量 AGENTFLOW_TOOLS_FIRECRAWL_API_KEY）
	APIKey string `yaml:"api_key" env:"API_KEY" json:"-"`
	// 基础 URL（可选，默认 https://api.firecrawl.dev）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// DuckDuckGoToolConfig DuckDuckGo 搜索工具配置（完全免费）
type DuckDuckGoToolConfig struct {
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// SearXNGToolConfig SearXNG 搜索工具配置（自托管，无需 API Key）
type SearXNGToolConfig struct {
	// SearXNG 实例地址（必填，如 https://searx.example.com）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// HTTPScrapeToolConfig 纯 HTTP 抓取工具配置（零依赖，无需 API Key）
type HTTPScrapeToolConfig struct {
	// 自定义 User-Agent
	UserAgent string `yaml:"user_agent" env:"USER_AGENT"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// --- 配置加载器 ---

// Loader 配置加载器（Builder 模式）
type Loader struct {
	configPath string
	envPrefix  string
	validators []func(*Config) error
}

// NewLoader 创建新的配置加载器
func NewLoader() *Loader {
	return &Loader{
		envPrefix:  "AGENTFLOW",
		validators: make([]func(*Config) error, 0),
	}
}

// WithConfigPath 设置配置文件路径
func (l *Loader) WithConfigPath(path string) *Loader {
	l.configPath = path
	return l
}

// WithEnvPrefix 设置环境变量前缀
func (l *Loader) WithEnvPrefix(prefix string) *Loader {
	l.envPrefix = prefix
	return l
}

// WithValidator 添加配置验证器
func (l *Loader) WithValidator(v func(*Config) error) *Loader {
	l.validators = append(l.validators, v)
	return l
}

// Load 加载配置
// 优先级: 默认值 → YAML 文件 → 环境变量
func (l *Loader) Load() (*Config, error) {
	// 1. 从默认值开始
	cfg := DefaultConfig()

	// 2. 如果指定了配置文件，从文件加载
	if l.configPath != "" {
		if err := l.loadFromFile(cfg); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}

	// 3. 从环境变量覆盖
	if err := l.loadFromEnv(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from env: %w", err)
	}

	// 4. X-012: JWT 默认 exp 值
	if (cfg.Server.JWT.Secret != "" || cfg.Server.JWT.PublicKey != "") && cfg.Server.JWT.Expiration == 0 {
		cfg.Server.JWT.Expiration = time.Hour
	}

	// 5. 运行验证器
	for _, v := range l.validators {
		if err := v(cfg); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}
	}

	return cfg, nil
}

// loadFromFile 从 YAML 文件加载配置
func (l *Loader) loadFromFile(cfg *Config) error {
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，使用默认值
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// loadFromEnv 从环境变量加载配置
func (l *Loader) loadFromEnv(cfg *Config) error {
	return l.setFieldsFromEnv(reflect.ValueOf(cfg).Elem(), l.envPrefix)
}

// setFieldsFromEnv 递归设置结构体字段
func (l *Loader) setFieldsFromEnv(v reflect.Value, prefix string) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// 获取 env tag
		envTag := fieldType.Tag.Get("env")
		if envTag == "" || envTag == "-" {
			continue
		}

		envKey := prefix + "_" + envTag

		// 如果是结构体，递归处理
		if field.Kind() == reflect.Struct {
			if err := l.setFieldsFromEnv(field, envKey); err != nil {
				return err
			}
			continue
		}

		// 获取环境变量值
		envValue := os.Getenv(envKey)
		if envValue == "" {
			continue
		}

		// 设置字段值
		if err := setFieldValue(field, envValue); err != nil {
			return fmt.Errorf("failed to set %s (field %s) from value %q: %w",
				envKey, fieldType.Name, envValue, err)
		}
	}

	return nil
}

// setFieldValue 设置字段值
func setFieldValue(field reflect.Value, value string) error {
	if !field.CanSet() {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// 特殊处理 time.Duration
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
		} else {
			i, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(i)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(u)

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(f)

	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)

	case reflect.Slice:
		// 支持逗号分隔的字符串切片
		if field.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			field.Set(reflect.ValueOf(parts))
		}
	}

	return nil
}

// --- 辅助函数 ---

// LoadFromEnv 仅从环境变量加载配置
func LoadFromEnv() (*Config, error) {
	return NewLoader().Load()
}

// Validate 验证配置
func (c *Config) Validate() error {
	var errs []string

	// 验证服务器配置
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		errs = append(errs, "invalid HTTP port")
	}

	// V-008: Agent.Model required validation
	if c.Agent.Model == "" {
		errs = append(errs, "agent.model is required")
	}

	// 验证 Agent 配置
	if c.Agent.MaxIterations <= 0 {
		errs = append(errs, "max_iterations must be positive")
	}

	// V-009: MaxIterations upper bound
	if c.Agent.MaxIterations > validateMaxIterationsMax {
		errs = append(errs, "agent.max_iterations must not exceed 10000")
	}

	if c.Agent.Temperature < 0 || c.Agent.Temperature > validateTemperatureMax {
		errs = append(errs, "temperature must be between 0 and 2")
	}
	if c.Agent.Checkpoint.Enabled {
		backend := strings.TrimSpace(strings.ToLower(c.Agent.Checkpoint.Backend))
		switch backend {
		case StorageTypeFile:
			if strings.TrimSpace(c.Agent.Checkpoint.FilePath) == "" {
				errs = append(errs, "agent.checkpoint.file_path is required when backend=file")
			}
		case StorageTypeRedis:
			if strings.TrimSpace(c.Redis.Addr) == "" {
				errs = append(errs, "redis.addr is required when agent.checkpoint.backend=redis")
			}
			if strings.TrimSpace(c.Agent.Checkpoint.RedisPrefix) == "" {
				errs = append(errs, "agent.checkpoint.redis_prefix is required when backend=redis")
			}
		case StorageTypePostgres:
			if strings.TrimSpace(c.Database.Driver) != StorageTypePostgres {
				errs = append(errs, "database.driver must be postgres when agent.checkpoint.backend=postgres")
			}
		default:
			errs = append(errs, "agent.checkpoint.backend must be one of: file, redis, postgres")
		}
	}
	if c.Multimodal.ReferenceMaxSizeBytes <= 0 {
		errs = append(errs, "multimodal.reference_max_size_bytes must be positive")
	}
	if c.Multimodal.ReferenceTTL <= 0 {
		errs = append(errs, "multimodal.reference_ttl must be positive")
	}
	if strings.ToLower(strings.TrimSpace(c.Multimodal.ReferenceStoreBackend)) != StorageTypeRedis {
		errs = append(errs, "multimodal.reference_store_backend must be redis")
	}
	if c.Multimodal.Enabled && strings.TrimSpace(c.Redis.Addr) == "" {
		errs = append(errs, "redis.addr is required when multimodal.reference_store_backend=redis")
	}

	// V-010: MaxTokens range validation
	if c.Agent.MaxTokens < 0 || c.Agent.MaxTokens > validateMaxTokensMax {
		errs = append(errs, "agent.max_tokens must be between 0 and 128000")
	}

	// V-008: Database.Driver required if database is configured
	if c.Database.Host != "" && c.Database.Driver == "" {
		errs = append(errs, "database.driver is required when database is configured")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// DSN 返回数据库连接字符串
// WARNING: DSN() 返回含明文密码的连接字符串，不要在日志中使用。
// 日志记录请使用 SafeDSN()，它会对密码进行掩码处理。
func (d *DatabaseConfig) DSN() string {
	switch d.Driver {
	case StorageTypePostgres:
		return fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
		)
	case "mysql":
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true",
			d.User, d.Password, d.Host, d.Port, d.Name,
		)
	case "sqlite":
		return d.Name
	default:
		return ""
	}
}

// SafeDSN 返回用于日志记录的安全连接字符串（密码已掩码）
// 使用此方法而非 DSN() 来记录日志，防止敏感信息泄露
func (d *DatabaseConfig) SafeDSN() string {
	maskedPassword := MaskSensitive(d.Password)
	switch d.Driver {
	case StorageTypePostgres:
		return fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.User, maskedPassword, d.Name, d.SSLMode,
		)
	case "mysql":
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true",
			d.User, maskedPassword, d.Host, d.Port, d.Name,
		)
	case "sqlite":
		return d.Name
	default:
		return ""
	}
}

// MaskSensitive 掩码敏感信息，用于日志记录
// 例如: "mysecretpassword" -> "mys***ord"
func MaskSensitive(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 3 {
		return "***"
	}
	if len(s) <= 6 {
		return s[:1] + "***" + s[len(s)-1:]
	}
	return s[:3] + "***" + s[len(s)-3:]
}

// MaskAPIKey 掩码 API Key，用于日志记录
// 例如: "sk-1234567890abcdef" -> "sk-123...def"
func MaskAPIKey(key string) string {
	if len(key) == 0 {
		return ""
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:6] + "..." + key[len(key)-3:]
}

// CacheConfig LLM 缓存配置
type CacheConfig struct {
	// 是否启用缓存
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// 本地缓存最大条目数
	LocalMaxSize int `yaml:"local_max_size" env:"LOCAL_MAX_SIZE"`
	// 本地缓存 TTL
	LocalTTL time.Duration `yaml:"local_ttl" env:"LOCAL_TTL"`
	// 是否启用 Redis 缓存
	EnableRedis bool `yaml:"enable_redis" env:"ENABLE_REDIS"`
	// Redis 缓存 TTL
	RedisTTL time.Duration `yaml:"redis_ttl" env:"REDIS_TTL"`
	// 缓存键策略: hash | hierarchical
	KeyStrategy string `yaml:"key_strategy" env:"KEY_STRATEGY"`
}

// BudgetConfig Token 预算管理配置
type BudgetConfig struct {
	// 是否启用预算管理
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// 单次请求最大 Token 数
	MaxTokensPerRequest int `yaml:"max_tokens_per_request" env:"MAX_TOKENS_PER_REQUEST"`
	// 每分钟最大 Token 数
	MaxTokensPerMinute int `yaml:"max_tokens_per_minute" env:"MAX_TOKENS_PER_MINUTE"`
	// 每小时最大 Token 数
	MaxTokensPerHour int `yaml:"max_tokens_per_hour" env:"MAX_TOKENS_PER_HOUR"`
	// 每天最大 Token 数
	MaxTokensPerDay int `yaml:"max_tokens_per_day" env:"MAX_TOKENS_PER_DAY"`
	// 单次请求最大花费 (USD)
	MaxCostPerRequest float64 `yaml:"max_cost_per_request" env:"MAX_COST_PER_REQUEST"`
	// 每天最大花费 (USD)
	MaxCostPerDay float64 `yaml:"max_cost_per_day" env:"MAX_COST_PER_DAY"`
	// 告警阈值 (0.0-1.0)
	AlertThreshold float64 `yaml:"alert_threshold" env:"ALERT_THRESHOLD"`
	// 是否在达到分钟阈值时自动节流
	AutoThrottle bool `yaml:"auto_throttle" env:"AUTO_THROTTLE"`
	// 自动节流持续时间
	ThrottleDelay time.Duration `yaml:"throttle_delay" env:"THROTTLE_DELAY"`
}
