package channelstore

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

// ---------------------------------------------------------------------------
// AdaptiveWeightedSelector
// ---------------------------------------------------------------------------

func TestAdaptiveSelector_BoostsHighSuccessChannel(t *testing.T) {
	store := NewStaticStore(StaticStoreConfig{
		Channels: []Channel{
			{ID: "ch1", Provider: "openai", BaseURL: "https://a", Priority: 1, Weight: 10},
			{ID: "ch2", Provider: "openai", BaseURL: "https://b", Priority: 1, Weight: 10},
		},
		Keys: []Key{
			{ID: "k1", ChannelID: "ch1", Weight: 1},
			{ID: "k2", ChannelID: "ch2", Weight: 1},
		},
		Mappings: []ModelMapping{
			{ID: "m1", ChannelID: "ch1", PublicModel: "gpt-4", RemoteModel: "gpt-4", Provider: "openai"},
			{ID: "m2", ChannelID: "ch2", PublicModel: "gpt-4", RemoteModel: "gpt-4", Provider: "openai"},
		},
		Secrets: map[string]Secret{
			"k1": {APIKey: "sk-1"},
			"k2": {APIKey: "sk-2"},
		},
	})

	metrics := NewInMemoryMetricsSource()
	// ch1: 100% success, ch2: 100% failures with 429
	for i := 0; i < 20; i++ {
		metrics.Record("ch1", true, false)
		metrics.Record("ch2", false, true)
	}

	sel := NewAdaptiveWeightedSelector(store, metrics, AdaptiveSelectorOptions{
		Random: rand.New(rand.NewSource(42)),
	})

	// Run selections and count which channel is chosen
	counts := map[string]int{"ch1": 0, "ch2": 0}
	for i := 0; i < 100; i++ {
		result, err := sel.SelectChannel(context.Background(),
			&router.ChannelRouteRequest{RequestedModel: "gpt-4"},
			&router.ModelResolution{ResolvedModel: "gpt-4"},
			[]router.ChannelModelMapping{
				{MappingID: "m1", ChannelID: "ch1", Provider: "openai", PublicModel: "gpt-4", RemoteModel: "gpt-4"},
				{MappingID: "m2", ChannelID: "ch2", Provider: "openai", PublicModel: "gpt-4", RemoteModel: "gpt-4"},
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil selection")
		}
		counts[result.ChannelID]++
	}

	// ch1 should be selected much more often due to higher adaptive weight
	if counts["ch1"] < 70 {
		t.Errorf("expected ch1 to dominate, got ch1=%d ch2=%d", counts["ch1"], counts["ch2"])
	}
}

func TestComputeAdaptiveWeight(t *testing.T) {
	cfg := AdaptiveWeightConfig{}.withDefaults()

	// Perfect success
	w := computeAdaptiveWeight(10, &RuntimeMetrics{
		TotalCalls: 100, SuccessCount: 100,
	}, cfg)
	if w <= 10 {
		t.Errorf("expected boosted weight > 10, got %d", w)
	}

	// Pure 429 failures
	w = computeAdaptiveWeight(10, &RuntimeMetrics{
		TotalCalls: 100, FailureCount: 100, RateLimitCount: 100,
	}, cfg)
	if w >= 10 {
		t.Errorf("expected penalized weight < 10, got %d", w)
	}

	// Below MinCalls threshold — computeAdaptiveWeight still applies (MinCalls guard is in applyAdaptiveWeights)
	// With 2/2 success, factor = 0.6 + 1.0*0.8 = 1.4, weight = round(10*1.4) = 14
	w = computeAdaptiveWeight(10, &RuntimeMetrics{TotalCalls: 2, SuccessCount: 2}, cfg)
	if w != 14 {
		t.Errorf("expected weight 14 for small sample with full success, got %d", w)
	}
}

// ---------------------------------------------------------------------------
// InMemoryQuotaPolicy
// ---------------------------------------------------------------------------

func TestQuotaPolicy_DailyLimit(t *testing.T) {
	store := StaticQuotaStore{Default: QuotaConfig{DailyLimit: 3}}
	policy := NewInMemoryQuotaPolicy(store)

	sel := &router.ChannelSelection{KeyID: "k1"}
	for i := 0; i < 3; i++ {
		if err := policy.Allow(context.Background(), nil, sel); err != nil {
			t.Fatalf("call %d should be allowed: %v", i+1, err)
		}
	}
	if err := policy.Allow(context.Background(), nil, sel); err == nil {
		t.Fatal("4th call should exceed daily limit")
	}
}

func TestQuotaPolicy_RateLimit(t *testing.T) {
	store := StaticQuotaStore{Default: QuotaConfig{RateLimitPerMinute: 2}}
	policy := NewInMemoryQuotaPolicy(store)

	sel := &router.ChannelSelection{KeyID: "k1"}
	for i := 0; i < 2; i++ {
		if err := policy.Allow(context.Background(), nil, sel); err != nil {
			t.Fatalf("call %d should be allowed: %v", i+1, err)
		}
	}
	if err := policy.Allow(context.Background(), nil, sel); err == nil {
		t.Fatal("3rd call should exceed rate limit")
	}
}

func TestQuotaPolicy_ConcurrencyLimit(t *testing.T) {
	store := StaticQuotaStore{Default: QuotaConfig{ConcurrencyLimit: 2}}
	policy := NewInMemoryQuotaPolicy(store)

	sel := &router.ChannelSelection{KeyID: "k1"}
	// 2 concurrent allowed
	_ = policy.Allow(context.Background(), nil, sel)
	_ = policy.Allow(context.Background(), nil, sel)

	if err := policy.Allow(context.Background(), nil, sel); err == nil {
		t.Fatal("3rd concurrent call should be rejected")
	}

	// Release one
	_ = policy.RecordUsage(context.Background(), &router.ChannelUsageRecord{KeyID: "k1"})

	if err := policy.Allow(context.Background(), nil, sel); err != nil {
		t.Fatalf("after release, should be allowed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CascadeCooldownController
// ---------------------------------------------------------------------------

func TestCooldown_KeyLevelCooldown(t *testing.T) {
	store := NewStaticStore(StaticStoreConfig{
		Keys: []Key{
			{ID: "k1", ChannelID: "ch1"},
			{ID: "k2", ChannelID: "ch1"}, // second key prevents cascade
		},
	})
	cc := NewCascadeCooldownController(store, CooldownConfig{
		KeyCooldownDuration:     100 * time.Millisecond,
		ChannelCooldownDuration: 5 * time.Minute,
		FailureThreshold:        2,
	})

	sel := &router.ChannelSelection{ChannelID: "ch1", KeyID: "k1"}

	// First failure — not yet in cooldown
	_ = cc.RecordResult(context.Background(), &router.ChannelUsageRecord{
		ChannelID: "ch1", KeyID: "k1", Success: false,
	})
	if err := cc.Allow(context.Background(), nil, sel); err != nil {
		t.Fatalf("should not be in cooldown after 1 failure: %v", err)
	}

	// Second failure — k1 enters cooldown, but channel does NOT cascade (k2 still healthy)
	_ = cc.RecordResult(context.Background(), &router.ChannelUsageRecord{
		ChannelID: "ch1", KeyID: "k1", Success: false,
	})
	if err := cc.Allow(context.Background(), nil, sel); err == nil {
		t.Fatal("k1 should be in cooldown after 2 failures")
	}

	// Wait for key cooldown to expire
	time.Sleep(120 * time.Millisecond)
	if err := cc.Allow(context.Background(), nil, sel); err != nil {
		t.Fatalf("k1 cooldown should have expired: %v", err)
	}
}

func TestCooldown_CascadeToChannel(t *testing.T) {
	store := NewStaticStore(StaticStoreConfig{
		Keys: []Key{
			{ID: "k1", ChannelID: "ch1"},
			{ID: "k2", ChannelID: "ch1"},
		},
	})
	cc := NewCascadeCooldownController(store, CooldownConfig{
		KeyCooldownDuration:     100 * time.Millisecond,
		ChannelCooldownDuration: 200 * time.Millisecond,
		FailureThreshold:        1,
	})

	// Fail both keys
	_ = cc.RecordResult(context.Background(), &router.ChannelUsageRecord{
		ChannelID: "ch1", KeyID: "k1", Success: false,
	})
	_ = cc.RecordResult(context.Background(), &router.ChannelUsageRecord{
		ChannelID: "ch1", KeyID: "k2", Success: false,
	})

	// Channel should be in cooldown
	if !cc.IsChannelCoolingDown("ch1") {
		t.Fatal("channel should be in cascade cooldown")
	}

	// A different key in this channel should also be blocked
	sel := &router.ChannelSelection{ChannelID: "ch1", KeyID: "k3"}
	if err := cc.Allow(context.Background(), nil, sel); err == nil {
		t.Fatal("channel-level cooldown should block all keys")
	}
}

// ---------------------------------------------------------------------------
// AsyncUsageRecorder
// ---------------------------------------------------------------------------

func TestAsyncRecorder_RecordsAreDelivered(t *testing.T) {
	var mu sync.Mutex
	var received []*router.ChannelUsageRecord

	sink := FuncSink(func(_ context.Context, u *router.ChannelUsageRecord) error {
		mu.Lock()
		received = append(received, u)
		mu.Unlock()
		return nil
	})

	recorder := NewAsyncUsageRecorder(sink, AsyncUsageRecorderConfig{QueueSize: 64, Workers: 2})

	for i := 0; i < 10; i++ {
		_ = recorder.RecordUsage(context.Background(), &router.ChannelUsageRecord{
			KeyID:   "k1",
			Success: true,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := recorder.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 10 {
		t.Fatalf("expected 10 records, got %d", len(received))
	}
}

func TestAsyncRecorder_ShutdownDrainsQueue(t *testing.T) {
	var mu sync.Mutex
	count := 0

	sink := FuncSink(func(_ context.Context, u *router.ChannelUsageRecord) error {
		// Simulate slow persistence
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	recorder := NewAsyncUsageRecorder(sink, AsyncUsageRecorderConfig{QueueSize: 256, Workers: 1})

	for i := 0; i < 20; i++ {
		_ = recorder.RecordUsage(context.Background(), &router.ChannelUsageRecord{KeyID: "k1"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := recorder.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if count != 20 {
		t.Fatalf("expected all 20 records drained, got %d", count)
	}
}
