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

const (
	ServerEnvironmentDevelopment = "development"
	ServerEnvironmentTest        = "test"
	ServerEnvironmentProduction  = "production"
)

// Checkpoint 存储类型常量，替代 magic string
const (
	StorageTypeFile     = "file"
	StorageTypeRedis    = "redis"
	StorageTypePostgres = "postgres"
	StorageTypeMemory   = "memory"
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

	// Pinecone 向量存储配置
	Pinecone PineconeConfig `yaml:"pinecone" env:"PINECONE"`

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

	// RAG RAG 检索配置
	RAG RAGConfig `yaml:"rag" env:"RAG"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	// HTTP 端口
	HTTPPort int `yaml:"http_port" env:"HTTP_PORT" reload:"HTTP server port" restart:"true" sensitive:"false"`
	// Metrics 端口
	MetricsPort int `yaml:"metrics_port" env:"METRICS_PORT" reload:"Metrics server port" restart:"true" sensitive:"false"`
	// Metrics 监听地址；默认仅监听 loopback，生产若需外部抓取必须显式放开。
	MetricsBindAddress string `yaml:"metrics_bind_address" env:"METRICS_BIND_ADDRESS" reload:"Metrics server bind address" restart:"true" sensitive:"false"`
	// 运行环境；用于固化生产环境安全默认值。
	Environment string `yaml:"environment" env:"ENVIRONMENT" json:"environment,omitempty"`
	// 是否启用 pprof 诊断端点；默认关闭，避免在 metrics 端口暴露 profiling 能力。
	EnablePProf bool `yaml:"enable_pprof" env:"ENABLE_PPROF" json:"enable_pprof,omitempty" reload:"Enable pprof endpoints on the metrics server" restart:"true" sensitive:"false"`
	// 读取超时
	ReadTimeout time.Duration `yaml:"read_timeout" env:"READ_TIMEOUT" reload:"HTTP read timeout" restart:"true" sensitive:"false"`
	// 写入超时
	WriteTimeout time.Duration `yaml:"write_timeout" env:"WRITE_TIMEOUT" reload:"HTTP write timeout" restart:"true" sensitive:"false"`
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
	// AllowNoAuth 允许在无认证配置时跳过 HTTP 鉴权（默认 false）。
	// 仅 development/test 环境允许开启；production 会在配置校验阶段直接拒绝启动。
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
	MaxIterations int `yaml:"max_iterations" env:"MAX_ITERATIONS" reload:"Maximum agent iterations" restart:"false" sensitive:"false"`
	// 温度参数
	Temperature float64 `yaml:"temperature" env:"TEMPERATURE" reload:"LLM temperature parameter" restart:"false" sensitive:"false"`
	// 最大 Token 数
	MaxTokens int `yaml:"max_tokens" env:"MAX_TOKENS" reload:"Maximum tokens for LLM" restart:"false" sensitive:"false"`
	// 超时时间
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT" reload:"Agent execution timeout" restart:"false" sensitive:"false"`
	// 是否启用流式输出
	StreamEnabled bool `yaml:"stream_enabled" env:"STREAM_ENABLED" reload:"Enable streaming responses" restart:"false" sensitive:"false"`
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
	Addr string `yaml:"addr" env:"ADDR" reload:"Redis address" restart:"true" sensitive:"false"`
	// 密码
	Password string `yaml:"password" env:"PASSWORD" reload:"Redis password" restart:"true" sensitive:"true"`
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
	Host string `yaml:"host" env:"HOST" reload:"Database host" restart:"true" sensitive:"false"`
	// 端口
	Port int `yaml:"port" env:"PORT" reload:"Database port" restart:"true" sensitive:"false"`
	// 用户名
	User string `yaml:"user" env:"USER"`
	// 密码
	Password string `yaml:"password" env:"PASSWORD" reload:"Database password" restart:"true" sensitive:"true"`
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
	Host string `yaml:"host" env:"HOST" reload:"Qdrant host" restart:"true" sensitive:"false"`
	// gRPC 端口
	Port int `yaml:"port" env:"PORT"`
	// API Key（可选）
	APIKey string `yaml:"api_key" env:"API_KEY" reload:"Qdrant API key" restart:"true" sensitive:"true"`
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

// PineconeConfig Pinecone 向量存储配置
type PineconeConfig struct {
	// API Key（必需）
	APIKey string `yaml:"api_key" env:"API_KEY"`
	// Index 名称（用于通过 Controller API 解析 BaseURL）
	Index string `yaml:"index" env:"INDEX"`
	// BaseURL（数据平面地址，若设置则优先使用，格式 https://...svc...pinecone.io）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// Namespace（可选，用于隔离向量空间）
	Namespace string `yaml:"namespace" env:"NAMESPACE"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
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
	APIKey string `yaml:"api_key" env:"API_KEY" reload:"LLM API key" restart:"true" sensitive:"true"`
	// 工具调用阶段 API Key（可选，未设置时回退 api_key）
	ToolAPIKey string `yaml:"tool_api_key" env:"TOOL_API_KEY"`
	// 基础 URL（可选）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 工具调用阶段基础 URL（可选，未设置时回退 base_url）
	ToolBaseURL string `yaml:"tool_base_url" env:"TOOL_BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT" reload:"LLM request timeout" restart:"false" sensitive:"false"`
	// 工具调用阶段请求超时（可选，未设置时回退 timeout）
	ToolTimeout time.Duration `yaml:"tool_timeout" env:"TOOL_TIMEOUT"`
	// 最大重试次数
	MaxRetries int `yaml:"max_retries" env:"MAX_RETRIES" reload:"Maximum LLM request retries" restart:"false" sensitive:"false"`
	// 工具调用阶段最大重试次数（可选，未设置时回退 max_retries）
	ToolMaxRetries int `yaml:"tool_max_retries" env:"TOOL_MAX_RETRIES"`
	// 模型目录 JSON 快照路径（可选，未设置时使用内置默认快照）。
	ModelCatalogPath string `yaml:"model_catalog_path" env:"MODEL_CATALOG_PATH"`
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
	ReferenceMaxSizeBytes int64 `yaml:"reference_max_size_bytes" env:"REFERENCE_MAX_SIZE_BYTES" reload:"Multimodal reference max upload size in bytes" restart:"false" sensitive:"false"`
	// 引用图默认存活时长
	ReferenceTTL time.Duration `yaml:"reference_ttl" env:"REFERENCE_TTL" reload:"Multimodal reference TTL" restart:"false" sensitive:"false"`
	// 引用图存储后端（仅支持 redis）
	ReferenceStoreBackend string `yaml:"reference_store_backend" env:"REFERENCE_STORE_BACKEND" reload:"Multimodal reference store backend (redis only)" restart:"true" sensitive:"false"`
	// 引用图存储 key 前缀（Redis 后端使用）
	ReferenceStoreKeyPrefix string `yaml:"reference_store_key_prefix" env:"REFERENCE_STORE_KEY_PREFIX" reload:"Multimodal reference store key prefix" restart:"true" sensitive:"false"`
	// 默认图像提供商标识（openai/gemini 等）
	DefaultImageProvider string `yaml:"default_image_provider" env:"DEFAULT_IMAGE_PROVIDER" reload:"Default multimodal image provider" restart:"false" sensitive:"false"`
	// 默认视频提供商标识（runway/veo 等）
	DefaultVideoProvider string `yaml:"default_video_provider" env:"DEFAULT_VIDEO_PROVIDER" reload:"Default multimodal video provider" restart:"false" sensitive:"false"`
	// 默认对话模型（多模态 chat 未传 model 时使用；空则回退 agent.model）
	DefaultChatModel string `yaml:"default_chat_model" env:"DEFAULT_CHAT_MODEL"`
	// 图像提供商配置
	Image MultimodalImageConfig `yaml:"image" env:"IMAGE"`
	// 视频提供商配置
	Video MultimodalVideoConfig `yaml:"video" env:"VIDEO"`
}

type MultimodalImageConfig struct {
	OpenAIAPIKey     string `yaml:"openai_api_key" env:"OPENAI_API_KEY" json:"-" reload:"Multimodal OpenAI image API key" restart:"true" sensitive:"true"`
	OpenAIBaseURL    string `yaml:"openai_base_url" env:"OPENAI_BASE_URL"`
	GeminiAPIKey     string `yaml:"gemini_api_key" env:"GEMINI_API_KEY" json:"-" reload:"Multimodal Gemini image API key" restart:"true" sensitive:"true"`
	FluxAPIKey       string `yaml:"flux_api_key" env:"FLUX_API_KEY" json:"-" reload:"Multimodal Flux (BFL) image API key" restart:"true" sensitive:"true"`
	FluxBaseURL      string `yaml:"flux_base_url" env:"FLUX_BASE_URL" reload:"Multimodal Flux image base URL" restart:"true" sensitive:"false"`
	StabilityAPIKey  string `yaml:"stability_api_key" env:"STABILITY_API_KEY" json:"-" reload:"Multimodal Stability AI image API key" restart:"true" sensitive:"true"`
	StabilityBaseURL string `yaml:"stability_base_url" env:"STABILITY_BASE_URL" reload:"Multimodal Stability AI image base URL" restart:"true" sensitive:"false"`
	IdeogramAPIKey   string `yaml:"ideogram_api_key" env:"IDEOGRAM_API_KEY" json:"-" reload:"Multimodal Ideogram image API key" restart:"true" sensitive:"true"`
	IdeogramBaseURL  string `yaml:"ideogram_base_url" env:"IDEOGRAM_BASE_URL" reload:"Multimodal Ideogram image base URL" restart:"true" sensitive:"false"`
	TongyiAPIKey     string `yaml:"tongyi_api_key" env:"TONGYI_API_KEY" json:"-" reload:"Multimodal Tongyi Wanxiang (阿里通义万相) image API key" restart:"true" sensitive:"true"`
	TongyiBaseURL    string `yaml:"tongyi_base_url" env:"TONGYI_BASE_URL" reload:"Multimodal Tongyi image base URL" restart:"true" sensitive:"false"`
	ZhipuAPIKey      string `yaml:"zhipu_api_key" env:"ZHIPU_API_KEY" json:"-" reload:"Multimodal Zhipu (智谱) image API key" restart:"true" sensitive:"true"`
	ZhipuBaseURL     string `yaml:"zhipu_base_url" env:"ZHIPU_BASE_URL" reload:"" restart:"true" sensitive:"false"`
	BaiduAPIKey      string `yaml:"baidu_api_key" env:"BAIDU_API_KEY" json:"-" reload:"Multimodal Baidu (文心) image API key (client_id)" restart:"true" sensitive:"true"`
	BaiduSecretKey   string `yaml:"baidu_secret_key" env:"BAIDU_SECRET_KEY" json:"-" reload:"Multimodal Baidu image secret (client_secret)" restart:"true" sensitive:"true"`
	BaiduBaseURL     string `yaml:"baidu_base_url" env:"BAIDU_BASE_URL" reload:"" restart:"true" sensitive:"false"`
	DoubaoAPIKey     string `yaml:"doubao_api_key" env:"DOUBAO_API_KEY" json:"-" reload:"Multimodal Doubao (豆包/火山) image API key" restart:"true" sensitive:"true"`
	DoubaoBaseURL    string `yaml:"doubao_base_url" env:"DOUBAO_BASE_URL" reload:"" restart:"true" sensitive:"false"`
	TencentSecretId  string `yaml:"tencent_secret_id" env:"TENCENT_SECRET_ID" json:"-" reload:"Multimodal Tencent Hunyuan (腾讯混元生图) SecretId" restart:"true" sensitive:"true"`
	TencentSecretKey string `yaml:"tencent_secret_key" env:"TENCENT_SECRET_KEY" json:"-" reload:"Multimodal Tencent Hunyuan SecretKey" restart:"true" sensitive:"true"`
	TencentBaseURL   string `yaml:"tencent_base_url" env:"TENCENT_BASE_URL" reload:"" restart:"true" sensitive:"false"`
}

type MultimodalVideoConfig struct {
	RunwayAPIKey    string `yaml:"runway_api_key" env:"RUNWAY_API_KEY" json:"-" reload:"Multimodal Runway video API key" restart:"true" sensitive:"true"`
	RunwayBaseURL   string `yaml:"runway_base_url" env:"RUNWAY_BASE_URL" reload:"Multimodal Runway video base URL" restart:"true" sensitive:"false"`
	VeoAPIKey       string `yaml:"veo_api_key" env:"VEO_API_KEY" json:"-" reload:"Multimodal Veo video API key" restart:"true" sensitive:"true"`
	VeoBaseURL      string `yaml:"veo_base_url" env:"VEO_BASE_URL" reload:"Multimodal Veo video base URL" restart:"true" sensitive:"false"`
	GoogleAPIKey    string `yaml:"google_api_key" env:"GOOGLE_API_KEY" json:"-" reload:"Multimodal Google video API key" restart:"true" sensitive:"true"`
	GoogleBaseURL   string `yaml:"google_base_url" env:"GOOGLE_BASE_URL" reload:"Multimodal Google multimodal base URL" restart:"true" sensitive:"false"`
	SoraAPIKey      string `yaml:"sora_api_key" env:"SORA_API_KEY" json:"-" reload:"Multimodal Sora video API key" restart:"true" sensitive:"true"`
	SoraBaseURL     string `yaml:"sora_base_url" env:"SORA_BASE_URL" reload:"Multimodal Sora video base URL" restart:"true" sensitive:"false"`
	KlingAPIKey     string `yaml:"kling_api_key" env:"KLING_API_KEY" json:"-" reload:"Multimodal Kling video API key" restart:"true" sensitive:"true"`
	KlingBaseURL    string `yaml:"kling_base_url" env:"KLING_BASE_URL" reload:"Multimodal Kling video base URL" restart:"true" sensitive:"false"`
	LumaAPIKey      string `yaml:"luma_api_key" env:"LUMA_API_KEY" json:"-" reload:"Multimodal Luma video API key" restart:"true" sensitive:"true"`
	LumaBaseURL     string `yaml:"luma_base_url" env:"LUMA_BASE_URL" reload:"Multimodal Luma video base URL" restart:"true" sensitive:"false"`
	MiniMaxAPIKey   string `yaml:"minimax_api_key" env:"MINIMAX_API_KEY" json:"-" reload:"Multimodal MiniMax video API key" restart:"true" sensitive:"true"`
	MiniMaxBaseURL  string `yaml:"minimax_base_url" env:"MINIMAX_BASE_URL" reload:"Multimodal MiniMax video base URL" restart:"true" sensitive:"false"`
	SeedanceAPIKey  string `yaml:"seedance_api_key" env:"SEEDANCE_API_KEY" json:"-" reload:"Multimodal Seedance (即梦) video API key" restart:"true" sensitive:"true"`
	SeedanceBaseURL string `yaml:"seedance_base_url" env:"SEEDANCE_BASE_URL" reload:"Multimodal Seedance (即梦) video base URL" restart:"true" sensitive:"false"`
}

// LogConfig 日志配置
type LogConfig struct {
	// 日志级别: debug, info, warn, error
	Level string `yaml:"level" env:"LEVEL" reload:"Log level (debug, info, warn, error)" restart:"false" sensitive:"false"`
	// 输出格式: json, console
	Format string `yaml:"format" env:"FORMAT" reload:"Log format (json, console)" restart:"false" sensitive:"false"`
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
	Enabled bool `yaml:"enabled" env:"ENABLED" reload:"Enable telemetry" restart:"false" sensitive:"false"`
	// OTLP 端点
	OTLPEndpoint string `yaml:"otlp_endpoint" env:"OTLP_ENDPOINT"`
	// 是否使用非加密连接（仅用于开发/测试环境）
	OTLPInsecure bool `yaml:"otlp_insecure" env:"OTLP_INSECURE"`
	// 服务名称
	ServiceName string `yaml:"service_name" env:"SERVICE_NAME"`
	// 采样率
	SampleRate float64 `yaml:"sample_rate" env:"SAMPLE_RATE" reload:"Telemetry sample rate" restart:"false" sensitive:"false"`
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
	// Brave 搜索配置（需要 API Key）
	Brave BraveToolConfig `yaml:"brave" env:"BRAVE"`
	// Bing 搜索配置（需要 API Key）
	Bing BingToolConfig `yaml:"bing" env:"BING"`
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

// BraveToolConfig Brave 搜索工具配置
type BraveToolConfig struct {
	// API Key（建议使用环境变量 AGENTFLOW_TOOLS_BRAVE_API_KEY）
	APIKey string `yaml:"api_key" env:"API_KEY" json:"-"`
	// 基础 URL（可选，默认 https://api.search.brave.com）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
}

// BingToolConfig Bing 搜索工具配置
type BingToolConfig struct {
	// API Key（建议使用环境变量 AGENTFLOW_TOOLS_BING_API_KEY）
	APIKey string `yaml:"api_key" env:"API_KEY" json:"-"`
	// 基础 URL（可选，默认 https://api.bing.microsoft.com）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
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

	c.Server.Environment = normalizeServerEnvironment(c.Server.Environment)

	// 验证服务器配置
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		errs = append(errs, "invalid HTTP port")
	}
	switch c.Server.Environment {
	case ServerEnvironmentDevelopment, ServerEnvironmentTest, ServerEnvironmentProduction:
	default:
		errs = append(errs, "server.environment must be one of: development, test, production")
	}
	if c.Server.Environment == ServerEnvironmentProduction && c.Server.AllowNoAuth {
		errs = append(errs, "server.allow_no_auth cannot be true when server.environment=production")
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
	if strings.ToLower(strings.TrimSpace(c.Multimodal.ReferenceStoreBackend)) != StorageTypeRedis &&
		strings.ToLower(strings.TrimSpace(c.Multimodal.ReferenceStoreBackend)) != StorageTypeMemory {
		errs = append(errs, "multimodal.reference_store_backend must be redis or memory")
	}
	if c.Multimodal.Enabled && strings.TrimSpace(c.Redis.Addr) == "" {
		errs = append(errs, "redis.addr is required when multimodal.reference_store_backend=redis")
	}
	if c.HostedTools.Approval.GrantTTL <= 0 {
		errs = append(errs, "hosted_tools.approval.grant_ttl must be positive")
	}
	if c.HostedTools.Approval.HistoryMaxEntries <= 0 {
		errs = append(errs, "hosted_tools.approval.history_max_entries must be positive")
	}
	switch strings.TrimSpace(strings.ToLower(c.HostedTools.Approval.Backend)) {
	case "memory":
	case "file":
		if strings.TrimSpace(c.HostedTools.Approval.PersistPath) == "" {
			errs = append(errs, "hosted_tools.approval.persist_path is required when backend=file")
		}
	case "redis":
		if strings.TrimSpace(c.Redis.Addr) == "" {
			errs = append(errs, "redis.addr is required when hosted_tools.approval.backend=redis")
		}
		if strings.TrimSpace(c.HostedTools.Approval.RedisPrefix) == "" {
			errs = append(errs, "hosted_tools.approval.redis_prefix is required when backend=redis")
		}
	default:
		errs = append(errs, "hosted_tools.approval.backend must be one of: memory, file, redis")
	}
	switch strings.TrimSpace(strings.ToLower(c.HostedTools.Approval.Scope)) {
	case "request", "agent_tool", "tool":
	default:
		errs = append(errs, "hosted_tools.approval.scope must be one of: request, agent_tool, tool")
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

func normalizeServerEnvironment(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ServerEnvironmentDevelopment
	}
	return normalized
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

// RAGConfig RAG 检索配置
type RAGConfig struct {
	// WebSearch 网络检索增强配置
	WebSearch RAGWebSearchConfig `yaml:"web_search" env:"WEB_SEARCH"`
}

// RAGWebSearchConfig RAG 网络检索增强配置
type RAGWebSearchConfig struct {
	// 是否启用网络检索增强
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// 网络搜索超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
	// 缓存最大条目数
	MaxCacheEntries int `yaml:"max_cache_entries" env:"MAX_CACHE_ENTRIES"`
	// 缓存 TTL
	CacheTTL time.Duration `yaml:"cache_ttl" env:"CACHE_TTL"`
}
