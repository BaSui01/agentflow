package llm

import (
	"context"
	"fmt"
	"testing"
	"time"

	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// --- Canary: UpdateStage with DB ---

func setupCanaryDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	db.Exec(`CREATE TABLE sc_llm_canary_deployments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_id INTEGER,
		canary_version TEXT,
		stable_version TEXT,
		traffic_percent INTEGER,
		stage TEXT,
		max_error_rate REAL,
		max_latency_p95_ms INTEGER,
		auto_rollback BOOLEAN,
		started_at DATETIME,
		completed_at DATETIME,
		rollback_reason TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE sc_audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER,
		user_id INTEGER,
		action TEXT,
		resource_type TEXT,
		resource_id TEXT,
		details TEXT,
		created_at DATETIME
	)`)
	return db
}

func TestCanaryConfig_UpdateStage_AllStages(t *testing.T) {
	db := setupCanaryDB(t)
	cc := NewCanaryConfig(db, zap.NewNop())
	t.Cleanup(cc.Stop)

	require.NoError(t, cc.SetDeployment(&CanaryDeployment{
		ProviderID:     1,
		CanaryVersion:  "v2",
		StableVersion:  "v1",
		TrafficPercent: 0,
		Stage:          CanaryStageInit,
		StartTime:      time.Now(),
	}))

	tests := []struct {
		stage       CanaryStage
		wantTraffic int
	}{
		{CanaryStage10Pct, 10},
		{CanaryStage50Pct, 50},
		{CanaryStage100Pct, 100},
		{CanaryStageRollback, 0},
	}

	for _, tt := range tests {
		err := cc.UpdateStage(1, tt.stage)
		require.NoError(t, err)
		d := cc.GetDeployment(1)
		assert.Equal(t, tt.stage, d.Stage)
		assert.Equal(t, tt.wantTraffic, d.TrafficPercent)
	}
}

func TestCanaryConfig_TriggerRollback_WithDB(t *testing.T) {
	db := setupCanaryDB(t)
	cc := NewCanaryConfig(db, zap.NewNop())
	t.Cleanup(cc.Stop)

	require.NoError(t, cc.SetDeployment(&CanaryDeployment{
		ProviderID:     1,
		TrafficPercent: 50,
		Stage:          CanaryStage50Pct,
		CanaryVersion:  "v2", StableVersion: "v1",
		StartTime: time.Now(),
	}))

	err := cc.TriggerRollback(1, "high error rate")
	require.NoError(t, err)

	d := cc.GetDeployment(1)
	assert.Equal(t, CanaryStageRollback, d.Stage)
	assert.Equal(t, 0, d.TrafficPercent)
	assert.Equal(t, "high error rate", d.RollbackReason)
}

func TestCanaryConfig_SetDeployment_WithDB(t *testing.T) {
	db := setupCanaryDB(t)
	cc := NewCanaryConfig(db, zap.NewNop())
	t.Cleanup(cc.Stop)

	deployment := &CanaryDeployment{
		ProviderID:     1,
		CanaryVersion:  "v2",
		StableVersion:  "v1",
		TrafficPercent: 10,
		Stage:          CanaryStage10Pct,
		StartTime:      time.Now(),
	}

	err := cc.SetDeployment(deployment)
	require.NoError(t, err)
	assert.NotNil(t, cc.GetDeployment(1))
	assert.NotZero(t, deployment.ID)
}

// --- ResilientProvider: Completion with retry ---

func TestResilientProvider_Completion_RetryOnRetryableError(t *testing.T) {
	calls := 0
	inner := &testProvider{
		name: "retry-test",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			calls++
			if calls < 3 {
				return nil, &types.Error{
					Code:      "RATE_LIMITED",
					Message:   "rate limited",
					Retryable: true,
				}
			}
			return &ChatResponse{Model: "ok"}, nil
		},
	}

	rp := NewResilientProvider(inner, &ResilientConfig{
		RetryPolicy: &llmpolicy.RetryPolicy{
			MaxRetries:     3,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			Multiplier:     2.0,
		},
		CircuitBreaker:    DefaultCircuitBreakerConfig(),
		EnableIdempotency: true,
		IdempotencyTTL:    time.Hour,
	}, zap.NewNop())

	resp, err := rp.Completion(context.Background(), &ChatRequest{
		Model:    "m",
		Messages: []Message{{Content: "retry-test"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Model)
	assert.Equal(t, 3, calls)
}

func TestResilientProvider_Completion_NonRetryableError(t *testing.T) {
	inner := &testProvider{
		name: "non-retry",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			return nil, fmt.Errorf("non-retryable error")
		},
	}

	rp := NewResilientProvider(inner, &ResilientConfig{
		RetryPolicy: &llmpolicy.RetryPolicy{
			MaxRetries:     3,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     10 * time.Millisecond,
			Multiplier:     2.0,
		},
		CircuitBreaker: DefaultCircuitBreakerConfig(),
	}, zap.NewNop())

	_, err := rp.Completion(context.Background(), &ChatRequest{
		Model:    "m",
		Messages: []Message{{Content: "fail"}},
	})
	require.Error(t, err)
}

func TestResilientProvider_Completion_IdempotencyExpired(t *testing.T) {
	calls := 0
	inner := &testProvider{
		name: "expire-test",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			calls++
			return &ChatResponse{Model: fmt.Sprintf("call-%d", calls)}, nil
		},
	}

	rp := NewResilientProvider(inner, &ResilientConfig{
		RetryPolicy:       llmpolicy.DefaultRetryPolicy(),
		CircuitBreaker:    DefaultCircuitBreakerConfig(),
		EnableIdempotency: true,
		IdempotencyTTL:    time.Millisecond, // very short TTL
	}, zap.NewNop())

	req := &ChatRequest{Model: "m", Messages: []Message{{Content: "expire"}}}
	_, err := rp.Completion(context.Background(), req)
	require.NoError(t, err)

	time.Sleep(5 * time.Millisecond) // wait for TTL to expire

	resp, err := rp.Completion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 2, calls) // should have called again after expiry
	assert.Equal(t, "call-2", resp.Model)
}

func TestResilientProvider_Completion_BackoffCap(t *testing.T) {
	calls := 0
	inner := &testProvider{
		name: "backoff-cap",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			calls++
			if calls <= 3 {
				return nil, &types.Error{Code: "ERR", Message: "fail", Retryable: true}
			}
			return &ChatResponse{Model: "ok"}, nil
		},
	}

	rp := NewResilientProvider(inner, &ResilientConfig{
		RetryPolicy: &llmpolicy.RetryPolicy{
			MaxRetries:     5,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     2 * time.Millisecond, // cap at 2ms
			Multiplier:     10.0,                 // aggressive multiplier
		},
		CircuitBreaker: DefaultCircuitBreakerConfig(),
	}, zap.NewNop())

	resp, err := rp.Completion(context.Background(), &ChatRequest{
		Model:    "m",
		Messages: []Message{{Content: "cap"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Model)
}

// --- ResilientProvider: Stream when circuit is closed ---

func TestResilientProvider_Stream_Success(t *testing.T) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Delta: Message{Content: "hello"}}
	close(ch)

	inner := &testProvider{
		name: "stream-ok",
		streamFn: func(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
			return ch, nil
		},
	}

	rp := NewResilientProvider(inner, nil, zap.NewNop())
	result, err := rp.Stream(context.Background(), &ChatRequest{Model: "m"})
	require.NoError(t, err)

	chunk := <-result
	assert.Equal(t, "hello", chunk.Delta.Content)
}

// --- Canary: loadFromDB with DB ---

func TestCanaryConfig_LoadFromDB(t *testing.T) {
	db := setupCanaryDB(t)

	// Insert a deployment record
	db.Exec(`INSERT INTO sc_llm_canary_deployments
		(provider_id, canary_version, stable_version, traffic_percent, stage, max_error_rate, max_latency_p95_ms, auto_rollback, started_at)
		VALUES (1, 'v2', 'v1', 10, '10pct', 0.05, 5000, 1, datetime('now'))`)

	cc := NewCanaryConfig(db, zap.NewNop())
	t.Cleanup(cc.Stop)

	d := cc.GetDeployment(1)
	assert.NotNil(t, d)
	assert.Equal(t, "v2", d.CanaryVersion)
	assert.Equal(t, CanaryStage10Pct, d.Stage)
}
