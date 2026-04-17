package main

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/llm"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestNewMultimodalRedisReferenceStore_RejectsNonLoopbackPlainHostPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Redis.Addr = "10.0.0.8:6379"

	client, store, err := bootstrap.BuildMultimodalRedisReferenceStore(cfg, "agentflow:test:mm", time.Hour, zap.NewNop())
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Nil(t, store)
	assert.ErrorContains(t, err, "requires rediss://")
}

func TestNewMultimodalRedisReferenceStore_RejectsNonLoopbackRedisScheme(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Redis.Addr = "redis://example.com:6379"

	client, store, err := bootstrap.BuildMultimodalRedisReferenceStore(cfg, "agentflow:test:mm", time.Hour, zap.NewNop())
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Nil(t, store)
	assert.ErrorContains(t, err, "insecure redis://")
}

func TestNewMultimodalRedisReferenceStore_AllowsLoopbackPlaintext(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	cfg := config.DefaultConfig()
	cfg.Redis.Addr = mr.Addr()

	client, store, err := bootstrap.BuildMultimodalRedisReferenceStore(cfg, "agentflow:test:mm", time.Hour, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, store)
	require.NotNil(t, client)
	assert.NoError(t, client.Close())
}

func TestIsLoopbackHost(t *testing.T) {
	assert.True(t, bootstrap.IsLoopbackHost("localhost"))
	assert.True(t, bootstrap.IsLoopbackHost("127.0.0.1"))
	assert.True(t, bootstrap.IsLoopbackHost("::1"))
	assert.False(t, bootstrap.IsLoopbackHost("example.com"))
}

func TestInitHandlers_MultimodalRejectsNonRedisBackend(t *testing.T) {
	const mode = "test-multimodal-init"
	bootstrap.UnregisterMainProviderBuilder(mode)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(mode,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llm.Provider, error) {
			return &hotReloadProvider{name: "provider-mm", content: "mm"}, nil
		}))
	defer bootstrap.UnregisterMainProviderBuilder(mode)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = mode
	cfg.Multimodal.Enabled = true
	cfg.Multimodal.ReferenceStoreBackend = "memory"

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	err := s.initHandlers()
	require.Error(t, err)
	assert.ErrorContains(t, err, "multimodal.reference_store_backend must be redis")
}

func TestInitHandlers_RequiresLLMRuntime(t *testing.T) {
	cfg := config.DefaultConfig()

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	err := s.initHandlers()
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to initialize llm runtime")
}
