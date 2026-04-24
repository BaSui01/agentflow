package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildToolApprovalGrantStore_RedisBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "redis"
	cfg.HostedTools.Approval.RedisPrefix = "agentflow:test:approval"
	cfg.Redis.Addr = mr.Addr()

	client, store, err := BuildToolApprovalGrantStore(cfg, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, store)
	defer client.Close()

	grant := &ToolApprovalGrant{
		Fingerprint: "fp-1",
		ApprovalID:  "grant:fp-1",
		Scope:       "request",
		ToolName:    "run_command",
		AgentID:     "agent-a",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	require.NoError(t, store.Put(context.Background(), grant))

	loaded, err := store.Get(context.Background(), "fp-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "grant:fp-1", loaded.ApprovalID)
}

func TestRedisToolApprovalGrantStore_ExpiresByTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisToolApprovalGrantStore(client, "agentflow:test:approval", zap.NewNop())
	require.NoError(t, store.Put(context.Background(), &ToolApprovalGrant{
		Fingerprint: "fp-expire",
		ApprovalID:  "grant:fp-expire",
		Scope:       "request",
		ToolName:    "run_command",
		ExpiresAt:   time.Now().Add(50 * time.Millisecond),
	}))

	mr.FastForward(100 * time.Millisecond)

	loaded, err := store.Get(context.Background(), "fp-expire")
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestBuildToolApprovalHistoryStore_RedisBackend(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := config.DefaultConfig()
	cfg.HostedTools.Approval.Backend = "redis"
	cfg.HostedTools.Approval.RedisPrefix = "agentflow:test:approval"
	cfg.Redis.Addr = mr.Addr()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store, err := BuildToolApprovalHistoryStore(cfg, client)
	require.NoError(t, err)
	require.NotNil(t, store)

	require.NoError(t, store.Append(context.Background(), &ToolApprovalHistoryEntry{
		EventType: "approval_requested",
		ToolName:  "run_command",
		Timestamp: time.Now(),
	}))
	rows, err := store.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "approval_requested", rows[0].EventType)
}
