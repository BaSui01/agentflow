package channelstore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

// QuotaConfig defines per-key or per-channel quota limits.
type QuotaConfig struct {
	// DailyLimit is the maximum number of calls per day. 0 = unlimited.
	DailyLimit int64

	// RateLimitPerMinute is the maximum number of calls per minute. 0 = unlimited.
	RateLimitPerMinute int64

	// ConcurrencyLimit is the maximum concurrent calls. 0 = unlimited.
	ConcurrencyLimit int64
}

// QuotaStore provides per-key quota configuration.
// Returning nil QuotaConfig means no quota limits.
type QuotaStore interface {
	GetQuota(ctx context.Context, keyID string) (*QuotaConfig, error)
}

// InMemoryQuotaPolicy enforces per-key daily limits, rate limits, and
// concurrency limits using in-memory counters. Suitable for single-instance
// deployments; use a Redis-backed implementation for distributed setups.
type InMemoryQuotaPolicy struct {
	Store QuotaStore

	mu       sync.Mutex
	daily    map[string]*dailyCounter
	rate     map[string]*rateCounter
	inflight map[string]*int64
	resetDay int // day of year for daily reset
}

var _ router.QuotaPolicy = (*InMemoryQuotaPolicy)(nil)

type dailyCounter struct {
	count int64
	day   int
}

type rateCounter struct {
	count int64
	reset time.Time
}

// NewInMemoryQuotaPolicy creates a quota policy backed by in-memory counters.
func NewInMemoryQuotaPolicy(store QuotaStore) *InMemoryQuotaPolicy {
	return &InMemoryQuotaPolicy{
		Store:    store,
		daily:    make(map[string]*dailyCounter),
		rate:     make(map[string]*rateCounter),
		inflight: make(map[string]*int64),
		resetDay: time.Now().YearDay(),
	}
}

// Allow checks whether the selected route is within quota limits.
func (p *InMemoryQuotaPolicy) Allow(ctx context.Context, _ *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	if p == nil || p.Store == nil || selection == nil {
		return nil
	}
	keyID := strings.TrimSpace(selection.KeyID)
	if keyID == "" {
		return nil
	}

	quota, err := p.Store.GetQuota(ctx, keyID)
	if err != nil || quota == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	today := now.YearDay()

	// Daily limit check
	if quota.DailyLimit > 0 {
		dc := p.daily[keyID]
		if dc == nil || dc.day != today {
			dc = &dailyCounter{day: today}
			p.daily[keyID] = dc
		}
		if dc.count >= quota.DailyLimit {
			return fmt.Errorf("key %s exceeded daily limit (%d/%d)", keyID, dc.count, quota.DailyLimit)
		}
	}

	// Rate limit check (per minute)
	if quota.RateLimitPerMinute > 0 {
		rc := p.rate[keyID]
		if rc == nil || now.After(rc.reset) {
			rc = &rateCounter{reset: now.Add(time.Minute)}
			p.rate[keyID] = rc
		}
		if rc.count >= quota.RateLimitPerMinute {
			return fmt.Errorf("key %s exceeded rate limit (%d/%d per minute)", keyID, rc.count, quota.RateLimitPerMinute)
		}
	}

	// Concurrency limit check
	if quota.ConcurrencyLimit > 0 {
		inf := p.inflight[keyID]
		if inf == nil {
			var zero int64
			inf = &zero
			p.inflight[keyID] = inf
		}
		if *inf >= quota.ConcurrencyLimit {
			return fmt.Errorf("key %s exceeded concurrency limit (%d/%d)", keyID, *inf, quota.ConcurrencyLimit)
		}
		*inf++
	}

	// Increment counters (daily + rate)
	if quota.DailyLimit > 0 {
		p.daily[keyID].count++
	}
	if quota.RateLimitPerMinute > 0 {
		p.rate[keyID].count++
	}

	return nil
}

// RecordUsage decrements the concurrency counter when a call completes.
func (p *InMemoryQuotaPolicy) RecordUsage(_ context.Context, usage *router.ChannelUsageRecord) error {
	if p == nil || usage == nil {
		return nil
	}
	keyID := strings.TrimSpace(usage.KeyID)
	if keyID == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	inf := p.inflight[keyID]
	if inf != nil && *inf > 0 {
		*inf--
	}
	return nil
}

// ResetDaily clears all daily counters. Call this at the start of each day.
func (p *InMemoryQuotaPolicy) ResetDaily() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.daily = make(map[string]*dailyCounter)
	p.resetDay = time.Now().YearDay()
}

// StaticQuotaStore returns the same QuotaConfig for all keys.
type StaticQuotaStore struct {
	Default QuotaConfig
}

func (s StaticQuotaStore) GetQuota(_ context.Context, _ string) (*QuotaConfig, error) {
	return &s.Default, nil
}

// MapQuotaStore returns per-key QuotaConfig from a map. Keys not in the map
// get no limits.
type MapQuotaStore struct {
	Quotas map[string]QuotaConfig
}

func (s MapQuotaStore) GetQuota(_ context.Context, keyID string) (*QuotaConfig, error) {
	q, ok := s.Quotas[keyID]
	if !ok {
		return nil, nil
	}
	return &q, nil
}
