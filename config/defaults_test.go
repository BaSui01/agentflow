package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- DefaultConfig aggregate ---

func TestDefaultConfig_ContainsAllSubConfigs(t *testing.T) {
	cfg := DefaultConfig()
	require.NotNil(t, cfg)

	// Each sub-config should be non-zero
	assert.NotEqual(t, ServerConfig{}, cfg.Server)
	assert.NotEqual(t, AgentConfig{}, cfg.Agent)
	assert.NotEqual(t, RedisConfig{}, cfg.Redis)
	assert.NotEqual(t, DatabaseConfig{}, cfg.Database)
	assert.NotEqual(t, QdrantConfig{}, cfg.Qdrant)
	assert.NotEqual(t, WeaviateConfig{}, cfg.Weaviate)
	assert.NotEqual(t, MilvusConfig{}, cfg.Milvus)
	assert.NotEqual(t, LLMConnectionConfig{}, cfg.LLM)
	assert.NotEqual(t, MultimodalConfig{}, cfg.Multimodal)
	assert.NotEqual(t, LogConfig{}, cfg.Log)
	assert.NotEqual(t, TelemetryConfig{}, cfg.Telemetry)
}

// --- Individual Default*Config functions ---

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 9090, cfg.GRPCPort)
	assert.Equal(t, 9091, cfg.MetricsPort)
	assert.Equal(t, 30*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 15*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 100, cfg.RateLimitRPS)
	assert.Equal(t, 200, cfg.RateLimitBurst)
}

func TestDefaultAgentConfig(t *testing.T) {
	cfg := DefaultAgentConfig()
	assert.Equal(t, "default-agent", cfg.Name)
	assert.Equal(t, "gpt-4", cfg.Model)
	assert.Equal(t, 10, cfg.MaxIterations)
	assert.InDelta(t, 0.7, cfg.Temperature, 0.001)
	assert.Equal(t, 4096, cfg.MaxTokens)
	assert.Equal(t, 5*time.Minute, cfg.Timeout)
	assert.True(t, cfg.StreamEnabled)
	assert.NotEmpty(t, cfg.SystemPrompt)
	assert.NotEmpty(t, cfg.Description)

	// Memory sub-config
	assert.True(t, cfg.Memory.Enabled)
	assert.Equal(t, "buffer", cfg.Memory.Type)
	assert.Equal(t, 100, cfg.Memory.MaxMessages)
	assert.Equal(t, 8000, cfg.Memory.TokenLimit)
	assert.True(t, cfg.Checkpoint.Enabled)
	assert.Equal(t, "file", cfg.Checkpoint.Backend)
	assert.Equal(t, "./checkpoints", cfg.Checkpoint.FilePath)
	assert.Equal(t, "agentflow:checkpoint", cfg.Checkpoint.RedisPrefix)
	assert.Equal(t, 24*time.Hour, cfg.Checkpoint.RedisTTL)
}

func TestDefaultRedisConfig(t *testing.T) {
	cfg := DefaultRedisConfig()
	assert.Equal(t, "localhost:6379", cfg.Addr)
	assert.Empty(t, cfg.Password)
	assert.Equal(t, 0, cfg.DB)
	assert.Equal(t, 10, cfg.PoolSize)
	assert.Equal(t, 2, cfg.MinIdleConns)
}

func TestDefaultDatabaseConfig(t *testing.T) {
	cfg := DefaultDatabaseConfig()
	assert.Equal(t, "postgres", cfg.Driver)
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 5432, cfg.Port)
	assert.Equal(t, "agentflow", cfg.User)
	assert.Empty(t, cfg.Password)
	assert.Equal(t, "agentflow", cfg.Name)
	assert.Equal(t, "require", cfg.SSLMode)
	assert.Equal(t, 25, cfg.MaxOpenConns)
	assert.Equal(t, 10, cfg.MaxIdleConns)
	assert.Equal(t, 5*time.Minute, cfg.ConnMaxLifetime)
}

func TestDefaultQdrantConfig(t *testing.T) {
	cfg := DefaultQdrantConfig()
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 6334, cfg.Port)
	assert.Empty(t, cfg.APIKey)
	assert.Equal(t, "agentflow_vectors", cfg.Collection)
}

func TestDefaultWeaviateConfig(t *testing.T) {
	cfg := DefaultWeaviateConfig()
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "http", cfg.Scheme)
	assert.Equal(t, "AgentFlowDocuments", cfg.ClassName)
	assert.True(t, cfg.AutoCreateSchema)
	assert.Equal(t, "cosine", cfg.Distance)
	assert.InDelta(t, 0.5, cfg.HybridAlpha, 0.001)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestDefaultMilvusConfig(t *testing.T) {
	cfg := DefaultMilvusConfig()
	assert.Equal(t, "localhost", cfg.Host)
	assert.Equal(t, 19530, cfg.Port)
	assert.Equal(t, "default", cfg.Database)
	assert.Equal(t, "agentflow_vectors", cfg.Collection)
	assert.Equal(t, 1536, cfg.VectorDimension)
	assert.Equal(t, "IVF_FLAT", cfg.IndexType)
	assert.Equal(t, "COSINE", cfg.MetricType)
	assert.True(t, cfg.AutoCreateCollection)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 1000, cfg.BatchSize)
	assert.Equal(t, "Strong", cfg.ConsistencyLevel)
}

func TestDefaultLLMConfig(t *testing.T) {
	cfg := DefaultLLMConfig()
	assert.Equal(t, LLMMainProviderModeLegacy, cfg.MainProviderMode)
	assert.Equal(t, "openai", cfg.DefaultProvider)
	assert.Empty(t, cfg.APIKey)
	assert.Empty(t, cfg.BaseURL)
	assert.Equal(t, 2*time.Minute, cfg.Timeout)
	assert.Equal(t, 3, cfg.MaxRetries)
}

func TestDefaultMultimodalConfig(t *testing.T) {
	cfg := DefaultMultimodalConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, int64(8<<20), cfg.ReferenceMaxSizeBytes)
	assert.Equal(t, 2*time.Hour, cfg.ReferenceTTL)
	assert.Equal(t, "redis", cfg.ReferenceStoreBackend)
	assert.Equal(t, "agentflow:mm:ref", cfg.ReferenceStoreKeyPrefix)
	assert.Empty(t, cfg.DefaultImageProvider)
	assert.Empty(t, cfg.DefaultVideoProvider)
	assert.Empty(t, cfg.Image.OpenAIAPIKey)
	assert.Empty(t, cfg.Video.RunwayAPIKey)
}

func TestDefaultHostedToolsConfig(t *testing.T) {
	cfg := DefaultHostedToolsConfig()
	assert.False(t, cfg.FileOps.Enabled)
	assert.Equal(t, int64(10<<20), cfg.FileOps.MaxFileSize)
	assert.False(t, cfg.Shell.Enabled)
	assert.Equal(t, 30*time.Second, cfg.Shell.Timeout)
	assert.Equal(t, "file", cfg.Approval.Backend)
	assert.Equal(t, 15*time.Minute, cfg.Approval.GrantTTL)
	assert.Equal(t, "request", cfg.Approval.Scope)
	assert.Equal(t, "./checkpoints/tool_approval_grants.json", cfg.Approval.PersistPath)
	assert.Equal(t, "agentflow:tool_approval", cfg.Approval.RedisPrefix)
	assert.Equal(t, 200, cfg.Approval.HistoryMaxEntries)
}

func TestDefaultLogConfig(t *testing.T) {
	cfg := DefaultLogConfig()
	assert.Equal(t, "info", cfg.Level)
	assert.Equal(t, "json", cfg.Format)
	assert.Equal(t, []string{"stdout"}, cfg.OutputPaths)
	assert.True(t, cfg.EnableCaller)
	assert.False(t, cfg.EnableStacktrace)
}

func TestDefaultTelemetryConfig(t *testing.T) {
	cfg := DefaultTelemetryConfig()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "localhost:4317", cfg.OTLPEndpoint)
	assert.Equal(t, "agentflow", cfg.ServiceName)
	assert.InDelta(t, 0.1, cfg.SampleRate, 0.001)
}
