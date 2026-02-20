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
	assert.NotEqual(t, LLMConfig{}, cfg.LLM)
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
	assert.False(t, cfg.AllowQueryAPIKey)
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
	assert.Equal(t, "disable", cfg.SSLMode)
	assert.Equal(t, 25, cfg.MaxOpenConns)
	assert.Equal(t, 5, cfg.MaxIdleConns)
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
	assert.Equal(t, "openai", cfg.DefaultProvider)
	assert.Empty(t, cfg.APIKey)
	assert.Empty(t, cfg.BaseURL)
	assert.Equal(t, 2*time.Minute, cfg.Timeout)
	assert.Equal(t, 3, cfg.MaxRetries)
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
