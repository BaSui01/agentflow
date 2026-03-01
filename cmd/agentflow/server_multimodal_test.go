package main

import (
	"testing"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewMultimodalRedisReferenceStore_RejectsNonLoopbackPlainHostPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Redis.Addr = "10.0.0.8:6379"

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	store, err := s.newMultimodalRedisReferenceStore("agentflow:test:mm", time.Hour)
	require.Error(t, err)
	assert.Nil(t, store)
	assert.ErrorContains(t, err, "requires rediss://")
}

func TestNewMultimodalRedisReferenceStore_RejectsNonLoopbackRedisScheme(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Redis.Addr = "redis://example.com:6379"

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	store, err := s.newMultimodalRedisReferenceStore("agentflow:test:mm", time.Hour)
	require.Error(t, err)
	assert.Nil(t, store)
	assert.ErrorContains(t, err, "insecure redis://")
}

func TestNewMultimodalRedisReferenceStore_AllowsLoopbackPlaintext(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	cfg := config.DefaultConfig()
	cfg.Redis.Addr = mr.Addr()

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	store, err := s.newMultimodalRedisReferenceStore("agentflow:test:mm", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, store)
	require.NotNil(t, s.multimodalRedis)
	assert.NoError(t, s.multimodalRedis.Close())
}

func TestIsLoopbackHost(t *testing.T) {
	assert.True(t, isLoopbackHost("localhost"))
	assert.True(t, isLoopbackHost("127.0.0.1"))
	assert.True(t, isLoopbackHost("::1"))
	assert.False(t, isLoopbackHost("example.com"))
}

func TestInitHandlers_MultimodalRejectsNonRedisBackend(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Multimodal.Enabled = true
	cfg.Multimodal.ReferenceStoreBackend = "memory"

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	err := s.initHandlers()
	require.Error(t, err)
	assert.ErrorContains(t, err, "multimodal.reference_store_backend must be redis")
}

