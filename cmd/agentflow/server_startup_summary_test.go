package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/llm"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

func TestStartupSummary_ReportsCapabilitiesDependenciesAndRestartBoundaries(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "redis"
	cfg.Multimodal.Enabled = false

	s := &Server{
		cfg:              cfg,
		configPath:       "config.yaml",
		logger:           logger,
		db:               &gorm.DB{},
		healthHandler:    nil,
		provider:         &hotReloadProvider{name: "summary-provider", content: "ok"},
		agentHandler:     nil,
		chatHandler:      nil,
		protocolHandler:  nil,
		ragHandler:       nil,
		workflowHandler:  nil,
		hotReloadManager: &config.HotReloadManager{},
	}
	s.logStartupSummary()

	entries := logs.All()
	require.Len(t, entries, 1)
	assert.Equal(t, "Startup summary", entries[0].Message)

	summary := s.startupSummary()
	assert.Contains(t, summary.EnabledCapabilities, "hot_reload")
	assert.Contains(t, summary.DisabledCapabilities, "chat")
	assert.Contains(t, summary.DisabledCapabilities, "health")
	assert.Contains(t, summary.DisabledCapabilities, "protocol")
	assert.Contains(t, summary.DisabledCapabilities, "workflow")
	assert.Contains(t, summary.DisabledCapabilities, "multimodal")
	assert.Equal(t, "required+ready", summary.DependencyStatus["database"])
	assert.Equal(t, "required+missing", summary.DependencyStatus["mongodb"])
	assert.Equal(t, "required+ready", summary.DependencyStatus["llm_runtime"])
	assert.Equal(t, "required+missing", summary.DependencyStatus["tool_approval_store"])
	assert.Contains(t, summary.RestartRequiredRoutes, "chat")
	assert.Contains(t, summary.RestartRequiredRoutes, "cost")
	assert.Contains(t, summary.RestartRequiredRoutes, "rag")
	assert.Contains(t, summary.RestartRequiredRoutes, "multimodal")
}

func TestInitHandlers_FailsWhenConfiguredMainProviderBuilderErrors(t *testing.T) {
	const mode = "test-llm-unavailable"
	bootstrap.UnregisterMainProviderBuilder(mode)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(mode,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llm.Provider, error) {
			return nil, assert.AnError
		}))
	defer bootstrap.UnregisterMainProviderBuilder(mode)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = mode
	cfg.Multimodal.Enabled = false

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	err := s.initHandlers()
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to initialize llm runtime")
	assert.ErrorContains(t, err, assert.AnError.Error())
}

func TestInitHandlers_RedisDependencySurfacesInReadinessProbe(t *testing.T) {
	const mode = "test-redis-ready"
	bootstrap.UnregisterMainProviderBuilder(mode)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(mode,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llm.Provider, error) {
			return &hotReloadProvider{name: "provider-redis", content: "redis"}, nil
		}))
	defer bootstrap.UnregisterMainProviderBuilder(mode)

	mr, err := miniredis.Run()
	require.NoError(t, err)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = mode
	cfg.Multimodal.Enabled = true
	cfg.Redis.Addr = mr.Addr()
	cfg.Budget.Enabled = false
	cfg.Agent.Checkpoint.Enabled = false

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	require.NoError(t, s.initHandlers())
	require.NotNil(t, s.healthHandler)

	mr.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	s.healthHandler.HandleReady(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"redis"`)
	assert.Contains(t, rec.Body.String(), `"fail"`)
}

func TestInitHandlers_MongoDependencySurfacesInReadinessProbe(t *testing.T) {
	cfg := config.DefaultConfig()

	client, err := mongoclient.NewClientFromOptions(
		options.Client().ApplyURI("mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=50&connectTimeoutMS=50"),
		cfg.MongoDB.Database,
		zap.NewNop(),
	)
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = client.Close(ctx)
	}()

	s := &Server{
		cfg:         cfg,
		logger:      zap.NewNop(),
		mongoClient: client,
		healthHandler: handlers.NewHealthHandler(zap.NewNop()),
	}
	s.healthHandler.RegisterCheck(handlers.NewDatabaseHealthCheck("mongodb", func(ctx context.Context) error {
		return s.mongoClient.Ping(ctx)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	s.healthHandler.HandleReady(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), `"mongodb"`)
	assert.Contains(t, rec.Body.String(), `"fail"`)
}

func TestInitMongoDB_FailsWhenMongoUnavailable(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MongoDB.Host = "127.0.0.1"
	cfg.MongoDB.Port = 1
	cfg.MongoDB.ConnectTimeout = 50 * time.Millisecond
	cfg.MongoDB.Timeout = 50 * time.Millisecond
	cfg.MongoDB.HealthCheckInterval = time.Hour

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	err := s.initMongoDB()
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to connect to MongoDB")
}
